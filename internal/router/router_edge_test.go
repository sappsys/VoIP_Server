package router

import "testing"

func TestRouteDialEdgeCases(t *testing.T) {
	fc := FeatureCodes{
		Redial: "*70", CallReturn: "*71", Transfer: "*72", Park: "*73",
		ParkRetrieve: "*88", DNDActivate: "*78", DNDDeactivate: "*79",
	}
	cases := []struct {
		dial string
		kind Kind
		target string
	}{
		{"*88", KindParkRetrieve, ""},
		{"*88102", KindParkRetrieve, "102"},
		{"*73", KindPark, ""},
		{"*99", KindPaging, "99"},
		{"90", KindUnknown, "90"},
		{"9012345", KindTrunk, "9012345"},
		{"50", KindUnknown, "50"},
		{"700", KindUnknown, "700"},
	}
	for _, c := range cases {
		r := RouteDial(c.dial, fc)
		if r.Kind != c.kind {
			t.Fatalf("RouteDial(%q) kind=%v want %v", c.dial, r.Kind, c.kind)
		}
		if c.target != "" && r.Target != c.target {
			t.Fatalf("RouteDial(%q) target=%q want %q", c.dial, r.Target, c.target)
		}
	}
}

func TestRouteDialParkRetrieveBareCode(t *testing.T) {
	fc := DefaultFeatureCodes()
	r := RouteDial(fc.ParkRetrieve, fc)
	if r.Kind != KindParkRetrieve {
		t.Fatalf("*86 alone should be park retrieve, got %v", r.Kind)
	}
	if r.Target != "" {
		t.Fatalf("bare park retrieve target should be empty, got %q", r.Target)
	}
}
