package config

import "testing"

func TestSIPBindHostUsesExternalWhenWildcard(t *testing.T) {
	cfg := &Config{Server: ServerConfig{BindHost: "0.0.0.0", ExternalHost: "192.168.1.10"}}
	if got := cfg.SIPBindHost(); got != "192.168.1.10" {
		t.Fatalf("SIPBindHost=%q", got)
	}
}

func TestSIPBindHostKeepsExplicitBind(t *testing.T) {
	cfg := &Config{Server: ServerConfig{BindHost: "10.0.0.5", ExternalHost: "192.168.1.10"}}
	if got := cfg.SIPBindHost(); got != "10.0.0.5" {
		t.Fatalf("SIPBindHost=%q", got)
	}
}
