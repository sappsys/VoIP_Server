package audiobridge

import "sync/atomic"

// RelayStats counts bytes relayed by an active PCM bridge (for requirement tests).
// LegAToB is bytes read from leg A and written toward leg B; LegBToA the reverse.
type RelayStats struct {
	LegAToB atomic.Int64
	LegBToA atomic.Int64
}

func (s *RelayStats) Snapshot() (legAToB, legBToA int64) {
	if s == nil {
		return 0, 0
	}
	return s.LegAToB.Load(), s.LegBToA.Load()
}

func (s *RelayStats) add(aToB bool, n int) {
	if s == nil || n <= 0 {
		return
	}
	if aToB {
		s.LegAToB.Add(int64(n))
	} else {
		s.LegBToA.Add(int64(n))
	}
}
