package call

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media/sdp"
)

// REQ-HOLD-4: our dial-tone sendonly re-INVITE must not drive leave() while pending.
func TestREQ_HOLD_DialTonePendingBlocksFalseUnhold(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendonly)
	in := testServerDialog("in")
	in.InitMediaSession(ms, nil, nil)

	ac := &ActiveCall{
		CallerExt:       "111",
		CalleeExt:       "110",
		HoldActive:      true,
		HolderExt:       "111",
		holderPhoneHold: true,
		dialTonePending: true,
		holdEnteredAt:   time.Now(),
	}
	h := &holdController{ac: ac, in: in}

	if h.holderReleasedForLeave(in.Media(), true) {
		t.Fatal("REQ-HOLD-4: dial-tone sendonly must not trigger leave while dialTonePending")
	}
}

// REQ-HOLD-4: after dial-tone completes, server sendonly must NOT trigger leave.
// Real phones emit follow-up media updates on this SDP — leave() here was killing MOH/dial tone.
func TestREQ_HOLD_DialToneSendonlyAfterEnterMustNotLeave(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendonly)
	in := testServerDialog("in")
	in.InitMediaSession(ms, nil, nil)

	ac := &ActiveCall{
		CallerExt:       "111",
		CalleeExt:       "110",
		HoldActive:      true,
		HolderExt:       "111",
		holderPhoneHold: true,
		holderLegHeld:   true,
		dialTonePending: false,
		holdEnteredAt:   time.Now().Add(-3 * time.Second),
	}
	h := &holdController{ac: ac, in: in}

	if h.shouldLeaveOnPhoneUnhold(in.Media()) {
		t.Fatal("REQ-HOLD-4: server sendonly dial tone must not trigger leave after hold entered")
	}
}

// REQ-HOLD-5: sendrecv alone must not trigger leave unless recvonly hold was seen first.
func TestREQ_HOLD_SendrecvWithoutRecvonlyHoldMustNotLeave(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	in := testServerDialog("in")
	in.InitMediaSession(ms, nil, nil)

	ac := &ActiveCall{
		CallerExt:       "111",
		HoldActive:      true,
		HolderExt:       "111",
		holderPhoneHold: true,
		holderSawRecvonly: false,
		holdEnteredAt:   time.Now().Add(-5 * time.Second),
	}
	h := &holdController{ac: ac, in: in}

	if h.shouldLeaveOnPhoneUnhold(in.Media()) {
		t.Fatal("REQ-HOLD-5: sendrecv during phone hold without prior recvonly must not leave")
	}
}

// REQ-HOLD-5: sendrecv after dial-tone leg is up (legHeld) is phone release → leave, not reassert.
func TestREQ_HOLD_SendrecvAfterDialToneTriggersLeave(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	in := testServerDialog("in")
	in.InitMediaSession(ms, nil, nil)

	ac := &ActiveCall{
		CallerExt:         "111",
		CalleeExt:         "110",
		HoldActive:        true,
		HolderExt:         "111",
		holderPhoneHold:   true,
		holderSawRecvonly: true,
		holderLegHeld:     true,
		holderPrevDir:     sdp.ModeSendonly,
		holdEnteredAt:     time.Now().Add(-1 * time.Second),
	}
	h := &holdController{ac: ac, in: in}

	if !h.shouldLeaveOnPhoneUnhold(in.Media()) {
		t.Fatal("REQ-HOLD-5: sendrecv after dial tone leg must trigger leave (phone release)")
	}
	if h.shouldReassertHolderHold(in.Media()) {
		t.Fatal("REQ-HOLD-5: must not reassert sendonly after dial tone leg is up")
	}
}

// REQ-HOLD-5: sendrecv before dial-tone leg (no legHeld) during enter may still reassert.
func TestREQ_HOLD_SendrecvChurnBeforeDialToneLegReasserts(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	in := testServerDialog("in")
	in.InitMediaSession(ms, nil, nil)

	ac := &ActiveCall{
		CallerExt:         "111",
		CalleeExt:         "110",
		HoldActive:        true,
		HolderExt:         "111",
		holderPhoneHold:   true,
		holderSawRecvonly: true,
		holderLegHeld:     false,
		holderPrevDir:     sdp.ModeRecvonly,
		holdEntering:      true,
		holdEnteredAt:     time.Now(),
	}
	h := &holdController{ac: ac, in: in}

	if h.shouldLeaveOnPhoneUnhold(in.Media()) {
		t.Fatal("REQ-HOLD-4: sendrecv churn before dial-tone leg must not leave immediately")
	}
	if !h.shouldReassertHolderHold(in.Media()) {
		t.Fatal("REQ-HOLD-4: sendrecv churn before dial-tone leg must reassert sendonly")
	}
}

// REQ-HOLD-5: legHeld + sendrecv is always phone release (matches IP phone hold toggle).
func TestREQ_HOLD_LegHeldSendrecvAlwaysLeaves(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	in := testServerDialog("in")
	in.InitMediaSession(ms, nil, nil)

	ac := &ActiveCall{
		CallerExt:         "111",
		CalleeExt:         "110",
		HoldActive:        true,
		HolderExt:         "111",
		holderPhoneHold:   true,
		holderSawRecvonly: true,
		holderLegHeld:     true,
		holderPrevDir:     sdp.ModeSendonly,
		holdEnteredAt:     time.Now().Add(-5 * time.Second),
		holdPlayer:        &holdPlayer{},
	}
	h := &holdController{ac: ac, in: in}

	if !h.shouldLeaveOnPhoneUnhold(in.Media()) {
		t.Fatal("REQ-HOLD-5: legHeld + sendrecv must trigger leave even when MOH bytes are counted")
	}
}

// REQ-HOLD-5: recvonly→sendrecv before dial tone completes must still allow leave.
func TestREQ_HOLD_PhoneUnholdSendrecvTriggersLeave(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	in := testServerDialog("in")
	in.InitMediaSession(ms, nil, nil)

	ac := &ActiveCall{
		CallerExt:         "111",
		CalleeExt:         "110",
		HoldActive:        true,
		HolderExt:         "111",
		holderPhoneHold:   true,
		holderSawRecvonly: true,
		holderPrevDir:     sdp.ModeRecvonly,
	}
	h := &holdController{ac: ac, in: in}

	if !h.shouldLeaveOnPhoneUnhold(in.Media()) {
		t.Fatal("REQ-HOLD-5: recvonly→sendrecv phone unhold must allow leave")
	}
}

// REQ-HOLD-4/5: onInboundMediaUpdate leave triggering (REQ-HOLD-4 dial-tone guard, REQ-HOLD-5 unhold).
func TestREQ_HOLD_MediaUpdateLeaveSequence(t *testing.T) {
	t.Run("dial_tone_sendonly_no_leave", func(t *testing.T) {
		var leaves atomic.Int32
		HoldLeaveHook = func() { leaves.Add(1) }
		t.Cleanup(func() { HoldLeaveHook = nil })

		ms := mediaSessionWithLocalDirection(t, sdp.ModeSendonly)
		in := testServerDialog("in")
		in.InitMediaSession(ms, nil, nil)

		ac := &ActiveCall{
			CallerExt: "111", CalleeExt: "110",
			HoldActive: true, HolderExt: "111",
			holderPhoneHold: true, dialTonePending: true,
			holdEnteredAt: time.Now(),
		}
		h := &holdController{ac: ac, in: in}
		h.onInboundMediaUpdate(in.Media())
		time.Sleep(50 * time.Millisecond)
		if leaves.Load() != 0 {
			t.Fatal("REQ-HOLD-4: dial-tone sendonly must not call leave()")
		}
	})

	t.Run("dial_tone_sendonly_after_enter_no_leave", func(t *testing.T) {
		var leaves atomic.Int32
		HoldLeaveHook = func() { leaves.Add(1) }
		t.Cleanup(func() { HoldLeaveHook = nil })

		ms := mediaSessionWithLocalDirection(t, sdp.ModeSendonly)
		in := testServerDialog("in")
		in.InitMediaSession(ms, nil, nil)

		ac := &ActiveCall{
			CallerExt: "111", CalleeExt: "110",
			HoldActive: true, HolderExt: "111",
			holderPhoneHold: true, holderLegHeld: true,
			dialTonePending: false,
			holdEnteredAt:   time.Now().Add(-3 * time.Second),
		}
		h := &holdController{ac: ac, in: in}
		h.onInboundMediaUpdate(in.Media())
		time.Sleep(50 * time.Millisecond)
		if leaves.Load() != 0 {
			t.Fatal("REQ-HOLD-4: post-enter sendonly media update must not call leave()")
		}
	})

	t.Run("phone_unhold_triggers_leave", func(t *testing.T) {
		var leaves atomic.Int32
		HoldLeaveHook = func() { leaves.Add(1) }
		t.Cleanup(func() { HoldLeaveHook = nil })

		testEnableHolderSendMedia = func(h *holdController, ctx context.Context, holderIsCaller bool) bool {
			return false
		}
		t.Cleanup(func() { testEnableHolderSendMedia = nil })

		msIn := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
		msOut := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
		in := testServerDialog("in")
		out := &diago.DialogClientSession{}
		in.InitMediaSession(msIn, nil, nil)
		out.InitMediaSession(msOut, nil, nil)

		ac := &ActiveCall{
			CallerExt: "111", CalleeExt: "110",
			HoldActive: true, HolderExt: "111",
			holderPhoneHold: true, holderLegHeld: true,
			holderSawRecvonly: true,
			holderPrevDir:     sdp.ModeSendonly,
		}
		h := &holdController{
			ac: ac, in: in, out: out,
			ctx: t.Context(), b: &BridgePair{},
		}
		h.onInboundMediaUpdate(in.Media())
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) && leaves.Load() < 1 {
			time.Sleep(10 * time.Millisecond)
		}
		if leaves.Load() != 1 {
			t.Fatal("REQ-HOLD-5: phone unhold must call leave()")
		}
	})
}

func TestREQ_HOLD_PhoneReleaseDuringSlowEnterAborts(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	in := testServerDialog("in")
	in.InitMediaSession(ms, nil, nil)

	ac := &ActiveCall{
		CallerExt:         "111",
		CalleeExt:         "110",
		holderPhoneHold:   true,
		holderSawRecvonly: true,
		holdEntering:      true,
		holdEnterStartedAt: time.Now().Add(-3 * time.Second),
	}
	h := &holdController{ac: ac, in: in}

	if !h.phoneReleasedDuringEnter(in.Media()) {
		t.Fatal("sendrecv after slow hold entry must be treated as phone release")
	}
	h.abortHoldEntry("test")
	if !ac.holdEnterAborted {
		t.Fatal("abortHoldEntry must set holdEnterAborted")
	}
}

func TestREQ_HOLD_SendrecvChurnDuringEnterNotRelease(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	in := testServerDialog("in")
	in.InitMediaSession(ms, nil, nil)

	ac := &ActiveCall{
		CallerExt:          "111",
		CalleeExt:          "110",
		holderPhoneHold:    true,
		holderSawRecvonly:  true,
		holdEntering:       true,
		holdEnterStartedAt: time.Now().Add(-40 * time.Millisecond),
	}
	h := &holdController{ac: ac, in: in}

	if h.phoneReleasedDuringEnter(in.Media()) {
		t.Fatal("brief sendrecv churn during hold entry must not count as phone release")
	}
}
