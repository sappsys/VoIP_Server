package paging

import (
	"context"
	"log/slog"

	"github.com/emiago/diago"
	"github.com/sappsys/VoIP_Server/internal/call"
	"github.com/sappsys/VoIP_Server/internal/media/audiobridge"
	"github.com/sappsys/VoIP_Server/internal/registrar"
	"github.com/sappsys/VoIP_Server/internal/store"
)

type Handler struct {
	Reg         *registrar.Registrar
	Log         *slog.Logger
	DialTimeout int // seconds; 0 = 15
}

func NewHandler(reg *registrar.Registrar, log *slog.Logger, dialTimeout int) *Handler {
	return &Handler{Reg: reg, Log: log, DialTimeout: dialTimeout}
}

func (h *Handler) Page(ctx context.Context, dg *diago.Diago, in *diago.DialogServerSession, group *store.PagingGroup, members []string) error {
	in.Trying()
	if err := call.AnswerSession(in); err != nil {
		return err
	}
	call.EnsureSessionDTMFCodec(in)

	var dstLegs []audiobridge.SessionLeg
	var clients []*diago.DialogClientSession

	headers := call.IntercomHeaders()
	for _, ext := range members {
		dCtx, cancel := context.WithTimeout(ctx, call.DialTimeout(h.DialTimeout))
		out, err := call.InviteExtension(dCtx, dg, h.Reg, ext, diago.InviteOptions{Headers: headers}, nil)
		cancel()
		if err != nil {
			continue
		}
		call.EnsureClientDTMFCodec(out)
		dstLegs = append(dstLegs, out)
		clients = append(clients, out)
	}

	defer func() {
		for _, l := range clients {
			l.Close()
		}
	}()

	if len(dstLegs) == 0 {
		return nil
	}

	return audiobridge.FanoutTranscoded(ctx, h.Log, in, dstLegs)
}
