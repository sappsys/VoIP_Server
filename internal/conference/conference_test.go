package conference

import (
	"testing"

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

func TestRoomMaxDefault(t *testing.T) {
	m := NewManager(nil)
	room := m.room("601", 0)
	if room.max != 16 {
		t.Fatalf("default max=%d want 16", room.max)
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
