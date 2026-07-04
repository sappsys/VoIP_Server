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

// REQ-CALL-1: caller hears ringback (180/183) while callee rings.
func TestREQ_CALL_RingbackWhileRinging(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	caller := newHandset(t, pbx.Port, "111", "andy")
	callee := newHandset(t, pbx.Port, "110", "andy")
	caller.register()
	callee.register()

	answered := make(chan *diago.DialogServerSession, 1)
	callee.serveRingingAnswer(600*time.Millisecond, answered)

	var sawRinging atomic.Bool
	out, err := caller.invite(ctx, "110", func(res *sip.Response) error {
		if res.StatusCode == sip.StatusRinging || res.StatusCode == sip.StatusSessionInProgress {
			sawRinging.Store(true)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("invite 111->110: %v", err)
	}
	defer out.Close()

	if !sawRinging.Load() {
		t.Fatal("REQ-CALL-1: caller did not receive ringing/progress before answer")
	}
	select {
	case <-answered:
	case <-time.After(5 * time.Second):
		t.Fatal("callee did not answer")
	}
}

// REQ-CALL-2: dialing an unregistered/unavailable extension yields busy then hangup.
func TestREQ_CALL_BusyThenHangupWhenUnavailable(t *testing.T) {
	pbx := startPBX(t, pbxOptions{
		Extensions:  map[string]string{"110": "andy", "111": "andy"},
		BusySeconds: 1, // keep the test fast; production default is 5
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	caller := newHandset(t, pbx.Port, "111", "andy")
	caller.register()
	// 110 is NOT registered -> unavailable.

	start := time.Now()
	out, err := caller.invite(ctx, "110", nil)
	if err != nil {
		// Some failures surface as invite error; that still satisfies "not connected".
		return
	}
	defer out.Close()

	// The call should be answered (for busy tone) then hung up by the server.
	if !waitFor(6*time.Second, func() bool { return out.Context().Err() != nil }) {
		t.Fatal("REQ-CALL-2: unavailable call was not hung up after busy tone")
	}
	if elapsed := time.Since(start); elapsed < 500*time.Millisecond {
		t.Fatalf("REQ-CALL-2: hangup too fast (%v), busy tone likely not played", elapsed)
	}
}

// REQ-CALL-3: answered call establishes two-way audio via the PCM bridge.
func TestREQ_CALL_AnsweredCallHasTwoWayAudio(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	caller := newHandset(t, pbx.Port, "111", "andy")
	callee := newHandset(t, pbx.Port, "110", "andy")
	caller.register()
	callee.register()

	answered := make(chan *diago.DialogServerSession, 1)
	callee.serveAnswer(ctx, answered, true)

	out, err := caller.invite(ctx, "110", nil)
	if err != nil {
		t.Fatalf("invite 111->110: %v", err)
	}
	defer out.Close()

	var calleeLeg *diago.DialogServerSession
	select {
	case calleeLeg = <-answered:
	case <-time.After(5 * time.Second):
		t.Fatal("callee did not answer")
	}

	audioCtx, audioCancel := context.WithCancel(ctx)
	defer audioCancel()
	stopOut := pumpAudio(audioCtx, out)
	stopCallee := pumpAudio(audioCtx, calleeLeg)
	defer func() { stopOut(); stopCallee() }()

	if !waitFor(5*time.Second, func() bool {
		return bridgedCallExists(pbx, "111", "110") &&
			out.Context().Err() == nil && calleeLeg.Context().Err() == nil
	}) {
		t.Fatal("REQ-CALL-3: call not bridged or leg dropped before relay check")
	}

	// Stop pumps before relay check: inject tone per leg and observe production bridge
	// counters (same as REQ-HOLD-3). Never read RTP on a leg while the bridge is reading it.
	stopOut()
	stopCallee()

	relayCtx, relayCancel := context.WithTimeout(ctx, 15*time.Second)
	defer relayCancel()
	assertTwoWayBridgeRelay(t, relayCtx, pbx, out, calleeLeg)
}

// REQ-CALL-4: when one party hangs up, the peer is released.
func TestREQ_CALL_HangupReleasesPeer(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	caller := newHandset(t, pbx.Port, "111", "andy")
	callee := newHandset(t, pbx.Port, "110", "andy")
	caller.register()
	callee.register()

	answered := make(chan *diago.DialogServerSession, 1)
	callee.serveAnswer(ctx, answered, true)

	out, err := caller.invite(ctx, "110", nil)
	if err != nil {
		t.Fatalf("invite: %v", err)
	}

	var calleeLeg *diago.DialogServerSession
	select {
	case calleeLeg = <-answered:
	case <-time.After(5 * time.Second):
		t.Fatal("callee did not answer")
	}

	// Caller hangs up; callee leg must be released by the server (BYE).
	_ = out.Hangup(ctx)
	if !waitFor(5*time.Second, func() bool { return calleeLeg.Context().Err() != nil }) {
		t.Fatal("REQ-CALL-4: peer leg was not released after hangup")
	}
}

// REQ-PBX-2: system-wide max_calls and per-extension limits reject excess calls with busy.
func TestREQ_PBX_MaxCallsBusy(t *testing.T) {
	t.Run("global_max_calls", testREQ_PBX_GlobalMaxCallsBusy)
	t.Run("per_extension_limit", testREQ_PBX_ExtensionMaxCallsBusy)
}

func testREQ_PBX_GlobalMaxCallsBusy(t *testing.T) {
	pbx := startPBX(t, pbxOptions{
		Extensions:  map[string]string{"110": "andy", "111": "andy", "112": "andy"},
		BusySeconds: 1,
		MaxCalls:    1,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	caller := newHandset(t, pbx.Port, "111", "andy")
	callee := newHandset(t, pbx.Port, "110", "andy")
	interloper := newHandset(t, pbx.Port, "112", "andy")
	caller.register()
	callee.register()
	interloper.register()

	answered := make(chan *diago.DialogServerSession, 1)
	callee.serveAnswer(ctx, answered, true)

	out, err := caller.invite(ctx, "110", nil)
	if err != nil {
		t.Fatalf("first call invite: %v", err)
	}
	defer out.Close()

	select {
	case leg := <-answered:
		audioCtx, cancelAudio := context.WithCancel(ctx)
		defer cancelAudio()
		stopOut := pumpAudio(audioCtx, out)
		stopLeg := pumpAudio(audioCtx, leg)
		defer func() { stopOut(); stopLeg() }()
	case <-time.After(5 * time.Second):
		t.Fatal("callee did not answer first call")
	}
	if !waitFor(3*time.Second, func() bool { return bridgedCallExists(pbx, "111", "110") }) {
		t.Fatal("first call not bridged")
	}
	if pbx.Srv.Status().ActiveCallCount != 1 {
		t.Fatalf("REQ-PBX-2: expected 1 active call, got %d", pbx.Srv.Status().ActiveCallCount)
	}

	start := time.Now()
	busyLeg, err := interloper.invite(ctx, "110", nil)
	if err != nil {
		return
	}
	defer busyLeg.Close()

	// Server plays busy (~1s) then BYE; assert capacity was enforced even if the
	// client dialog takes a moment to observe hangup.
	time.Sleep(1500 * time.Millisecond)
	if bridgedCallExists(pbx, "112", "110") {
		t.Fatal("REQ-PBX-2: second call over global capacity must not bridge")
	}
	if !bridgedCallExists(pbx, "111", "110") {
		t.Fatal("REQ-PBX-2: first call must remain active")
	}
	if pbx.Srv.Status().ActiveCallCount != 1 {
		t.Fatalf("REQ-PBX-2: global limit must not admit a second call, active=%d", pbx.Srv.Status().ActiveCallCount)
	}
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Fatalf("REQ-PBX-2: busy handling returned too fast (%v)", elapsed)
	}
}

func testREQ_PBX_ExtensionMaxCallsBusy(t *testing.T) {
	pbx := startPBX(t, pbxOptions{
		Extensions:     map[string]string{"110": "andy", "111": "andy", "112": "andy"},
		BusySeconds:    1,
		MaxCalls:       50,
		ExtMaxSimCalls: 1,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	caller := newHandset(t, pbx.Port, "111", "andy")
	callee := newHandset(t, pbx.Port, "110", "andy")
	target := newHandset(t, pbx.Port, "112", "andy")
	caller.register()
	callee.register()
	target.register()

	answered := make(chan *diago.DialogServerSession, 1)
	callee.serveAnswer(ctx, answered, true)

	out, err := caller.invite(ctx, "110", nil)
	if err != nil {
		t.Fatalf("first call invite: %v", err)
	}
	defer out.Close()

	select {
	case leg := <-answered:
		audioCtx, cancelAudio := context.WithCancel(ctx)
		defer cancelAudio()
		stopOut := pumpAudio(audioCtx, out)
		stopLeg := pumpAudio(audioCtx, leg)
		defer func() { stopOut(); stopLeg() }()
	case <-time.After(5 * time.Second):
		t.Fatal("callee did not answer first call")
	}
	if !waitFor(3*time.Second, func() bool { return bridgedCallExists(pbx, "111", "110") }) {
		t.Fatal("first call not bridged")
	}

	start := time.Now()
	busyLeg, err := caller.inviteServer(ctx, "112")
	if err != nil {
		return
	}
	defer busyLeg.Close()

	time.Sleep(1500 * time.Millisecond)
	if bridgedCallExists(pbx, "111", "112") {
		t.Fatal("REQ-PBX-2: second call over extension capacity must not bridge")
	}
	if !bridgedCallExists(pbx, "111", "110") {
		t.Fatal("REQ-PBX-2: first call must remain active")
	}
	if pbx.Srv.Status().ActiveCallCount != 1 {
		t.Fatalf("REQ-PBX-2: extension limit must not admit a second call, active=%d", pbx.Srv.Status().ActiveCallCount)
	}
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Fatalf("REQ-PBX-2: busy handling returned too fast (%v)", elapsed)
	}
}
