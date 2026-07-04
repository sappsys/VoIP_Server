package call

import (
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/media/tones"
)

// REQ-CALL-1: while callee is ringing, caller hears a geographic ringback tone.
// REQ-CALL-2: when callee is not connected, caller hears busy tone then hangup (default 5s).

func TestREQ_CALL_BusyDurationDefaultsFiveSeconds(t *testing.T) {
	d := busyDuration(tones.Profile{})
	if d != 5*time.Second {
		t.Fatalf("REQ-CALL-2: default busy duration=%v want 5s", d)
	}
}

func TestREQ_CALL_BusyDurationFromProfile(t *testing.T) {
	d := busyDuration(tones.Profile{BusySeconds: 7})
	if d != 7*time.Second {
		t.Fatalf("REQ-CALL-2: busy duration=%v want 7s", d)
	}
}

func TestREQ_CALL_GeographicRingProfilesDiffer(t *testing.T) {
	uk := tones.ProfileForRegion(tones.RegionUK)
	usa := tones.ProfileForRegion(tones.RegionUSA)
	if len(uk.Ring.Cadence) == 0 || len(usa.Ring.Cadence) == 0 {
		t.Fatal("REQ-CALL-1: ring profiles must define cadence")
	}
	if len(uk.Ring.Cadence) == len(usa.Ring.Cadence) {
		t.Fatal("REQ-CALL-1: UK and USA ring cadences must differ")
	}
}

func TestREQ_CALL_BridgeUsesConfiguredTones(t *testing.T) {
	bp := &BridgePair{Tones: tones.ProfileForRegion(tones.RegionUSA)}
	p := bp.TonesProfile()
	if p.Region != tones.RegionUSA {
		t.Fatalf("REQ-CALL-1: bridge must use configured region, got %q", p.Region)
	}
}

func TestREQ_CALL_RingbackOnRingingResponses(t *testing.T) {
	if !responseStartsRingback(sip.StatusRinging) {
		t.Fatal("REQ-CALL-1: 180 Ringing must start ringback")
	}
	if !responseStartsRingback(sip.StatusSessionInProgress) {
		t.Fatal("REQ-CALL-1: 183 Session Progress must start ringback")
	}
	if responseStartsRingback(sip.StatusOK) {
		t.Fatal("200 OK must not start ringback")
	}
}
