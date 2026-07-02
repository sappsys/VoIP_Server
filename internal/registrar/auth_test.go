package registrar

import (
	"log/slog"
	"testing"

	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
)

func TestSenderAuthorizedMatchesBindingSource(t *testing.T) {
	reg := New("test", config.ServerConfig{}, map[string]*config.Extension{
		"101": {Extension: "101", Password: "x", Enabled: true},
	}, slog.Default())
	reg.RegisterForTest("101", sip.Uri{User: "101", Host: "192.168.1.50", Port: 15060})

	if !reg.SenderAuthorized("101", "192.168.1.50:15060") {
		t.Fatal("expected authorized")
	}
	if reg.SenderAuthorized("101", "10.0.0.1:5060") {
		t.Fatal("expected unauthorized for spoofed source")
	}
	if reg.SenderAuthorized("102", "192.168.1.50:15060") {
		t.Fatal("expected unknown extension unauthorized")
	}
}
