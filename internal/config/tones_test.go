package config

import (
	"testing"

	"github.com/sappsys/VoIP_Server/internal/media/tones"
)

func TestTonesConfigProfile(t *testing.T) {
	cfg := TonesConfig{Region: "usa"}
	p, err := cfg.Profile()
	if err != nil {
		t.Fatal(err)
	}
	if p.Region != tones.RegionUSA {
		t.Fatalf("region=%q", p.Region)
	}
	if len(p.Dial.Frequencies) != 2 {
		t.Fatalf("usa dial freqs=%v", p.Dial.Frequencies)
	}
}

func TestTonesConfigBusySeconds(t *testing.T) {
	cfg := TonesConfig{Region: "uk", BusySeconds: 7}
	p, err := cfg.Profile()
	if err != nil {
		t.Fatal(err)
	}
	if p.BusySeconds != 7 {
		t.Fatalf("busy_seconds=%d", p.BusySeconds)
	}
}

func TestTonesConfigDefaultUK(t *testing.T) {
	var cfg TonesConfig
	cfg.ApplyDefaults()
	p, err := cfg.Profile()
	if err != nil {
		t.Fatal(err)
	}
	if p.Region != tones.RegionUK {
		t.Fatalf("region=%q", p.Region)
	}
}

func TestTonesConfigInvalid(t *testing.T) {
	cfg := TonesConfig{Region: "mars"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	}
}
