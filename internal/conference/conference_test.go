package conference

import (
	"testing"
	"time"

	"github.com/sappsys/VoIP_Server/internal/media/audiobridge"
	"github.com/sappsys/VoIP_Server/internal/store"
)

func TestRoomParticipantCount(t *testing.T) {
	m := NewManager(nil)
	if m.Participants("600") != 0 {
		t.Fatal("unknown room should have 0 participants")
	}
	room := m.room("600", 8)
	room.mu.Lock()
	room.count = 2
	room.mu.Unlock()
	if m.Participants("600") != 2 {
		t.Fatalf("expected 2 participants")
	}
}

func TestReconcileMode(t *testing.T) {
	room := &Room{Number: "600", Mixer: audiobridge.NewConferenceMixer(nil)}
	if n := len(room.aliveSessionsLocked()); n != 0 {
		t.Fatalf("alive=%d", n)
	}
}

func TestRoomMaxDefault(t *testing.T) {
	m := NewManager(nil)
	room := m.room("601", 0)
	if room.max != 16 {
		t.Fatalf("default max=%d want 16", room.max)
	}
}

func TestPinDigitTimeout(t *testing.T) {
	deadline := time.Now().Add(35 * time.Second)
	if d := pinDigitTimeout(deadline); d != pinPromptInterval {
		t.Fatalf("got %v want %v", d, pinPromptInterval)
	}
	near := time.Now().Add(5 * time.Second)
	if d := pinDigitTimeout(near); d > 5*time.Second || d <= 0 {
		t.Fatalf("near deadline got %v", d)
	}
	past := time.Now().Add(-time.Second)
	if d := pinDigitTimeout(past); d != 0 {
		t.Fatalf("past deadline got %v", d)
	}
}

func TestConferencePINHashCheck(t *testing.T) {
	hash, err := store.HashPassword("1234")
	if err != nil {
		t.Fatal(err)
	}
	if !store.CheckPassword(hash, "1234") {
		t.Fatal("PIN check failed")
	}
	if store.CheckPassword(hash, "0000") {
		t.Fatal("wrong PIN should fail")
	}
}
