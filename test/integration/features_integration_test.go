//go:build integration

package integration_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo/sip"
)

// REQ-PBX-3: *78 enables DND; inbound calls ring at the caller but never reach the DND extension.
func TestREQ_PBX_DNDBlocksInboundDial(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	callee := newHandset(t, pbx.Port, "110", "andy")
	caller := newHandset(t, pbx.Port, "111", "andy")
	callee.register()
	caller.register()

	var inbound atomic.Int32
	callee.setInbound(func(in *diago.DialogServerSession) {
		inbound.Add(1)
		_ = in.Respond(sip.StatusBusyHere, "Busy", nil)
	})

	dndCtx, dndCancel := context.WithTimeout(ctx, 8*time.Second)
	dndLeg, err := callee.inviteServer(dndCtx, "*78")
	dndCancel()
	if err != nil {
		t.Fatalf("activate DND via *78: %v", err)
	}
	_ = dndLeg.Hangup(ctx)

	callCtx, callCancel := context.WithTimeout(ctx, 8*time.Second)
	defer callCancel()
	out, err := caller.invite(callCtx, "110", nil)
	if err != nil {
		// Unreachable/DND may fail the invite; still verify callee was not rung.
		if inbound.Load() != 0 {
			t.Fatal("REQ-PBX-3: DND extension must not receive inbound INVITE")
		}
		return
	}
	defer out.Close()

	time.Sleep(2 * time.Second)
	if inbound.Load() != 0 {
		t.Fatal("REQ-PBX-3: DND extension received inbound INVITE")
	}
}
