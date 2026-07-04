package call

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/media"
	"github.com/sappsys/VoIP_Server/internal/media/tones"
)

// responseStartsRingback reports SIP responses that should play regional ringback to the caller.
func responseStartsRingback(statusCode int) bool {
	return statusCode == sip.StatusRinging || statusCode == sip.StatusSessionInProgress
}

// BridgePair connects caller (in) to callee (out) with correct ring-then-answer order.
type BridgePair struct {
	MOHDir            string
	Tones             tones.Profile
	Log               *slog.Logger
	Registry          *Registry
	RecordCall        func(callerExt, calleeExt string)
	OnCallStateChange func(callerExt, calleeExt string, active bool)
}

func (b *BridgePair) TonesProfile() tones.Profile {
	if b != nil && b.Tones.Region != "" {
		return b.Tones
	}
	return tones.DefaultProfile()
}

// ConnectOpts configures a bridged call.
type ConnectOpts struct {
	Headers         []sip.Header
	Username        string
	Password        string
	VideoEnabled    bool
	ExternalIP      string
	CallerExt          string
	CalleeExt          string
	DialDestination    string // host:port for outbound INVITE (from registration)
	DialTransport      string // udp/tcp from registration contact
	DialTimeoutSeconds int // per-attempt outbound INVITE timeout (0 = 15s)
	RingTimeoutSeconds int // ringback duration before busy (0 = 30s)
}

// Connect dials the callee, answers the caller, optionally relays video, and
// bridges audio until either leg hangs up. Registers the call in Registry for
// transfer/park star codes.
func (b *BridgePair) Connect(ctx context.Context, dg Dialer, in *diago.DialogServerSession, outURI sip.Uri, opts ConnectOpts, mohDir string) error {
	if b.Log != nil {
		b.Log.Debug("bridge connect", "caller", opts.CallerExt, "callee", opts.CalleeExt, "uri", outURI.String())
	}
	in.Trying()

	headers := opts.Headers
	if headers == nil {
		headers = []sip.Header{}
	}

	callerOffer := in.InviteRequest.Body()
	wantVideo := opts.VideoEnabled && media.HasVideo(callerOffer)

	hc := &holdController{b: b, ctx: ctx, mohDir: mohDir, in: in, log: b.Log, toneSet: b.TonesProfile()}

	var stopRing context.CancelFunc
	dialCtx, dialCancel := context.WithTimeout(ctx, RingTimeout(opts.RingTimeoutSeconds))
	defer dialCancel()

	out, err := InviteOutLeg(dialCtx, dg, outURI, opts, diago.InviteOptions{
		Originator: in,
		Headers:    headers,
		Username:   opts.Username,
		Password:   opts.Password,
		OnResponse: func(res *sip.Response) error {
			if responseStartsRingback(res.StatusCode) {
				if res.StatusCode == sip.StatusRinging {
					if err := in.Ringing(); err != nil {
						return err
					}
				}
				if stopRing == nil {
					stopRing = StartRingback(ctx, in, b.TonesProfile(), b.Log)
				}
				return nil
			}
			if res.StatusCode >= 200 && res.StatusCode < 300 && stopRing != nil {
				stopRing()
				stopRing = nil
			}
			return nil
		},
	}, func(dm *diago.DialogMedia) {
		hc.onOutboundMediaUpdate(dm)
	})
	if err != nil {
		if stopRing != nil {
			stopRing()
		}
		if b.Log != nil {
			b.Log.Warn("outbound invite failed", "uri", outURI.String(), "dest", opts.DialDestination, "error", err)
		}
		PlayBusyThenHangup(ctx, in, b.TonesProfile(), b.Log)
		return err
	}

	ac := &ActiveCall{
		CallerExt: opts.CallerExt,
		CalleeExt: opts.CalleeExt,
		In:        in,
		Out:       out,
	}
	hc.ac = ac
	hc.out = out
	defer func() {
		if !ac.Parked {
			out.Close()
		}
	}()

	if b.OnCallStateChange != nil && opts.CallerExt != "" && opts.CalleeExt != "" {
		defer func() { b.OnCallStateChange(opts.CallerExt, opts.CalleeExt, false) }()
	}

	if stopRing != nil {
		stopRing()
		stopRing = nil
	}

	if err := AnswerSessionOptions(in, diago.AnswerOptions{
		OnRefer: b.makeTransferHandler(ctx, in, out, ac),
		OnMediaUpdate: func(dm *diago.DialogMedia) {
			hc.onInboundMediaUpdate(dm)
		},
	}); err != nil {
		out.Hangup(ctx)
		return err
	}

	if b.Registry != nil {
		b.Registry.Register(ac)
		defer b.Registry.Unregister(in.ID)
	}

	if b.RecordCall != nil && opts.CallerExt != "" && opts.CalleeExt != "" {
		b.RecordCall(opts.CallerExt, opts.CalleeExt)
	}
	if b.OnCallStateChange != nil && opts.CallerExt != "" && opts.CalleeExt != "" {
		b.OnCallStateChange(opts.CallerExt, opts.CalleeExt, true)
	}

	if wantVideo && media.HasVideo(out.InviteResponse.Body()) {
		b.startVideo(ctx, in, out, callerOffer, out.InviteResponse.Body(), opts.ExternalIP)
	}

	stopBridge, stats, err := b.startControllableBridge(in, out)
	if err != nil {
		return err
	}
	ac.setBridgeStop(stopBridge, stats)
	hc.markBridgeReady()

	b.waitBridgedLegs(ctx, in, out, ac)
	return nil
}

// Join bridges an already-answered outbound leg (hunt, etc.).
func (b *BridgePair) Join(ctx context.Context, in *diago.DialogServerSession, out *diago.DialogClientSession, opts ConnectOpts, mohDir string, preHold ...*holdController) error {
	ac := &ActiveCall{
		CallerExt: opts.CallerExt,
		CalleeExt: opts.CalleeExt,
		In:        in,
		Out:       out,
	}
	if b.OnCallStateChange != nil && opts.CallerExt != "" && opts.CalleeExt != "" {
		defer func() { b.OnCallStateChange(opts.CallerExt, opts.CalleeExt, false) }()
	}

	var hc *holdController
	if len(preHold) > 0 && preHold[0] != nil {
		hc = preHold[0]
		hc.ac = ac
		hc.out = out
	} else {
		hc = newHoldController(b, ctx, ac, mohDir, in, out)
	}
	// Outbound leg may already be answered (hunt); ensure callee-side hold is tracked.
	if out != nil {
		EnsureClientDTMFCodec(out)
	}

	if err := AnswerSessionOptions(in, diago.AnswerOptions{
		OnRefer: b.makeTransferHandler(ctx, in, out, ac),
		OnMediaUpdate: func(dm *diago.DialogMedia) {
			hc.onInboundMediaUpdate(dm)
		},
	}); err != nil {
		return err
	}

	if b.Registry != nil {
		b.Registry.Register(ac)
		defer b.Registry.Unregister(in.ID)
	}

	if b.RecordCall != nil && opts.CallerExt != "" && opts.CalleeExt != "" {
		b.RecordCall(opts.CallerExt, opts.CalleeExt)
	}
	if b.OnCallStateChange != nil && opts.CallerExt != "" && opts.CalleeExt != "" {
		b.OnCallStateChange(opts.CallerExt, opts.CalleeExt, true)
	}

	callerOffer := in.InviteRequest.Body()
	if opts.VideoEnabled && media.HasVideo(callerOffer) && media.HasVideo(out.InviteResponse.Body()) {
		b.startVideo(ctx, in, out, callerOffer, out.InviteResponse.Body(), opts.ExternalIP)
	}

	stopBridge, stats, err := b.startControllableBridge(in, out)
	if err != nil {
		return err
	}
	ac.setBridgeStop(stopBridge, stats)
	hc.markBridgeReady()

	b.waitBridgedLegs(ctx, in, out, ac)
	return nil
}

// waitBridgedLegs blocks until a leg hangs up, then signals the peer and tears down media.
func (b *BridgePair) waitBridgedLegs(ctx context.Context, in *diago.DialogServerSession, out *diago.DialogClientSession, ac *ActiveCall) {
	waitDialogPair(ctx, in, out, ac)
}

// WaitDialogPair blocks until either dialog ends and BYEs the peer.
func WaitDialogPair(ctx context.Context, a, b diago.DialogSession, ac *ActiveCall) {
	waitDialogPair(ctx, a, b, ac)
}

// WaitDialogPairWithBridge is like WaitDialogPair and stops the PCM bridge on teardown.
func WaitDialogPairWithBridge(ctx context.Context, a, b diago.DialogSession, stop func() error) {
	waitDialogPair(ctx, a, b, &ActiveCall{bridgeStop: stop})
}

func waitDialogPair(ctx context.Context, a, b diago.DialogSession, ac *ActiveCall) {
	hangupCtx := context.Background()
	select {
	case <-a.Context().Done():
		hangupDialogSession(hangupCtx, b)
	case <-b.Context().Done():
		hangupDialogSession(hangupCtx, a)
	case <-ctx.Done():
		hangupDialogSession(hangupCtx, a)
		hangupDialogSession(hangupCtx, b)
	}
	if ac != nil {
		if ac.bridgeStop != nil {
			_ = ac.bridgeStop()
		}
		if ac.holdCancel != nil {
			ac.holdCancel()
		}
	}
}

func hangupDialogSession(ctx context.Context, d diago.DialogSession) {
	if d != nil && d.Context().Err() == nil {
		_ = d.Hangup(ctx)
	}
}

func hangupServerLeg(ctx context.Context, in *diago.DialogServerSession) {
	hangupDialogSession(ctx, in)
}

func hangupClientLeg(ctx context.Context, out *diago.DialogClientSession) {
	hangupDialogSession(ctx, out)
}
// On attended transfer, the consult leg (ConsultIn) is hung up first.
func (b *BridgePair) makeTransferHandler(ctx context.Context, in *diago.DialogServerSession, peer *diago.DialogClientSession, ac *ActiveCall) func(*diago.DialogClientSession) error {
	return func(referDialog *diago.DialogClientSession) error {
		if ac != nil {
			ac.StopBridge()
			ac.cancelHold()
		}
		if err := referDialog.Invite(ctx, diago.InviteClientOptions{}); err != nil {
			return err
		}
		if err := referDialog.Ack(ctx); err != nil {
			return err
		}

		// Attended transfer: tear down consult leg if present.
		if ac != nil && ac.ConsultIn != nil {
			ac.ConsultIn.Hangup(ctx)
		}

		stopBridge, _, err := b.startTranscodingBridge(peer, referDialog)
		if err != nil {
			return err
		}
		go func() {
			<-referDialog.Context().Done()
			hangupDialogSession(ctx, in)
		}()
		waitDialogPair(ctx, peer, referDialog, &ActiveCall{bridgeStop: stopBridge})
		return nil
	}
}

func (b *BridgePair) startVideo(ctx context.Context, in *diago.DialogServerSession, out *diago.DialogClientSession, callerOffer, calleeAnswer []byte, externalIP string) {
	if externalIP == "" {
		return
	}
	portA, portB, connA, connB, err := media.AllocateVideoPorts()
	if err != nil {
		if b.Log != nil {
			b.Log.Warn("video ports", "error", err)
		}
		return
	}

	vCtx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-in.Context().Done():
		case <-out.Context().Done():
		}
		cancel()
	}()

	callerRemote, err := media.ParseVideo(callerOffer)
	if err != nil {
		_ = connA.Close()
		_ = connB.Close()
		return
	}
	calleeRemote, err := media.ParseVideo(calleeAnswer)
	if err != nil {
		_ = connA.Close()
		_ = connB.Close()
		return
	}

	inAudio := in.MediaSession().LocalSDP()
	outAudio := out.MediaSession().LocalSDP()
	inMerged, err := media.AppendVideo(inAudio, callerOffer, externalIP, portA)
	if err != nil {
		_ = connA.Close()
		_ = connB.Close()
		return
	}
	outMerged, err := media.AppendVideo(outAudio, calleeAnswer, externalIP, portB)
	if err != nil {
		_ = connA.Close()
		_ = connB.Close()
		return
	}

	if err := reInviteSDP(vCtx, in, inMerged); err != nil && b.Log != nil {
		b.Log.Warn("video reinvite caller", "error", err)
	}
	if err := reInviteSDP(vCtx, out, outMerged); err != nil && b.Log != nil {
		b.Log.Warn("video reinvite callee", "error", err)
	}

	relay := &media.Relay{Log: b.Log}
	relay.Start(vCtx, connA, connB, callerRemote.RemoteAddr, calleeRemote.RemoteAddr)
}

// CompleteTransfer bridges the held party to a new target and disconnects the transferor.
// Used after *77 when the user dials the transfer destination extension.
func (b *BridgePair) CompleteTransfer(ctx context.Context, dg Dialer, ac *ActiveCall, transferorIn *diago.DialogServerSession, targetURI sip.Uri, dialOpts ConnectOpts, headers []sip.Header) error {
	if ac == nil {
		return fmt.Errorf("no active call")
	}
	ac.cancelHold()
	if ac.bridgeStop != nil {
		_ = ac.bridgeStop()
		ac.bridgeStop = nil
	}
	ac.TransferReady = false

	heldIn, heldOut := ac.In, ac.Out
	if heldIn == nil && heldOut == nil {
		return fmt.Errorf("no held party")
	}

	targetOut, err := func() (*diago.DialogClientSession, error) {
		dialCtx, cancel := context.WithTimeout(ctx, DialTimeout(dialOpts.DialTimeoutSeconds))
		defer cancel()
		return InviteOutLeg(dialCtx, dg, targetURI, dialOpts, diago.InviteOptions{Headers: headers}, nil)
	}()
	if err != nil {
		return err
	}
	defer targetOut.Close()

	var held diago.DialogSession
	if heldOut != nil {
		held = heldOut
	} else {
		held = heldIn
	}

	stopBridge, _, err := b.startControllableBridgeSessions(held, targetOut)
	if err != nil {
		return err
	}

	transferorIn.Hangup(ctx)
	if ac.ConsultIn != nil {
		ac.ConsultIn.Hangup(ctx)
	}
	if heldOut != nil && ac.In != nil {
		ac.In.Hangup(ctx)
	} else if heldIn != nil && ac.Out != nil {
		ac.Out.Hangup(ctx)
	}

	waitDialogPair(ctx, held, targetOut, &ActiveCall{bridgeStop: stopBridge})
	return nil
}

// BridgeParked connects a retriever to a parked call.
func (b *BridgePair) BridgeParked(ctx context.Context, retrieverIn *diago.DialogServerSession, pc *ParkedCall) error {
	if pc == nil {
		return fmt.Errorf("no parked call")
	}
	if pc.cancel != nil {
		pc.cancel()
	}

	if err := AnswerSession(retrieverIn); err != nil {
		return err
	}

	var held diago.DialogSession
	if pc.HeldOut != nil {
		held = pc.HeldOut
	} else if pc.HeldIn != nil {
		held = pc.HeldIn
	} else {
		return fmt.Errorf("park slot empty")
	}

	stopBridge, _, err := b.startControllableBridgeSessions(retrieverIn, held)
	if err != nil {
		return err
	}

	waitDialogPair(ctx, retrieverIn, held, &ActiveCall{bridgeStop: stopBridge})
	return nil
}

func reInviteSDP(ctx context.Context, sess interface {
	RemoteContact() *sip.ContactHeader
	Do(context.Context, *sip.Request) (*sip.Response, error)
}, body []byte) error {
	contact := sess.RemoteContact()
	if contact == nil {
		return fmt.Errorf("no remote contact")
	}
	req := sip.NewRequest(sip.INVITE, contact.Address)
	req.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
	req.SetBody(body)
	res, err := sess.Do(ctx, req)
	if err != nil {
		return err
	}
	if !res.IsSuccess() {
		return fmt.Errorf("reinvite status %d", res.StatusCode)
	}
	return nil
}
