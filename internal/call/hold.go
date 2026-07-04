package call

import (
	"context"
	"log/slog"

	"github.com/emiago/diago"
)

// PlayMOHToServer streams WAV files from mohDir to an inbound dialog leg.
// Prefer StartMOHServer when playback must be stopped before other media.
func PlayMOHToServer(ctx context.Context, in *diago.DialogServerSession, mohDir string, log *slog.Logger) {
	sess := StartMOHServer(ctx, in, mohDir, log)
	if sess == nil {
		return
	}
	defer sess.Stop()
	<-ctx.Done()
}

// PlayMOHToClient streams WAV files from mohDir to an outbound dialog leg.
// Prefer StartMOHClient when playback must be stopped before other media.
func PlayMOHToClient(ctx context.Context, out *diago.DialogClientSession, mohDir string, log *slog.Logger) {
	sess := StartMOHClient(ctx, out, mohDir, log)
	if sess == nil {
		return
	}
	defer sess.Stop()
	<-ctx.Done()
}
