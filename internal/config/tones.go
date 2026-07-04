package config

import (
	"fmt"

	"github.com/sappsys/VoIP_Server/internal/media/tones"
)

// TonesConfig selects regional signalling tones (dial, ring, busy).
type TonesConfig struct {
	// Region is uk, eu, or usa (default uk).
	Region string `toml:"region"`
	// BusySeconds is how long the busy tone plays before hangup (default 5).
	BusySeconds int `toml:"busy_seconds"`
}

func (t *TonesConfig) ApplyDefaults() {
	if t.Region == "" {
		t.Region = string(tones.RegionUK)
	}
	if t.BusySeconds <= 0 {
		t.BusySeconds = 5
	}
}

func (t *TonesConfig) Profile() (tones.Profile, error) {
	t.ApplyDefaults()
	r, err := tones.ParseRegion(t.Region)
	if err != nil {
		return tones.Profile{}, err
	}
	p := tones.ProfileForRegion(r)
	p.BusySeconds = t.BusySeconds
	return p, nil
}

// Validate checks tone configuration.
func (t *TonesConfig) Validate() error {
	t.ApplyDefaults()
	if _, err := tones.ParseRegion(t.Region); err != nil {
		return fmt.Errorf("tones: %w", err)
	}
	return nil
}
