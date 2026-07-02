package call

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/emiago/diago"
)

// playWavFile plays a single WAV file once through pb. Returns false when the
// call or ctx ends, or playback fails. Missing files are skipped (returns true).
func playWavFile(ctx context.Context, callDone context.Context, pb *diago.AudioPlayback, path string, log *slog.Logger) bool {
	select {
	case <-ctx.Done():
		return false
	case <-callDone.Done():
		return false
	default:
	}

	codec := pb.Codec()
	reader, mime, err := openWavPlaybackReader(path, codec)
	if err != nil {
		if log != nil {
			log.Warn("wav open failed", "path", path, "error", err)
		}
		return false
	}

	var closeFn func()
	if f, ok := reader.(*os.File); ok {
		closeFn = func() { _ = f.Close() }
	} else {
		closeFn = func() {}
	}
	defer closeFn()

	if mime == "audio/wav" {
		reader = bufio.NewReaderSize(reader, 64*1024)
	}

	_, err = pb.Play(reader, mime)
	if err != nil && err != io.EOF {
		if log != nil {
			log.Warn("wav playback failed", "path", path, "error", err)
		}
		return false
	}
	return true
}
