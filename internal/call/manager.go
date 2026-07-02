package call

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
)

type Manager struct {
	maxCalls int64
	active   atomic.Int64
	mu       sync.Mutex
	byID     map[string]*Session
	extCalls map[string]int
}

type Session struct {
	ID         string
	Caller     string
	Callee     string
	CallerName string
}

type Dialer interface {
	Invite(ctx context.Context, recipient sip.Uri, opts diago.InviteOptions) (*diago.DialogClientSession, error)
}

func NewManager(max int) *Manager {
	if max <= 0 {
		max = 200
	}
	return &Manager{
		maxCalls: int64(max),
		byID:     make(map[string]*Session),
		extCalls: make(map[string]int),
	}
}

func (m *Manager) TryAcquire(id, caller, callee, callerName string, exts map[string]*config.Extension) (*Session, error) {
	if err := m.checkExtensionLimit(caller, exts); err != nil {
		return nil, err
	}
	if err := m.checkExtensionLimit(callee, exts); err != nil {
		return nil, err
	}
	for {
		cur := m.active.Load()
		if cur >= m.maxCalls {
			return nil, fmt.Errorf("call limit reached")
		}
		if m.active.CompareAndSwap(cur, cur+1) {
			s := &Session{ID: id, Caller: caller, Callee: callee, CallerName: callerName}
			m.mu.Lock()
			m.byID[id] = s
			m.extCalls[caller]++
			m.extCalls[callee]++
			m.mu.Unlock()
			return s, nil
		}
	}
}

func (m *Manager) checkExtensionLimit(ext string, exts map[string]*config.Extension) error {
	e, ok := exts[ext]
	if !ok {
		return nil
	}
	m.mu.Lock()
	n := m.extCalls[ext]
	m.mu.Unlock()
	max := e.MaxSimultaneousCalls
	if max <= 0 {
		max = 4
	}
	if n >= max {
		if e.CallWaiting && n == max {
			return nil
		}
		if n >= max {
			return fmt.Errorf("extension %s busy", ext)
		}
	}
	return nil
}

func (m *Manager) Release(id string) {
	m.mu.Lock()
	if s, ok := m.byID[id]; ok {
		delete(m.byID, id)
		m.active.Add(-1)
		if s.Caller != "" {
			m.extCalls[s.Caller]--
			if m.extCalls[s.Caller] <= 0 {
				delete(m.extCalls, s.Caller)
			}
		}
		if s.Callee != "" {
			m.extCalls[s.Callee]--
			if m.extCalls[s.Callee] <= 0 {
				delete(m.extCalls, s.Callee)
			}
		}
	}
	m.mu.Unlock()
}

func (m *Manager) Active() int64 { return m.active.Load() }

// ActiveSessions returns a snapshot of in-progress call sessions.
func (m *Manager) ActiveSessions() []Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Session, 0, len(m.byID))
	for _, sess := range m.byID {
		out = append(out, *sess)
	}
	return out
}

func (m *Manager) ExtensionActive(ext string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.extCalls[ext]
}

func CallerNameHeader(name string) sip.Header {
	if name == "" {
		return nil
	}
	return sip.NewHeader("X-Caller-Name", name)
}

func PAssertedIdentity(display, user, host string) sip.Header {
	if display != "" {
		return sip.NewHeader("P-Asserted-Identity", fmt.Sprintf(`"%s" <sip:%s@%s>`, display, user, host))
	}
	return sip.NewHeader("P-Asserted-Identity", fmt.Sprintf("<sip:%s@%s>", user, host))
}

func IntercomHeaders() []sip.Header {
	return []sip.Header{
		sip.NewHeader("Alert-Info", "<http://127.0.0.1>;info=alert-autoanswer"),
		sip.NewHeader("Call-Info", "answer-after=0"),
	}
}

func OutboundHeaders(callerName, fromUser, host string) []sip.Header {
	var h []sip.Header
	if hdr := CallerNameHeader(callerName); hdr != nil {
		h = append(h, hdr)
	}
	if hdr := PAssertedIdentity(callerName, fromUser, host); hdr != nil {
		h = append(h, hdr)
	}
	return h
}
