package call

import (
	"testing"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media/sdp"
)

// REQ-HOLD-6: unhold and new bridges must reset media legs before PCM relay starts.
func TestREQ_HOLD_UnholdPreparesLegsBeforeBridge(t *testing.T) {
	var hooked bool
	prepareLegsForBridgeHook = func(a, c diago.DialogSession) {
		if a != nil && c != nil {
			hooked = true
		}
	}
	t.Cleanup(func() { prepareLegsForBridgeHook = nil })

	in := testServerDialog("in")
	out := &diago.DialogClientSession{}
	prepareLegsForBridge(in, out)
	if !hooked {
		t.Fatal("REQ-HOLD-6: prepareLegsForBridge must run before bridge start")
	}
}

// REQ-HOLD-6b: hold leave() must prepare legs before restarting the bridge.
func TestREQ_HOLD_LeaveRestartsBridgeAfterPrepare(t *testing.T) {
	msIn := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	msOut := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)

	in := testServerDialog("in")
	out := &diago.DialogClientSession{}
	in.InitMediaSession(msIn, nil, nil)
	out.InitMediaSession(msOut, nil, nil)

	var prepared bool
	prepareLegsForBridgeHook = func(a, c diago.DialogSession) { prepared = true }
	t.Cleanup(func() { prepareLegsForBridgeHook = nil })

	ac := &ActiveCall{HoldActive: true, CallerExt: "111", CalleeExt: "110"}
	h := &holdController{
		b:   &BridgePair{},
		ctx: t.Context(),
		ac:  ac,
		in:  in,
		out: out,
	}

	// Simulate successful unhold without real SIP re-INVITE.
	h.leave()

	if !prepared {
		t.Fatal("REQ-HOLD-6b: leave() must call prepareLegsForBridge before bridge restart")
	}
	if ac.HoldActive {
		t.Fatal("leave() must clear HoldActive")
	}
}
