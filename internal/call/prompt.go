package call

import (
	"context"
	"log/slog"
	"os"

	"github.com/emiago/diago"
)

// PlayPromptToServer answers the inbound leg if needed and plays a single WAV
// prompt once. It blocks until playback finishes, the file ends, or the caller
// hangs up. Empty path is a no-op (prompt disabled).
func PlayPromptToServer(ctx context.Context, in *diago.DialogServerSession, path string, log *slog.Logger) error {
	if path == "" || in == nil {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		if log != nil {
			log.Warn("prompt file missing", "path", path, "error", err)
		}
		return err
	}
	pb, err := in.PlaybackCreate()
	if err != nil {
		return err
	}
	if !playWavFile(ctx, in.Context(), &pb, path, log) {
		if log != nil {
			log.Debug("prompt playback ended", "path", path)
		}
	}
	return nil
}

// AnswerAndPrompt sends Trying, answers, plays a prompt once, then hangs up.
// Useful for terminal announcements (busy, wrong number, unavailable).
func AnswerAndPrompt(ctx context.Context, in *diago.DialogServerSession, path string, log *slog.Logger) {
	if in == nil {
		return
	}
	in.Trying()
	if err := AnswerSession(in); err != nil {
		if log != nil {
			log.Debug("prompt answer failed", "path", path, "error", err)
		}
		return
	}
	_ = PlayPromptToServer(ctx, in, path, log)
	in.Hangup(ctx)
}
