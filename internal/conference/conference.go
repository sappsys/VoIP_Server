package conference

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/call"
	"github.com/sappsys/VoIP_Server/internal/store"
)

type Room struct {
	Number    string
	Bridge    *diago.BridgeMix
	mu        sync.Mutex
	sessions  []*diago.DialogServerSession
	count     int
	max       int
	mohCancel context.CancelFunc
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

// JoinOptions carries media prompt paths for a conference join.
type JoinOptions struct {
	MOHDir       string // directory of hold-music WAVs (single participant)
	PINPrompt    string // WAV asking for the PIN
	PINBadPrompt string // WAV played when the PIN is wrong (before re-request)
}

func (m *Manager) HandleJoin(ctx context.Context, in *diago.DialogServerSession, conf *store.Conference, opts JoinOptions) error {
	in.Trying()

	room := m.room(conf.Number, conf.MaxParticipants)
	room.mu.Lock()
	if room.count >= room.max {
		room.mu.Unlock()
		_ = in.Respond(sip.StatusBusyHere, "Conference Full", nil)
		return nil
	}
	room.count++
	room.sessions = append(room.sessions, in)
	room.mu.Unlock()

	defer room.leave(in, opts.MOHDir, m.log)

	if err := call.AnswerSession(in); err != nil {
		if m.log != nil {
			m.log.Warn("conference answer failed", "room", conf.Number, "from", in.FromUser(), "error", err)
		}
		return err
	}

	if conf.PINHash != "" {
		if !m.collectPIN(ctx, in, conf.PINHash, opts) {
			in.Hangup(ctx)
			if m.log != nil {
				m.log.Info("conference pin rejected", "room", conf.Number, "from", in.FromUser())
			}
			return nil
		}
	}

	if m.log != nil {
		m.log.Info("conference joined", "room", conf.Number, "from", in.FromUser(), "participants", room.count)
	}
	room.reconcile(opts.MOHDir, m.log)

	<-in.Context().Done()
	return nil
}

func (r *Room) leave(in *diago.DialogServerSession, mohDir string, log *slog.Logger) {
	r.mu.Lock()
	r.count--
	for i, s := range r.sessions {
		if s == in {
			r.sessions = append(r.sessions[:i], r.sessions[i+1:]...)
			break
		}
	}
	r.mu.Unlock()
	r.reconcile(mohDir, log)
}

func (r *Room) reconcile(mohDir string, log *slog.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.mohCancel != nil {
		r.mohCancel()
		r.mohCancel = nil
	}

	for _, d := range r.Bridge.DialogSessionsList() {
		_ = r.Bridge.RemoveDialogSession(d)
	}

	alive := r.aliveSessionsLocked()
	switch len(alive) {
	case 0:
		return
	case 1:
		if mohDir == "" {
			return
		}
		if _, err := call.MOHTracks(mohDir); err != nil {
			if log != nil {
				log.Warn("conference moh unavailable", "room", r.Number, "dir", mohDir, "error", err)
			}
			return
		}
		mohCtx, cancel := context.WithCancel(context.Background())
		r.mohCancel = cancel
		sess := alive[0]
		go call.PlayMOHToServer(mohCtx, sess, mohDir, log)
		if log != nil {
			log.Info("conference moh started", "room", r.Number, "dir", mohDir)
		}
	default:
		for _, s := range alive {
			if err := r.Bridge.AddDialogSession(s); err != nil && log != nil {
				log.Warn("conference bridge add failed", "room", r.Number, "error", err)
			}
		}
		if log != nil {
			log.Debug("conference bridge started", "room", r.Number, "participants", len(alive))
		}
	}
}

func (r *Room) aliveSessionsLocked() []*diago.DialogServerSession {
	out := make([]*diago.DialogServerSession, 0, len(r.sessions))
	for _, s := range r.sessions {
		if s != nil && s.Context().Err() == nil {
			out = append(out, s)
		}
	}
	return out
}

const maxPINAttempts = 3

func (m *Manager) collectPIN(ctx context.Context, in *diago.DialogServerSession, pinHash string, opts JoinOptions) bool {
	for attempt := 0; attempt < maxPINAttempts; attempt++ {
		if ctx.Err() != nil || in.Context().Err() != nil {
			return false
		}
		// Prompt for the PIN (played before each attempt).
		_ = call.PlayPromptToServer(ctx, in, opts.PINPrompt, m.log)

		entered, ok := call.ReadDTMFDigits(ctx, in, 20*time.Second, m.log)
		if !ok {
			if m.log != nil {
				m.log.Info("conference pin entry failed", "from", in.FromUser(), "attempt", attempt+1)
			}
			return false
		}
		if store.CheckPassword(pinHash, entered) {
			return true
		}
		if m.log != nil {
			m.log.Debug("conference pin incorrect", "from", in.FromUser(), "attempt", attempt+1)
		}
		// Wrong PIN: play the failure prompt, then loop to re-request.
		_ = call.PlayPromptToServer(ctx, in, opts.PINBadPrompt, m.log)
	}
	return false
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
