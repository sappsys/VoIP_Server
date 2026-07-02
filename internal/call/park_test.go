package call

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestParkLotRetrieve(t *testing.T) {
	lot := NewParkLot()
	var ran atomic.Bool
	pc := lot.Park("101", nil, nil, func(ctx context.Context) {
		ran.Store(true)
		<-ctx.Done()
	})
	if pc == nil || !lot.Has("101") {
		t.Fatal("expected parked slot")
	}

	got := lot.Retrieve("101")
	if got != pc || lot.Has("101") {
		t.Fatal("retrieve should remove slot")
	}
	if lot.Retrieve("101") != nil {
		t.Fatal("second retrieve should be nil")
	}

	time.Sleep(20 * time.Millisecond)
	if !ran.Load() {
		t.Fatal("hold goroutine should have started")
	}
	pc.Release()
}

func TestParkLotReplaceSlot(t *testing.T) {
	lot := NewParkLot()
	lot.Park("101", nil, nil, nil)
	lot.Park("101", nil, nil, nil)
	if !lot.Has("101") {
		t.Fatal("expected slot after replace")
	}
}
