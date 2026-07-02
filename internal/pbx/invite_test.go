package pbx

import (
	"testing"

	"github.com/emiago/diago"
	"github.com/sappsys/VoIP_Server/internal/call"
)

func TestInviteFlowRecordsHistory(t *testing.T) {
	srv, st, _ := newTestServerLight(t)
	srv.recordCall("101", "102")
	dialed, _ := st.GetLastDialed("101")
	if dialed != "102" {
		t.Fatalf("history dialed=%q", dialed)
	}
}

func TestParkThenRetrieveLifecycle(t *testing.T) {
	srv, _, _ := newTestServerLight(t)
	ac := &call.ActiveCall{
		CallerExt: "101", CalleeExt: "102",
		In: testInviteDialog("parked-call", "101", "102"),
	}
	srv.registry.Register(ac)

	heldIn, heldOut := ac.StealHeldLeg("101")
	srv.park.Park("101", heldIn, heldOut, nil)
	if !srv.park.Has("101") {
		t.Fatal("park slot missing")
	}

	pc := srv.park.Retrieve("101")
	if pc == nil {
		t.Fatal("retrieve failed")
	}
	if srv.park.Has("101") {
		t.Fatal("slot should be empty after retrieve")
	}
}

func TestTransferReadyIntercept(t *testing.T) {
	srv, _, _ := newTestServerLight(t)
	ac := &call.ActiveCall{
		CallerExt: "101", CalleeExt: "102",
		In:  testInviteDialog("held", "101", "102"),
		Out: &diago.DialogClientSession{},
	}
	srv.registry.Register(ac)
	ac.TransferReady = true

	// Transfer complete path checks extension route
	if ac := srv.registry.FindByExtension("101"); ac == nil || !ac.TransferReady {
		t.Fatal("transfer not armed")
	}
}

func TestCallManagerLimitOnRetrieve(t *testing.T) {
	srv, _, _ := newTestServerLight(t)
	srv.calls = call.NewManager(1)
	in1 := testInviteDialog("c1", "101", "102")
	in2 := testInviteDialog("c2", "101", "103")
	if _, err := srv.calls.TryAcquire(in1.ID, "101", "102", "", srv.exts); err != nil {
		t.Fatal(err)
	}
	_, err := srv.calls.TryAcquire(in2.ID, "101", "103", "", srv.exts)
	if err == nil {
		t.Fatal("expected global call limit busy")
	}
}
