package config

import (
	"fmt"
	"strings"

	"github.com/sappsys/VoIP_Server/internal/router"
)

// FeaturesConfig holds configurable star-code strings from config.toml.
type FeaturesConfig struct {
	Redial        string `toml:"redial"`
	CallReturn    string `toml:"call_return"`
	Transfer      string `toml:"transfer"`
	Park          string `toml:"park"`
	ParkRetrieve  string `toml:"park_retrieve"`
	DNDActivate   string `toml:"dnd_activate"`
	DNDDeactivate string `toml:"dnd_deactivate"`
}

func normalizeStarCode(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "*") {
		return "*" + s
	}
	return s
}

func setFeatureDefaults(f *FeaturesConfig) {
	def := router.DefaultFeatureCodes()
	if f.Redial == "" {
		f.Redial = def.Redial
	}
	if f.CallReturn == "" {
		f.CallReturn = def.CallReturn
	}
	if f.Transfer == "" {
		f.Transfer = def.Transfer
	}
	if f.Park == "" {
		f.Park = def.Park
	}
	if f.ParkRetrieve == "" {
		f.ParkRetrieve = def.ParkRetrieve
	}
	if f.DNDActivate == "" {
		f.DNDActivate = def.DNDActivate
	}
	if f.DNDDeactivate == "" {
		f.DNDDeactivate = def.DNDDeactivate
	}
	f.Redial = normalizeStarCode(f.Redial)
	f.CallReturn = normalizeStarCode(f.CallReturn)
	f.Transfer = normalizeStarCode(f.Transfer)
	f.Park = normalizeStarCode(f.Park)
	f.ParkRetrieve = normalizeStarCode(f.ParkRetrieve)
	f.DNDActivate = normalizeStarCode(f.DNDActivate)
	f.DNDDeactivate = normalizeStarCode(f.DNDDeactivate)
}

// FeatureCodes returns router feature codes from config.
func (f FeaturesConfig) FeatureCodes() router.FeatureCodes {
	return router.FeatureCodes{
		Redial:        f.Redial,
		CallReturn:    f.CallReturn,
		Transfer:      f.Transfer,
		Park:          f.Park,
		ParkRetrieve:  f.ParkRetrieve,
		DNDActivate:   f.DNDActivate,
		DNDDeactivate: f.DNDDeactivate,
	}
}

func (c *Config) validateFeatures() error {
	setFeatureDefaults(&c.Features)
	codes := []struct {
		name, code string
	}{
		{"redial", c.Features.Redial},
		{"call_return", c.Features.CallReturn},
		{"transfer", c.Features.Transfer},
		{"park", c.Features.Park},
		{"park_retrieve", c.Features.ParkRetrieve},
		{"dnd_activate", c.Features.DNDActivate},
		{"dnd_deactivate", c.Features.DNDDeactivate},
	}
	seen := map[string]string{}
	for _, item := range codes {
		if !strings.HasPrefix(item.code, "*") {
			return fmt.Errorf("features.%s: must start with *", item.name)
		}
		if prev, ok := seen[item.code]; ok {
			return fmt.Errorf("features: duplicate code %q (%s and %s)", item.code, prev, item.name)
		}
		seen[item.code] = item.name
	}
	if len(c.Features.ParkRetrieve) < 2 {
		return fmt.Errorf("features.park_retrieve: invalid code")
	}
	return nil
}
