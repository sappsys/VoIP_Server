package call

import (
	"context"
	"sort"
	"sync"

	"github.com/emiago/diago"
)

// ParkedCall is a call waiting on hold at a park slot (parker's extension number).
type ParkedCall struct {
	Slot    string
	HeldIn  *diago.DialogServerSession
	HeldOut *diago.DialogClientSession
	cancel  context.CancelFunc
}

// ParkLot stores parked calls keyed by slot (extension number).
type ParkLot struct {
	mu    sync.Mutex
	slots map[string]*ParkedCall
}

func NewParkLot() *ParkLot {
	return &ParkLot{slots: make(map[string]*ParkedCall)}
}

func (p *ParkLot) Park(slot string, heldIn *diago.DialogServerSession, heldOut *diago.DialogClientSession, holdFn func(context.Context)) *ParkedCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing := p.slots[slot]; existing != nil {
		existing.Release()
	}
	ctx, cancel := context.WithCancel(context.Background())
	pc := &ParkedCall{Slot: slot, HeldIn: heldIn, HeldOut: heldOut, cancel: cancel}
	p.slots[slot] = pc
	if holdFn != nil {
		go holdFn(ctx)
	}
	return pc
}

func (p *ParkLot) Retrieve(slot string) *ParkedCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	pc := p.slots[slot]
	if pc == nil {
		return nil
	}
	delete(p.slots, slot)
	return pc
}

func (p *ParkLot) Has(slot string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.slots[slot] != nil
}

// Slots returns parked slot ids in alphanumeric order.
func (p *ParkLot) Slots() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.slots))
	for slot := range p.slots {
		out = append(out, slot)
	}
	sort.Strings(out)
	return out
}

func (pc *ParkedCall) Release() {
	if pc.cancel != nil {
		pc.cancel()
	}
	if pc.HeldIn != nil {
		pc.HeldIn.Hangup(context.Background())
	}
	if pc.HeldOut != nil {
		pc.HeldOut.Hangup(context.Background())
		pc.HeldOut.Close()
	}
}
