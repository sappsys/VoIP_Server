package tones

import (
	"fmt"
	"strings"
	"time"
)

// Region selects a built-in national tone plan.
type Region string

const (
	RegionUK  Region = "uk"
	RegionEU  Region = "eu"
	RegionUSA Region = "usa"
)

// Tone is a generated signalling tone (dial, ring, busy).
type Tone struct {
	Frequencies []float64
	// Cadence alternates on/off segments; empty means continuous tone.
	Cadence []CadenceStep
}

type CadenceStep struct {
	On       bool
	Duration time.Duration
}

// Profile holds dial, ring, and busy tones for a region.
type Profile struct {
	Region      Region
	Dial        Tone
	Ring        Tone
	Busy        Tone
	BusySeconds int // how long to play busy before hangup (0 = 5)
}

func ParseRegion(s string) (Region, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "uk", "gb", "britain":
		return RegionUK, nil
	case "eu", "europe", "de", "fr":
		return RegionEU, nil
	case "us", "usa", "na", "northamerica":
		return RegionUSA, nil
	default:
		return "", fmt.Errorf("unknown tones region %q (use uk, eu, or usa)", s)
	}
}

func ProfileForRegion(r Region) Profile {
	switch r {
	case RegionEU:
		return europeanProfile()
	case RegionUSA:
		return usaProfile()
	default:
		return ukProfile()
	}
}

// ukProfile follows BT / UK PSTN specifications.
func ukProfile() Profile {
	return Profile{
		Region: RegionUK,
		Dial: Tone{
			Frequencies: []float64{350, 450},
		},
		Ring: Tone{
			Frequencies: []float64{400, 450},
			Cadence: []CadenceStep{
				{On: true, Duration: 400 * time.Millisecond},
				{On: false, Duration: 200 * time.Millisecond},
				{On: true, Duration: 400 * time.Millisecond},
				{On: false, Duration: 2000 * time.Millisecond},
			},
		},
		Busy: Tone{
			Frequencies: []float64{400},
			Cadence: []CadenceStep{
				{On: true, Duration: 375 * time.Millisecond},
				{On: false, Duration: 375 * time.Millisecond},
			},
		},
	}
}

// europeanProfile uses common continental 425 Hz patterns (ITU-T E.180).
func europeanProfile() Profile {
	return Profile{
		Region: RegionEU,
		Dial: Tone{
			Frequencies: []float64{425},
		},
		Ring: Tone{
			Frequencies: []float64{425},
			Cadence: []CadenceStep{
				{On: true, Duration: 1000 * time.Millisecond},
				{On: false, Duration: 4000 * time.Millisecond},
			},
		},
		Busy: Tone{
			Frequencies: []float64{425},
			Cadence: []CadenceStep{
				{On: true, Duration: 500 * time.Millisecond},
				{On: false, Duration: 500 * time.Millisecond},
			},
		},
	}
}

// usaProfile follows North American PSTN (FCC / ANSI T1.405).
func usaProfile() Profile {
	return Profile{
		Region: RegionUSA,
		Dial: Tone{
			Frequencies: []float64{350, 440},
		},
		Ring: Tone{
			Frequencies: []float64{440, 480},
			Cadence: []CadenceStep{
				{On: true, Duration: 2000 * time.Millisecond},
				{On: false, Duration: 4000 * time.Millisecond},
			},
		},
		Busy: Tone{
			Frequencies: []float64{480, 620},
			Cadence: []CadenceStep{
				{On: true, Duration: 500 * time.Millisecond},
				{On: false, Duration: 500 * time.Millisecond},
			},
		},
	}
}
