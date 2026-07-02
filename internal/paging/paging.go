package paging

import (
	"context"
	"io"
	"log/slog"
	"net"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/call"
	"github.com/sappsys/VoIP_Server/internal/registrar"
	"github.com/sappsys/VoIP_Server/internal/store"
)

type Handler struct {
	Reg *registrar.Registrar
	Log *slog.Logger
}

func NewHandler(reg *registrar.Registrar, log *slog.Logger) *Handler {
	return &Handler{Reg: reg, Log: log}
}

func (h *Handler) Page(ctx context.Context, dg *diago.Diago, in *diago.DialogServerSession, group *store.PagingGroup, members []string) error {
	in.Trying()
	if err := in.Answer(); err != nil {
		return err
	}

	reader, err := in.AudioReader()
	if err != nil {
		return err
	}

	type leg struct {
		out    *diago.DialogClientSession
		writer io.Writer
	}
	var legs []leg
	var writers []io.Writer

	if group.Mode == "multicast" && group.MulticastAddress != "" {
		if conn, err := net.Dial("udp", group.MulticastAddress); err == nil {
			defer conn.Close()
			writers = append(writers, conn)
		} else if h.Log != nil {
			h.Log.Warn("multicast dial failed", "addr", group.MulticastAddress, "error", err)
		}
	}

	headers := call.IntercomHeaders()
	for _, ext := range members {
		out, err := call.InviteExtension(ctx, dg, h.Reg, ext, diago.InviteOptions{Headers: headers})
		if err != nil {
			continue
		}
		w, err := out.AudioWriter()
		if err != nil {
			out.Close()
			continue
		}
		legs = append(legs, leg{out: out, writer: w})
		writers = append(writers, w)
	}

	defer func() {
		for _, l := range legs {
			l.out.Close()
		}
	}()

	if len(writers) == 0 {
		return nil
	}

	buf := make([]byte, media.RTPBufSize)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			for _, w := range writers {
				_, _ = w.Write(chunk)
			}
		}
		if err != nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-in.Context().Done():
			return nil
		default:
		}
	}
	return nil
}
