package config

import (
	"path/filepath"
	"testing"

	"github.com/sappsys/VoIP_Server/internal/media/tones"
)

func TestSoundDefaults(t *testing.T) {
	var s SoundsConfig
	setSoundDefaults(&s)
	if s.Dir != defaultSoundsDir {
		t.Fatalf("dir=%q", s.Dir)
	}
	if s.Busy != "" {
		t.Fatalf("busy should default empty (uses tones): %q", s.Busy)
	}
	if s.WrongNumber != defaultSoundWrongNumber {
		t.Fatalf("wrong_number=%q", s.WrongNumber)
	}
	if s.ConfPIN != defaultSoundConfPIN || s.ConfPINBad != defaultSoundConfPINBad {
		t.Fatalf("conf pins=%q/%q", s.ConfPIN, s.ConfPINBad)
	}
	if s.Extension != defaultSoundExtension {
		t.Fatalf("extension=%q", s.Extension)
	}
}

func TestSoundDefaultsPreserveOverrides(t *testing.T) {
	s := SoundsConfig{Busy: "custom-busy.wav"}
	setSoundDefaults(&s)
	if s.Busy != "custom-busy.wav" {
		t.Fatalf("override lost: %q", s.Busy)
	}
	if s.ConfPIN != defaultSoundConfPIN {
		t.Fatalf("default not filled: %q", s.ConfPIN)
	}
}

func TestSoundPathResolution(t *testing.T) {
	s := SoundsConfig{Dir: "assets/sounds"}

	// Empty filename -> disabled.
	if got := s.SoundPath("/app", ""); got != "" {
		t.Fatalf("empty should disable, got %q", got)
	}

	// Relative dir joined with cfgDir.
	got := s.SoundPath("/app", "busy.wav")
	want := filepath.Join("/app", "assets/sounds", "busy.wav")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// Absolute filename returned as-is.
	if got := s.SoundPath("/app", "/snd/busy.wav"); got != "/snd/busy.wav" {
		t.Fatalf("absolute filename changed: %q", got)
	}

	// Absolute dir not prefixed by cfgDir.
	sAbs := SoundsConfig{Dir: "/opt/snd"}
	if got := sAbs.SoundPath("/app", "busy.wav"); got != filepath.Join("/opt/snd", "busy.wav") {
		t.Fatalf("absolute dir: %q", got)
	}
}

func TestLoadConfigSoundDefaults(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)
	if cfg.Sounds.Dir != defaultSoundsDir {
		t.Fatalf("sounds dir default not set: %q", cfg.Sounds.Dir)
	}
	if cfg.Sounds.WrongNumber == "" {
		t.Fatal("sound filenames not defaulted")
	}
	if cfg.Tones.Region != string(tones.RegionUK) {
		t.Fatalf("tones region=%q", cfg.Tones.Region)
	}
	if cfg.Tones.BusySeconds != 5 {
		t.Fatalf("busy_seconds=%d", cfg.Tones.BusySeconds)
	}
	if cfg.Limits.RingTimeoutSeconds != 30 {
		t.Fatalf("ring_timeout=%d", cfg.Limits.RingTimeoutSeconds)
	}
}
