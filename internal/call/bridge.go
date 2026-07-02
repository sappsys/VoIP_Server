package call

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media/sdp"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/media"
)

// BridgePair connects caller (in) to callee (out) with correct ring-then-answer order.
type BridgePair struct {
	MOHDir     string
	Log        *slog.Logger
	Registry   *Registry
	RecordCall func(callerExt, calleeExt string)
}

// ConnectOpts configures a bridged call.
type ConnectOpts struct {
	Headers      []sip.Header
	Username     string
	Password     string
	VideoEnabled bool
	ExternalIP   string
	CallerExt    string
	CalleeExt    string
}

// Connect dials the callee, answers the caller, optionally relays video, and
// bridges audio until either leg hangs up. Registers the call in Registry for
// transfer/park star codes.
func (b *BridgePair) Connect(ctx context.Context, dg Dialer, in *diago.DialogServerSession, outURI sip.Uri, opts ConnectOpts, mohDir string) error {
	in.Trying()

	headers := opts.Headers
	if headers == nil {
		headers = []sip.Header{}
	}

	callerOffer := in.InviteRequest.Body()
	wantVideo := opts.VideoEnabled && media.HasVideo(callerOffer)

	out, err := dg.Invite(ctx, outURI, diago.InviteOptions{
		Originator: in,
		Headers:    headers,
		Username:   opts.Username,
		Password:   opts.Password,
		OnResponse: func(res *sip.Response) error {
			if res.StatusCode == sip.StatusRinging {
				return in.Ringing()
			}
			if res.StatusCode == sip.StatusSessionInProgress {
				return in.ProgressMedia()
			}
			return nil
		},
	})
	if err != nil {
		_ = in.Respond(sip.StatusTemporarilyUnavailable, "Unavailable", nil)
		return err
	}

	ac := &ActiveCall{
		CallerExt: opts.CallerExt,
		CalleeExt: opts.CalleeExt,
		In:        in,
		Out:       out,
	}
	defer func() {
		if !ac.Parked {
			out.Close()
		}
	}()

	if b.Registry != nil {
		b.Registry.Register(ac)
		defer b.Registry.Unregister(in.ID)
	}

	holdMOH := func(dm *diago.DialogMedia) {
		b.maybeMOH(ctx, in, mohDir, dm)
	}

	if err := in.AnswerOptions(diago.AnswerOptions{
		OnRefer:       b.makeTransferHandler(ctx, in, out, ac),
		OnMediaUpdate: holdMOH,
	}); err != nil {
		out.Hangup(ctx)
		return err
	}

	if b.RecordCall != nil && opts.CallerExt != "" && opts.CalleeExt != "" {
		b.RecordCall(opts.CallerExt, opts.CalleeExt)
	}

	if wantVideo && media.HasVideo(out.InviteResponse.Body()) {
		b.startVideo(ctx, in, out, callerOffer, out.InviteResponse.Body(), opts.ExternalIP)
	}

	bridge := diago.NewBridge()
	if err := bridge.AddDialogSession(in); err != nil {
		return err
	}
	if err := bridge.AddDialogSession(out); err != nil {
		return err
	}

	select {
	case <-in.Context().Done():
	case <-out.Context().Done():
	}
	return nil
}

// Join bridges an already-answered outbound leg (hunt, etc.).
func (b *BridgePair) Join(ctx context.Context, in *diago.DialogServerSession, out *diago.DialogClientSession, opts ConnectOpts, mohDir string) error {
	ac := &ActiveCall{
		CallerExt: opts.CallerExt,
		CalleeExt: opts.CalleeExt,
		In:        in,
		Out:       out,
	}
	if b.Registry != nil {
		b.Registry.Register(ac)
		defer b.Registry.Unregister(in.ID)
	}

	holdMOH := func(dm *diago.DialogMedia) {
		b.maybeMOH(ctx, in, mohDir, dm)
	}

	if err := in.AnswerOptions(diago.AnswerOptions{
		OnRefer:       b.makeTransferHandler(ctx, in, out, ac),
		OnMediaUpdate: holdMOH,
	}); err != nil {
		return err
	}

	if b.RecordCall != nil && opts.CallerExt != "" && opts.CalleeExt != "" {
		b.RecordCall(opts.CallerExt, opts.CalleeExt)
	}

	callerOffer := in.InviteRequest.Body()
	if opts.VideoEnabled && media.HasVideo(callerOffer) && media.HasVideo(out.InviteResponse.Body()) {
		b.startVideo(ctx, in, out, callerOffer, out.InviteResponse.Body(), opts.ExternalIP)
	}

	bridge := diago.NewBridge()
	if err := bridge.AddDialogSession(in); err != nil {
		return err
	}
	if err := bridge.AddDialogSession(out); err != nil {
		return err
	}

	select {
	case <-in.Context().Done():
	case <-out.Context().Done():
	}
	return nil
}

// makeTransferHandler bridges the remote party (peer) to the REFER target.
// On attended transfer, the consult leg (ConsultIn) is hung up first.
func (b *BridgePair) makeTransferHandler(ctx context.Context, in *diago.DialogServerSession, peer *diago.DialogClientSession, ac *ActiveCall) func(*diago.DialogClientSession) error {
	return func(referDialog *diago.DialogClientSession) error {
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

		bridge := diago.NewBridge()
		if err := bridge.AddDialogSession(peer); err != nil {
			return err
		}
		if err := bridge.AddDialogSession(referDialog); err != nil {
			return err
		}
		go func() {
			<-referDialog.Context().Done()
			in.Hangup(ctx)
		}()
		<-referDialog.Context().Done()
		return nil
	}
}

var mohMu sync.Mutex
var mohActive = map[string]bool{}

func (b *BridgePair) maybeMOH(ctx context.Context, sess *diago.DialogServerSession, mohDir string, dm *diago.DialogMedia) {
	if mohDir == "" || dm == nil {
		return
	}
	ms := dm.MediaSession()
	if ms == nil || ms.Mode != sdp.ModeRecvonly {
		return
	}
	id := sess.ID
	mohMu.Lock()
	if mohActive[id] {
		mohMu.Unlock()
		return
	}
	mohActive[id] = true
	mohMu.Unlock()
	defer func() {
		mohMu.Lock()
		delete(mohActive, id)
		mohMu.Unlock()
	}()

	tracks, err := MOHTracks(mohDir)
	if err != nil || len(tracks) == 0 {
		if b.Log != nil && err != nil {
			b.Log.Warn("moh directory missing", "path", mohDir, "error", err)
		}
		return
	}

	pb, err := sess.PlaybackCreate()
	if err != nil {
		return
	}
	go func() {
		for {
			for _, path := range tracks {
				if !playMOHFile(ctx, sess, &pb, path) {
					return
				}
			}
		}
	}()
}

func playMOHFile(ctx context.Context, sess *diago.DialogServerSession, pb *diago.AudioPlayback, path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-sess.Context().Done():
			return false
		default:
			_, err := pb.Play(f, "audio/wav")
			if err == io.EOF {
				return true
			}
			if err != nil {
				return false
			}
		}
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
func (b *BridgePair) CompleteTransfer(ctx context.Context, dg Dialer, ac *ActiveCall, transferorIn *diago.DialogServerSession, targetURI sip.Uri, headers []sip.Header) error {
	if ac == nil {
		return fmt.Errorf("no active call")
	}
	ac.TransferReady = false

	heldIn, heldOut := ac.In, ac.Out
	if heldIn == nil && heldOut == nil {
		return fmt.Errorf("no held party")
	}

	targetOut, err := dg.Invite(ctx, targetURI, diago.InviteOptions{Headers: headers})
	if err != nil {
		return err
	}

	bridge := diago.NewBridge()
	if heldOut != nil {
		if err := bridge.AddDialogSession(heldOut); err != nil {
			targetOut.Close()
			return err
		}
	} else {
		if err := bridge.AddDialogSession(heldIn); err != nil {
			targetOut.Close()
			return err
		}
	}
	if err := bridge.AddDialogSession(targetOut); err != nil {
		targetOut.Close()
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

	<-targetOut.Context().Done()
	targetOut.Close()
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

	if err := retrieverIn.Answer(); err != nil {
		return err
	}

	bridge := diago.NewBridge()
	if pc.HeldOut != nil {
		if err := bridge.AddDialogSession(retrieverIn); err != nil {
			return err
		}
		if err := bridge.AddDialogSession(pc.HeldOut); err != nil {
			return err
		}
	} else if pc.HeldIn != nil {
		if err := bridge.AddDialogSession(pc.HeldIn); err != nil {
			return err
		}
		if err := bridge.AddDialogSession(retrieverIn); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("park slot empty")
	}

	select {
	case <-retrieverIn.Context().Done():
	case <-ctx.Done():
	}
	if pc.HeldOut != nil {
		pc.HeldOut.Close()
	}
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
