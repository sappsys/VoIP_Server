package hunt

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/call"
	"github.com/sappsys/VoIP_Server/internal/registrar"
)

type Dialer interface {
	Invite(ctx context.Context, recipient sip.Uri, opts diago.InviteOptions) (*diago.DialogClientSession, error)
}

type Handler struct {
	Reg         *registrar.Registrar
	Log         *slog.Logger
	Bridge      *call.BridgePair
	DialTimeout int // seconds; 0 = 15
}

func NewHandler(reg *registrar.Registrar, log *slog.Logger, bridge *call.BridgePair, dialTimeout int) *Handler {
	return &Handler{Reg: reg, Log: log, Bridge: bridge, DialTimeout: dialTimeout}
}

func (h *Handler) ringTimeout(opts call.ConnectOpts) time.Duration {
	return call.RingTimeout(opts.RingTimeoutSeconds)
}

func (h *Handler) Run(ctx context.Context, dg Dialer, in *diago.DialogServerSession, members []string, strategy string, ringTimeout int, headers []sip.Header, opts call.ConnectOpts, mohPath string) error {
	if len(members) == 0 {
		call.PlayBusyThenHangup(ctx, in, h.Bridge.TonesProfile(), h.Log)
		return nil
	}
	_ = ringTimeout
	in.Trying()
	_ = in.Ringing()
	ringStop := call.StartRingback(ctx, in, h.Bridge.TonesProfile(), h.Log)
	stopRing := func() {
		if ringStop != nil {
			ringStop()
			ringStop = nil
		}
	}

	if strategy == "sequential" {
		err := h.sequential(ctx, dg, in, members, headers, opts, mohPath, stopRing)
		stopRing()
		return err
	}
	err := h.simultaneous(ctx, dg, in, members, headers, opts, mohPath, stopRing)
	stopRing()
	return err
}

func (h *Handler) memberDialTimeout() time.Duration {
	sec := h.DialTimeout
	if sec <= 0 {
		sec = 15
	}
	return time.Duration(sec) * time.Second
}

func (h *Handler) relayRinging(in *diago.DialogServerSession) func(*sip.Response) error {
	return func(res *sip.Response) error {
		if res.StatusCode == sip.StatusRinging || res.StatusCode == sip.StatusSessionInProgress {
			return in.Ringing()
		}
		return nil
	}
}

func (h *Handler) sequential(ctx context.Context, dg Dialer, in *diago.DialogServerSession, members []string, headers []sip.Header, opts call.ConnectOpts, mohPath string, stopRing func()) error {
	deadline := time.Now().Add(h.ringTimeout(opts))
	for _, ext := range members {
		if time.Now().After(deadline) {
			break
		}
		perAttempt := h.memberDialTimeout()
		if remaining := time.Until(deadline); remaining < perAttempt {
			perAttempt = remaining
		}
		dCtx, cancel := context.WithTimeout(ctx, perAttempt)
		hc := &call.HoldWatcher{Bridge: h.Bridge, Ctx: ctx, MOHDir: mohPath, In: in, Log: h.Log}
		out, err := call.InviteExtension(dCtx, dg, h.Reg, ext, diago.InviteOptions{
			Originator: in,
			Headers:    headers,
			OnResponse: h.relayRinging(in),
		}, hc.OutboundMediaUpdate())
		cancel()
		if err != nil {
			continue
		}
		hc.BindOut(out)
		stopRing()
		if h.Bridge != nil {
			opts.CalleeExt = ext
			if err := h.Bridge.Join(ctx, in, out, opts, mohPath, hc.Controller()); err != nil {
				out.Close()
				return err
			}
			return nil
		}
		if err := h.joinFallback(ctx, in, out); err != nil {
			out.Close()
			return err
		}
		return nil
	}
	call.PlayBusyThenHangup(ctx, in, h.Bridge.TonesProfile(), h.Log)
	return nil
}

func (h *Handler) simultaneous(ctx context.Context, dg Dialer, in *diago.DialogServerSession, members []string, headers []sip.Header, opts call.ConnectOpts, mohPath string, stopRing func()) error {
	type answeredLeg struct {
		out *diago.DialogClientSession
		hc  *call.HoldWatcher
	}
	answered := make(chan answeredLeg, 1)
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, ext := range members {
		wg.Add(1)
		go func(member string) {
			defer wg.Done()
			dCtx, c := context.WithTimeout(ctx, h.memberDialTimeout())
			defer c()
			hc := &call.HoldWatcher{Bridge: h.Bridge, Ctx: ctx, MOHDir: mohPath, In: in, Log: h.Log}
			out, err := call.InviteExtension(dCtx, dg, h.Reg, member, diago.InviteOptions{
				Originator: in,
				Headers:    headers,
				OnResponse: h.relayRinging(in),
			}, hc.OutboundMediaUpdate())
			if err != nil {
				return
			}
			hc.BindOut(out)
			select {
			case answered <- answeredLeg{out: out, hc: hc}:
			default:
				out.Hangup(ctx)
				out.Close()
			}
		}(ext)
	}

	select {
	case leg := <-answered:
		cancel()
		stopRing()
		if h.Bridge != nil {
			opts.CalleeExt = leg.out.ToUser()
			if err := h.Bridge.Join(ctx, in, leg.out, opts, mohPath, leg.hc.Controller()); err != nil {
				leg.out.Close()
				return err
			}
			return nil
		}
		if err := h.joinFallback(ctx, in, leg.out); err != nil {
			leg.out.Close()
			return err
		}
		return nil
	case <-time.After(h.ringTimeout(opts)):
		call.PlayBusyThenHangup(ctx, in, h.Bridge.TonesProfile(), h.Log)
		return nil
	}
}

func (h *Handler) joinFallback(ctx context.Context, in *diago.DialogServerSession, out *diago.DialogClientSession) error {
	if err := call.AnswerSession(in); err != nil {
		return err
	}
	stop, err := call.BridgeLegsPCM(h.Log, in, out)
	if err != nil {
		return err
	}
	call.WaitDialogPairWithBridge(ctx, in, out, stop)
	out.Close()
	return nil
}
