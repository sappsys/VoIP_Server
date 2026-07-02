package trunk

import (
	"testing"

	"github.com/sappsys/VoIP_Server/internal/config"
)

func TestTrunkServerDest(t *testing.T) {
	if got := trunkServerDest("sip.carrier.com:5061"); got != "sip.carrier.com:5061" {
		t.Fatalf("got %q", got)
	}
	if got := trunkServerDest("10.0.0.1"); got != "10.0.0.1:5060" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeTrunkKeepaliveUsedByHandler(t *testing.T) {
	mode, err := config.NormalizeTrunkKeepalive("")
	if err != nil || mode != "options" {
		t.Fatalf("default: %q %v", mode, err)
	}
	mode, err = config.NormalizeTrunkKeepalive("register")
	if err != nil || mode != "register" {
		t.Fatalf("register: %q %v", mode, err)
	}
	_, err = config.NormalizeTrunkKeepalive("bogus")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateTrunkRegisterKeepaliveRequiresUsername(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{RegisterMinExpiry: 60, RegisterMaxExpiry: 3600},
		Trunks: []config.TrunkConfig{{
			Name: "PSTN", Prefix: "9", Server: "gw.example.com",
			Keepalive: "register", Enabled: true,
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestTrunkPerTrunkIntervals(t *testing.T) {
	tr := config.TrunkConfig{KeepaliveSeconds: 45, RegisterExpirySeconds: 7200}
	if tr.KeepaliveInterval() != 45*1e9 {
		t.Fatalf("interval=%v", tr.KeepaliveInterval())
	}
	if tr.RegisterExpiry() != 7200*1e9 {
		t.Fatalf("expiry=%v", tr.RegisterExpiry())
	}
	tr = config.TrunkConfig{}
	if tr.KeepaliveInterval() != 30*1e9 {
		t.Fatalf("default interval=%v", tr.KeepaliveInterval())
	}
	if tr.RegisterExpiry() != 3600*1e9 {
		t.Fatalf("default expiry=%v", tr.RegisterExpiry())
	}
}

func TestValidateRegisterExpiryMin(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{RegisterMinExpiry: 60, RegisterMaxExpiry: 3600},
		Trunks: []config.TrunkConfig{{
			Name: "PSTN", Prefix: "9", Server: "gw.example.com",
			RegisterExpirySeconds: 30, Enabled: true,
		}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
