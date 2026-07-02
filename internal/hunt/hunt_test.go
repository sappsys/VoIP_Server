package hunt

import (
	"testing"

	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/registrar"
)

func TestReachableMembers(t *testing.T) {
	exts := map[string]*config.Extension{
		"101": {Extension: "101", Enabled: true},
		"102": {Extension: "102", Enabled: true},
		"103": {Extension: "103", Enabled: true},
	}
	reg := registrar.New("test.local", config.ServerConfig{}, exts, nil)
	reg.RegisterForTest("102", sip.Uri{User: "102", Host: "127.0.0.1", Port: 5060})

	h := NewHandler(reg, nil, nil)
	members := []string{"101", "102", "103"}
	var reachable []string
	for _, m := range members {
		if _, ok := h.Reg.ContactURI(m); ok {
			reachable = append(reachable, m)
		}
	}
	if len(reachable) != 1 || reachable[0] != "102" {
		t.Fatalf("reachable=%v want [102]", reachable)
	}
}
