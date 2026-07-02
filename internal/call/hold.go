package call

import (
	"context"
	"log/slog"

	"github.com/emiago/diago"
)

// PlayMOHToClient streams WAV files from mohDir to an outbound dialog leg.
// Tracks are played in alphanumeric filename order, looping the playlist.
func PlayMOHToClient(ctx context.Context, out *diago.DialogClientSession, mohDir string, log *slog.Logger) {
	if mohDir == "" || out == nil {
		return
	}
	tracks, err := MOHTracks(mohDir)
	if err != nil || len(tracks) == 0 {
		if log != nil && err != nil {
			log.Warn("moh directory missing", "path", mohDir, "error", err)
		}
		return
	}
	pb, err := out.PlaybackCreate()
	if err != nil {
		if log != nil {
			log.Warn("moh playback create failed", "error", err)
		}
		return
	}
	for {
		for _, path := range tracks {
			if !playWavFile(ctx, out.Context(), &pb, path, log) {
				return
			}
		}
	}
}

// PlayMOHToServer streams WAV files from mohDir to an inbound dialog leg.
// Tracks are played in alphanumeric filename order, looping the playlist.
func PlayMOHToServer(ctx context.Context, in *diago.DialogServerSession, mohDir string, log *slog.Logger) {
	if mohDir == "" || in == nil {
		return
	}
	tracks, err := MOHTracks(mohDir)
	if err != nil || len(tracks) == 0 {
		if log != nil && err != nil {
			log.Warn("moh directory missing", "path", mohDir, "error", err)
		}
		return
	}
	pb, err := in.PlaybackCreate()
	if err != nil {
		if log != nil {
			log.Warn("moh playback create failed", "error", err)
		}
		return
	}
	for {
		for _, path := range tracks {
			if !playWavFile(ctx, in.Context(), &pb, path, log) {
				return
			}
		}
	}
}
