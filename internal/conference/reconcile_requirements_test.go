package conference

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emiago/diago"
	"github.com/sappsys/VoIP_Server/internal/media/audiobridge"
)

// REQ-CONF-1: MOH must stop before mixer starts.
// REQ-CONF-2: exactly one participant → MOH only, no mixer adds.
// REQ-CONF-3: two or more participants → mixer only, no MOH.
// REQ-CONF-4: when dropping from 2→1, MOH restarts.
// REQ-CONF-5: phone hold does not reduce admitted count; no MOH while 2+ admitted.

type mediaEventLog struct {
	events []string
}

func (l *mediaEventLog) add(ev string) {
	l.events = append(l.events, ev)
}

func (l *mediaEventLog) joined() string {
	return strings.Join(l.events, ",")
}

type recordingMixer struct {
	log *mediaEventLog
}

func (m *recordingMixer) RemoveAll() {
	m.log.add("mixer_remove")
}

func (m *recordingMixer) Add(_ audiobridge.SessionLeg) error {
	m.log.add("mixer_add")
	return nil
}

type fakeMOH struct {
	log    *mediaEventLog
	name   string
	stopped bool
}

func (f *fakeMOH) Stop() {
	if !f.stopped {
		f.log.add(f.name + "_stop")
		f.stopped = true
	}
}

func testMOHDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "moh.wav"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func dummySessions(n int) []*diago.DialogServerSession {
	out := make([]*diago.DialogServerSession, n)
	for i := range out {
		out[i] = &diago.DialogServerSession{}
	}
	return out
}

func newTestRoom(t *testing.T, log *mediaEventLog) *Room {
	t.Helper()
	orig := conferenceMOHStart
	t.Cleanup(func() { conferenceMOHStart = orig })
	conferenceMOHStart = func(_ context.Context, _ *diago.DialogServerSession, _ string, _ *slog.Logger) stoppableMOH {
		log.add("moh_start")
		return &fakeMOH{log: log, name: "moh"}
	}
	return &Room{Number: "600", Mixer: &recordingMixer{log: log}}
}

func TestREQ_CONF_OneParticipantStartsMOHOnly(t *testing.T) {
	log := &mediaEventLog{}
	room := newTestRoom(t, log)
	room.count = 1

	room.applyMediaLocked(dummySessions(1), testMOHDir(t), nil)

	got := log.joined()
	if !strings.Contains(got, "mixer_remove") {
		t.Fatalf("expected mixer_remove first, got %q", got)
	}
	if !strings.Contains(got, "moh_start") {
		t.Fatalf("expected moh_start, got %q", got)
	}
	if strings.Contains(got, "mixer_add") {
		t.Fatalf("REQ-CONF-2: must not add to mixer with one participant, got %q", got)
	}
}

func TestREQ_CONF_TwoParticipantsMixerOnlyNoMOH(t *testing.T) {
	log := &mediaEventLog{}
	room := newTestRoom(t, log)
	room.count = 2

	room.applyMediaLocked(dummySessions(2), testMOHDir(t), nil)

	got := log.joined()
	if want := "mixer_remove,mixer_add,mixer_add"; got != want {
		t.Fatalf("REQ-CONF-3: got %q want %q", got, want)
	}
	if strings.Contains(got, "moh_start") {
		t.Fatalf("REQ-CONF-3: must not start MOH with 2 participants, got %q", got)
	}
}

func TestREQ_CONF_MOHStopsBeforeMixerOnSecondJoin(t *testing.T) {
	log := &mediaEventLog{}
	room := newTestRoom(t, log)
	mohDir := testMOHDir(t)

	room.count = 1
	room.applyMediaLocked(dummySessions(1), mohDir, nil)
	if room.moh == nil {
		t.Fatal("expected active MOH after first reconcile")
	}

	room.count = 2
	room.applyMediaLocked(dummySessions(2), mohDir, nil)

	got := log.joined()
	// Order: initial MOH, then on 2nd join MOH stops before mixer adds.
	want := "mixer_remove,moh_start,moh_stop,mixer_remove,mixer_add,mixer_add"
	if got != want {
		t.Fatalf("REQ-CONF-1: got %q want %q", got, want)
	}
}

func TestREQ_CONF_TwoToOneRestartsMOH(t *testing.T) {
	log := &mediaEventLog{}
	room := newTestRoom(t, log)
	mohDir := testMOHDir(t)

	room.count = 2
	room.applyMediaLocked(dummySessions(2), mohDir, nil)
	room.count = 1
	room.applyMediaLocked(dummySessions(1), mohDir, nil)

	got := log.joined()
	want := "mixer_remove,mixer_add,mixer_add,mixer_remove,moh_start"
	if got != want {
		t.Fatalf("REQ-CONF-4: got %q want %q", got, want)
	}
	if room.moh == nil {
		t.Fatal("expected active MOH for lone participant")
	}
}

// REQ-CONF-5: phone hold must not drop a room to solo MOH while two are admitted.
func TestREQ_CONF_TwoAdmittedHoldDoesNotStartMOH(t *testing.T) {
	log := &mediaEventLog{}
	room := newTestRoom(t, log)
	room.count = 2

	room.applyMediaLocked(dummySessions(1), testMOHDir(t), nil)

	got := log.joined()
	if strings.Contains(got, "moh_start") {
		t.Fatalf("REQ-CONF-5: must not start MOH with two admitted participants, got %q", got)
	}
	if !strings.Contains(got, "mixer_add") {
		t.Fatalf("REQ-CONF-5: expected mixer refresh with one alive leg, got %q", got)
	}
}
