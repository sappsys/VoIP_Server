package call

import (
	"testing"

	"github.com/emiago/diago/media/sdp"
)

// REQ-HOLD-1: server may send MOH/dial-tone when phone hold answers recvonly.
// REQ-HOLD-2: bridge restarts only when both legs are sendrecv.

func TestREQ_HOLD_LegCanSendMediaOnPhoneHold(t *testing.T) {
	ms := newTestMediaSession(t)
	if err := ms.RemoteSDP(holdRemoteSDP(sdp.ModeRecvonly)); err != nil {
		t.Fatal(err)
	}
	if !legCanSendMedia(ms) {
		t.Fatal("REQ-HOLD-1: recvonly (phone hold) must allow server to send MOH/dial tone")
	}
}

func TestREQ_HOLD_BridgeRequiresSendRecv(t *testing.T) {
	ms := newTestMediaSession(t)
	if err := ms.RemoteSDP(holdRemoteSDP(sdp.ModeSendrecv)); err != nil {
		t.Fatal(err)
	}
	if negotiatedMediaDirection(ms) != sdp.ModeSendrecv {
		t.Fatalf("REQ-HOLD-2: bridge needs sendrecv, got %q", negotiatedMediaDirection(ms))
	}
}
