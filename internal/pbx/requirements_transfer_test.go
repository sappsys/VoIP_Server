package pbx

import (
	"testing"

	"github.com/sappsys/VoIP_Server/internal/call"
)

// REQ-XFER-1: attended transfer (*code) arms TransferReady and MOH for the other party.
// REQ-XFER-2: blind transfer (REFER / hangup after dial) bridges held party to target.

func TestREQ_XFER_AttendedTransferArmsReady(t *testing.T) {
	srv, _, _ := newTestServerLight(t)
	ac := &call.ActiveCall{
		CallerExt: "101", CalleeExt: "102",
		In: testInviteDialog("active-1", "101", "102"),
	}
	srv.registry.Register(ac)
	if !srv.registry.SetTransferReady("101") {
		t.Fatal("REQ-XFER-1: transfer must arm TransferReady")
	}
	if !ac.TransferReady {
		t.Fatal("REQ-XFER-1: active call must be transfer-ready")
	}
}

func TestREQ_XFER_ConsultLegLinkedWhileOnHold(t *testing.T) {
	srv, _, _ := newTestServerLight(t)
	ac := &call.ActiveCall{
		CallerExt: "101",
		CalleeExt: "102",
		HoldActive: true,
		In:        testInviteDialog("main", "101", "102"),
	}
	srv.registry.Register(ac)
	consult := testInviteDialog("consult", "101", "103")
	srv.registry.SetConsult("101", consult)
	if ac.ConsultIn != consult {
		t.Fatal("REQ-XFER-1: consult leg must link while holder dials third extension")
	}
}

func TestREQ_XFER_BlindTransferUsesReferHandler(t *testing.T) {
	body := readBridgeTransferSource(t)
	if !containsAll(body, "makeTransferHandler", "startTranscodingBridge") {
		t.Fatal("REQ-XFER-2: blind REFER must bridge held party to refer target via PCM bridge")
	}
}

func TestREQ_XFER_CompleteTransferBridgesHeldParty(t *testing.T) {
	body := readBridgeTransferSource(t)
	if !containsAll(body, "CompleteTransfer", "startControllableBridgeSessions") {
		t.Fatal("REQ-XFER-2: attended complete must bridge held party to dialed target")
	}
}
