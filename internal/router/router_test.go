package router

import "testing"

func TestRouteDial(t *testing.T) {
	fc := DefaultFeatureCodes()
	cases := []struct {
		dial string
		kind Kind
	}{
		{"101", KindExtension},
		{"500", KindHunt},
		{"600", KindConference},
		{"*80", KindPaging},
		{"9012345678", KindTrunk},
		{fc.Redial, KindRedial},
		{fc.CallReturn, KindCallReturn},
		{fc.Transfer, KindTransfer},
		{fc.Park, KindPark},
		{fc.ParkRetrieve + "101", KindParkRetrieve},
		{fc.DNDActivate, KindDNDActivate},
		{fc.DNDDeactivate, KindDNDDeactivate},
	}
	for _, c := range cases {
		r := RouteDial(c.dial, fc)
		if r.Kind != c.kind {
			t.Fatalf("RouteDial(%q) = %v want %v", c.dial, r.Kind, c.kind)
		}
	}
	if r := RouteDial(fc.ParkRetrieve+"101", fc); r.Target != "101" {
		t.Fatalf("park retrieve target = %q", r.Target)
	}
}

func TestRouteDialCustomCodes(t *testing.T) {
	fc := FeatureCodes{
		Redial:        "*70",
		CallReturn:    "*71",
		Transfer:      "*72",
		Park:          "*73",
		ParkRetrieve:  "*74",
		DNDActivate:   "*75",
		DNDDeactivate: "*76",
	}
	if r := RouteDial("*70", fc); r.Kind != KindRedial {
		t.Fatalf("custom redial: got %v", r.Kind)
	}
	if r := RouteDial("*74102", fc); r.Kind != KindParkRetrieve || r.Target != "102" {
		t.Fatalf("custom park retrieve: got %+v", r)
	}
}
