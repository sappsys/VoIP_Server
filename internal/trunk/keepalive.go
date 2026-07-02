package trunk

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
)

const trunkPingTimeout = 5 * time.Second

// RunBackground starts per-trunk OPTIONS and REGISTER keepalive loops.
// OPTIONS use the same bound SIP socket as outbound calls so NAT mappings stay open.
func (h *Handler) RunBackground(ctx context.Context, dg *diago.Diago, ua *sipgo.UserAgent) {
	if h == nil {
		return
	}
	go h.runOptionsSupervisor(ctx, ua)
	go h.runRegisterSupervisor(ctx, dg)
}

func (h *Handler) UpdateConfig(cfg *config.Config) {
	h.cfgMu.Lock()
	h.Cfg = cfg
	h.cfgMu.Unlock()
}

func (h *Handler) cfgSnapshot() *config.Config {
	h.cfgMu.RLock()
	defer h.cfgMu.RUnlock()
	return h.Cfg
}

func (h *Handler) trunkByID(id int) *config.TrunkConfig {
	cfg := h.cfgSnapshot()
	if cfg == nil {
		return nil
	}
	return cfg.TrunkByID(id)
}

// newSignalClient creates a SIP client bound to the PBX signaling socket (same as diago outbound).
// Outbound keepalives must use this client so NAT/firewall pinholes match real call traffic.
func newSignalClient(ua *sipgo.UserAgent, cfg *config.Config, log *slog.Logger) (*sipgo.Client, error) {
	if ua == nil || cfg == nil {
		return nil, fmt.Errorf("signal client: missing ua or config")
	}
	bindHost := cfg.SIPBindHost()
	bindPort := cfg.Server.BindPort
	if bindPort <= 0 {
		bindPort = 5060
	}
	extHost := cfg.ExternalHost()
	opts := []sipgo.ClientOption{
		sipgo.WithClientNAT(),
	}
	if log != nil {
		opts = append(opts, sipgo.WithClientLogger(log))
	}
	if bindHost != "" {
		opts = append(opts, sipgo.WithClientConnectionAddr(net.JoinHostPort(bindHost, strconv.Itoa(bindPort))))
	}
	if extHost != "" && extHost != bindHost {
		opts = append(opts, sipgo.WithClientHostname(extHost))
		opts = append(opts, sipgo.WithClientPort(bindPort))
	}
	return sipgo.NewClient(ua, opts...)
}

func (h *Handler) runOptionsSupervisor(ctx context.Context, ua *sipgo.UserAgent) {
	if ua == nil {
		return
	}
	active := make(map[int]context.CancelFunc)
	var mu sync.Mutex

	reconcile := func() {
		cfg := h.cfgSnapshot()
		if cfg == nil {
			return
		}
		client, err := newSignalClient(ua, cfg, h.Log)
		if err != nil {
			if h.Log != nil {
				h.Log.Warn("trunk signal client unavailable", "error", err)
			}
			return
		}

		want := map[int]config.TrunkConfig{}
		for _, t := range cfg.EnabledTrunks() {
			mode, err := config.NormalizeTrunkKeepalive(t.Keepalive)
			if err != nil || mode != "options" {
				continue
			}
			want[t.ID] = t
		}

		mu.Lock()
		defer mu.Unlock()
		for id, cancel := range active {
			if _, ok := want[id]; !ok {
				cancel()
				delete(active, id)
			}
		}
		for id, t := range want {
			if _, ok := active[id]; ok {
				continue
			}
			trunkCtx, cancel := context.WithCancel(ctx)
			active[id] = cancel
			go h.maintainTrunkOptions(trunkCtx, client, t)
		}
	}

	reconcile()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, cancel := range active {
				cancel()
			}
			mu.Unlock()
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

func (h *Handler) maintainTrunkOptions(ctx context.Context, client *sipgo.Client, t config.TrunkConfig) {
	for {
		if ctx.Err() != nil {
			return
		}
		current := h.trunkByID(t.ID)
		if current == nil || !current.Enabled {
			return
		}
		mode, err := config.NormalizeTrunkKeepalive(current.Keepalive)
		if err != nil || mode != "options" {
			return
		}
		h.pingTrunkOptions(ctx, client, *current)
		interval := current.KeepaliveInterval()
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (h *Handler) pingTrunkOptions(ctx context.Context, client *sipgo.Client, t config.TrunkConfig) {
	cfg := h.cfgSnapshot()
	uri := parseTrunkURI(t.Server, t.Username)
	dest := trunkServerDest(t.Server)
	req := sip.NewRequest(sip.OPTIONS, uri)
	req.SetDestination(dest)
	if cfg != nil {
		appendNATContact(req, cfg, t.Transport)
	}

	pingCtx, cancel := context.WithTimeout(ctx, trunkPingTimeout)
	defer cancel()

	res, err := client.Do(pingCtx, req)
	if err != nil {
		if h.Log != nil {
			h.Log.Debug("trunk options keepalive failed", "trunk", t.Name, "server", dest, "error", err)
		}
		return
	}
	if res.StatusCode == sip.StatusUnauthorized || res.StatusCode == sip.StatusProxyAuthRequired {
		user := t.Username
		if user == "" {
			user = uri.User
		}
		if t.Password == "" {
			if h.Log != nil {
				h.Log.Debug("trunk options keepalive unauthorized", "trunk", t.Name, "server", dest, "status", res.StatusCode)
			}
			return
		}
		res, err = client.DoDigestAuth(pingCtx, req, res, sipgo.DigestAuth{
			Username: user,
			Password: t.Password,
		})
		if err != nil {
			if h.Log != nil {
				h.Log.Debug("trunk options keepalive digest failed", "trunk", t.Name, "server", dest, "error", err)
			}
			return
		}
	}
	if res != nil && res.StatusCode >= 200 && res.StatusCode < 300 {
		if h.Log != nil {
			h.Log.Debug("trunk options keepalive ok", "trunk", t.Name, "server", dest, "status", res.StatusCode)
		}
		return
	}
	code := 0
	if res != nil {
		code = res.StatusCode
	}
	if h.Log != nil {
		h.Log.Debug("trunk options keepalive bad status", "trunk", t.Name, "server", dest, "status", code)
	}
}

// appendNATContact sets Contact to the public signaling address so keepalives
// traverse the same NAT path as outbound INVITE/REGISTER traffic.
func appendNATContact(req *sip.Request, cfg *config.Config, transport string) {
	if transport == "" {
		transport = cfg.Server.Transport
	}
	if transport == "" {
		transport = "udp"
	}
	port := cfg.Server.BindPort
	if port <= 0 {
		port = 5060
	}
	contact := sip.ContactHeader{
		Address: sip.Uri{
			Host:      cfg.ExternalHost(),
			Port:      port,
			UriParams: sip.NewParams(),
		},
	}
	contact.Address.UriParams.Add("transport", transport)
	req.AppendHeader(&contact)
}

func (h *Handler) runRegisterSupervisor(ctx context.Context, dg *diago.Diago) {
	if dg == nil {
		return
	}
	active := make(map[int]context.CancelFunc)
	var mu sync.Mutex

	reconcile := func() {
		cfg := h.cfgSnapshot()
		if cfg == nil {
			return
		}
		want := map[int]config.TrunkConfig{}
		for _, t := range cfg.EnabledTrunks() {
			mode, err := config.NormalizeTrunkKeepalive(t.Keepalive)
			if err != nil || mode != "register" {
				continue
			}
			want[t.ID] = t
		}

		mu.Lock()
		defer mu.Unlock()
		for id, cancel := range active {
			if _, ok := want[id]; !ok {
				cancel()
				delete(active, id)
			}
		}
		for id, t := range want {
			if _, ok := active[id]; ok {
				continue
			}
			trunkCtx, cancel := context.WithCancel(ctx)
			active[id] = cancel
			go h.maintainTrunkRegister(trunkCtx, dg, t)
		}
	}

	reconcile()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, cancel := range active {
				cancel()
			}
			mu.Unlock()
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

func (h *Handler) maintainTrunkRegister(ctx context.Context, dg *diago.Diago, t config.TrunkConfig) {
	for {
		if ctx.Err() != nil {
			return
		}
		current := h.trunkByID(t.ID)
		if current == nil || !current.Enabled {
			return
		}
		mode, err := config.NormalizeTrunkKeepalive(current.Keepalive)
		if err != nil || mode != "register" {
			return
		}
		err = h.registerSession(ctx, dg, *current)
		if err == nil || ctx.Err() != nil {
			return
		}
		if h.Log != nil {
			h.Log.Warn("trunk register keepalive failed", "trunk", current.Name, "server", current.Server, "error", err)
		}
		retry := current.KeepaliveInterval()
		select {
		case <-ctx.Done():
			return
		case <-time.After(retry):
		}
	}
}

func (h *Handler) registerSession(ctx context.Context, dg *diago.Diago, t config.TrunkConfig) error {
	recipient := parseTrunkURI(t.Server, t.Username)
	if t.Transport != "" {
		recipient.UriParams.Add("transport", t.Transport)
	}
	expiry := t.RegisterExpiry()
	opts := diago.RegisterOptions{
		Username:  t.Username,
		Password:  t.Password,
		ProxyHost: trunkServerDest(t.Server),
		Expiry:    expiry,
	}
	rtx, err := dg.RegisterTransaction(ctx, recipient, opts)
	if err != nil {
		return err
	}
	if err := rtx.Register(ctx); err != nil {
		return err
	}
	if h.Log != nil {
		h.Log.Info("trunk register ok", "trunk", t.Name, "server", t.Server, "expires_sec", int(expiry.Seconds()))
	}
	return rtx.QualifyLoop(ctx)
}

func trunkServerDest(server string) string {
	uri := parseTrunkURI(server, "")
	return fmt.Sprintf("%s:%d", uri.Host, uri.Port)
}
