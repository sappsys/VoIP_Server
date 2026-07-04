package config

import "testing"

func TestNATDefaults(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)
	if cfg.NAT.STUNPort != 3478 {
		t.Fatalf("stun_port=%d", cfg.NAT.STUNPort)
	}
	if cfg.NAT.SIPProxyPort != 5061 {
		t.Fatalf("sip_proxy_port=%d", cfg.NAT.SIPProxyPort)
	}
}

func TestSTUNBindHost(t *testing.T) {
	cfg := &Config{Server: ServerConfig{BindHost: "0.0.0.0"}}
	if cfg.STUNBindHost() != "0.0.0.0" {
		t.Fatalf("got %q", cfg.STUNBindHost())
	}
	cfg.NAT.STUNBindHost = "127.0.0.1"
	if cfg.STUNBindHost() != "127.0.0.1" {
		t.Fatalf("got %q", cfg.STUNBindHost())
	}
}

func TestValidateNATPortConflict(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			BindPort:          5060,
			RegisterMinExpiry: 60,
			RegisterMaxExpiry: 3600,
		},
		NAT: NATConfig{
			STUNEnabled: true,
			STUNPort:    5060,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected stun/bind_port conflict error")
	}
}
