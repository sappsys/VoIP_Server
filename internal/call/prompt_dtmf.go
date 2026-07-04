package call

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/emiago/diago"
)

// DTMFCollectOpts configures digit collection behaviour.
type DTMFCollectOpts struct {
	// OnDigit is invoked for every detected digit (including '#').
	OnDigit func(rune)
	// AcceptPartialOnTimeout returns entered digits when the timeout fires even
	// without a trailing '#'. Used for conference PIN entry.
	AcceptPartialOnTimeout bool
}

// PlayPromptWhileReadDigits plays promptPath while collecting DTMF in parallel.
// Playback stops as soon as any digit is detected. The inbound leg must already
// be answered. Empty promptPath skips playback.
func PlayPromptWhileReadDigits(ctx context.Context, in *diago.DialogServerSession, promptPath string, timeout time.Duration, log *slog.Logger, opts DTMFCollectOpts) (string, bool) {
	if in == nil {
		return "", false
	}

	var pb *diago.AudioPlaybackControl
	var stopOnce sync.Once
	stopPrompt := func() {
		stopOnce.Do(func() {
			if pb != nil {
				pb.Stop()
			}
		})
	}

	if promptPath != "" {
		if _, err := os.Stat(promptPath); err == nil {
			if in.MediaSession() != nil {
				if p, err := in.PlaybackControlCreate(); err == nil {
					pb = &p
					promptCtx, promptCancel := context.WithCancel(ctx)
					defer promptCancel()
					go func() {
						defer promptCancel()
						playPromptFile(promptCtx, dialogContext(in), pb, promptPath, log)
					}()
				} else if log != nil {
					log.Warn("prompt playback control unavailable", "error", err)
				}
			} else if log != nil {
				log.Debug("prompt skipped, media not negotiated", "path", promptPath)
			}
		} else if log != nil {
			log.Warn("prompt file missing", "path", promptPath, "error", err)
		}
	}

	if opts.OnDigit == nil {
		opts.OnDigit = func(rune) {}
	}
	orig := opts.OnDigit
	opts.OnDigit = func(d rune) {
		stopPrompt()
		orig(d)
	}

	return readDTMFDigitsForPrompt(ctx, in, timeout, log, opts)
}

// readDTMFDigitsForPrompt is the DTMF collector used during prompts; tests may replace it.
var readDTMFDigitsForPrompt = ReadDTMFDigits

func playPromptFile(ctx context.Context, callDone context.Context, pb *diago.AudioPlaybackControl, path string, log *slog.Logger) {
	codec := pb.Codec()
	reader, mime, err := openAudioPlaybackReader(path, codec)
	if err != nil {
		if log != nil {
			log.Warn("prompt open failed", "path", path, "error", err)
		}
		return
	}
	var closeFn func()
	if f, ok := reader.(*os.File); ok {
		closeFn = func() { _ = f.Close() }
	} else {
		closeFn = func() {}
	}
	defer closeFn()

	playDone := make(chan struct{})
	go func() {
		defer close(playDone)
		_, _ = pb.Play(reader, mime)
	}()

	if callDone == nil {
		select {
		case <-ctx.Done():
			pb.Stop()
		case <-playDone:
		}
		return
	}
	select {
	case <-ctx.Done():
		pb.Stop()
	case <-callDone.Done():
		pb.Stop()
	case <-playDone:
	}
}
