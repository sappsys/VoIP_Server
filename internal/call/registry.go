package call

import (
	"sync"

	"github.com/emiago/diago"
)

// ActiveCall tracks a bridged pair for transfer and consult handling.
type ActiveCall struct {
	CallerExt     string
	CalleeExt     string
	In            *diago.DialogServerSession
	Out           *diago.DialogClientSession
	ConsultIn     *diago.DialogServerSession
	TransferReady bool
	Parked        bool
}

// Registry indexes active bridged calls by extension and server dialog ID.
type Registry struct {
	mu       sync.Mutex
	byDialog map[string]*ActiveCall
	byExt    map[string]*ActiveCall
}

func NewRegistry() *Registry {
	return &Registry{
		byDialog: make(map[string]*ActiveCall),
		byExt:    make(map[string]*ActiveCall),
	}
}

func (r *Registry) Register(ac *ActiveCall) {
	if ac == nil || ac.In == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byDialog[ac.In.ID] = ac
	if ac.CallerExt != "" {
		r.byExt[ac.CallerExt] = ac
	}
}

func (r *Registry) Unregister(inID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ac, ok := r.byDialog[inID]
	if !ok {
		return
	}
	delete(r.byDialog, inID)
	if ac.CallerExt != "" && r.byExt[ac.CallerExt] == ac {
		delete(r.byExt, ac.CallerExt)
	}
}

// CallSnapshot is a bridged call without dialog handles (safe for status/UI).
type CallSnapshot struct {
	CallerExt     string
	CalleeExt     string
	TransferReady bool
	Parked        bool
}

// Snapshots returns all active bridged calls.
func (r *Registry) Snapshots() []CallSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[string]bool{}
	var out []CallSnapshot
	for _, ac := range r.byDialog {
		if ac == nil || ac.In == nil {
			continue
		}
		if seen[ac.In.ID] {
			continue
		}
		seen[ac.In.ID] = true
		out = append(out, CallSnapshot{
			CallerExt:     ac.CallerExt,
			CalleeExt:     ac.CalleeExt,
			TransferReady: ac.TransferReady,
			Parked:        ac.Parked,
		})
	}
	return out
}

func (r *Registry) ByExtension(ext string) *ActiveCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byExt[ext]
}

// FindByExtension returns an active call where ext is caller or callee.
func (r *Registry) FindByExtension(ext string) *ActiveCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ac := r.byExt[ext]; ac != nil {
		return ac
	}
	for _, ac := range r.byDialog {
		if ac.CalleeExt == ext {
			return ac
		}
	}
	return nil
}

func (r *Registry) ByDialog(inID string) *ActiveCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byDialog[inID]
}

func (r *Registry) SetConsult(ext string, consultIn *diago.DialogServerSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ac := r.byExt[ext]; ac != nil {
		ac.ConsultIn = consultIn
	}
}

func (r *Registry) SetTransferReady(ext string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	ac := r.findLocked(ext)
	if ac == nil {
		return false
	}
	ac.TransferReady = true
	return true
}

func (r *Registry) findLocked(ext string) *ActiveCall {
	if ac := r.byExt[ext]; ac != nil {
		return ac
	}
	for _, ac := range r.byDialog {
		if ac.CalleeExt == ext {
			return ac
		}
	}
	return nil
}

// StealHeldLeg removes the held party leg from the call for parking. Caller must not close it.
func (ac *ActiveCall) StealHeldLeg(parker string) (heldIn *diago.DialogServerSession, heldOut *diago.DialogClientSession) {
	ac.Parked = true
	if parker == ac.CallerExt {
		heldOut = ac.Out
		ac.Out = nil
		return nil, heldOut
	}
	heldIn = ac.In
	ac.In = nil
	return heldIn, nil
}
