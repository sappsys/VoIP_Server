package trunk

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/call"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/registrar"
	"github.com/sappsys/VoIP_Server/internal/store"
)

type Handler struct {
	Store *store.Store
	Reg   *registrar.Registrar
	Cfg   *config.Config
	Log   *slog.Logger
	cfgMu sync.RWMutex
}

func NewHandler(st *store.Store, reg *registrar.Registrar, cfg *config.Config, log *slog.Logger) *Handler {
	return &Handler{Store: st, Reg: reg, Cfg: cfg, Log: log}
}

func parseTrunkURI(host, user string) sip.Uri {
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		h = host
		p = "5060"
	}
	port, _ := strconv.Atoi(p)
	return sip.Uri{User: user, Host: h, Port: port}
}

func (h *Handler) Outbound(ctx context.Context, dg call.Dialer, in *diago.DialogServerSession, prefix, rest string, opts call.ConnectOpts, mohPath string, bp *call.BridgePair) error {
	t := h.Cfg.TrunkByPrefix(prefix)
	if t == nil {
		_ = in.Respond(sip.StatusNotFound, "Trunk Not Found", nil)
		return fmt.Errorf("trunk prefix %s", prefix)
	}
	uri := parseTrunkURI(t.Server, rest)
	if h.Log != nil {
		h.Log.Debug("trunk outbound", "trunk", t.Name, "prefix", prefix, "dest", uri.String(), "from", opts.CallerExt)
	}
	opts.Username = t.Username
	opts.Password = t.Password
	if bp == nil {
		bp = &call.BridgePair{Log: h.Log}
	}
	return bp.Connect(ctx, dg, in, uri, opts, mohPath)
}
