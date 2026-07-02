package pbx

import (
	"testing"

	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/presence"
	"github.com/sappsys/VoIP_Server/internal/registrar"
)

func TestPresenceState(t *testing.T) {
	exts := map[string]*config.Extension{
		"101": {Extension: "101", DisplayName: "Alice", Enabled: true},
		"102": {Extension: "102", DisplayName: "Bob", Enabled: true, DND: true},
	}
	reg := registrar.New("test", config.ServerConfig{}, exts, nil)
	reg.RegisterForTest("101", sip.Uri{User: "101", Host: "127.0.0.1", Port: 5060})

	s := &Server{exts: exts, reg: reg}
	st := s.presenceState("101")
	if st.Basic != presence.BasicOpen {
		t.Fatalf("101=%s", st.Basic)
	}
	st = s.presenceState("102")
	if st.Basic != presence.BasicClosed {
		t.Fatalf("102 offline=%s", st.Basic)
	}
	reg.RegisterForTest("102", sip.Uri{User: "102", Host: "127.0.0.1", Port: 5061})
	st = s.presenceState("102")
	if st.Basic != presence.BasicBusy {
		t.Fatalf("102 dnd=%s", st.Basic)
	}
}
