package conference

import (
	"strings"
	"testing"
)

// REQ-CONF-1b: when MOH is active, mixer_add must never precede moh_stop.
func TestREQ_CONF_MixerNeverStartsBeforeMOHStops(t *testing.T) {
	log := &mediaEventLog{}
	room := newTestRoom(t, log)
	mohDir := testMOHDir(t)

	room.applyMediaLocked(dummySessions(1), mohDir, nil)
	room.applyMediaLocked(dummySessions(2), mohDir, nil)

	got := log.events
	stopIdx := -1
	addIdx := -1
	for i, ev := range got {
		if ev == "moh_stop" && stopIdx < 0 {
			stopIdx = i
		}
		if ev == "mixer_add" && addIdx < 0 {
			addIdx = i
		}
	}
	if stopIdx < 0 {
		t.Fatal("REQ-CONF-1: expected moh_stop when leaving solo MOH for mixer")
	}
	if addIdx < 0 {
		t.Fatal("expected mixer_add when second participant joins")
	}
	if addIdx < stopIdx {
		t.Fatalf("REQ-CONF-1: mixer_add at %d before moh_stop at %d; log=%s", addIdx, stopIdx, strings.Join(got, ","))
	}
}
