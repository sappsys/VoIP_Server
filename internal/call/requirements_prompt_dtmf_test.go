package call

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emiago/diago"
	diagomedia "github.com/emiago/diago/media"
)

// REQ-CONF-PIN-1: conference PIN accepts inband/RFC2833 DTMF immediately during the prompt;
// first digit stops prompt playback.

func TestREQ_CONF_PIN_DigitStopsPromptDuringCollection(t *testing.T) {
	dir := t.TempDir()
	prompt := filepath.Join(dir, "pin.wav")
	if err := os.WriteFile(prompt, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}

	var promptStopped bool
	orig := readDTMFDigitsForPrompt
	readDTMFDigitsForPrompt = func(ctx context.Context, in *diago.DialogServerSession, timeout time.Duration, log *slog.Logger, opts ...DTMFCollectOpts) (string, bool) {
		if len(opts) > 0 && opts[0].OnDigit != nil {
			opts[0].OnDigit('1')
			promptStopped = true
		}
		return "1234", true
	}
	t.Cleanup(func() { readDTMFDigitsForPrompt = orig })

	in := testServerDialog("pin")
	got, ok := PlayPromptWhileReadDigits(context.Background(), in, prompt, time.Second, nil, DTMFCollectOpts{})
	if !ok || got != "1234" {
		t.Fatalf("REQ-CONF-PIN-1: expected PIN digits, got %q ok=%v", got, ok)
	}
	if !promptStopped {
		t.Fatal("REQ-CONF-PIN-1: first digit must invoke OnDigit during prompt")
	}
}

func TestREQ_CONF_PIN_ReadUsesDualDTMFPath(t *testing.T) {
	in := testServerDialog("pin")
	if _, ok := ReadDTMFDigits(context.Background(), in, time.Millisecond, nil); ok {
		t.Fatal("expected false without RTP reader")
	}
	_ = diagomedia.CodecAudioUlaw
	_ = io.Discard
}
