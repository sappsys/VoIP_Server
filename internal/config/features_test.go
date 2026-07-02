package config

import "testing"

func TestFeatureDefaults(t *testing.T) {
	var f FeaturesConfig
	setFeatureDefaults(&f)
	if f.Redial != "*66" || f.ParkRetrieve != "*86" {
		t.Fatalf("defaults: %+v", f)
	}
}

func TestFeatureCodesNormalize(t *testing.T) {
	f := FeaturesConfig{Redial: "66", ParkRetrieve: "86"}
	setFeatureDefaults(&f)
	if f.Redial != "*66" || f.ParkRetrieve != "*86" {
		t.Fatalf("normalize: %+v", f)
	}
}

func TestValidateFeaturesDuplicate(t *testing.T) {
	cfg := &Config{
		Features: FeaturesConfig{
			Redial:       "*66",
			CallReturn:   "*66",
			Transfer:     "*77",
			Park:         "*85",
			ParkRetrieve: "*86",
			DNDActivate:  "*78",
			DNDDeactivate: "*79",
		},
	}
	if err := cfg.validateFeatures(); err == nil {
		t.Fatal("expected duplicate error")
	}
}
