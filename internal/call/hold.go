package call

import (
	"context"
	"io"
	"log/slog"
	"os"

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
	w, err := out.AudioWriter()
	if err != nil {
		return
	}
	for {
		for _, path := range tracks {
			if !streamFileToClient(ctx, out, w, path) {
				return
			}
		}
	}
}

func streamFileToClient(ctx context.Context, out *diago.DialogClientSession, w io.Writer, path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true // skip missing track
	}
	defer f.Close()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-out.Context().Done():
			return false
		default:
			buf := make([]byte, 4096)
			n, err := f.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
			}
			if err == io.EOF {
				return true
			}
			if err != nil {
				return false
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
		return
	}
	for {
		for _, path := range tracks {
			if !playFileToServer(ctx, in, &pb, path) {
				return
			}
		}
	}
}

func playFileToServer(ctx context.Context, in *diago.DialogServerSession, pb *diago.AudioPlayback, path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-in.Context().Done():
			return false
		default:
			_, err := pb.Play(f, "audio/wav")
			if err == io.EOF {
				return true
			}
			if err != nil {
				return false
			}
		}
	}
}
