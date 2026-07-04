package call

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/emiago/diago"
	"github.com/sappsys/VoIP_Server/internal/media/tones"
)

func busyDuration(profile tones.Profile) time.Duration {
	sec := profile.BusySeconds
	if sec <= 0 {
		sec = 5
	}
	return time.Duration(sec) * time.Second
}

func playToneLoop(ctx context.Context, callDone context.Context, pb *diago.AudioPlayback, tone tones.Tone, log *slog.Logger) {
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
			if log != nil {
				log.Debug("tone playback ended", "error", err)
			}
			return
		}
	}
}

func playToneFor(ctx context.Context, callDone context.Context, pb *diago.AudioPlayback, tone tones.Tone, duration time.Duration, log *slog.Logger) {
	if duration <= 0 {
		duration = 5 * time.Second
	}
	playCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	gen := tones.NewGenerator(tone)
	_, _ = pb.Play(gen, "audio/pcm")
	<-playCtx.Done()
	_ = callDone
}

// PlayToneToServer streams a signalling tone to an inbound leg (hold dial tone only).
func PlayToneToServer(ctx context.Context, in *diago.DialogServerSession, tone tones.Tone, log *slog.Logger) {
	if in == nil {
		return
	}
	pb, err := in.PlaybackCreate()
	if err != nil {
		if log != nil {
			log.Warn("tone playback create failed", "error", err)
		}
		return
	}
	playToneLoop(ctx, in.Context(), &pb, tone, log)
}

// PlayToneToClient streams a signalling tone to an outbound leg (hold dial tone only).
func PlayToneToClient(ctx context.Context, out *diago.DialogClientSession, tone tones.Tone, log *slog.Logger) {
	if out == nil {
		return
	}
	pb, err := out.PlaybackCreate()
	if err != nil {
		if log != nil {
			log.Warn("tone playback create failed", "error", err)
		}
		return
	}
	playToneLoop(ctx, out.Context(), &pb, tone, log)
}

// ensureEarlyMedia opens an early-media session so ringback can be sent before 200 OK.
func ensureEarlyMedia(in *diago.DialogServerSession, log *slog.Logger) {
	if in == nil || in.MediaSession() != nil {
		return
	}
	if err := in.ProgressMedia(); err != nil && log != nil {
		log.Debug("early media for ringback", "error", err)
	}
	EnsureSessionDTMFCodec(in)
}

// StartRingback plays the regional ring tone to the caller until cancel is invoked.
// Use only while an outbound call is ringing and unanswered.
func StartRingback(ctx context.Context, in *diago.DialogServerSession, profile tones.Profile, log *slog.Logger) context.CancelFunc {
	if in == nil {
		return func() {}
	}
	ensureEarlyMedia(in, log)
	ringCtx, cancel := context.WithCancel(ctx)

	var pbRef *diago.AudioPlaybackControl
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		pb, err := in.PlaybackControlCreate()
		if err != nil {
			if log != nil {
				log.Debug("ringback playback create failed", "error", err)
			}
			return
		}
		mu.Lock()
		pbRef = &pb
		mu.Unlock()
		playToneControlled(ringCtx, in.Context(), &pb, profile.Ring, log, nil)
	}()

	return func() {
		cancel()
		mu.Lock()
		pb := pbRef
		mu.Unlock()
		if pb != nil {
			pb.Stop()
		}
		wg.Wait()
	}
}

// PlayBusyThenHangup answers if needed, plays the regional busy tone, then hangs up.
// Use when an outbound call fails or is not answered within the ring timeout.
func PlayBusyThenHangup(ctx context.Context, in *diago.DialogServerSession, profile tones.Profile, log *slog.Logger) {
	if in == nil {
		return
	}
	if err := AnswerSession(in); err != nil {
		if log != nil {
			log.Debug("busy tone answer failed", "error", err)
		}
		in.Hangup(ctx)
		return
	}
	pb, err := in.PlaybackCreate()
	if err != nil {
		in.Hangup(ctx)
		return
	}
	playToneFor(ctx, in.Context(), &pb, profile.Busy, busyDuration(profile), log)
	in.Hangup(ctx)
}

// AnswerAndPlayBusy answers, plays the regional busy tone, then hangs up.
// Use when rejecting an inbound call (capacity, DND, etc.).
func AnswerAndPlayBusy(ctx context.Context, in *diago.DialogServerSession, profile tones.Profile, log *slog.Logger) {
	if in == nil {
		return
	}
	in.Trying()
	PlayBusyThenHangup(ctx, in, profile, log)
}
