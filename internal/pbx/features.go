package pbx

import (
	"context"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/call"
	"github.com/sappsys/VoIP_Server/internal/router"
)

// handleFeature dispatches configurable star codes from [features] in config.toml.
func (s *Server) handleFeature(ctx context.Context, in *diago.DialogServerSession, from string, route router.Route) {
	switch route.Kind {
	case router.KindRedial:
		s.handleRedial(ctx, in, from)
	case router.KindCallReturn:
		s.handleCallReturn(ctx, in, from)
	case router.KindTransfer:
		s.handleTransfer(ctx, in, from)
	case router.KindPark:
		s.handlePark(ctx, in, from)
	case router.KindParkRetrieve:
		s.handleParkRetrieve(ctx, in, from, route.Target)
	case router.KindDNDActivate:
		s.handleDND(ctx, in, from, true)
	case router.KindDNDDeactivate:
		s.handleDND(ctx, in, from, false)
	default:
		_ = in.Respond(sip.StatusNotFound, "Not Found", nil)
	}
}

func (s *Server) handleRedial(ctx context.Context, in *diago.DialogServerSession, from string) {
	last, err := s.store.GetLastDialed(from)
	if err != nil || last == "" {
		_ = in.Respond(sip.StatusNotFound, "No Redial Number", nil)
		return
	}
	s.dialResolved(ctx, in, from, last, displayName(in.InviteRequest))
}

func (s *Server) handleCallReturn(ctx context.Context, in *diago.DialogServerSession, from string) {
	last, err := s.store.GetLastCaller(from)
	if err != nil || last == "" {
		_ = in.Respond(sip.StatusNotFound, "No Caller To Return", nil)
		return
	}
	host := s.cfg.ExternalHost()
	headers := call.OutboundHeaders("", from, host)
	opts := s.connectOpts(from, last)
	opts.Headers = headers
	s.dialResolvedWithOpts(ctx, in, from, last, opts)
}

// handleTransfer arms attended transfer: marks the active call TransferReady and
// plays MOH to the other party. The user then dials the target extension.
func (s *Server) handleTransfer(ctx context.Context, in *diago.DialogServerSession, from string) {
	ac := s.registry.FindByExtension(from)
	if ac == nil {
		_ = in.Respond(sip.StatusTemporarilyUnavailable, "No Active Call", nil)
		return
	}
	if !s.registry.SetTransferReady(from) {
		_ = in.Respond(sip.StatusTemporarilyUnavailable, "No Active Call", nil)
		return
	}
	s.holdOtherParty(ctx, ac, from)
	in.Trying()
	if err := in.Answer(); err != nil {
		return
	}
	in.Hangup(ctx)
}

// handlePark moves the other party into the park lot at slot = parker's extension,
// plays MOH, and disconnects the parker from the call.
func (s *Server) handlePark(ctx context.Context, in *diago.DialogServerSession, from string) {
	ac := s.registry.FindByExtension(from)
	if ac == nil {
		_ = in.Respond(sip.StatusTemporarilyUnavailable, "No Active Call", nil)
		return
	}
	callerExt := ac.CallerExt
	slot := from
	heldIn, heldOut := ac.StealHeldLeg(from)
	mohDir := s.mohDir()
	holdFn := func(hctx context.Context) {
		if heldOut != nil {
			call.PlayMOHToClient(hctx, heldOut, mohDir, s.log)
		}
		if heldIn != nil {
			call.PlayMOHToServer(hctx, heldIn, mohDir, s.log)
		}
	}
	s.park.Park(slot, heldIn, heldOut, holdFn)

	if from == callerExt && ac.In != nil {
		ac.In.Hangup(ctx)
	} else if ac.Out != nil {
		ac.Out.Hangup(ctx)
	}

	in.Trying()
	if err := in.Answer(); err != nil {
		return
	}
	in.Hangup(ctx)
	if s.log != nil {
		s.log.Info("call parked", "slot", slot, "by", from)
	}
}

func (s *Server) handleParkRetrieve(ctx context.Context, in *diago.DialogServerSession, from, slot string) {
	if slot == "" {
		_ = in.Respond(sip.StatusBadRequest, "Slot Required", nil)
		return
	}
	pc := s.park.Retrieve(slot)
	if pc == nil {
		_ = in.Respond(sip.StatusNotFound, "No Parked Call", nil)
		return
	}
	sess, err := s.calls.TryAcquire(in.ID, from, s.features.ParkRetrieve+slot, displayName(in.InviteRequest), s.exts)
	if err != nil {
		_ = in.Respond(sip.StatusBusyHere, "Busy Here", nil)
		pc.Release()
		return
	}
	defer s.calls.Release(in.ID)
	_ = sess

	if err := s.bridge.BridgeParked(ctx, in, pc); err != nil {
		pc.Release()
		_ = in.Respond(sip.StatusTemporarilyUnavailable, "Retrieve Failed", nil)
		if s.log != nil {
			s.log.Warn("park retrieve failed", "slot", slot, "by", from, "error", err)
		}
		return
	}
	if s.log != nil {
		s.log.Info("call retrieved", "slot", slot, "by", from)
	}
}

func (s *Server) handleTransferComplete(ctx context.Context, in *diago.DialogServerSession, from string, ac *call.ActiveCall, target string) {
	uri, dest, transport, ok := s.reg.DialTarget(target)
	if !ok {
		_ = in.Respond(sip.StatusTemporarilyUnavailable, "Unregistered", nil)
		return
	}
	sess, err := s.calls.TryAcquire(in.ID, from, target, displayName(in.InviteRequest), s.exts)
	if err != nil {
		_ = in.Respond(sip.StatusBusyHere, "Busy Here", nil)
		return
	}
	defer s.calls.Release(in.ID)

	host := s.cfg.ExternalHost()
	headers := call.OutboundHeaders(displayName(in.InviteRequest), from, host)
	dialOpts := call.ConnectOpts{DialDestination: dest, DialTransport: transport}
	if err := s.bridge.CompleteTransfer(ctx, s.inviteDialer(), ac, in, uri, dialOpts, headers); err != nil {
		_ = in.Respond(sip.StatusTemporarilyUnavailable, "Transfer Failed", nil)
		if s.log != nil {
			s.log.Warn("transfer failed", "from", from, "target", target, "error", err)
		}
		return
	}
	_ = sess
	if s.log != nil {
		s.log.Info("transfer complete", "from", from, "target", target)
	}
}

func (s *Server) holdOtherParty(ctx context.Context, ac *call.ActiveCall, parker string) {
	mohDir := s.mohDir()
	if parker == ac.CallerExt && ac.Out != nil {
		go call.PlayMOHToClient(ctx, ac.Out, mohDir, s.log)
	} else if parker == ac.CalleeExt && ac.In != nil {
		go call.PlayMOHToServer(ctx, ac.In, mohDir, s.log)
	}
}

func (s *Server) dialResolved(ctx context.Context, in *diago.DialogServerSession, from, dial, callerName string) {
	host := s.cfg.ExternalHost()
	headers := call.OutboundHeaders(callerName, from, host)
	opts := s.connectOpts(from, dial)
	opts.Headers = headers
	s.dialResolvedWithOpts(ctx, in, from, dial, opts)
}

func (s *Server) dialResolvedWithOpts(ctx context.Context, in *diago.DialogServerSession, from, dial string, opts call.ConnectOpts) {
	route := router.RouteDial(dial, s.features)
	opts.CallerExt = from

	sess, err := s.calls.TryAcquire(in.ID, from, dial, displayName(in.InviteRequest), s.exts)
	if err != nil {
		_ = in.Respond(sip.StatusBusyHere, "Busy Here", nil)
		return
	}
	defer s.calls.Release(in.ID)
	_ = sess

	switch route.Kind {
	case router.KindExtension:
		opts.CalleeExt = route.Target
		_ = s.bridgeToExtension(ctx, in, route.Target, opts)
	case router.KindHunt:
		_ = s.handleHunt(ctx, in, route.Target, opts)
	case router.KindTrunk:
		_ = s.trunk.Outbound(ctx, s.inviteDialer(), in, route.Prefix, route.Rest, opts, s.mohDir(), &s.bridge)
	default:
		_ = in.Respond(sip.StatusNotFound, "Not Found", nil)
	}
}

func (s *Server) recordCall(callerExt, calleeExt string) {
	if callerExt == "" || calleeExt == "" {
		return
	}
	if err := s.store.SetLastDialed(callerExt, calleeExt); err != nil && s.log != nil {
		s.log.Warn("record last dialed", "error", err)
	}
	if err := s.store.SetLastCaller(calleeExt, callerExt); err != nil && s.log != nil {
		s.log.Warn("record last caller", "error", err)
	}
	s.logCall(callerExt, calleeExt, "", "internal", "", "")
}
