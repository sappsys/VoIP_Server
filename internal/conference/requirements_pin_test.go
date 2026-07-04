package conference

import (
	"testing"

	"github.com/sappsys/VoIP_Server/internal/call"
)

// REQ-CONF-PIN-1: PIN entry accepts partial digits during prompt (immediate DTMF).
// REQ-CONF-PIN-2: wrong PIN retries up to maxPINAttempts.

func TestREQ_CONF_PIN_AcceptsPartialDigitsDuringPrompt(t *testing.T) {
	opts := conferencePINCollectOpts()
	if !opts.AcceptPartialOnTimeout {
		t.Fatal("REQ-CONF-PIN-1: conference PIN must accept partial digits without trailing #")
	}
}

func TestREQ_CONF_PIN_UsesPromptWhileReadDigits(t *testing.T) {
	// collectPIN must route through PlayPromptWhileReadDigits for parallel prompt+DTMF.
	body := readConferenceCollectPINSource(t)
	if !containsAll(body, "PlayPromptWhileReadDigits", "conferencePINCollectOpts") {
		t.Fatal("REQ-CONF-PIN-1: collectPIN must use PlayPromptWhileReadDigits with conferencePINCollectOpts")
	}
}

func TestREQ_CONF_PIN_MaxAttempts(t *testing.T) {
	if maxPINAttempts < 1 {
		t.Fatalf("REQ-CONF-PIN-2: must allow at least one wrong attempt, max=%d", maxPINAttempts)
	}
	_ = call.DTMFCollectOpts{}
}
