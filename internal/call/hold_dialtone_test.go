package call

import (
	"sync/atomic"
	"testing"

	"github.com/emiago/diago/media/sdp"
)

func TestLegCanSendMedia(t *testing.T) {
	tests := []struct {
		remoteMode string
		want       bool
	}{
		{sdp.ModeSendrecv, true},
		{sdp.ModeSendonly, false}, // phone hold: we answer recvonly
		{sdp.ModeRecvonly, true},  // we answer sendonly
		{sdp.ModeInactive, false},
	}
	for _, tc := range tests {
		ms := newTestMediaSession(t)
		if err := ms.RemoteSDP(holdRemoteSDP(tc.remoteMode)); err != nil {
			t.Fatalf("remote=%s: %v", tc.remoteMode, err)
		}
		if got := legCanSendMedia(ms); got != tc.want {
			t.Fatalf("remote=%s got=%v want=%v dir=%q", tc.remoteMode, got, tc.want, negotiatedMediaDirection(ms))
		}
	}
}

// REQ-HOLD-1: phone hold negotiates recvonly; dial tone must not wait on a racing Hold() re-INVITE.
func TestREQ_HOLD_PhoneHoldDialToneWithoutReinvite(t *testing.T) {
	var holds atomic.Int32
	testHolderHoldInvoked = func(bool) { holds.Add(1) }
	t.Cleanup(func() { testHolderHoldInvoked = nil })

	ms := newTestMediaSession(t)
	if err := ms.RemoteSDP(holdRemoteSDP(sdp.ModeSendonly)); err != nil {
		t.Fatal(err)
	}
	if negotiatedMediaDirection(ms) != sdp.ModeRecvonly {
		t.Fatalf("phone hold answer should be recvonly, got %q", negotiatedMediaDirection(ms))
	}
	in := testServerDialog("in")
	in.InitMediaSession(ms, nil, nil)

	ac := &ActiveCall{CallerExt: "111", CalleeExt: "110", holderPhoneHold: true}
	h := &holdController{ac: ac, in: in, ctx: t.Context()}

	if !h.enableHolderSendMedia(t.Context(), true) {
		t.Fatal("REQ-HOLD-1: phone hold must enable holder dial tone without server re-INVITE")
	}
	if holds.Load() != 0 {
		t.Fatal("REQ-HOLD-1: phone hold must not invoke holder Hold() re-INVITE")
	}
	if negotiatedMediaDirection(in.MediaSession()) != sdp.ModeSendonly {
		t.Fatalf("REQ-HOLD-1: holder leg must be sendable after phone hold, got %q",
			negotiatedMediaDirection(in.MediaSession()))
	}
}
