//go:build integration

package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media"
	"github.com/emiago/diago/media/sdp"
	"github.com/sappsys/VoIP_Server/internal/call"
)

func holdActive(pbx *testPBX, callerExt, calleeExt string) bool {
	for _, bc := range pbx.Srv.Status().BridgedCalls {
		if bc.HoldActive &&
			((bc.CallerExt == callerExt && bc.CalleeExt == calleeExt) ||
				(bc.CallerExt == calleeExt && bc.CalleeExt == callerExt)) {
			return true
		}
	}
	return false
}

func bridgedCallExists(pbx *testPBX, a, b string) bool {
	for _, bc := range pbx.Srv.Status().BridgedCalls {
		if (bc.CallerExt == a && bc.CalleeExt == b) || (bc.CallerExt == b && bc.CalleeExt == a) {
			return true
		}
	}
	return false
}

func establishCall(t *testing.T, pbx *testPBX, ctx context.Context) (caller *handset, callee *handset, out *diago.DialogClientSession, calleeLeg *diago.DialogServerSession, audioCancel context.CancelFunc) {
	t.Helper()
	caller = newHandset(t, pbx.Port, "111", "andy")
	callee = newHandset(t, pbx.Port, "110", "andy")
	caller.register()
	callee.register()

	answered := make(chan *diago.DialogServerSession, 1)
	callee.serveAnswer(ctx, answered, true)

	var err error
	out, err = caller.invite(ctx, "110", nil)
	if err != nil {
		t.Fatalf("invite 111->110: %v", err)
	}

	select {
	case calleeLeg = <-answered:
	case <-time.After(5 * time.Second):
		t.Fatal("callee did not answer")
	}

	audioCtx, cancel := context.WithCancel(ctx)
	stopOut := pumpAudio(audioCtx, out)
	stopCallee := pumpAudio(audioCtx, calleeLeg)
	audioCancel = func() {
		stopOut()
		stopCallee()
		cancel()
	}

	if !waitFor(3*time.Second, func() bool { return bridgedCallExists(pbx, "111", "110") }) {
		t.Fatal("call not registered as bridged")
	}
	time.Sleep(300 * time.Millisecond)
	return
}

type holdMediaSignal struct {
	mu               sync.Mutex
	moh, dialTone    bool
}

func (s *holdMediaSignal) reset() {
	s.mu.Lock()
	s.moh, s.dialTone = false, false
	s.mu.Unlock()
}

func (s *holdMediaSignal) install(t *testing.T) {
	t.Helper()
	s.reset()
	call.HoldMediaStartedHook = func(moh, dialTone bool) {
		s.mu.Lock()
		s.moh = s.moh || moh
		s.dialTone = s.dialTone || dialTone
		s.mu.Unlock()
	}
	t.Cleanup(func() { call.HoldMediaStartedHook = nil })
}

func (s *holdMediaSignal) waitFor(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		ok := s.moh && s.dialTone
		s.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.mu.Lock()
	gotMOH, gotDial := s.moh, s.dialTone
	s.mu.Unlock()
	if !gotMOH {
		t.Fatal("REQ-HOLD-2: server did not start MOH for held party")
	}
	if !gotDial {
		t.Fatal("REQ-HOLD-1: server did not start dial tone for holder")
	}
}

func assertHoldEntered(t *testing.T, pbx *testPBX, sig *holdMediaSignal) {
	t.Helper()
	if !waitFor(10*time.Second, func() bool { return holdActive(pbx, "111", "110") }) {
		t.Fatal("server did not enter hold state")
	}
	sig.waitFor(t, 8*time.Second)
}

// REQ-HOLD-1/2: caller (111) holds — holder hears dial tone, callee hears MOH.
func TestREQ_HOLD_CallerHoldDialToneAndMOH(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	_, _, out, _, audioCancel := establishCall(t, pbx, ctx)
	defer out.Close()
	defer audioCancel()

	audioCancel()

	holdCtx, holdCancel := context.WithTimeout(ctx, 8*time.Second)
	defer holdCancel()
	if err := out.Hold(holdCtx); err != nil {
		t.Fatalf("caller hold re-INVITE failed: %v", err)
	}

	assertHoldEntered(t, pbx, &sig)

	mohCtx, mohCancel := context.WithTimeout(ctx, 10*time.Second)
	defer mohCancel()
	if err := waitHoldMOHBytes(mohCtx, pbx, "111", "110", minMOHBytes); err != nil {
		t.Fatalf("REQ-HOLD-2: server did not send MOH to held party: %v", err)
	}
}

// REQ-HOLD-1/2: callee (110) holds — holder hears dial tone, caller hears MOH.
func TestREQ_HOLD_CalleeHoldDialToneAndMOH(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	_, _, out, calleeLeg, audioCancel := establishCall(t, pbx, ctx)
	defer out.Close()
	defer audioCancel()

	audioCancel()

	holdCtx, holdCancel := context.WithTimeout(ctx, 8*time.Second)
	defer holdCancel()
	if err := calleeLeg.Hold(holdCtx); err != nil {
		t.Fatalf("callee hold re-INVITE failed: %v", err)
	}

	assertHoldEntered(t, pbx, &sig)
}

// REQ-HOLD-3: repeated hold/unhold cycles must restore two-way audio.
func TestREQ_HOLD_UnholdRestoresAudio(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	_, _, out, calleeLeg, audioCancel := establishCall(t, pbx, ctx)
	defer out.Close()
	defer audioCancel()

	for cycle := 1; cycle <= 2; cycle++ {
		sig.reset()
		audioCancel() // stop test pumps before hold/unhold; bridge owns media after unhold

		holdCtx, holdCancel := context.WithTimeout(ctx, 8*time.Second)
		err := out.Hold(holdCtx)
		holdCancel()
		if err != nil {
			t.Fatalf("hold cycle %d: %v", cycle, err)
		}
		assertHoldEntered(t, pbx, &sig)

		mohCtx, mohCancel := context.WithTimeout(ctx, 8*time.Second)
		if err := waitHoldMOHBytes(mohCtx, pbx, "111", "110", minMOHBytes); err != nil {
			mohCancel()
			t.Fatalf("REQ-HOLD-2 cycle %d: %v", cycle, err)
		}
		mohCancel()

		unholdCtx, unholdCancel := context.WithTimeout(ctx, 20*time.Second)
		err = out.Unhold(unholdCtx)
		unholdCancel()
		if err != nil {
			t.Fatalf("unhold cycle %d: %v", cycle, err)
		}

		if !waitFor(10*time.Second, func() bool {
			return !holdActive(pbx, "111", "110") && bridgedCallExists(pbx, "111", "110")
		}) {
			t.Fatalf("REQ-HOLD-3: bridge not restored after unhold cycle %d", cycle)
		}

		relayCtx, relayCancel := context.WithTimeout(ctx, 15*time.Second)
		assertTwoWayBridgeRelay(t, relayCtx, pbx, out, calleeLeg)
		relayCancel()

		if out.Context().Err() != nil || calleeLeg.Context().Err() != nil {
			t.Fatalf("REQ-HOLD-3: leg dropped during cycle %d", cycle)
		}
	}
}

// REQ-HOLD-4: hold must stay active after dial-tone sendonly re-INVITE (no false unhold).
func TestREQ_HOLD_StableAfterDialToneReinvite(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	_, _, out, _, audioCancel := establishCall(t, pbx, ctx)
	defer out.Close()
	defer audioCancel()
	audioCancel()

	holdCtx, holdCancel := context.WithTimeout(ctx, 8*time.Second)
	if err := out.Hold(holdCtx); err != nil {
		t.Fatalf("hold: %v", err)
	}
	holdCancel()

	assertHoldEntered(t, pbx, &sig)

	stableUntil := time.Now().Add(3 * time.Second)
	for time.Now().Before(stableUntil) {
		if !holdActive(pbx, "111", "110") {
			t.Fatal("REQ-HOLD-4: hold dropped during dial-tone / sustained hold window")
		}
		time.Sleep(100 * time.Millisecond)
	}

	mohCtx, mohCancel := context.WithTimeout(ctx, 8*time.Second)
	defer mohCancel()
	if err := waitHoldMOHBytes(mohCtx, pbx, "111", "110", minMOHBytes*2); err != nil {
		t.Fatalf("REQ-HOLD-2/4: MOH must continue while hold stays active: %v", err)
	}
}

// REQ-HOLD-2/4: MOH must keep flowing for several seconds after hold entry (catches spurious leave()).
func TestREQ_HOLD_SustainedMOHDuringHold(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	_, _, out, _, audioCancel := establishCall(t, pbx, ctx)
	defer out.Close()
	defer audioCancel()
	audioCancel()

	holdCtx, holdCancel := context.WithTimeout(ctx, 8*time.Second)
	if err := out.Hold(holdCtx); err != nil {
		t.Fatalf("hold: %v", err)
	}
	holdCancel()
	assertHoldEntered(t, pbx, &sig)

	mohCtx, mohCancel := context.WithTimeout(ctx, 8*time.Second)
	if err := waitHoldMOHBytes(mohCtx, pbx, "111", "110", minMOHBytes); err != nil {
		mohCancel()
		t.Fatalf("REQ-HOLD-2: MOH not running after hold enter: %v", err)
	}
	mohCancel()

	base, ok := pbx.Srv.HoldMOHBytesSent("111", "110")
	if !ok || base < minMOHBytes {
		t.Fatalf("REQ-HOLD-2: MOH not running after hold enter (bytes=%d)", base)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !holdActive(pbx, "111", "110") {
			t.Fatal("REQ-HOLD-4: hold dropped while MOH should still be playing")
		}
		time.Sleep(100 * time.Millisecond)
	}

	after, ok := pbx.Srv.HoldMOHBytesSent("111", "110")
	if !ok {
		t.Fatal("REQ-HOLD-2: hold ended unexpectedly during sustained MOH check")
	}
	if after-base < minMOHBytes {
		t.Fatalf("REQ-HOLD-2: MOH bytes stalled during hold (delta %d, need >= %d)", after-base, minMOHBytes)
	}
}

// REQ-HOLD-1/2/3 on wideband codec path — MOH uses generated tone fallback when WAV unsupported.
func TestREQ_HOLD_G722CallerHoldMOHAndUnhold(t *testing.T) {
	g722 := media.Codec{PayloadType: 9, SampleRate: 8000, Name: "G722"}
	pbx := startPBX(t, pbxOptions{
		Extensions: map[string]string{"110": "andy", "111": "andy"},
		Codecs:     []string{"G722", "PCMU", "PCMA"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	caller := newHandset(t, pbx.Port, "111", "andy", g722, media.CodecAudioUlaw)
	callee := newHandset(t, pbx.Port, "110", "andy", g722, media.CodecAudioUlaw)
	caller.register()
	callee.register()

	answered := make(chan *diago.DialogServerSession, 1)
	callee.serveAnswer(ctx, answered, true)

	out, err := caller.invite(ctx, "110", nil)
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	defer out.Close()

	var calleeLeg *diago.DialogServerSession
	select {
	case calleeLeg = <-answered:
	case <-time.After(5 * time.Second):
		t.Fatal("callee did not answer")
	}

	audioCtx, audioCancel := context.WithCancel(ctx)
	stopOut := pumpAudio(audioCtx, out)
	stopCallee := pumpAudio(audioCtx, calleeLeg)
	stopOut()
	stopCallee()
	audioCancel()

	if !waitFor(5*time.Second, func() bool { return bridgedCallExists(pbx, "111", "110") }) {
		t.Fatal("call not bridged")
	}

	holdCtx, holdCancel := context.WithTimeout(ctx, 8*time.Second)
	if err := out.Hold(holdCtx); err != nil {
		t.Fatalf("G722 hold: %v", err)
	}
	holdCancel()
	assertHoldEntered(t, pbx, &sig)

	mohCtx, mohCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := waitHoldMOHBytes(mohCtx, pbx, "111", "110", minMOHBytes); err != nil {
		mohCancel()
		t.Fatalf("REQ-HOLD-2 G722: %v", err)
	}
	mohCancel()

	unholdCtx, unholdCancel := context.WithTimeout(ctx, 20*time.Second)
	if err := out.Unhold(unholdCtx); err != nil {
		t.Fatalf("G722 unhold: %v", err)
	}
	unholdCancel()

	if !waitFor(10*time.Second, func() bool {
		return !holdActive(pbx, "111", "110") && bridgedCallExists(pbx, "111", "110")
	}) {
		t.Fatal("REQ-HOLD-3 G722: bridge not restored after unhold")
	}

	relayCtx, relayCancel := context.WithTimeout(ctx, 15*time.Second)
	assertTwoWayBridgeRelay(t, relayCtx, pbx, out, calleeLeg)
	relayCancel()
}

// REQ-HOLD-3/5: unhold must restore bidirectional RTP, not just flags.
func TestREQ_HOLD_UnholdRestoresRTP(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	_, _, out, calleeLeg, audioCancel := establishCall(t, pbx, ctx)
	defer out.Close()
	defer audioCancel()
	audioCancel()

	holdCtx, holdCancel := context.WithTimeout(ctx, 8*time.Second)
	if err := out.Hold(holdCtx); err != nil {
		t.Fatalf("hold: %v", err)
	}
	holdCancel()
	assertHoldEntered(t, pbx, &sig)

	mohCtx, mohCancel := context.WithTimeout(ctx, 10*time.Second)
	defer mohCancel()
	if err := waitHoldMOHBytes(mohCtx, pbx, "111", "110", minMOHBytes); err != nil {
		t.Fatalf("REQ-HOLD-2: server did not send MOH during hold: %v", err)
	}

	unholdCtx, unholdCancel := context.WithTimeout(ctx, 20*time.Second)
	if err := out.Unhold(unholdCtx); err != nil {
		t.Fatalf("unhold: %v", err)
	}
	unholdCancel()

	relayCtx, relayCancel := context.WithTimeout(ctx, 15*time.Second)
	assertTwoWayBridgeRelay(t, relayCtx, pbx, out, calleeLeg)
	relayCancel()
}

// holdWithSendrecvChurn simulates a real phone: hold re-INVITE then a quick sendrecv
// refresh (codec/direction churn) while the PBX is still entering hold.
func holdWithSendrecvChurn(ctx context.Context, out *diago.DialogClientSession, churnDelay time.Duration) error {
	churnCtx, churnCancel := context.WithTimeout(ctx, 8*time.Second)
	defer churnCancel()
	churnDone := make(chan struct{})
	go func() {
		defer close(churnDone)
		time.Sleep(churnDelay)
		_ = out.ReInviteMediaMode(churnCtx, sdp.ModeSendrecv)
	}()
	err := out.Hold(ctx)
	<-churnDone
	return err
}

func assertHoldStable(t *testing.T, pbx *testPBX, duration time.Duration) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if !holdActive(pbx, "111", "110") {
			t.Fatal("REQ-HOLD-4: hold dropped during stability window after phone SDP churn")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// assertNoBridgeRelayDuringHold verifies the PCM bridge stays stopped while hold is active.
// A spurious bridge restart during hold entry would relay injected tone (production bug on phones).
func assertNoBridgeRelayDuringHold(t *testing.T, ctx context.Context, pbx *testPBX, callerLeg, calleeLeg diago.DialogSession) {
	t.Helper()
	baseToCallee, baseToCaller, ok := bridgedRelayDelta(pbx, "111", "110")
	if !ok {
		t.Fatal("bridged call not found for relay check during hold")
	}

	stopCaller := writeToneFrames(ctx, callerLeg)
	stopCallee := writeToneFrames(ctx, calleeLeg)
	defer stopCaller()
	defer stopCallee()

	time.Sleep(2 * time.Second)

	toCallee, toCaller, ok := bridgedRelayDelta(pbx, "111", "110")
	if !ok {
		t.Fatal("bridged call lost during hold relay check")
	}
	if toCallee-baseToCallee >= minBridgeRelayBytes || toCaller-baseToCaller >= minBridgeRelayBytes {
		t.Fatalf("REQ-HOLD-4: bridge relayed audio during hold (delta caller→callee=%d callee→caller=%d); bridge must stay stopped",
			toCallee-baseToCallee, toCaller-baseToCaller)
	}
}

// REQ-HOLD-4/5: sendrecv SDP churn during hold entry must not abort hold or restart the bridge.
func TestREQ_HOLD_SendrecvChurnDuringHoldEnter(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	_, _, out, calleeLeg, audioCancel := establishCall(t, pbx, ctx)
	defer out.Close()
	defer audioCancel()
	audioCancel()

	relayCtx, relayCancel := context.WithTimeout(ctx, 10*time.Second)
	assertTwoWayBridgeRelay(t, relayCtx, pbx, out, calleeLeg)
	relayCancel()

	holdCtx, holdCancel := context.WithTimeout(ctx, 12*time.Second)
	if err := holdWithSendrecvChurn(holdCtx, out, 40*time.Millisecond); err != nil {
		holdCancel()
		t.Fatalf("hold with sendrecv churn: %v", err)
	}
	holdCancel()

	assertHoldEntered(t, pbx, &sig)
	assertHoldStable(t, pbx, 3*time.Second)

	mohCtx, mohCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := waitHoldMOHBytes(mohCtx, pbx, "111", "110", minMOHBytes); err != nil {
		mohCancel()
		t.Fatalf("REQ-HOLD-2: MOH must run after churn during hold enter: %v", err)
	}
	mohCancel()

	frozenCtx, frozenCancel := context.WithTimeout(ctx, 5*time.Second)
	assertNoBridgeRelayDuringHold(t, frozenCtx, pbx, out, calleeLeg)
	frozenCancel()
}

// REQ-HOLD-5: sendrecv while held (dial-tone leg up) must release — real phone hold toggle.
func TestREQ_HOLD_SendrecvWhileHeldReleasesCall(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	_, _, out, _, audioCancel := establishCall(t, pbx, ctx)
	defer out.Close()
	defer audioCancel()
	audioCancel()

	holdCtx, holdCancel := context.WithTimeout(ctx, 8*time.Second)
	if err := out.Hold(holdCtx); err != nil {
		holdCancel()
		t.Fatalf("hold: %v", err)
	}
	holdCancel()
	assertHoldEntered(t, pbx, &sig)

	releaseCtx, releaseCancel := context.WithTimeout(ctx, 8*time.Second)
	if err := out.ReInviteMediaMode(releaseCtx, sdp.ModeSendrecv); err != nil {
		releaseCancel()
		t.Fatalf("sendrecv release while held: %v", err)
	}
	releaseCancel()

	if !waitFor(10*time.Second, func() bool {
		return !holdActive(pbx, "111", "110") && bridgedCallExists(pbx, "111", "110")
	}) {
		t.Fatal("REQ-HOLD-5: sendrecv while held must release hold and restore bridge")
	}
}

// REQ-HOLD-3/4: churn during enter then intentional unhold must restore bidirectional bridge relay.
func TestREQ_HOLD_ChurnDuringEnterThenUnholdRestoresRelay(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	_, _, out, calleeLeg, audioCancel := establishCall(t, pbx, ctx)
	defer out.Close()
	defer audioCancel()
	audioCancel()

	holdCtx, holdCancel := context.WithTimeout(ctx, 12*time.Second)
	if err := holdWithSendrecvChurn(holdCtx, out, 40*time.Millisecond); err != nil {
		holdCancel()
		t.Fatalf("hold with churn: %v", err)
	}
	holdCancel()
	assertHoldEntered(t, pbx, &sig)
	assertHoldStable(t, pbx, 2*time.Second)

	unholdCtx, unholdCancel := context.WithTimeout(ctx, 20*time.Second)
	if err := out.Unhold(unholdCtx); err != nil {
		unholdCancel()
		t.Fatalf("unhold after churn: %v", err)
	}
	unholdCancel()

	if !waitFor(10*time.Second, func() bool {
		return !holdActive(pbx, "111", "110") && bridgedCallExists(pbx, "111", "110")
	}) {
		t.Fatal("REQ-HOLD-3: bridge not restored after unhold following hold-entry churn")
	}

	relayCtx, relayCancel := context.WithTimeout(ctx, 15*time.Second)
	assertTwoWayBridgeRelay(t, relayCtx, pbx, out, calleeLeg)
	relayCancel()
}

// REQ-HOLD-1/2/3/4 on G722 with sendrecv churn during hold entry (wideband phone path).
func TestREQ_HOLD_G722SendrecvChurnDuringHoldEnter(t *testing.T) {
	g722 := media.Codec{PayloadType: 9, SampleRate: 8000, Name: "G722"}
	pbx := startPBX(t, pbxOptions{
		Extensions: map[string]string{"110": "andy", "111": "andy"},
		Codecs:     []string{"G722", "PCMU", "PCMA"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	caller := newHandset(t, pbx.Port, "111", "andy", g722, media.CodecAudioUlaw)
	callee := newHandset(t, pbx.Port, "110", "andy", g722, media.CodecAudioUlaw)
	caller.register()
	callee.register()

	answered := make(chan *diago.DialogServerSession, 1)
	callee.serveAnswer(ctx, answered, true)

	out, err := caller.invite(ctx, "110", nil)
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	defer out.Close()

	var calleeLeg *diago.DialogServerSession
	select {
	case calleeLeg = <-answered:
	case <-time.After(5 * time.Second):
		t.Fatal("callee did not answer")
	}

	audioCtx, audioCancel := context.WithCancel(ctx)
	stopOut := pumpAudio(audioCtx, out)
	stopCallee := pumpAudio(audioCtx, calleeLeg)
	stopOut()
	stopCallee()
	audioCancel()

	if !waitFor(5*time.Second, func() bool { return bridgedCallExists(pbx, "111", "110") }) {
		t.Fatal("call not bridged")
	}

	holdCtx, holdCancel := context.WithTimeout(ctx, 12*time.Second)
	if err := holdWithSendrecvChurn(holdCtx, out, 40*time.Millisecond); err != nil {
		holdCancel()
		t.Fatalf("G722 hold with churn: %v", err)
	}
	holdCancel()
	assertHoldEntered(t, pbx, &sig)
	assertHoldStable(t, pbx, 3*time.Second)

	mohCtx, mohCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := waitHoldMOHBytes(mohCtx, pbx, "111", "110", minMOHBytes); err != nil {
		mohCancel()
		t.Fatalf("REQ-HOLD-2 G722 churn: %v", err)
	}
	mohCancel()

	unholdCtx, unholdCancel := context.WithTimeout(ctx, 20*time.Second)
	if err := out.Unhold(unholdCtx); err != nil {
		unholdCancel()
		t.Fatalf("G722 unhold after churn: %v", err)
	}
	unholdCancel()

	if !waitFor(10*time.Second, func() bool {
		return !holdActive(pbx, "111", "110") && bridgedCallExists(pbx, "111", "110")
	}) {
		t.Fatal("REQ-HOLD-3 G722: bridge not restored after unhold following churn")
	}

	relayCtx, relayCancel := context.WithTimeout(ctx, 15*time.Second)
	assertTwoWayBridgeRelay(t, relayCtx, pbx, out, calleeLeg)
	relayCancel()
}

// REQ-HOLD-5: each sendrecv while held releases (phone hold toggle); re-hold cycle works.
func TestREQ_HOLD_HoldReleaseHoldCycle(t *testing.T) {
	pbx := startPBX(t, pbxOptions{Extensions: map[string]string{"110": "andy", "111": "andy"}})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var sig holdMediaSignal
	sig.install(t)

	_, _, out, _, audioCancel := establishCall(t, pbx, ctx)
	defer out.Close()
	defer audioCancel()
	audioCancel()

	for cycle := 1; cycle <= 2; cycle++ {
		sig.reset()
		holdCtx, holdCancel := context.WithTimeout(ctx, 8*time.Second)
		if err := out.Hold(holdCtx); err != nil {
			holdCancel()
			t.Fatalf("hold cycle %d: %v", cycle, err)
		}
		holdCancel()
		assertHoldEntered(t, pbx, &sig)

		releaseCtx, releaseCancel := context.WithTimeout(ctx, 8*time.Second)
		if err := out.ReInviteMediaMode(releaseCtx, sdp.ModeSendrecv); err != nil {
			releaseCancel()
			t.Fatalf("release cycle %d: %v", cycle, err)
		}
		releaseCancel()
		if !waitFor(10*time.Second, func() bool {
			return !holdActive(pbx, "111", "110") && bridgedCallExists(pbx, "111", "110")
		}) {
			t.Fatalf("REQ-HOLD-5: hold cycle %d did not release on sendrecv", cycle)
		}
	}
}
