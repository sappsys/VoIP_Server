package config

import "testing"

func TestSIPTransportsDefault(t *testing.T) {
	s := &ServerConfig{}
	setDefaults(&Config{Server: *s})
	if got := s.SIPTransports(); len(got) != 1 || got[0] != "udp" {
		t.Fatalf("got %v want [udp]", got)
	}
}

func TestSIPTransportsList(t *testing.T) {
	s := &ServerConfig{Transports: []string{"tcp", "udp", "tcp"}}
	got := s.SIPTransports()
	if len(got) != 2 || got[0] != "tcp" || got[1] != "udp" {
		t.Fatalf("got %v", got)
	}
}

func TestSIPTransportsCommaSeparated(t *testing.T) {
	s := &ServerConfig{Transport: "udp, tcp"}
	got := s.SIPTransports()
	if len(got) != 2 || got[0] != "udp" || got[1] != "tcp" {
		t.Fatalf("got %v", got)
	}
}

func TestSIPTransportsListOverridesTransport(t *testing.T) {
	s := &ServerConfig{Transport: "udp", Transports: []string{"tcp"}}
	got := s.SIPTransports()
	if len(got) != 1 || got[0] != "tcp" {
		t.Fatalf("got %v want [tcp]", got)
	}
}

func TestValidateSIPTransports(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Transports:        []string{"udp", "tcp"},
			RegisterMinExpiry: 60,
			RegisterMaxExpiry: 3600,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Server.Transports = []string{"sctp"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unsupported transport error")
	}
}

func TestDefaultSIPTransport(t *testing.T) {
	s := &ServerConfig{Transports: []string{"tcp", "udp"}}
	if s.DefaultSIPTransport() != "tcp" {
		t.Fatalf("got %q want tcp", s.DefaultSIPTransport())
	}
}
