package audiobridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/emiago/diago"
	diagomedia "github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/media/pcmcodec"
)

// SessionLeg is implemented by inbound and outbound dialog sessions.
type SessionLeg interface {
	Context() context.Context
	AudioReader(opts ...diago.AudioReaderOption) (io.Reader, error)
	AudioWriter(opts ...diago.AudioWriterOption) (io.Writer, error)
	MediaSession() *diagomedia.MediaSession
}

// NegotiatedCodec returns the active audio codec for a leg.
func NegotiatedCodec(ms *diagomedia.MediaSession) (diagomedia.Codec, bool) {
	if ms == nil {
		return diagomedia.Codec{}, false
	}
	if c, ok := diagomedia.CodecAudioFromList(ms.CommonCodecs()); ok {
		return c, true
	}
	return diagomedia.CodecAudioFromList(ms.Codecs)
}

// CanTranscode reports whether we can decode/encode this codec through PCM.
func CanTranscode(c diagomedia.Codec) bool {
	return pcmcodec.Supported(c)
}

// NeedsTranscoding is true when two legs negotiated different audio codecs.
func NeedsTranscoding(a, b diagomedia.Codec) bool {
	return a.PayloadType != b.PayloadType || a.Name != b.Name
}

// StartTranscodingBridge relays audio between two legs through PCM transcoding.
// This is the only supported two-party media path.
func StartTranscodingBridge(log *slog.Logger, legA, legB SessionLeg) (stop func() error, stats *RelayStats, err error) {
	return startPCMBridge(log, legA, legB)
}

// StartPCMBridge relays audio between legs with minimal buffering. Transcoding is
// used only when negotiated codecs differ. Relay writers use WriteSamples so RTP
// is forwarded immediately (not playback-paced).
func StartPCMBridge(log *slog.Logger, legA, legB SessionLeg) (stop func() error, stats *RelayStats, err error) {
	return startPCMBridge(log, legA, legB)
}

func startPCMBridge(log *slog.Logger, legA, legB SessionLeg) (func() error, *RelayStats, error) {
	codecA, ok := NegotiatedCodec(legA.MediaSession())
	if !ok {
		return nil, nil, fmt.Errorf("leg A codec unknown")
	}
	codecB, ok := NegotiatedCodec(legB.MediaSession())
	if !ok {
		return nil, nil, fmt.Errorf("leg B codec unknown")
	}
	if !CanTranscode(codecA) || !CanTranscode(codecB) {
		return nil, nil, fmt.Errorf("transcoding unsupported codecA=%s codecB=%s", codecA.Name, codecB.Name)
	}

	stats := &RelayStats{}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-ctx.Done():
		case <-legDone(legA):
		case <-legDone(legB):
		}
		cancel()
	}()

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	startRelay := func(from, to SessionLeg, fromCodec, toCodec diagomedia.Codec, aToB bool) {
		defer wg.Done()
		errCh <- runRelay(ctx, log, stats, aToB, from, to, fromCodec, toCodec)
	}

	go startRelay(legA, legB, codecA, codecB, true)
	go startRelay(legB, legA, codecB, codecA, false)

	stop := func() error {
		cancel()
		if legA != nil {
			stopLegMedia(legA)
		}
		if legB != nil {
			stopLegMedia(legB)
		}
		wg.Wait()
		var err error
		for range 2 {
			err = errors.Join(err, <-errCh)
		}
		return err
	}
	return stop, stats, nil
}

func legDone(leg SessionLeg) <-chan struct{} {
	if leg == nil {
		return make(chan struct{})
	}
	ctx := leg.Context()
	if ctx == nil {
		return make(chan struct{})
	}
	return ctx.Done()
}

func clearSessionLegMedia(leg SessionLeg) {
	switch s := leg.(type) {
	case *diago.DialogServerSession:
		s.SetAudioReader(nil)
		s.SetAudioWriter(nil)
	case *diago.DialogClientSession:
		s.SetAudioReader(nil)
		s.SetAudioWriter(nil)
	}
}

func stopLegMedia(leg SessionLeg) {
	clearSessionLegMedia(leg)
	switch s := leg.(type) {
	case *diago.DialogServerSession:
		if s.Media() != nil && s.Media().MediaSession() != nil {
			// Stop read (1) so bridge relay goroutines exit; write (2) stops playback.
			_ = s.Media().StopRTP(3, 0)
		}
	case *diago.DialogClientSession:
		if s.Media() != nil && s.Media().MediaSession() != nil {
			_ = s.Media().StopRTP(3, 0)
		}
	}
}

// readWithContext performs a Read but returns promptly when ctx is cancelled
// (plain Read ignores bridge teardown and blocks hold/unhold). Each Read uses
// its own buffer so a cancelled in-flight read cannot race with the next one.
func readWithContext(ctx context.Context, r io.Reader, buf []byte) (int, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	type readResult struct {
		n   int
		err error
	}
	ch := make(chan readResult, 1)
	go func() {
		local := make([]byte, len(buf))
		n, err := r.Read(local)
		if n > 0 {
			copy(buf, local[:n])
		}
		ch <- readResult{n: n, err: err}
	}()
	select {
	case <-ctx.Done():
		res := <-ch
		_ = res
		return 0, ctx.Err()
	case res := <-ch:
		return res.n, res.err
	}
}

func runRelay(ctx context.Context, log *slog.Logger, stats *RelayStats, aToB bool, from, to SessionLeg, fromCodec, toCodec diagomedia.Codec) error {
	if !NeedsTranscoding(fromCodec, toCodec) {
		return runRelayCopy(ctx, stats, aToB, from, to, toCodec)
	}

	rawR, err := from.AudioReader()
	if err != nil {
		return err
	}
	rawW, err := audioRelayWriter(to, toCodec)
	if err != nil {
		return err
	}

	pcmDec, err := pcmcodec.NewPCMReader(fromCodec, rawR)
	if err != nil {
		return err
	}
	pcmEnc, err := pcmcodec.NewPCMWriter(toCodec, rawW)
	if err != nil {
		return err
	}

	readBuf := make([]byte, diagomedia.RTPBufSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := readWithContext(ctx, pcmDec, readBuf)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if n == 0 {
			continue
		}
		stats.add(aToB, n)
		if _, err := pcmEnc.Write(readBuf[:n]); err != nil {
			if log != nil {
				log.Debug("pcm bridge encode ended", "error", err)
			}
			return err
		}
	}
}

func runRelayCopy(ctx context.Context, stats *RelayStats, aToB bool, from, to SessionLeg, toCodec diagomedia.Codec) error {
	rawR, err := from.AudioReader()
	if err != nil {
		return err
	}
	if rawR == nil {
		return fmt.Errorf("no audio reader")
	}
	rawW, err := audioRelayWriter(to, toCodec)
	if err != nil {
		return err
	}
	buf := make([]byte, diagomedia.RTPBufSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := readWithContext(ctx, rawR, buf)
		if n > 0 {
			stats.add(aToB, n)
			if _, werr := rawW.Write(buf[:n]); werr != nil {
				if !errors.Is(werr, context.Canceled) {
					return werr
				}
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
	}
}

func pcmcodecBytesToSamples(b []byte) []int16 {
	if len(b) < 2 {
		return nil
	}
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(uint16(b[i*2]) | uint16(b[i*2+1])<<8)
	}
	return out
}

func pcmcodecSamplesToBytes(samples []int16, dst []byte) {
	for i, s := range samples {
		dst[i*2] = byte(s)
		dst[i*2+1] = byte(s >> 8)
	}
}
