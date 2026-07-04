//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo/sip"
)

// REQ-XFER-3: attended transfer — 111 calls 110, presses *77, dials 112;
// after 111 hangs up, 110 must be bridged to 112.
func TestREQ_XFER_AttendedTransferBridges(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{
		"110": "andy", "111": "andy", "112": "andy",
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	transferor := newHandset(t, pbx.Port, "111", "andy")
	partyB := newHandset(t, pbx.Port, "110", "andy")
	target := newHandset(t, pbx.Port, "112", "andy")
	transferor.register()
	partyB.register()
	target.register()

	bAnswered := make(chan *diago.DialogServerSession, 1)
	partyB.serveAnswer(ctx, bAnswered, true)

	tAnswered := make(chan *diago.DialogServerSession, 1)
	target.serveAnswer(ctx, tAnswered, true)

	call1, err := transferor.invite(ctx, "110", nil)
	if err != nil {
		t.Fatalf("invite 111->110: %v", err)
	}
	var bLeg *diago.DialogServerSession
	select {
	case bLeg = <-bAnswered:
	case <-time.After(5 * time.Second):
		t.Fatal("110 did not answer")
	}
	audioCtx, audioStop := context.WithCancel(ctx)
	defer audioStop()
	stop1 := pumpAudio(audioCtx, call1)
	stopB := pumpAudio(audioCtx, bLeg)
	defer func() { stop1(); stopB() }()

	if !waitFor(3*time.Second, func() bool { return bridgedCallExists(pbx, "111", "110") }) {
		t.Fatal("original call not bridged")
	}
	time.Sleep(300 * time.Millisecond)

	xferCtx, xferCancel := context.WithTimeout(ctx, 8*time.Second)
	defer xferCancel()
	starCall, err := transferor.invite(xferCtx, "*77", nil)
	if err == nil && starCall != nil {
		defer starCall.Close()
	} else if err != nil {
		t.Fatalf("REQ-XFER-3: *77 invite failed: %v", err)
	}

	if !waitFor(5*time.Second, func() bool {
		for _, bc := range pbx.Srv.Status().BridgedCalls {
			if bc.TransferReady {
				return true
			}
		}
		return false
	}) {
		t.Fatal("REQ-XFER-3: *77 did not arm transfer-ready")
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, 8*time.Second)
	defer dialCancel()
	targetCall, err := transferor.inviteServer(dialCtx, "112")
	if err == nil && targetCall != nil {
		defer targetCall.Close()
	} else if err != nil {
		t.Fatalf("REQ-XFER-3: transfer target invite failed: %v", err)
	}

	select {
	case <-tAnswered:
	case <-time.After(8 * time.Second):
		t.Fatal("REQ-XFER-3: transfer target 112 did not receive the call")
	}

	if !waitFor(5*time.Second, func() bool { return bLeg.Context().Err() == nil }) {
		t.Fatal("REQ-XFER-3: held party 110 dropped instead of being transferred")
	}
	_ = call1
}

// REQ-XFER-2: blind REFER from the transferor bridges the held party to the target.
func TestREQ_XFER_BlindTransferBridgesTarget(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{
		"110": "andy", "111": "andy", "112": "andy",
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	transferor := newHandset(t, pbx.Port, "111", "andy")
	partyB := newHandset(t, pbx.Port, "110", "andy")
	target := newHandset(t, pbx.Port, "112", "andy")
	transferor.register()
	partyB.register()
	target.register()

	bAnswered := make(chan *diago.DialogServerSession, 1)
	partyB.serveAnswer(ctx, bAnswered, true)

	tAnswered := make(chan *diago.DialogServerSession, 1)
	target.serveAnswer(ctx, tAnswered, true)

	call1, err := transferor.invite(ctx, "110", nil)
	if err != nil {
		t.Fatalf("invite 111->110: %v", err)
	}
	defer call1.Close()

	var bLeg *diago.DialogServerSession
	select {
	case bLeg = <-bAnswered:
	case <-time.After(5 * time.Second):
		t.Fatal("110 did not answer")
	}
	audioCtx, audioStop := context.WithCancel(ctx)
	defer audioStop()
	stop1 := pumpAudio(audioCtx, call1)
	stopB := pumpAudio(audioCtx, bLeg)
	defer func() { stop1(); stopB() }()

	if !waitFor(3*time.Second, func() bool { return bridgedCallExists(pbx, "111", "110") }) {
		t.Fatal("original call not bridged")
	}
	time.Sleep(300 * time.Millisecond)

	referTo := sip.Uri{User: "112", Host: "127.0.0.1", Port: pbx.Port}
	if err := call1.Refer(ctx, referTo); err != nil {
		t.Fatalf("REQ-XFER-2: REFER to 112 failed: %v", err)
	}

	select {
	case <-tAnswered:
	case <-time.After(8 * time.Second):
		t.Fatal("REQ-XFER-2: transfer target 112 did not receive REFER invite")
	}

	if !waitFor(5*time.Second, func() bool { return bLeg.Context().Err() == nil }) {
		t.Fatal("REQ-XFER-2: held party 110 dropped instead of being transferred")
	}
}
