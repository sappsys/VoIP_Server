package conference

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/emiago/diago"
	"github.com/sappsys/VoIP_Server/internal/call"
	"github.com/sappsys/VoIP_Server/internal/media/audiobridge"
	"github.com/sappsys/VoIP_Server/internal/media/tones"
	"github.com/sappsys/VoIP_Server/internal/store"
)

type Room struct {
	Number string
	Mixer  roomMixer
	mu     sync.Mutex
	sessions []*diago.DialogServerSession
	count    int // admitted participants (past the PIN gate)
	reserved int // capacity slots held by callers still authenticating
	max      int
	moh      stoppableMOH
}

// roomMixer is the conference audio bridge for 2+ participants.
type roomMixer interface {
	RemoveAll()
	Add(leg audiobridge.SessionLeg) error
}

// stoppableMOH is hold music that must be stopped before bridge/mixer audio.
type stoppableMOH interface {
	Stop()
}

// conferenceMOHStart is overridden in tests to verify MOH lifecycle ordering.
var conferenceMOHStart = func(ctx context.Context, in *diago.DialogServerSession, mohDir string, log *slog.Logger) stoppableMOH {
	return call.StartMOHServer(ctx, in, mohDir, log)
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
	r := &Room{Number: number, Mixer: audiobridge.NewConferenceMixer(m.log), max: max}
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
	// Reserve a capacity slot (rejects over-capacity) without admitting the caller
	// into the room yet: unauthenticated callers must not be counted as participants
	// or added to the mixer until they clear the PIN gate.
	room.mu.Lock()
	if room.count+room.reserved >= room.max {
		room.mu.Unlock()
		call.AnswerAndPlayBusy(ctx, in, tones.DefaultProfile(), m.log)
		return nil
	}
	room.reserved++
	room.mu.Unlock()

	admitted := false
	defer func() {
		if admitted {
			room.leave(in, opts.MOHDir, m.log)
		} else {
			room.mu.Lock()
			room.reserved--
			room.mu.Unlock()
		}
	}()

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

	// PIN cleared: admit the participant (convert the reservation into a seat).
	room.mu.Lock()
	room.reserved--
	room.count++
	room.sessions = append(room.sessions, in)
	room.mu.Unlock()
	admitted = true

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

func (r *Room) stopMOHLocked() {
	if r.moh != nil {
		r.moh.Stop()
		r.moh = nil
	}
}

func (r *Room) reconcile(mohDir string, log *slog.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	alive := r.aliveSessionsLocked()
	r.applyMediaLocked(alive, mohDir, log)
}

// applyMediaLocked switches between MOH (1 participant) and mixer (2+).
// MOH must be fully stopped before the mixer starts (REQ-CONF-1).
func (r *Room) applyMediaLocked(alive []*diago.DialogServerSession, mohDir string, log *slog.Logger) {
	r.stopMOHLocked()
	r.Mixer.RemoveAll()

	for _, s := range alive {
		call.PrepareConferenceLeg(s)
	}

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
		sess := alive[0]
		r.moh = conferenceMOHStart(context.Background(), sess, mohDir, log)
		if r.moh == nil {
			if log != nil {
				log.Warn("conference moh start failed", "room", r.Number)
			}
			return
		}
		if log != nil {
			log.Info("conference moh started", "room", r.Number, "dir", mohDir)
		}
	default:
		for _, s := range alive {
			if err := r.Mixer.Add(s); err != nil && log != nil {
				log.Warn("conference mixer add failed", "room", r.Number, "error", err)
			}
		}
		if log != nil {
			log.Info("conference mixer started", "room", r.Number, "participants", len(alive))
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

const (
	maxPINAttempts     = 3
	pinPromptInterval  = 15 * time.Second
	pinSessionDeadline = 35 * time.Second
)

func pinDigitTimeout(deadline time.Time) time.Duration {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0
	}
	if remaining < pinPromptInterval {
		return remaining
	}
	return pinPromptInterval
}

// conferencePINCollectOpts returns DTMF collection settings for the conference PIN gate.
func conferencePINCollectOpts() call.DTMFCollectOpts {
	return call.DTMFCollectOpts{
		AcceptPartialOnTimeout: true,
	}
}

func (m *Manager) collectPIN(ctx context.Context, in *diago.DialogServerSession, pinHash string, opts JoinOptions) bool {
	deadline := time.Now().Add(pinSessionDeadline)
	wrongAttempts := 0

	for {
		if ctx.Err() != nil || in.Context().Err() != nil {
			return false
		}
		if time.Now().After(deadline) {
			if m.log != nil {
				m.log.Info("conference pin timed out", "from", in.FromUser())
			}
			return false
		}

		digitTimeout := pinDigitTimeout(deadline)
		if digitTimeout <= 0 {
			return false
		}

		entered, ok := call.PlayPromptWhileReadDigits(ctx, in, opts.PINPrompt, digitTimeout, m.log, conferencePINCollectOpts())
		if !ok {
			if m.log != nil {
				m.log.Debug("conference pin entry timeout, replaying prompt", "from", in.FromUser())
			}
			continue
		}
		if store.CheckPassword(pinHash, entered) {
			return true
		}

		wrongAttempts++
		if m.log != nil {
			m.log.Debug("conference pin incorrect", "from", in.FromUser(), "attempt", wrongAttempts)
		}
		if wrongAttempts >= maxPINAttempts {
			return false
		}
		_ = call.PlayPromptToServer(ctx, in, opts.PINBadPrompt, m.log)
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
