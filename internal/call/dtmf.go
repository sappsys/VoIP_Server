package call

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/emiago/diago"
)

var errDTMFComplete = errors.New("dtmf entry complete")

// ReadDTMFDigits collects DTMF digits until '#' terminates entry. Returns digits
// without '#'. Entry is not accepted on timeout unless '#' was pressed.
// Both RFC 2833 (telephone-event) and in-band G.711 tones are detected.
func ReadDTMFDigits(ctx context.Context, in *diago.DialogServerSession, timeout time.Duration, log *slog.Logger) (string, bool) {
	if in == nil {
		return "", false
	}

	dtmfCodec := EnsureSessionDTMFCodec(in)
	audioCodec, ok := audioCodecFromSession(in.MediaSession())
	if !ok {
		if log != nil {
			log.Warn("dtmf audio codec unknown")
		}
		return "", false
	}
	if log != nil {
		log.Debug("dtmf reader starting", "pt", dtmfCodec.PayloadType, "audio_pt", audioCodec.PayloadType, "inband", inbandSupportedCodec(audioCodec))
	}

	ar, err := in.AudioReader()
	if err != nil {
		if log != nil {
			log.Warn("dtmf audio reader unavailable", "error", err)
		}
		return "", false
	}

	collector, err := newDualDTMFCollector(audioCodec, dtmfCodec, in.RTPPacketReader, ar, in.MediaSession())
	if err != nil {
		if log != nil {
			log.Warn("dtmf collector init failed", "error", err)
		}
		return "", false
	}

	var entered strings.Builder
	var endedWithHash atomic.Bool
	done := make(chan error, 1)

	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	go func() {
		err := collector.listen(readCtx, func(dtmf rune) error {
			if dtmf == '#' {
				endedWithHash.Store(true)
				return errDTMFComplete
			}
			if dtmf >= '0' && dtmf <= '9' {
				entered.WriteRune(dtmf)
				if log != nil {
					log.Debug("dtmf digit", "len", entered.Len())
				}
			}
			return nil
		})
		if errors.Is(err, errDTMFComplete) {
			err = nil
		}
		done <- err
	}()

	select {
	case <-readCtx.Done():
		if log != nil {
			log.Debug("dtmf collection timed out", "error", readCtx.Err(), "digits", entered.Len(), "hash", endedWithHash.Load())
		}
		return "", false
	case <-in.Context().Done():
		return "", false
	case err := <-done:
		if err != nil && log != nil {
			log.Debug("dtmf reader stopped", "error", err, "digits", entered.Len())
		}
		if endedWithHash.Load() {
			return entered.String(), true
		}
		return "", false
	}
}

// PromptAndReadDigits plays a prompt (if path non-empty) then collects DTMF digits.
// The inbound leg must already be answered.
func PromptAndReadDigits(ctx context.Context, in *diago.DialogServerSession, promptPath string, timeout time.Duration, log *slog.Logger) (string, bool) {
	_ = PlayPromptToServer(ctx, in, promptPath, log)
	return ReadDTMFDigits(ctx, in, timeout, log)
}
