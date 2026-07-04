package call

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media/sdp"
	diagomedia "github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/media/audiobridge"
	"github.com/sappsys/VoIP_Server/internal/media/tones"
)

const (
	holdSilentReleaseAfter      = 3 * time.Second
	holdMOHAudibleBytes         = int64(320)
	reassertSettleDelay         = 400 * time.Millisecond
	holdEnterReleaseMinDuration = 2 * time.Second // sendrecv during enter after this → phone release, not SDP churn
	holdEnterInboundRTPWait     = 500 * time.Millisecond
	phoneHoldDialToneSettle     = 500 * time.Millisecond // let the phone hold 200 OK finish before RTP
)

func isRemoteHold(dm *diago.DialogMedia) bool {
	if dm == nil {
		return false
	}
	return remoteHoldMode(dm.MediaSession())
}

// negotiatedMediaDirection returns the active SDP direction after the last offer/answer.
// ms.Mode is only our preference; phones signal hold with a=sendonly which negotiates to recvonly.
func negotiatedMediaDirection(ms *diagomedia.MediaSession) string {
	if ms == nil {
		return ""
	}
	// After RemoteSDP/re-INVITE diago stores the negotiated direction in an unexported
	// field while InitWithSDP may still cache the original offer in LocalSDP().
	if mode := mediaSessionNegotiatedMode(ms); mode != "" {
		return mode
	}
	local := ms.LocalSDP()
	if len(local) == 0 {
		return ms.Mode
	}
	sd := make(sdp.SessionDescription)
	if err := sdp.Unmarshal(local, &sd); err != nil {
		return ms.Mode
	}
	return sd.MediaDirection()
}

func mediaSessionNegotiatedMode(ms *diagomedia.MediaSession) string {
	if ms == nil {
		return ""
	}
	f := reflect.ValueOf(ms).Elem().FieldByName("mode")
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}

// setMediaSessionNegotiatedMode overrides the post-answer direction used for RTP I/O.
// Phone hold answers recvonly (remote sendonly) but the server must still write dial tone
// without a second re-INVITE that races the phone's hold transaction (491 / multi-second delay).
func setMediaSessionNegotiatedMode(ms *diagomedia.MediaSession, mode string) {
	if ms == nil || mode == "" {
		return
	}
	ms.SetNegotiatedMode(mode)
}

func remoteHoldMode(ms *diagomedia.MediaSession) bool {
	if ms == nil {
		return false
	}
	switch negotiatedMediaDirection(ms) {
	case sdp.ModeRecvonly, sdp.ModeInactive:
		return true
	}
	// Do not treat sendonly as remote hold — that is our dial-tone re-INVITE to the holder.
	// Some endpoints keep sendrecv but set c=0.0.0.0 while holding.
	if ms.Raddr.IP != nil && ms.Raddr.IP.IsUnspecified() {
		if negotiatedMediaDirection(ms) == sdp.ModeSendrecv {
			return true
		}
	}
	return false
}

func isRemoteUnhold(dm *diago.DialogMedia) bool {
	if dm == nil {
		return false
	}
	ms := dm.MediaSession()
	return ms != nil && negotiatedMediaDirection(ms) == sdp.ModeSendrecv
}

// holderPhoneReleased is true when the holder's phone took the call off hold.
func holderPhoneReleased(ms *diagomedia.MediaSession, phoneSignalledHold bool) bool {
	if !phoneSignalledHold || ms == nil {
		return false
	}
	return negotiatedMediaDirection(ms) == sdp.ModeSendrecv
}

// holderSendrecvIsUnhold reports phone release on a hold that never delivered MOH
// (user pressed release, or media failed to start). Churn while MOH is audible uses reassert+poll.
func (h *holdController) holderSendrecvIsUnhold(legHeld, phoneHold, sawRecvonly bool) bool {
	if !phoneHold || !sawRecvonly || !legHeld {
		return false
	}
	h.ac.holdMu.Lock()
	mohBytes := int64(0)
	dialBytes := int64(0)
	if h.ac.holdPlayer != nil {
		mohBytes = h.ac.holdPlayer.MOHBytesSent()
		dialBytes = h.ac.holdPlayer.DialToneBytesSent()
	}
	enteredAt := h.ac.holdEnteredAt
	h.ac.holdMu.Unlock()
	if mohBytes >= holdMOHAudibleBytes && dialBytes >= holdMOHAudibleBytes {
		return false
	}
	return !enteredAt.IsZero() && time.Since(enteredAt) > holdSilentReleaseAfter
}

// shouldLeaveOnPhoneUnhold reports whether a holder-leg media update means the phone
// resumed the call. Server sendonly (dial tone) must NOT trigger leave — real phones
// fire extra re-INVITE/media updates that previously tore hold down immediately.
// sendrecv while dial tone is active is handled by reassertHolderHold first.
func (h *holdController) shouldLeaveOnPhoneUnhold(dm *diago.DialogMedia) bool {
	if dm == nil || h.ac == nil {
		return false
	}
	ms := dm.MediaSession()
	if ms == nil {
		return false
	}
	h.ac.holdMu.Lock()
	pending := h.ac.dialTonePending
	legHeld := h.ac.holderLegHeld
	phoneHold := h.ac.holderPhoneHold
	sawRecvonly := h.ac.holderSawRecvonly
	prevDir := h.ac.holderPrevDir
	h.ac.holdMu.Unlock()
	if pending {
		return false
	}
	mode := negotiatedMediaDirection(ms)
	switch mode {
	case sdp.ModeSendrecv:
		if !phoneHold || !sawRecvonly {
			return false
		}
		// Phone hold toggle off sends sendrecv. After our dial-tone sendonly leg is up
		// (legHeld), treat sendrecv as release — not churn to reassert (phone hold toggle).
		if legHeld {
			return true
		}
		h.ac.holdMu.Lock()
		entering := h.ac.holdEntering
		h.ac.holdMu.Unlock()
		switch prevDir {
		case sdp.ModeRecvonly, sdp.ModeInactive:
			if entering {
				return false
			}
			return true
		default:
			return false
		}
	case sdp.ModeSendonly:
		return false
	default:
		return false
	}
}

// shouldReassertHolderHold reports spurious sendrecv while the phone UI is still on hold.
// Re-apply sendonly dial tone instead of tearing hold down (real-phone SDP churn).
func (h *holdController) shouldReassertHolderHold(dm *diago.DialogMedia) bool {
	if dm == nil || h.ac == nil {
		return false
	}
	ms := dm.MediaSession()
	if ms == nil {
		return false
	}
	h.ac.holdMu.Lock()
	pending := h.ac.dialTonePending
	reasserting := h.ac.holderReasserting
	phoneHold := h.ac.holderPhoneHold
	legHeld := h.ac.holderLegHeld
	sawRecvonly := h.ac.holderSawRecvonly
	h.ac.holdMu.Unlock()
	if pending || reasserting || !phoneHold {
		return false
	}
	if negotiatedMediaDirection(ms) != sdp.ModeSendrecv {
		return false
	}
	if !legHeld && !sawRecvonly {
		return false
	}
	// After dial-tone leg is up, sendrecv is phone release — leave() handles it.
	if legHeld {
		return false
	}
	return true
}

func (h *holdController) holderMediaSession(holderIsCaller bool) *diagomedia.MediaSession {
	if holderIsCaller {
		if h.in != nil {
			return h.in.MediaSession()
		}
		return nil
	}
	if h.out != nil {
		return h.out.MediaSession()
	}
	return nil
}

// reassertHolderHold tries to put the holder leg back on sendonly after spurious sendrecv.
// If the phone refuses (true unhold), leave() is driven.
func (h *holdController) reassertHolderHold(holderIsCaller bool) {
	if h.ac == nil {
		return
	}
	h.ac.holdMu.Lock()
	if h.ac.holderReasserting || !h.ac.HoldActive {
		h.ac.holdMu.Unlock()
		return
	}
	h.ac.holderReasserting = true
	h.ac.holdMu.Unlock()
	defer func() {
		h.ac.holdMu.Lock()
		h.ac.holderReasserting = false
		h.ac.holdMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(h.ctx, 8*time.Second)
	defer cancel()

	if h.enableHolderSendMedia(ctx, holderIsCaller) {
		if ms := h.holderMediaSession(holderIsCaller); ms != nil &&
			negotiatedMediaDirection(ms) == sdp.ModeSendonly {
			if h.log != nil {
				h.log.Info("holder sendonly reasserted after sendrecv churn")
			}
			return
		}
	}

	ms := h.holderMediaSession(holderIsCaller)
	if ms != nil && negotiatedMediaDirection(ms) == sdp.ModeSendrecv {
		if h.log != nil {
			h.log.Info("holder kept sendrecv after reassert; treating as phone unhold")
		}
		go h.leave()
	}
}

// holderReleasedForLeave detects phone unhold for leave(). Ignores media updates while
// our dial-tone sendonly re-INVITE is in flight (REQ-HOLD-4).
func (h *holdController) holderReleasedForLeave(dm *diago.DialogMedia, phoneHold bool) bool {
	_ = phoneHold
	return h.shouldLeaveOnPhoneUnhold(dm)
}

func (h *holdController) holderStillOnHold(holderIsCaller bool) bool {
	if holderIsCaller {
		return h.in != nil && remoteHoldMode(h.in.MediaSession())
	}
	return h.out != nil && remoteHoldMode(h.out.MediaSession())
}

type holdController struct {
	b       *BridgePair
	ctx     context.Context
	ac      *ActiveCall
	mohDir  string
	in      *diago.DialogServerSession
	out     *diago.DialogClientSession
	log     *slog.Logger
	toneSet tones.Profile
}

func (h *holdController) profile() tones.Profile {
	if h != nil && h.toneSet.Region != "" {
		return h.toneSet
	}
	return tones.DefaultProfile()
}

func newHoldController(b *BridgePair, ctx context.Context, ac *ActiveCall, mohDir string, in *diago.DialogServerSession, out *diago.DialogClientSession) *holdController {
	log := b.Log
	toneSet := b.TonesProfile()
	return &holdController{b: b, ctx: ctx, ac: ac, mohDir: mohDir, in: in, out: out, log: log, toneSet: toneSet}
}

func (h *holdController) noteHolderHoldDirection(holderIsCaller bool, dm *diago.DialogMedia) {
	if h == nil || h.ac == nil || dm == nil {
		return
	}
	ms := dm.MediaSession()
	if ms == nil {
		return
	}
	dir := negotiatedMediaDirection(ms)
	h.ac.holdMu.Lock()
	if dir != "" {
		h.ac.holderPrevDir = dir
	}
	if remoteHoldMode(ms) {
		h.ac.holderSawRecvonly = true
		h.ac.holderPhoneHold = true
	}
	h.ac.holdMu.Unlock()
	_ = holderIsCaller
}

// seedHolderHoldDirection records the holder leg direction when hold starts.
func (h *holdController) seedHolderHoldDirection(holderIsCaller bool) {
	if h == nil || h.ac == nil {
		return
	}
	var ms *diagomedia.MediaSession
	if holderIsCaller {
		if h.in != nil {
			ms = h.in.MediaSession()
		}
	} else if h.out != nil {
		ms = h.out.MediaSession()
	}
	if ms == nil {
		return
	}
	dir := negotiatedMediaDirection(ms)
	h.ac.holdMu.Lock()
	if dir != "" {
		h.ac.holderPrevDir = dir
	}
	if remoteHoldMode(ms) {
		h.ac.holderSawRecvonly = true
	}
	h.ac.holdMu.Unlock()
}

// refreshHeldPartyMOH re-opens the MOH RTP send path after held-party SDP churn.
func (h *holdController) refreshHeldPartyMOH(heldOnOut bool) {
	if h.ac == nil {
		return
	}
	h.ac.holdMu.Lock()
	if !h.ac.HoldActive {
		h.ac.holdMu.Unlock()
		return
	}
	h.ac.holdMu.Unlock()

	ctx, cancel := context.WithTimeout(h.ctx, 8*time.Second)
	defer cancel()
	if !h.enableHeldPartyMOHMedia(ctx, heldOnOut) {
		if h.log != nil {
			h.log.Warn("held party MOH send path refresh failed")
		}
		return
	}
	if heldOnOut {
		startSessionMediaRTP(h.out)
		resetDialogLegRTP(h.out)
	} else {
		startSessionMediaRTP(h.in)
		resetDialogLegRTP(h.in)
	}
	if h.log != nil {
		h.log.Info("held party MOH send path refreshed")
	}
}

func (h *holdController) heldPartySession(heldOnOut bool) *diagomedia.MediaSession {
	if heldOnOut {
		if h.out != nil {
			return h.out.MediaSession()
		}
		return nil
	}
	if h.in != nil {
		return h.in.MediaSession()
	}
	return nil
}

func (h *holdController) startHeldPartyMOH(player *holdPlayer, heldOnOut bool) bool {
	if player == nil || h.ac == nil {
		return false
	}
	var sess diago.DialogSession
	var callDone context.Context
	var create func() (diago.AudioPlaybackControl, error)
	if heldOnOut {
		if h.out == nil {
			return false
		}
		sess = h.out
		callDone = dialogContext(h.out)
		create = mohPlaybackCreate(h.out)
	} else {
		if h.in == nil {
			return false
		}
		sess = h.in
		callDone = dialogContext(h.in)
		create = mohPlaybackCreate(h.in)
	}
	if h.ac.holdMediaRoot == nil {
		return false
	}
	h.ac.holdMu.Lock()
	if h.ac.mohCancel != nil {
		h.ac.mohCancel()
	}
	mohCtx, mohCancel := context.WithCancel(h.ac.holdMediaRoot)
	h.ac.mohCancel = mohCancel
	h.ac.heldPartyMS = h.heldPartySession(heldOnOut)
	h.ac.holdMu.Unlock()
	return player.startMOH(mohCtx, callDone, h.mohDir, h.profile(), h.log, sess, create)
}

// restartHeldPartyMOH stops and restarts MOH after the held-party media session changes.
func (h *holdController) restartHeldPartyMOH(heldOnOut bool) {
	if h.ac == nil {
		return
	}
	h.ac.holdMu.Lock()
	if !h.ac.HoldActive || h.ac.holdPlayer == nil {
		h.ac.holdMu.Unlock()
		return
	}
	player := h.ac.holdPlayer
	h.ac.holdMu.Unlock()

	ctx, cancel := context.WithTimeout(h.ctx, 8*time.Second)
	defer cancel()
	if !h.enableHeldPartyMOHMedia(ctx, heldOnOut) {
		if h.log != nil {
			h.log.Warn("held party MOH restart: send path unavailable")
		}
		return
	}
	if heldOnOut {
		startSessionMediaRTP(h.out)
		resetDialogLegRTP(h.out)
	} else {
		startSessionMediaRTP(h.in)
		resetDialogLegRTP(h.in)
	}
	if h.startHeldPartyMOH(player, heldOnOut) && h.log != nil {
		h.log.Info("held party MOH restarted after media update")
	}
}

func holdMediaSettleDelay(ctx context.Context) {
	if ctx == nil {
		time.Sleep(reassertSettleDelay)
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(reassertSettleDelay):
	}
}

func (h *holdController) restartHoldMedia(holderIsCaller bool) {
	h.ac.holdMu.Lock()
	active := h.ac.HoldActive
	holdCtx := h.ac.holdMediaRoot
	player := h.ac.holdPlayer
	h.ac.holdMu.Unlock()
	if !active || holdCtx == nil || player == nil {
		return
	}
	ctx, cancel := context.WithTimeout(h.ctx, 8*time.Second)
	defer cancel()
	if holderIsCaller {
		startSessionMediaRTP(h.in)
		startSessionMediaRTP(h.out)
		resetDialogLegRTP(h.in)
		resetDialogLegRTP(h.out)
		if h.enableHolderSendMedia(ctx, true) {
			holdMediaSettleDelay(ctx)
			h.startHolderDialTone(holdCtx, player, true, h.profile().Dial)
		}
		h.restartHeldPartyMOH(true)
		return
	}
	startSessionMediaRTP(h.in)
	startSessionMediaRTP(h.out)
	resetDialogLegRTP(h.in)
	resetDialogLegRTP(h.out)
	if h.enableHolderSendMedia(ctx, false) {
		holdMediaSettleDelay(ctx)
		h.startHolderDialTone(holdCtx, player, false, h.profile().Dial)
	}
	h.restartHeldPartyMOH(false)
}

func (h *holdController) startHoldMediaWatchdog(holderIsCaller bool) {
	for _, delay := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second} {
		timer := time.NewTimer(delay)
		select {
		case <-h.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		h.ac.holdMu.Lock()
		active := h.ac.HoldActive
		player := h.ac.holdPlayer
		var mohBytes, dialBytes int64
		if player != nil {
			mohBytes = player.MOHBytesSent()
			dialBytes = player.DialToneBytesSent()
		}
		h.ac.holdMu.Unlock()
		if !active || (mohBytes >= holdMOHAudibleBytes && dialBytes >= holdMOHAudibleBytes) {
			return
		}
		if h.log != nil {
			h.log.Info("hold media watchdog: restarting dial tone and MOH",
				"moh_bytes", mohBytes, "dial_tone_bytes", dialBytes)
		}
		h.restartHoldMedia(holderIsCaller)
	}
}

func (h *holdController) holdEnterAborted() bool {
	if h == nil || h.ac == nil {
		return false
	}
	h.ac.holdMu.Lock()
	defer h.ac.holdMu.Unlock()
	return h.ac.holdEnterAborted
}

// phoneReleasedDuringEnter is true when the holder sends sendrecv while enter() is still
// running long enough that this is a deliberate unhold, not the brief SDP churn phones
// emit right after hold (see holdWithSendrecvChurn in integration tests).
func (h *holdController) phoneReleasedDuringEnter(dm *diago.DialogMedia) bool {
	if h == nil || h.ac == nil || dm == nil {
		return false
	}
	ms := dm.MediaSession()
	if ms == nil || negotiatedMediaDirection(ms) != sdp.ModeSendrecv {
		return false
	}
	h.ac.holdMu.Lock()
	defer h.ac.holdMu.Unlock()
	if !h.ac.holdEntering || h.ac.HoldActive || !h.ac.holderPhoneHold || !h.ac.holderSawRecvonly {
		return false
	}
	if h.ac.holdEnterStartedAt.IsZero() {
		return false
	}
	return time.Since(h.ac.holdEnterStartedAt) >= holdEnterReleaseMinDuration
}

func (h *holdController) abortHoldEntry(reason string) {
	if h == nil || h.ac == nil {
		return
	}
	h.ac.holdMu.Lock()
	if !h.ac.holdEntering || h.ac.HoldActive {
		h.ac.holdMu.Unlock()
		return
	}
	h.ac.holdEnterAborted = true
	cancel := h.ac.holdCancel
	h.ac.holdMu.Unlock()
	if cancel != nil {
		cancel() // stop MOH/dial tone immediately; do not wait for enter() to finish
	}
	if h.log != nil {
		h.log.Info("hold entry aborted", "reason", reason, "caller", h.ac.CallerExt, "callee", h.ac.CalleeExt)
	}
}

// ensureBridgeAfterHoldAbort retries bridge restoration after a cancelled hold entry left
// MOH torn down but legs or codecs still settling (real IP phone / unregistered caller path).
func (h *holdController) ensureBridgeAfterHoldAbort() {
	if h == nil || h.ac == nil || h.b == nil {
		return
	}
	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
	defer cancel()

	h.unholdServerLegIfNeeded(ctx, h.in)
	h.unholdClientLegIfNeeded(ctx, h.out)
	resetServerLeg(h.in)
	resetClientLeg(h.out)
	resetLegRTP(h.in, h.out)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		h.ac.holdMu.Lock()
		busy := h.ac.HoldActive || h.ac.holdEntering
		hasBridge := h.ac.bridgeStop != nil
		h.ac.holdMu.Unlock()
		if busy || hasBridge {
			return
		}
		if !h.legsReadyForBridge() {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		h.ac.holdMu.Lock()
		if h.ac.bridgeStop == nil && !h.ac.HoldActive && !h.ac.holdEntering {
			h.recoverBridgeLocked()
			if h.log != nil && h.ac.bridgeStop != nil {
				h.log.Info("bridge restored after hold entry abort",
					"caller", h.ac.CallerExt, "callee", h.ac.CalleeExt)
			}
		}
		h.ac.holdMu.Unlock()
		return
	}
	if h.log != nil {
		h.log.Warn("bridge restore after hold entry abort timed out",
			"caller_dir", legDirection(h.in),
			"callee_dir", legDirectionOut(h.out))
	}
}

func (h *holdController) logMediaUpdate(leg string, dm *diago.DialogMedia, holdActive bool, holderExt string) {
	if h == nil || h.ac == nil || h.log == nil || dm == nil || dm.MediaSession() == nil {
		return
	}
	h.ac.holdMu.Lock()
	phoneHold := h.ac.holderPhoneHold
	legHeld := h.ac.holderLegHeld
	sawRecvonly := h.ac.holderSawRecvonly
	entering := h.ac.holdEntering
	pending := h.ac.dialTonePending
	h.ac.holdMu.Unlock()
	h.log.Debug("hold media update",
		"leg", leg,
		"dir", negotiatedMediaDirection(dm.MediaSession()),
		"remote_hold", remoteHoldMode(dm.MediaSession()),
		"hold_active", holdActive,
		"holder", holderExt,
		"phone_hold", phoneHold,
		"holder_leg_held", legHeld,
		"saw_recvonly", sawRecvonly,
		"entering", entering,
		"dial_tone_pending", pending)
}

func (h *holdController) onInboundMediaUpdate(dm *diago.DialogMedia) {
	if h == nil || h.ac == nil {
		return
	}
	holdActive, holderExt := h.ac.HoldSnapshot()
	h.logMediaUpdate("inbound", dm, holdActive, holderExt)
	if holdActive && holderExt == h.ac.CallerExt {
		if h.shouldLeaveOnPhoneUnhold(dm) {
			if h.log != nil {
				h.log.Info("phone unhold detected from holder media", "leg", "inbound", "holder", holderExt)
			}
			go h.leave()
			return
		}
		if h.shouldReassertHolderHold(dm) {
			if h.log != nil {
				h.log.Info("holder sendrecv treated as hold churn; reasserting", "leg", "inbound", "holder", holderExt)
			}
			go h.reassertHolderHold(true)
			return
		}
		h.noteHolderHoldDirection(true, dm)
	}
	if !holdActive {
		if h.phoneReleasedDuringEnter(dm) {
			h.abortHoldEntry("phone release during hold entry")
			return
		}
		if h.maybeRestartBridgeAfterUnhold() {
			go h.restartBridgeIfNeeded()
		}
		if dm != nil && dm.MediaSession() != nil &&
			negotiatedMediaDirection(dm.MediaSession()) == sdp.ModeSendrecv {
			h.ac.holdMu.Lock()
			entering := h.ac.holdEntering
			stuck := h.ac.holderPhoneHold && !entering
			if stuck {
				h.ac.holderPhoneHold = false
				h.ac.holderSawRecvonly = false
				h.ac.holderLegHeld = false
			}
			h.ac.holdMu.Unlock()
			if stuck && h.log != nil {
				h.log.Info("holder phone released after server hold teardown", "caller", h.ac.CallerExt)
			}
		}
		if isRemoteHold(dm) {
			if h.bridgeIsReady() {
				go h.enter(true)
			} else {
				h.recordPendingHold(true)
			}
			return
		}
	}
}

func (h *holdController) onOutboundMediaUpdate(dm *diago.DialogMedia) {
	if h == nil || h.ac == nil {
		return
	}
	holdActive, holderExt := h.ac.HoldSnapshot()
	h.logMediaUpdate("outbound", dm, holdActive, holderExt)
	if holdActive && holderExt == h.ac.CallerExt && h.out != nil {
		ms := h.out.MediaSession()
		h.ac.holdMu.Lock()
		prev := h.ac.heldPartyMS
		h.ac.holdMu.Unlock()
		if ms != nil && ms != prev {
			go h.restartHeldPartyMOH(true)
		} else if ms != nil && !legCanSendMedia(ms) {
			go h.refreshHeldPartyMOH(true)
		}
	}
	if holdActive && holderExt == h.ac.CalleeExt {
		if h.shouldLeaveOnPhoneUnhold(dm) {
			if h.log != nil {
				h.log.Info("phone unhold detected from holder media", "leg", "outbound", "holder", holderExt)
			}
			go h.leave()
			return
		}
		if h.shouldReassertHolderHold(dm) {
			if h.log != nil {
				h.log.Info("holder sendrecv treated as hold churn; reasserting", "leg", "outbound", "holder", holderExt)
			}
			go h.reassertHolderHold(false)
			return
		}
		h.noteHolderHoldDirection(false, dm)
	}
	if !holdActive {
		if h.phoneReleasedDuringEnter(dm) {
			h.abortHoldEntry("phone release during hold entry")
			return
		}
		if h.maybeRestartBridgeAfterUnhold() {
			go h.restartBridgeIfNeeded()
		}
		if isRemoteHold(dm) {
			if h.bridgeIsReady() {
				go h.enter(false)
			} else {
				h.recordPendingHold(false)
			}
			return
		}
	}
}

// bridgeIsReady reports whether the initial bridge has been established.
func (h *holdController) bridgeIsReady() bool {
	if h.ac == nil {
		return false
	}
	h.ac.holdMu.Lock()
	defer h.ac.holdMu.Unlock()
	return h.ac.bridgeReady
}

// bridgeNeedsRestart is true when the bridge was established but torn down without active hold.
func (h *holdController) bridgeNeedsRestart() bool {
	if h.ac == nil {
		return false
	}
	h.ac.holdMu.Lock()
	defer h.ac.holdMu.Unlock()
	return h.ac.bridgeReady && h.ac.bridgeStop == nil && !h.ac.HoldActive && !h.ac.holdEntering
}

// maybeRestartBridgeAfterUnhold reports whether a failed/aborted hold left the call bridged-less
// and both legs are back on sendrecv. Must not run while enter() is in progress.
func (h *holdController) maybeRestartBridgeAfterUnhold() bool {
	if h.ac == nil {
		return false
	}
	h.ac.holdMu.Lock()
	entering := h.ac.holdEntering
	needs := h.ac.bridgeReady && h.ac.bridgeStop == nil && !h.ac.HoldActive && !entering
	h.ac.holdMu.Unlock()
	if !needs {
		return false
	}
	return h.legsReadyForBridge()
}

func (h *holdController) restartBridgeIfNeeded() {
	if h.ac == nil || h.b == nil {
		return
	}
	h.ac.holdMu.Lock()
	if !h.ac.bridgeReady || h.ac.bridgeStop != nil || h.ac.HoldActive || h.ac.holdEntering {
		h.ac.holdMu.Unlock()
		return
	}
	if !h.legsReadyForBridge() {
		h.ac.holdMu.Unlock()
		return
	}
	h.ac.holdMu.Unlock()

	h.ac.holdMu.Lock()
	defer h.ac.holdMu.Unlock()
	if h.ac.bridgeStop != nil || h.ac.HoldActive || h.ac.holdEntering {
		return
	}
	if !h.legsReadyForBridge() {
		return
	}
	h.recoverBridgeLocked()
	if h.log != nil {
		h.log.Info("bridge restarted after failed hold teardown", "caller", h.ac.CallerExt, "callee", h.ac.CalleeExt)
	}
}

// recordPendingHold remembers a hold re-INVITE that arrived before the bridge
// was ready, so markBridgeReady can apply it once the call is bridged.
func (h *holdController) recordPendingHold(holderIsCaller bool) {
	if h.ac == nil {
		return
	}
	h.ac.holdMu.Lock()
	h.ac.pendingHold = true
	h.ac.pendingHoldCaller = holderIsCaller
	h.ac.holdMu.Unlock()
	if h.log != nil {
		h.log.Info("hold arrived before bridge ready; deferring", "holder_is_caller", holderIsCaller)
	}
}

// markBridgeReady marks the initial bridge established and applies any hold that
// arrived during setup. Called after startControllableBridge sets bridgeStop.
func (h *holdController) markBridgeReady() {
	if h.ac == nil {
		return
	}
	h.ac.holdMu.Lock()
	h.ac.bridgeReady = true
	pending := h.ac.pendingHold
	holderIsCaller := h.ac.pendingHoldCaller
	h.ac.pendingHold = false
	h.ac.holdMu.Unlock()
	if pending {
		go h.enter(holderIsCaller)
	}
}

func (h *holdController) setupHolderDialTone(
	holdMediaCtx context.Context,
	holdCtx context.Context,
	player *holdPlayer,
	holderIsCaller bool,
	holderExt string,
) bool {
	if h.holdEnterAborted() {
		return false
	}
	var holderSess *diagomedia.MediaSession
	if holderIsCaller {
		if h.in != nil {
			holderSess = h.in.MediaSession()
			waitForMediaDirection(holdMediaCtx, holderSess, sdp.ModeRecvonly, time.Second)
			if remoteHoldMode(holderSess) {
				h.ac.holdMu.Lock()
				h.ac.holderPhoneHold = true
				h.ac.holderSawRecvonly = true
				h.ac.holdMu.Unlock()
			}
		}
	} else if h.out != nil {
		holderSess = h.out.MediaSession()
		waitForMediaDirection(holdMediaCtx, holderSess, sdp.ModeRecvonly, time.Second)
		if remoteHoldMode(holderSess) {
			h.ac.holdMu.Lock()
			h.ac.holderPhoneHold = true
			h.ac.holderSawRecvonly = true
			h.ac.holdMu.Unlock()
		}
	}

	h.ac.holdMu.Lock()
	h.ac.dialTonePending = true
	h.ac.holdMu.Unlock()
	defer func() {
		h.ac.holdMu.Lock()
		h.ac.dialTonePending = false
		h.ac.holdMu.Unlock()
	}()

	if h.holdEnterAborted() {
		return false
	}
	if !h.enableHolderSendMedia(holdMediaCtx, holderIsCaller) {
		if h.log != nil {
			h.log.Warn("dial tone unavailable for holder", "ext", holderExt)
		}
		return false
	}
	if h.holdEnterAborted() {
		return false
	}
	var holderLeg diago.DialogSession
	if holderIsCaller {
		holderLeg = h.in
	} else {
		holderLeg = h.out
	}
	if holderLeg != nil && !waitForInboundRTP(holdMediaCtx, holderLeg, holdEnterInboundRTPWait) && h.log != nil {
		h.log.Warn("holder leg had no inbound RTP before dial tone", "ext", holderExt)
	}
	if h.holdEnterAborted() {
		return false
	}
	holdMediaSettleDelay(holdMediaCtx)
	return h.startHolderDialTone(holdCtx, player, holderIsCaller, h.profile().Dial)
}

func (h *holdController) setupHeldPartyMOH(
	holdMediaCtx context.Context,
	player *holdPlayer,
	holderIsCaller bool,
) bool {
	if h.holdEnterAborted() {
		return false
	}
	heldOnOut := holderIsCaller
	var heldExt string
	if heldOnOut {
		heldExt = h.ac.CalleeExt
		if h.out == nil {
			return false
		}
		resetClientLeg(h.out)
		resetDialogLegRTP(h.out)
	} else {
		heldExt = h.ac.CallerExt
		if h.in == nil {
			return false
		}
		resetServerLeg(h.in)
		resetDialogLegRTP(h.in)
	}
	if !h.enableHeldPartyMOHMedia(holdMediaCtx, heldOnOut) {
		if h.log != nil {
			h.log.Warn("held party MOH send path unavailable", "ext", heldExt)
		}
		return false
	}
	if h.holdEnterAborted() {
		return false
	}
	if heldOnOut {
		startSessionMediaRTP(h.out)
		resetDialogLegRTP(h.out)
		if !waitForInboundRTP(holdMediaCtx, h.out, holdEnterInboundRTPWait) && h.log != nil {
			h.log.Warn("held party leg had no inbound RTP before MOH", "ext", heldExt)
		}
	} else {
		startSessionMediaRTP(h.in)
		resetDialogLegRTP(h.in)
		if !waitForInboundRTP(holdMediaCtx, h.in, holdEnterInboundRTPWait) && h.log != nil {
			h.log.Warn("held party leg had no inbound RTP before MOH", "ext", heldExt)
		}
	}
	if h.holdEnterAborted() {
		return false
	}
	holdMediaSettleDelay(holdMediaCtx)
	mohOK := h.startHeldPartyMOH(player, heldOnOut)
	if mohOK && h.log != nil {
		leg := "caller"
		if heldOnOut {
			leg = "callee"
		}
		h.log.Info("moh started for held party", "leg", leg, "ext", heldExt)
	}
	return mohOK
}

func (h *holdController) enter(holderIsCaller bool) {
	h.ac.holdMu.Lock()
	if h.ac.HoldActive || h.ac.holdEntering {
		h.ac.holdMu.Unlock()
		return
	}
	if !h.holderStillOnHold(holderIsCaller) {
		h.ac.holdMu.Unlock()
		return
	}
	h.ac.holdEntering = true
	h.ac.holderPhoneHold = true
	h.ac.holdEnterStartedAt = time.Now()
	h.ac.holdEnterAborted = false
	bridgeStop := h.ac.bridgeStop
	h.ac.bridgeStop = nil
	h.ac.holdMu.Unlock()

	h.seedHolderHoldDirection(holderIsCaller)

	defer func() {
		h.ac.holdMu.Lock()
		h.ac.holdEntering = false
		h.ac.holdEnterStartedAt = time.Time{}
		h.ac.holdEnterAborted = false
		h.ac.holdMu.Unlock()
	}()

	h.saveBridgeCodecs()

	clearSessionMediaHooks(h.in)
	clearSessionMediaHooks(h.out)
	if bridgeStop != nil {
		_ = bridgeStop()
	}
	stopSessionMediaRTP(h.in)
	stopSessionMediaRTP(h.out)

	h.ac.holdMu.Lock()
	if h.ac.HoldActive {
		h.ac.holdMu.Unlock()
		return
	}
	h.ac.holdMu.Unlock()

	startSessionMediaRTP(h.in)
	startSessionMediaRTP(h.out)

	holdCtx, cancel := context.WithCancel(h.ctx)
	player := &holdPlayer{}

	h.ac.holdMu.Lock()
	h.ac.holdMediaRoot = holdCtx
	h.ac.holdCancel = cancel
	h.ac.holdMu.Unlock()

	holdMediaCtx, holdMediaCancel := context.WithTimeout(h.ctx, 30*time.Second)

	var holderExt string
	if holderIsCaller {
		holderExt = h.ac.CallerExt
	} else {
		holderExt = h.ac.CalleeExt
	}

	var mohOK, dialToneOK bool
	var mediaWG sync.WaitGroup
	mediaWG.Add(2)
	go func() {
		defer mediaWG.Done()
		dialToneOK = h.setupHolderDialTone(holdMediaCtx, holdCtx, player, holderIsCaller, holderExt)
	}()
	go func() {
		defer mediaWG.Done()
		mohOK = h.setupHeldPartyMOH(holdMediaCtx, player, holderIsCaller)
	}()
	mediaWG.Wait()
	holdMediaCancel()

	h.ac.holdMu.Lock()
	defer h.ac.holdMu.Unlock()
	if h.ac.holdEnterAborted {
		if h.log != nil {
			h.log.Info("hold entry cancelled before activation", "holder", holderExt)
		}
		player.stopAndWait()
		cancel()
		h.ac.holderPhoneHold = false
		h.ac.holderSawRecvonly = false
		h.ac.holderLegHeld = false
		h.ac.dialTonePending = false
		h.recoverBridgeLocked()
		go h.ensureBridgeAfterHoldAbort()
		return
	}
	if h.ac.HoldActive {
		player.stopAndWait()
		cancel()
		if bridgeStop != nil {
			h.recoverBridgeLocked()
		}
		return
	}
	if !dialToneOK && !mohOK {
		if h.log != nil {
			h.log.Warn("hold aborted: neither dial tone nor MOH started", "holder", holderExt)
		}
		player.stopAndWait()
		cancel()
		h.ac.holderPhoneHold = false
		h.ac.holderSawRecvonly = false
		h.ac.holderLegHeld = false
		h.recoverBridgeLocked()
		return
	}
	if !mohOK && h.log != nil {
		h.log.Warn("hold entered without MOH; watchdog will retry", "holder", holderExt)
	}
	if !dialToneOK && h.log != nil {
		h.log.Warn("hold entered without dial tone on holder", "holder", holderExt)
	}

	if HoldMediaStartedHook != nil {
		HoldMediaStartedHook(mohOK, dialToneOK)
	}

	h.ac.holdCancel = cancel
	h.ac.holdPlayer = player
	h.ac.HoldActive = true
	h.ac.HolderExt = holderExt
	h.ac.holdEnteredAt = time.Now()
	if h.ac.mohCancel == nil && mohOK {
		// startHeldPartyMOH sets mohCancel; ensure heldPartyMS is tracked
		h.ac.heldPartyMS = h.heldPartySession(holderExt == h.ac.CallerExt)
	}
	if h.log != nil {
		h.log.Info("call on hold", "holder", holderExt, "caller", h.ac.CallerExt, "callee", h.ac.CalleeExt)
	}
	go h.startHoldMediaWatchdog(holderIsCaller)
}

func (h *holdController) recoverBridgeLocked() {
	prepareLegsForBridge(h.in, h.out)
	stop, stats, err := h.b.startControllableBridge(h.in, h.out)
	if err != nil {
		if h.log != nil {
			h.log.Warn("bridge restart after failed hold failed", "error", err)
		}
		return
	}
	h.ac.bridgeStop = stop
	h.ac.relayStats = stats
}

func (h *holdController) saveBridgeCodecs() {
	if h == nil || h.ac == nil {
		return
	}
	if h.in != nil {
		if c, ok := audiobridge.NegotiatedCodec(h.in.MediaSession()); ok {
			h.ac.bridgeCodecA = c
		}
	}
	if h.out != nil {
		if c, ok := audiobridge.NegotiatedCodec(h.out.MediaSession()); ok {
			h.ac.bridgeCodecB = c
		}
	}
	h.ac.bridgeCodecsSaved = h.ac.bridgeCodecA.Name != "" && h.ac.bridgeCodecB.Name != ""
}

func (h *holdController) restoreBridgeCodecs() {
	if h == nil || h.ac == nil || !h.ac.bridgeCodecsSaved {
		return
	}
	if h.in != nil && h.in.MediaSession() != nil {
		h.in.MediaSession().RestorePrimaryCodec(h.ac.bridgeCodecA)
	}
	if h.out != nil && h.out.MediaSession() != nil {
		h.out.MediaSession().RestorePrimaryCodec(h.ac.bridgeCodecB)
	}
	if h.log != nil {
		h.log.Debug("restored pre-hold bridge codecs",
			"caller", h.ac.bridgeCodecA.Name,
			"callee", h.ac.bridgeCodecB.Name)
	}
}

func (h *holdController) stopBridge() {
	if h.ac == nil {
		return
	}
	// enter() already holds holdMu; use the locked helper to avoid self-deadlock.
	h.ac.stopBridgeLocked()
}

func (h *holdController) leave() {
	h.ac.holdMu.Lock()
	if !h.ac.HoldActive {
		h.ac.holdMu.Unlock()
		return
	}
	h.ac.HoldActive = false
	if HoldLeaveHook != nil {
		HoldLeaveHook()
	}

	if h.ac.holdCancel != nil {
		h.ac.holdCancel()
		h.ac.holdCancel = nil
	}
	if h.ac.holdPlayer != nil {
		h.ac.holdPlayer.stopAndWait()
		h.ac.holdPlayer = nil
	}

	// Stop any bridge started during a hold/enter race before we rebuild media.
	if h.ac.bridgeStop != nil {
		stop := h.ac.bridgeStop
		h.ac.bridgeStop = nil
		h.ac.relayStats = nil
		h.ac.holdMu.Unlock()
		_ = stop()
		h.ac.holdMu.Lock()
	}

	ctx := h.ctx
	holderLegHeld := h.ac.holderLegHeld
	holderExt := h.ac.HolderExt
	callerExt := h.ac.CallerExt
	calleeExt := h.ac.CalleeExt
	h.ac.holderLegHeld = false
	h.ac.holderPhoneHold = false
	h.ac.holderSawRecvonly = false
	h.ac.holderPrevDir = ""
	h.ac.holderReasserting = false
	if h.ac.mohCancel != nil {
		h.ac.mohCancel()
		h.ac.mohCancel = nil
	}
	h.ac.heldPartyMS = nil
	h.ac.holdMediaRoot = nil
	h.ac.HolderExt = ""
	h.ac.holdMu.Unlock()

	if holderLegHeld && holderExt != "" {
		unholdCtx, unholdCancel := context.WithTimeout(ctx, 8*time.Second)
		if holderExt == callerExt {
			h.unholdServerLegIfNeeded(unholdCtx, h.in)
		} else if holderExt == calleeExt {
			h.unholdClientLegIfNeeded(unholdCtx, h.out)
		}
		unholdCancel()
	}

	h.ac.holdMu.Lock()
	heldMOHSendonly := h.ac.heldPartyMOHSendonly
	h.ac.heldPartyMOHSendonly = false
	h.ac.holdMu.Unlock()
	if heldMOHSendonly {
		unholdCtx, unholdCancel := context.WithTimeout(ctx, 8*time.Second)
		if holderExt == callerExt {
			h.unholdClientLegIfNeeded(unholdCtx, h.out)
		} else if holderExt == calleeExt {
			h.unholdServerLegIfNeeded(unholdCtx, h.in)
		}
		unholdCancel()
	}

	deadline := time.Now().Add(5 * time.Second)
waitUnhold:
	for time.Now().Before(deadline) {
		if h.legsReadyForBridge() {
			break waitUnhold
		}
		select {
		case <-ctx.Done():
			break waitUnhold
		case <-time.After(50 * time.Millisecond):
		}
	}

	if !h.legsReadyForBridge() {
		if h.log != nil {
			h.log.Warn("unhold incomplete, waiting for sendrecv",
				"caller_dir", legDirection(h.in),
				"callee_dir", legDirectionOut(h.out))
		}
		for i := 0; i < 40 && !h.legsReadyForBridge(); i++ {
			select {
			case <-ctx.Done():
				break
			case <-time.After(50 * time.Millisecond):
			}
		}
	}

	if !h.legsReadyForBridge() {
		unholdCtx, unholdCancel := context.WithTimeout(ctx, 8*time.Second)
		if holderExt == callerExt {
			h.unholdServerLegIfNeeded(unholdCtx, h.in)
		} else if holderExt == calleeExt {
			h.unholdClientLegIfNeeded(unholdCtx, h.out)
		}
		unholdCancel()
		for i := 0; i < 40 && !h.legsReadyForBridge(); i++ {
			select {
			case <-ctx.Done():
				break
			case <-time.After(50 * time.Millisecond):
			}
		}
	}

	resetServerLeg(h.in)
	resetClientLeg(h.out)
	resetLegRTP(h.in, h.out)

	h.restoreBridgeCodecs()

	if !h.legsReadyForBridge() {
		if h.log != nil {
			h.log.Warn("unhold incomplete, skipping bridge restart",
				"caller_dir", legDirection(h.in),
				"callee_dir", legDirectionOut(h.out))
		}
		return
	}

	prepareLegsForBridge(h.in, h.out)

	var stop func() error
	var stats *audiobridge.RelayStats
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		stop, stats, err = h.b.startControllableBridge(h.in, h.out)
		if err == nil {
			break
		}
		if h.log != nil {
			h.log.Warn("bridge restart after unhold failed", "attempt", attempt+1, "error", err)
		}
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}
	if err != nil {
		if h.log != nil {
			h.log.Warn("bridge restart after unhold gave up", "error", err)
		}
		return
	}

	h.ac.holdMu.Lock()
	h.ac.bridgeStop = stop
	h.ac.relayStats = stats
	h.ac.holdMu.Unlock()
	if h.log != nil {
		h.log.Info("call off hold", "caller", callerExt, "callee", calleeExt)
	}
}

func legDirection(in *diago.DialogServerSession) string {
	if in == nil {
		return ""
	}
	return negotiatedMediaDirection(in.MediaSession())
}

func legDirectionOut(out *diago.DialogClientSession) string {
	if out == nil {
		return ""
	}
	return negotiatedMediaDirection(out.MediaSession())
}

func (h *holdController) legsReadyForBridge() bool {
	inOK := h.in == nil || negotiatedMediaDirection(h.in.MediaSession()) == sdp.ModeSendrecv
	outOK := h.out == nil || negotiatedMediaDirection(h.out.MediaSession()) == sdp.ModeSendrecv
	return inOK && outOK
}

func (h *holdController) unholdServerLegIfNeeded(ctx context.Context, in *diago.DialogServerSession) {
	if in == nil || negotiatedMediaDirection(in.MediaSession()) == sdp.ModeSendrecv {
		return
	}
	h.unholdServerLeg(ctx, in)
}

func (h *holdController) unholdClientLegIfNeeded(ctx context.Context, out *diago.DialogClientSession) {
	if out == nil || negotiatedMediaDirection(out.MediaSession()) == sdp.ModeSendrecv {
		return
	}
	h.unholdClientLeg(ctx, out)
}

func (h *holdController) unholdServerLeg(ctx context.Context, in *diago.DialogServerSession) {
	if in == nil {
		return
	}
	if err := in.Unhold(ctx); err != nil && h.log != nil {
		h.log.Warn("unhold caller leg failed", "error", err)
	}
}

func (h *holdController) unholdClientLeg(ctx context.Context, out *diago.DialogClientSession) {
	if out == nil {
		return
	}
	if err := out.Unhold(ctx); err != nil && h.log != nil {
		h.log.Warn("unhold callee leg failed", "error", err)
	}
}

func (b *BridgePair) startControllableBridge(in *diago.DialogServerSession, out *diago.DialogClientSession) (func() error, *audiobridge.RelayStats, error) {
	return b.startTranscodingBridge(in, out)
}

func (b *BridgePair) startControllableBridgeSessions(a, c diago.DialogSession) (func() error, *audiobridge.RelayStats, error) {
	return b.startTranscodingBridge(a, c)
}

// startTranscodingBridge is the only media path for two-party calls (extension,
// trunk, transfer, park, hunt, etc.).
func (b *BridgePair) startTranscodingBridge(a, c diago.DialogSession) (func() error, *audiobridge.RelayStats, error) {
	prepareLegsForBridge(a, c)

	if err := validateBridgeLegRTP(a); err != nil {
		return nil, nil, fmt.Errorf("bridge leg A: %w", err)
	}
	if err := validateBridgeLegRTP(c); err != nil {
		return nil, nil, fmt.Errorf("bridge leg B: %w", err)
	}

	msA := a.Media().MediaSession()
	msB := c.Media().MediaSession()
	legA, okA := audiobridge.NegotiatedCodec(msA)
	legB, okB := audiobridge.NegotiatedCodec(msB)
	if !okA || !okB {
		return nil, nil, fmt.Errorf("bridge: codec not negotiated")
	}
	if !audiobridge.CanTranscode(legA) || !audiobridge.CanTranscode(legB) {
		return nil, nil, fmt.Errorf("bridge: transcoding unsupported for %s and %s", legA.Name, legB.Name)
	}

	if b.Log != nil {
		b.Log.Debug("starting pcm bridge",
			"leg_a", legA.Name, "pt_a", legA.PayloadType,
			"leg_b", legB.Name, "pt_b", legB.PayloadType)
	}
	legAS, ok1 := sessionAsLeg(a)
	legBS, ok2 := sessionAsLeg(c)
	if !ok1 || !ok2 {
		return nil, nil, fmt.Errorf("pcm bridge: unsupported dialog type")
	}
	return audiobridge.StartTranscodingBridge(b.Log, legAS, legBS)
}

func validateBridgeLegRTP(d diago.DialogSession) error {
	if d == nil {
		return fmt.Errorf("nil dialog")
	}
	m := d.Media()
	if m == nil || m.MediaSession() == nil {
		return fmt.Errorf("media not negotiated")
	}
	switch s := d.(type) {
	case *diago.DialogServerSession:
		if s.RTPPacketReader == nil || s.RTPPacketWriter == nil {
			return fmt.Errorf("rtp not initialized")
		}
	case *diago.DialogClientSession:
		if s.RTPPacketReader == nil || s.RTPPacketWriter == nil {
			return fmt.Errorf("rtp not initialized")
		}
	default:
		return fmt.Errorf("unsupported dialog type")
	}
	return nil
}

func sessionAsLeg(d diago.DialogSession) (audiobridge.SessionLeg, bool) {
	switch s := d.(type) {
	case *diago.DialogServerSession:
		return s, true
	case *diago.DialogClientSession:
		return s, true
	default:
		return nil, false
	}
}

// HoldWatcher wires outbound re-INVITE hold detection before BridgePair.Join (hunt groups).
type HoldWatcher struct {
	Bridge *BridgePair
	Ctx    context.Context
	MOHDir string
	In     *diago.DialogServerSession
	Log    *slog.Logger

	hc  *holdController
	out *diago.DialogClientSession
}

func (w *HoldWatcher) OutboundMediaUpdate() func(*diago.DialogMedia) {
	if w.hc == nil {
		w.hc = &holdController{b: w.Bridge, ctx: w.Ctx, mohDir: w.MOHDir, in: w.In, log: w.Log, toneSet: w.Bridge.TonesProfile()}
	}
	return func(dm *diago.DialogMedia) {
		w.hc.onOutboundMediaUpdate(dm)
	}
}

func (w *HoldWatcher) BindOut(out *diago.DialogClientSession) {
	w.out = out
	if w.hc != nil {
		w.hc.out = out
	}
}

func (w *HoldWatcher) Controller() *holdController {
	if w.hc == nil {
		w.hc = &holdController{b: w.Bridge, ctx: w.Ctx, mohDir: w.MOHDir, in: w.In, log: w.Log, toneSet: w.Bridge.TonesProfile()}
	}
	return w.hc
}
