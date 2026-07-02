package call

import (
	"testing"

	"github.com/emiago/diago"
)

func TestRegistryRegisterFind(t *testing.T) {
	r := NewRegistry()
	in := testServerDialog("dlg-1")
	ac := &ActiveCall{CallerExt: "101", CalleeExt: "102", In: in}
	r.Register(ac)

	if got := r.ByExtension("101"); got != ac {
		t.Fatal("expected caller lookup")
	}
	if got := r.FindByExtension("102"); got != ac {
		t.Fatal("expected callee lookup")
	}
	if got := r.ByDialog("dlg-1"); got != ac {
		t.Fatal("expected dialog lookup")
	}

	r.Unregister("dlg-1")
	if r.ByExtension("101") != nil || r.FindByExtension("102") != nil {
		t.Fatal("expected unregister to clear indexes")
	}
}

func TestRegistrySetTransferReady(t *testing.T) {
	r := NewRegistry()
	ac := &ActiveCall{CallerExt: "101", CalleeExt: "102", In: testServerDialog("d1")}
	r.Register(ac)

	if !r.SetTransferReady("102") {
		t.Fatal("callee should find active call")
	}
	if !ac.TransferReady {
		t.Fatal("transfer flag not set")
	}
	if r.SetTransferReady("999") {
		t.Fatal("unknown ext should fail")
	}
}

func TestStealHeldLeg(t *testing.T) {
	out := &diago.DialogClientSession{}
	ac := &ActiveCall{CallerExt: "101", CalleeExt: "102", In: testServerDialog("d1"), Out: out}

	heldIn, heldOut := ac.StealHeldLeg("101")
	if heldIn != nil || heldOut != out || ac.Out != nil || !ac.Parked {
		t.Fatalf("caller park: in=%v out=%v ac.Out=%v parked=%v", heldIn, heldOut, ac.Out, ac.Parked)
	}

	ac2 := &ActiveCall{CallerExt: "101", CalleeExt: "102", In: testServerDialog("d2"), Out: out}
	heldIn, heldOut = ac2.StealHeldLeg("102")
	if heldIn == nil || heldOut != nil || ac2.In != nil {
		t.Fatalf("callee park: in=%v out=%v ac.In=%v", heldIn, heldOut, ac2.In)
	}
}
