package call

import (
	"testing"

	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
)

func TestManagerAcquireRelease(t *testing.T) {
	m := NewManager(10)
	s, err := m.TryAcquire("c1", "101", "102", "Alice", nil)
	if err != nil || s == nil {
		t.Fatalf("acquire: %v", err)
	}
	if m.Active() != 1 || m.ExtensionActive("101") != 1 {
		t.Fatalf("active counts wrong: %d %d", m.Active(), m.ExtensionActive("101"))
	}
	m.Release("c1")
	if m.Active() != 0 || m.ExtensionActive("101") != 0 {
		t.Fatal("expected release to clear counts")
	}
}

func TestManagerExtensionBusy(t *testing.T) {
	m := NewManager(10)
	exts := map[string]*config.Extension{
		"101": {Extension: "101", MaxSimultaneousCalls: 1, CallWaiting: false},
	}
	_, err := m.TryAcquire("c1", "101", "102", "", exts)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.TryAcquire("c2", "101", "103", "", exts)
	if err == nil {
		t.Fatal("expected busy")
	}
}

func TestManagerCallWaiting(t *testing.T) {
	m := NewManager(10)
	exts := map[string]*config.Extension{
		"101": {Extension: "101", MaxSimultaneousCalls: 1, CallWaiting: true},
	}
	_, err := m.TryAcquire("c1", "101", "102", "", exts)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.TryAcquire("c2", "101", "103", "", exts)
	if err != nil {
		t.Fatalf("call waiting should allow second call: %v", err)
	}
}

func TestOutboundHeaders(t *testing.T) {
	h := OutboundHeaders("Alice", "101", "pbx.local")
	if len(h) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(h))
	}
	if CallerNameHeader("") != nil {
		t.Fatal("empty name should be nil")
	}
	pai := PAssertedIdentity("Alice", "101", "pbx.local")
	if pai == nil {
		t.Fatal("expected PAI header")
	}
	_ = sip.Header(pai)
}

func TestIntercomHeaders(t *testing.T) {
	h := IntercomHeaders()
	if len(h) != 2 {
		t.Fatalf("expected 2 intercom headers, got %d", len(h))
	}
}
