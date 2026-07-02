package pbx

import (
	"sort"

	"github.com/sappsys/VoIP_Server/internal/call"
	"github.com/sappsys/VoIP_Server/internal/store"
)

// ExtensionStatus is live state for one extension.
type ExtensionStatus struct {
	Extension   string
	DisplayName string
	Registered  bool
	DND         bool
	ActiveCalls int
	InCallWith  string
}

// StatusReport is a point-in-time PBX snapshot for the web status page.
type StatusReport struct {
	ActiveCallCount int
	RegisteredCount int
	Registered      []string
	Extensions      []ExtensionStatus
	BridgedCalls    []call.CallSnapshot
	Connecting      []call.Session
	ParkedSlots     []string
}

// Status builds a live snapshot of registrations, calls, and park slots.
func (s *Server) Status() StatusReport {
	regSet := map[string]bool{}
	reg := s.reg.RegisteredExtensions()
	for _, ext := range reg {
		regSet[ext] = true
	}

	bridged := s.registry.Snapshots()
	inCall := map[string]string{}
	for _, bc := range bridged {
		if bc.CallerExt != "" && bc.CalleeExt != "" {
			inCall[bc.CallerExt] = bc.CalleeExt
			inCall[bc.CalleeExt] = bc.CallerExt
		}
	}

	var exts []ExtensionStatus
	for id, e := range s.exts {
		st := ExtensionStatus{
			Extension:   id,
			DisplayName: e.DisplayName,
			Registered:  regSet[id],
			DND:         e.DND,
			ActiveCalls: s.calls.ExtensionActive(id),
			InCallWith:  inCall[id],
		}
		exts = append(exts, st)
	}
	sort.Slice(exts, func(i, j int) bool { return exts[i].Extension < exts[j].Extension })

	return StatusReport{
		ActiveCallCount: int(s.calls.Active()),
		RegisteredCount: len(reg),
		Registered:      reg,
		Extensions:      exts,
		BridgedCalls:    bridged,
		Connecting:      s.calls.ActiveSessions(),
		ParkedSlots:     s.park.Slots(),
	}
}

func (s *Server) logCall(caller, callee, callerName, direction, trunkName, trunkPrefix string) {
	if caller == "" && callee == "" {
		return
	}
	if err := s.store.LogCall(store.CallLogEntry{
		Caller:      caller,
		Callee:      callee,
		CallerName:  callerName,
		Direction:   direction,
		TrunkName:   trunkName,
		TrunkPrefix: trunkPrefix,
	}); err != nil && s.log != nil {
		s.log.Warn("call log", "error", err)
	}
}
