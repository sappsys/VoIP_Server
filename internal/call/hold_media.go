package call

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/emiago/diago"
	diagomedia "github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/media/tones"
)

// holdPlayer tracks stoppable playback sessions during phone-initiated hold.
type holdPlayer struct {
	mu            sync.Mutex
	controls      []*diago.AudioPlaybackControl
	wg            sync.WaitGroup
	mohBytes      atomic.Int64
	dialToneBytes atomic.Int64
}

// MOHBytesSent returns RTP payload bytes written during hold MOH (server-side).
func (p *holdPlayer) MOHBytesSent() int64 {
	if p == nil {
		return 0
	}
	return p.mohBytes.Load()
}

// DialToneBytesSent returns RTP payload bytes written for holder dial tone.
func (p *holdPlayer) DialToneBytesSent() int64 {
	if p == nil {
		return 0
	}
	return p.dialToneBytes.Load()
}

func (p *holdPlayer) track(pc *diago.AudioPlaybackControl) {
	if p == nil || pc == nil {
		return
	}
	p.mu.Lock()
	p.controls = append(p.controls, pc)
	p.mu.Unlock()
}

func (p *holdPlayer) stopAndWait() {
	if p == nil {
		return
	}
	p.mu.Lock()
	for _, c := range p.controls {
		c.Stop()
	}
	p.controls = nil
	p.mu.Unlock()
	p.wg.Wait()
}

// holdPlaybackHook is set by tests to verify hold media sequencing (REQ-HOLD-7).
var holdPlaybackHook func(kind string) // "dial_tone" | "moh"

// HoldMediaStartedHook is set by integration tests to verify MOH/dial-tone startup.
var HoldMediaStartedHook func(mohStarted, dialToneStarted bool)

// HoldLeaveHook is set by requirement tests to verify unhold/leave sequencing.
var HoldLeaveHook func()

// wavPlaybackCodecSupported reports whether diago AudioPlayback can stream WAV to this codec.
func wavPlaybackCodecSupported(c diagomedia.Codec) bool {
	switch {
	case c.PayloadType == 0 && c.Name == "PCMU":
		return true
	case c.PayloadType == 8 && c.Name == "PCMA":
		return true
	default:
		return false
	}
}

// fallbackMOHTone is generated when no WAV tracks are available (REQ-HOLD-2).
func fallbackMOHTone(profile tones.Profile) tones.Tone {
	if profile.Region != "" && len(profile.Ring.Frequencies) > 0 {
		return profile.Ring
	}
	return tones.DefaultProfile().Ring
}

// startMOH plays hold music to the remote party. Returns true once playback is running.
// Uses pcmcodec RTP writes when supported so MOH survives held-party SDP churn.
func (p *holdPlayer) startMOH(
	ctx context.Context,
	callDone context.Context,
	mohDir string,
	profile tones.Profile,
	log *slog.Logger,
	sess diago.DialogSession,
	create func() (diago.AudioPlaybackControl, error),
) bool {
	if p == nil {
		return false
	}
	if sess != nil && !pcmToneCanSend(sess) {
		if log != nil {
			log.Warn("moh blocked: held party leg cannot send RTP")
		}
		return false
	}
	if holdPlaybackHook != nil {
		holdPlaybackHook("moh")
	}

	tracks, err := MOHTracks(mohDir)
	if err != nil || len(tracks) == 0 {
		if log != nil {
			if err != nil {
				log.Warn("moh wav unavailable, using generated hold tone", "path", mohDir, "error", err)
			} else {
				log.Warn("no moh wav files, using generated hold tone", "path", mohDir)
			}
		}
		return p.startToneLoop(ctx, callDone, fallbackMOHTone(profile), log, sess, create)
	}

	if sess != nil && holdMediaViaPCMCodec(sess) {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			runPCMCodecMOHLoop(ctx, callDone, sess, tracks, fallbackMOHTone(profile), log, &p.mohBytes)
		}()
		return true
	}

	if create == nil {
		return false
	}
	pb, err := create()
	if err != nil {
		if log != nil {
			log.Warn("moh playback create failed", "error", err)
		}
		return false
	}
	if !wavPlaybackCodecSupported(pb.Codec()) {
		if log != nil {
			log.Debug("moh wav skipped for codec, using generated hold tone", "codec", pb.Codec().Name, "pt", pb.Codec().PayloadType)
		}
		return p.startToneLoop(ctx, callDone, fallbackMOHTone(profile), log, sess, create)
	}
	p.track(&pb)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer recoverHoldPlayback("moh", log)
		tone := fallbackMOHTone(profile)
		for {
			for _, path := range tracks {
				if playWavControlled(ctx, callDone, &pb, path, log, &p.mohBytes) {
					continue
				}
				if log != nil {
					log.Debug("moh wav failed, falling back to generated tone", "path", path)
				}
				playToneControlled(ctx, callDone, &pb, tone, log, &p.mohBytes)
				return
			}
		}
	}()
	return true
}

func (p *holdPlayer) goMOH(ctx context.Context, callDone context.Context, mohDir string, log *slog.Logger, sess diago.DialogSession, create func() (diago.AudioPlaybackControl, error)) {
	_ = p.startMOH(ctx, callDone, mohDir, tones.DefaultProfile(), log, sess, create)
}

// recoverHoldPlayback keeps a media leg with an uninitialised RTP writer from
// crashing the server; hold music/tone is best-effort and must never panic.
func recoverHoldPlayback(kind string, log *slog.Logger) {
	if r := recover(); r != nil {
		if log != nil {
			log.Warn("hold playback recovered from panic", "kind", kind, "panic", r)
		}
	}
}

// startToneLoop plays a generated tone until ctx/callDone ends. Returns false if playback cannot start.
func (p *holdPlayer) startToneLoop(
	ctx context.Context,
	callDone context.Context,
	tone tones.Tone,
	log *slog.Logger,
	sess diago.DialogSession,
	create func() (diago.AudioPlaybackControl, error),
) bool {
	if p == nil {
		return false
	}
	if holdPlaybackHook != nil {
		holdPlaybackHook("dial_tone")
	}
	if sess != nil && holdMediaViaPCMCodec(sess) {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			runPCMCodecToneLoop(ctx, callDone, sess, tone, log, &p.dialToneBytes)
		}()
		return true
	}
	if create == nil {
		return false
	}
	pb, err := create()
	if err != nil {
		if log != nil {
			log.Warn("tone playback create failed", "error", err)
		}
		return false
	}
	p.track(&pb)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer recoverHoldPlayback("dial_tone", log)
		playToneControlled(ctx, callDone, &pb, tone, log, &p.dialToneBytes)
	}()
	return true
}

func (p *holdPlayer) goTone(ctx context.Context, callDone context.Context, tone tones.Tone, log *slog.Logger, sess diago.DialogSession, create func() (diago.AudioPlaybackControl, error)) {
	_ = p.startToneLoop(ctx, callDone, tone, log, sess, create)
}

func playWavControlled(ctx context.Context, callDone context.Context, pb *diago.AudioPlaybackControl, path string, log *slog.Logger, bytesSent *atomic.Int64) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}
	if callDone != nil {
		select {
		case <-callDone.Done():
			return false
		default:
		}
	}
	codec := pb.Codec()
	reader, mime, err := openAudioPlaybackReader(path, codec)
	if err != nil {
		if log != nil {
			log.Warn("wav open failed", "path", path, "error", err)
		}
		return false
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	if mime == "audio/wav" {
		reader = bufio.NewReaderSize(reader, 64*1024)
	}
	_, err = pb.Play(reader, mime)
	if bytesSent != nil && err == nil {
		bytesSent.Add(320)
	}
	if err != nil && err != io.EOF {
		if log != nil {
			log.Debug("wav playback ended", "path", path, "error", err)
		}
		return false
	}
	return true
}

func playToneControlled(ctx context.Context, callDone context.Context, pb *diago.AudioPlaybackControl, tone tones.Tone, log *slog.Logger, bytesSent *atomic.Int64) {
	gen := tones.NewGenerator(tone)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if callDone != nil {
			select {
			case <-callDone.Done():
				return
			default:
			}
		}
		if _, err := pb.Play(gen, "audio/pcm"); err != nil && err != io.EOF {
			return
		}
		if bytesSent != nil {
			bytesSent.Add(320)
		}
	}
}

func resetServerLeg(in *diago.DialogServerSession) {
	if in == nil || in.DialogServerSession == nil {
		return
	}
	in.SetAudioReader(nil)
	in.SetAudioWriter(nil)
}

func resetClientLeg(out *diago.DialogClientSession) {
	if out == nil || out.DialogClientSession == nil {
		return
	}
	out.SetAudioReader(nil)
	out.SetAudioWriter(nil)
}

func resetLegRTP(in *diago.DialogServerSession, out *diago.DialogClientSession) {
	resetDialogLegRTP(in)
	resetDialogLegRTP(out)
}

// MOHSession is stoppable hold music. Always call Stop before bridge/mixer audio.
type MOHSession struct {
	cancel context.CancelFunc
	player *holdPlayer
}

// StartMOHServer plays MOH on an inbound leg using PlaybackControlCreate so Stop()
// halts playback immediately (required before conference mixer or call bridge).
func StartMOHServer(ctx context.Context, in *diago.DialogServerSession, mohDir string, log *slog.Logger) *MOHSession {
	if in == nil {
		return nil
	}
	return newMOHSession(ctx, in.Context(), mohDir, log, in.PlaybackControlCreate)
}

// StartMOHClient plays MOH on an outbound leg with stoppable playback.
func StartMOHClient(ctx context.Context, out *diago.DialogClientSession, mohDir string, log *slog.Logger) *MOHSession {
	if out == nil {
		return nil
	}
	return newMOHSession(ctx, out.Context(), mohDir, log, out.PlaybackControlCreate)
}

func newMOHSession(
	ctx context.Context,
	callDone context.Context,
	mohDir string,
	log *slog.Logger,
	create func() (diago.AudioPlaybackControl, error),
) *MOHSession {
	if mohDir == "" || create == nil {
		return nil
	}
	if _, err := MOHTracks(mohDir); err != nil {
		if log != nil {
			log.Warn("moh directory missing", "path", mohDir, "error", err)
		}
		return nil
	}
	mohCtx, cancel := context.WithCancel(ctx)
	player := &holdPlayer{}
	player.goMOH(mohCtx, callDone, mohDir, log, nil, create)
	return &MOHSession{cancel: cancel, player: player}
}

func (s *MOHSession) Stop() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.player != nil {
		s.player.stopAndWait()
	}
}
