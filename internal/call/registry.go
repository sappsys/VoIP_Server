package call

import (
	"context"
	"sync"
	"time"

	"github.com/emiago/diago"
	diagomedia "github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/media/audiobridge"
)

// ActiveCall tracks a bridged pair for transfer and consult handling.
type ActiveCall struct {
	CallerExt     string
	CalleeExt     string
	In            *diago.DialogServerSession
	Out           *diago.DialogClientSession
	ConsultIn     *diago.DialogServerSession
	TransferReady bool
	HoldActive       bool
	HolderExt        string
	holderLegHeld    bool // server re-INVITEd holder sendonly for dial tone
	holderPhoneHold  bool // holder signalled hold with sendonly (our side recvonly)
	holderSawRecvonly bool // holder leg reached recvonly/inactive during this hold
	holderPrevDir    string // previous negotiated direction on holder leg (unhold transition)
	holderReasserting bool // true while re-applying sendonly after spurious sendrecv churn
	heldPartyMOHSendonly bool // true when held party leg was re-INVITEd sendonly for MOH
	holdEnteredAt    time.Time
	dialTonePending      bool // true while our sendonly re-INVITE for dial tone is in flight
	holdEntering         bool // true while enter() is tearing down bridge / starting MOH
	holdEnterStartedAt   time.Time
	holdEnterAborted     bool // phone released during slow hold entry; enter() must not activate hold
	bridgeCodecA         diagomedia.Codec
	bridgeCodecB         diagomedia.Codec
	bridgeCodecsSaved    bool
	Parked           bool

	holdMu     sync.Mutex
	holdCancel context.CancelFunc
	holdMediaRoot context.Context // parent for MOH restarts during hold
	mohCancel  context.CancelFunc
	heldPartyMS *diagomedia.MediaSession // last seen held-party session (MOH restart trigger)
	holdPlayer *holdPlayer
	bridgeStop func() error
	relayStats *audiobridge.RelayStats
	// bridgeReady is set once the initial bridge is established. Hold re-INVITEs
	// arriving before this are recorded in pendingHold and applied on ready.
	bridgeReady      bool
	pendingHold      bool
	pendingHoldCaller bool
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
	HoldActive    bool
	Parked        bool
}

// HoldSnapshot returns hold-related fields under holdMu (safe for status/UI).
func (ac *ActiveCall) HoldSnapshot() (holdActive bool, holderExt string) {
	if ac == nil {
		return false, ""
	}
	ac.holdMu.Lock()
	holdActive = ac.HoldActive
	holderExt = ac.HolderExt
	ac.holdMu.Unlock()
	return holdActive, holderExt
}

// RelayBytesSnapshot returns PCM bridge relay byte counts (caller→callee, callee→caller).
// Leg A is the caller server leg; leg B is the callee client leg.
func (ac *ActiveCall) RelayBytesSnapshot() (callerToCallee, calleeToCaller int64) {
	if ac == nil {
		return 0, 0
	}
	ac.holdMu.Lock()
	stats := ac.relayStats
	ac.holdMu.Unlock()
	if stats == nil {
		return 0, 0
	}
	return stats.Snapshot()
}

// HoldMOHBytesSent returns server-side MOH bytes sent on an active held call.
func (ac *ActiveCall) HoldMOHBytesSent() int64 {
	if ac == nil {
		return 0
	}
	ac.holdMu.Lock()
	player := ac.holdPlayer
	ac.holdMu.Unlock()
	if player == nil {
		return 0
	}
	return player.MOHBytesSent()
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
		holdActive, _ := ac.HoldSnapshot()
		out = append(out, CallSnapshot{
			CallerExt:     ac.CallerExt,
			CalleeExt:     ac.CalleeExt,
			TransferReady: ac.TransferReady,
			HoldActive:    holdActive,
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
	ac.StopBridge()
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

func (ac *ActiveCall) cancelHold() {
	ac.holdMu.Lock()
	defer ac.holdMu.Unlock()
	if ac.holdCancel != nil {
		ac.holdCancel()
		ac.holdCancel = nil
	}
	if ac.holdPlayer != nil {
		ac.holdPlayer.stopAndWait()
		ac.holdPlayer = nil
	}
	ac.HoldActive = false
	ac.HolderExt = ""
	ac.holderLegHeld = false
	ac.holderPhoneHold = false
	ac.holderSawRecvonly = false
	ac.holderPrevDir = ""
	ac.holderReasserting = false
	ac.heldPartyMOHSendonly = false
	ac.heldPartyMS = nil
	if ac.mohCancel != nil {
		ac.mohCancel()
		ac.mohCancel = nil
	}
	ac.holdMediaRoot = nil
	ac.holdEntering = false
	ac.holdEnterStartedAt = time.Time{}
	ac.holdEnterAborted = false
}
