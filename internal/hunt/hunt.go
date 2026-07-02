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
	Reg    *registrar.Registrar
	Log    *slog.Logger
	Bridge *call.BridgePair
}

func NewHandler(reg *registrar.Registrar, log *slog.Logger, bridge *call.BridgePair) *Handler {
	return &Handler{Reg: reg, Log: log, Bridge: bridge}
}

func (h *Handler) Run(ctx context.Context, dg Dialer, in *diago.DialogServerSession, members []string, strategy string, ringTimeout int, headers []sip.Header, opts call.ConnectOpts, mohPath string) error {
	if len(members) == 0 {
		return in.Respond(sip.StatusTemporarilyUnavailable, "No Members", nil)
	}
	if ringTimeout <= 0 {
		ringTimeout = 20
	}
	in.Trying()
	in.Ringing()

	if strategy == "sequential" {
		return h.sequential(ctx, dg, in, members, ringTimeout, headers, opts, mohPath)
	}
	return h.simultaneous(ctx, dg, in, members, headers, opts, mohPath)
}

func (h *Handler) sequential(ctx context.Context, dg Dialer, in *diago.DialogServerSession, members []string, ringTimeout int, headers []sip.Header, opts call.ConnectOpts, mohPath string) error {
	for _, ext := range members {
		dCtx, cancel := context.WithTimeout(ctx, time.Duration(ringTimeout)*time.Second)
		out, err := call.InviteExtension(dCtx, dg, h.Reg, ext, diago.InviteOptions{
			Originator: in,
			Headers:    headers,
		})
		cancel()
		if err != nil {
			continue
		}
		if h.Bridge != nil {
			opts.CalleeExt = ext
			if err := h.Bridge.Join(ctx, in, out, opts, mohPath); err != nil {
				out.Close()
				return err
			}
			return nil
		}
		bridge := diago.NewBridge()
		if err := in.Answer(); err != nil {
			out.Close()
			return err
		}
		if err := bridge.AddDialogSession(in); err != nil {
			out.Close()
			return err
		}
		if err := bridge.AddDialogSession(out); err != nil {
			out.Close()
			return err
		}
		select {
		case <-in.Context().Done():
			out.Hangup(ctx)
			out.Close()
			return nil
		case <-out.Context().Done():
			out.Close()
		}
	}
	return in.Respond(sip.StatusTemporarilyUnavailable, "No Answer", nil)
}

func (h *Handler) simultaneous(ctx context.Context, dg Dialer, in *diago.DialogServerSession, members []string, headers []sip.Header, opts call.ConnectOpts, mohPath string) error {
	answered := make(chan *diago.DialogClientSession, 1)
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, ext := range members {
		wg.Add(1)
		go func(member string) {
			defer wg.Done()
			dCtx, c := context.WithTimeout(ctx, 35*time.Second)
			defer c()
			out, err := call.InviteExtension(dCtx, dg, h.Reg, member, diago.InviteOptions{
				Originator: in,
				Headers:    headers,
			})
			if err != nil {
				return
			}
			select {
			case answered <- out:
			default:
				out.Hangup(ctx)
				out.Close()
			}
		}(ext)
	}

	select {
	case out := <-answered:
		cancel()
		if h.Bridge != nil {
			opts.CalleeExt = out.ToUser()
			if err := h.Bridge.Join(ctx, in, out, opts, mohPath); err != nil {
				out.Close()
				return err
			}
			return nil
		}
		bridge := diago.NewBridge()
		if err := in.Answer(); err != nil {
			out.Close()
			return err
		}
		if err := bridge.AddDialogSession(in); err != nil {
			return err
		}
		if err := bridge.AddDialogSession(out); err != nil {
			return err
		}
		<-in.Context().Done()
		out.Hangup(ctx)
		out.Close()
	case <-time.After(40 * time.Second):
		return in.Respond(sip.StatusTemporarilyUnavailable, "No Answer", nil)
	}
	wg.Wait()
	return nil
}
