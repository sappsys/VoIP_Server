package conference

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/store"
)

type Room struct {
	Number string
	Bridge *diago.BridgeMix
	mu     sync.Mutex
	count  int
	max    int
}

type Manager struct {
	mu    sync.Mutex
	rooms map[string]*Room
	log   *slog.Logger
}

func NewManager(log *slog.Logger) *Manager {
	return &Manager{rooms: make(map[string]*Room), log: log}
}

func (m *Manager) room(number string, max int) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rooms[number]; ok {
		return r
	}
	if max < 2 {
		max = 16
	}
	r := &Room{Number: number, Bridge: diago.NewBridgeMix(), max: max}
	m.rooms[number] = r
	return r
}

func (m *Manager) HandleJoin(ctx context.Context, in *diago.DialogServerSession, conf *store.Conference) error {
	in.Trying()
	in.Ringing()

	if conf.PINHash != "" {
		if !m.collectPIN(ctx, in, conf.PINHash) {
			_ = in.Respond(sip.StatusForbidden, "Invalid PIN", nil)
			return nil
		}
	}

	room := m.room(in.ToUser(), conf.MaxParticipants)
	room.mu.Lock()
	if room.count >= room.max {
		room.mu.Unlock()
		_ = in.Respond(sip.StatusBusyHere, "Conference Full", nil)
		return nil
	}
	room.count++
	room.mu.Unlock()
	defer func() {
		room.mu.Lock()
		room.count--
		room.mu.Unlock()
	}()

	if err := in.Answer(); err != nil {
		return err
	}
	if err := room.Bridge.AddDialogSession(in); err != nil {
		return err
	}
	<-in.Context().Done()
	return room.Bridge.RemoveDialogSession(in)
}

func (m *Manager) collectPIN(ctx context.Context, in *diago.DialogServerSession, pinHash string) bool {
	if err := in.ProgressMedia(); err != nil {
		return false
	}
	reader, err := in.AudioReaderDTMF()
	if err != nil {
		return false
	}
	var entered strings.Builder
	done := make(chan bool, 1)
	_ = reader.Listen(func(dtmf rune) error {
		if dtmf == '#' {
			done <- true
			return nil
		}
		entered.WriteRune(dtmf)
		return nil
	}, 25*time.Second)
	select {
	case <-ctx.Done():
		return false
	case <-in.Context().Done():
		return false
	case <-done:
		return store.CheckPassword(pinHash, entered.String())
	case <-time.After(25 * time.Second):
		return false
	}
}

func (m *Manager) Participants(number string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rooms[number]; ok {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.count
	}
	return 0
}
