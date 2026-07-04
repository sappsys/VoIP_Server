package call

import (
	"context"
	"io"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media/sdp"
	diagomedia "github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/media/tones"
)

const (
	holderMediaSettleDelay   = 300 * time.Millisecond
	holderMediaRetries       = 8
	heldPartyMOHReinviteMax  = 3
	holderMediaPollDelay     = 50 * time.Millisecond
	holderMediaPolls         = 20
)

// waitForMediaDirection polls until the session negotiates the wanted SDP direction.
func waitForMediaDirection(ctx context.Context, ms *diagomedia.MediaSession, want string, timeout time.Duration) bool {
	if ms == nil {
		return false
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if negotiatedMediaDirection(ms) == want {
			return true
		}
		select {
		case <-ctx.Done():
			return negotiatedMediaDirection(ms) == want
		case <-time.After(holderMediaPollDelay):
		}
	}
	return negotiatedMediaDirection(ms) == want
}

func pollLegCanSendMedia(ctx context.Context, ms *diagomedia.MediaSession) bool {
	for i := 0; i < holderMediaPolls; i++ {
		if legCanSendMedia(ms) {
			return true
		}
		select {
		case <-ctx.Done():
			return legCanSendMedia(ms)
		case <-time.After(holderMediaPollDelay):
		}
	}
	return legCanSendMedia(ms)
}

// pollLegCanSendMediaLive re-reads the media session each poll so it observes the
// post-re-INVITE session (Hold/Unhold replace the underlying MediaSession).
func pollLegCanSendMediaLive(ctx context.Context, live func() *diagomedia.MediaSession, phoneStillOnHold bool) bool {
	for i := 0; i < holderMediaPolls; i++ {
		if legCanSendDialTone(live(), phoneStillOnHold) {
			return true
		}
		select {
		case <-ctx.Done():
			return legCanSendDialTone(live(), phoneStillOnHold)
		case <-time.After(holderMediaPollDelay):
		}
	}
	return legCanSendDialTone(live(), phoneStillOnHold)
}

// legCanSendDialTone reports whether the server may write dial tone on this leg.
// While the phone is still on hold we require sendonly — sendrecv is spurious churn.
func legCanSendDialTone(ms *diagomedia.MediaSession, phoneStillOnHold bool) bool {
	if ms == nil {
		return false
	}
	mode := negotiatedMediaDirection(ms)
	if phoneStillOnHold {
		return mode == sdp.ModeSendonly
	}
	return legCanSendMedia(ms)
}

// legCanSendMedia reports whether the server may write RTP on this leg.
func legCanSendMedia(ms *diagomedia.MediaSession) bool {
	if ms == nil {
		return false
	}
	switch negotiatedMediaDirection(ms) {
	case sdp.ModeSendonly, sdp.ModeSendrecv:
		return true
	default:
		return false
	}
}

// testEnableHolderSendMedia overrides media negotiation in unit tests (REQ-HOLD-7).
var testEnableHolderSendMedia func(h *holdController, ctx context.Context, holderIsCaller bool) bool

// testEnableHeldPartyMOHMedia overrides held-party MOH media negotiation in unit tests.
var testEnableHeldPartyMOHMedia func(h *holdController, ctx context.Context, heldOnOut bool) bool

// testHolderHoldInvoked is set by unit tests to verify dial-tone re-INVITE runs on phone hold.
var testHolderHoldInvoked func(holderIsCaller bool)

// enableHolderSendMedia re-negotiates the holder leg so the server can send dial tone.
// After the phone's hold re-INVITE we are recvonly and WriteRTP is dropped until we
// complete our own re-INVITE. A short settle delay avoids 491 Request Pending.
func (h *holdController) enableHolderSendMedia(ctx context.Context, holderIsCaller bool) bool {
	if testEnableHolderSendMedia != nil {
		return testEnableHolderSendMedia(h, ctx, holderIsCaller)
	}
	// liveMS re-reads the media session each time: Hold/Unhold re-INVITEs fork and
	// replace the underlying MediaSession, so a cached pointer goes stale and never
	// reflects the newly negotiated direction.
	var liveMS func() *diagomedia.MediaSession
	hold := func(context.Context) error { return nil }

	if holderIsCaller {
		if h.in == nil {
			return false
		}
		liveMS = h.in.MediaSession
		hold = h.in.Hold
	} else {
		if h.out == nil {
			return false
		}
		liveMS = h.out.MediaSession
		hold = h.out.Hold
	}

	h.ac.holdMu.Lock()
	phoneHold := h.ac.holderPhoneHold
	h.ac.holdMu.Unlock()

	ms := liveMS()
	if ms != nil {
		mode := negotiatedMediaDirection(ms)
		if !phoneHold && mode == sdp.ModeSendonly {
			h.ac.holderLegHeld = true
			if h.log != nil {
				h.log.Info("holder media ready for dial tone", "mode", mode)
			}
			return true
		}
		if !phoneHold && mode == sdp.ModeSendrecv && !remoteHoldMode(ms) {
			if h.log != nil {
				h.log.Info("holder media already sendable", "mode", mode)
			}
			return true
		}
	}

	if h.holdEnterAborted() {
		return false
	}

	// Phone hold: we answered recvonly to the phone's sendonly re-INVITE. A server Hold()
	// re-INVITE races that transaction (491, multi-second retry) and still fails on some
	// real IP phone / unregistered-caller paths. Override local direction so dial tone can flow
	// on the existing leg once the phone hold answer completes.
	if phoneHold {
		ms = liveMS()
		if ms != nil && remoteHoldMode(ms) {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(phoneHoldDialToneSettle):
			}
			if h.holdEnterAborted() {
				return false
			}
			setMediaSessionNegotiatedMode(ms, sdp.ModeSendonly)
			h.ac.holderLegHeld = true
			if h.log != nil {
				h.log.Info("holder dial tone on phone-hold leg (no re-INVITE)",
					"mode", negotiatedMediaDirection(ms))
			}
			return true
		}
	}

	settle := holderMediaSettleDelay
	if phoneHold {
		settle = 100 * time.Millisecond
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(settle):
	}

	for attempt := 0; attempt < holderMediaRetries; attempt++ {
		if h.holdEnterAborted() {
			return false
		}
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
			}
		}
		if testHolderHoldInvoked != nil {
			testHolderHoldInvoked(holderIsCaller)
		}
		if err := hold(ctx); err != nil {
			if h.log != nil {
				h.log.Warn("holder sendonly reinvite failed", "attempt", attempt+1, "error", err)
			}
			continue
		}
		h.ac.holdMu.Lock()
		phoneHold = h.ac.holderPhoneHold
		h.ac.holdMu.Unlock()
		if pollLegCanSendMediaLive(ctx, liveMS, phoneHold) {
			h.ac.holderLegHeld = true
			if h.log != nil {
				h.log.Info("holder media ready for dial tone", "mode", negotiatedMediaDirection(liveMS()))
			}
			return true
		}
	}
	return false
}

// enableHeldPartyMOHMedia prepares the held-party leg for MOH RTP injection.
// Most phones stay on sendrecv when the remote party holds — inject MOH there without
// a sendonly re-INVITE (registered callee rejects that and leaves the leg unsendable).
// Re-INVITE only when the leg is recvonly/inactive and cannot send.
func (h *holdController) enableHeldPartyMOHMedia(ctx context.Context, heldOnOut bool) bool {
	if testEnableHeldPartyMOHMedia != nil {
		return testEnableHeldPartyMOHMedia(h, ctx, heldOnOut)
	}
	var liveMS func() *diagomedia.MediaSession
	sendonly := func(context.Context) error { return nil }
	unhold := func(context.Context) error { return nil }

	if heldOnOut {
		if h.out == nil {
			return false
		}
		liveMS = h.out.MediaSession
		sendonly = h.out.Hold
		unhold = h.out.Unhold
	} else {
		if h.in == nil {
			return false
		}
		liveMS = h.in.MediaSession
		sendonly = h.in.Hold
		unhold = h.in.Unhold
	}

	ms := liveMS()
	if ms != nil && legCanSendMedia(ms) {
		if h.log != nil {
			h.log.Info("held party media ready for MOH", "mode", negotiatedMediaDirection(ms))
		}
		return true
	}

	if ms != nil && h.log != nil {
		h.log.Info("held party leg not sendable, re-INVITE for MOH",
			"mode", negotiatedMediaDirection(ms),
			"held_on_out", heldOnOut)
	}

	if h.holdEnterAborted() {
		return false
	}

	select {
	case <-ctx.Done():
		return false
	case <-time.After(holderMediaSettleDelay):
	}

	reinviteFailed := false
	for attempt := 0; attempt < heldPartyMOHReinviteMax; attempt++ {
		if h.holdEnterAborted() {
			return legCanSendMedia(liveMS())
		}
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return legCanSendMedia(liveMS())
			case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
			}
		}
		if err := sendonly(ctx); err != nil {
			reinviteFailed = true
			if h.log != nil {
				h.log.Warn("held party sendonly reinvite failed", "attempt", attempt+1, "error", err)
			}
			continue
		}
		if pollLegCanSendMediaLive(ctx, liveMS, false) {
			if h.log != nil {
				h.log.Info("held party media ready for MOH", "mode", negotiatedMediaDirection(liveMS()))
			}
			if h.ac != nil {
				h.ac.holdMu.Lock()
				h.ac.heldPartyMOHSendonly = negotiatedMediaDirection(liveMS()) == sdp.ModeSendonly
				h.ac.holdMu.Unlock()
			}
			return true
		}
		reinviteFailed = true
	}

	if reinviteFailed {
		restoreCtx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
		_ = unhold(restoreCtx)
		cancel()
		if h.ac != nil {
			h.ac.holdMu.Lock()
			h.ac.heldPartyMOHSendonly = false
			h.ac.holdMu.Unlock()
		}
	}
	ok := legCanSendMedia(liveMS())
	if ok && h.log != nil {
		h.log.Info("held party MOH using existing send path after re-INVITE fallback",
			"mode", negotiatedMediaDirection(liveMS()))
	}
	return ok
}

func (h *holdController) startHolderDialTone(
	holdCtx context.Context,
	player *holdPlayer,
	holderIsCaller bool,
	tone tones.Tone,
) bool {
	if player == nil {
		return false
	}
	var callDone context.Context
	var create func() (diago.AudioPlaybackControl, error)
	if holderIsCaller {
		if h.in == nil {
			return false
		}
		callDone = dialogContext(h.in)
		create = mohPlaybackCreate(h.in)
	} else {
		if h.out == nil {
			return false
		}
		callDone = dialogContext(h.out)
		create = mohPlaybackCreate(h.out)
	}
	if create == nil {
		return false
	}
	var sess diago.DialogSession
	if holderIsCaller {
		sess = h.in
	} else {
		sess = h.out
	}
	ok := player.startToneLoop(holdCtx, callDone, tone, h.log, sess, create)
	if ok && h.log != nil {
		h.log.Info("dial tone started for holder")
	}
	return ok
}

func dialogContext(d diago.DialogSession) context.Context {
	if d == nil {
		return context.Background()
	}
	switch s := d.(type) {
	case *diago.DialogServerSession:
		if s.DialogServerSession == nil {
			return context.Background()
		}
	case *diago.DialogClientSession:
		if s.DialogClientSession == nil {
			return context.Background()
		}
	}
	if ctx := d.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func mohPlaybackCreate(d diago.DialogSession) func() (diago.AudioPlaybackControl, error) {
	if d == nil {
		return nil
	}
	switch s := d.(type) {
	case *diago.DialogServerSession:
		if s.DialogServerSession == nil {
			return testPlaybackControlCreate
		}
		return s.PlaybackControlCreate
	case *diago.DialogClientSession:
		if s.DialogClientSession == nil {
			return testPlaybackControlCreate
		}
		return s.PlaybackControlCreate
	default:
		return nil
	}
}

// testPlaybackControlCreate is used when dialog legs lack SIP state (unit tests).
func testPlaybackControlCreate() (diago.AudioPlaybackControl, error) {
	return diago.NewAudioPlaybackControl(
		diago.NewAudioPlayback(io.Discard, diagomedia.CodecAudioUlaw),
	), nil
}
