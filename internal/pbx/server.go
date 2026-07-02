package pbx

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	diagomedia "github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/call"
	"github.com/sappsys/VoIP_Server/internal/conference"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/hunt"
	mediacodecs "github.com/sappsys/VoIP_Server/internal/media"
	"github.com/sappsys/VoIP_Server/internal/paging"
	"github.com/sappsys/VoIP_Server/internal/registrar"
	"github.com/sappsys/VoIP_Server/internal/router"
	"github.com/sappsys/VoIP_Server/internal/store"
	"github.com/sappsys/VoIP_Server/internal/trunk"
)

type Server struct {
	cfg      *config.Config
	cfgDir   string
	extDir   string
	exts     map[string]*config.Extension
	store    *store.Store
	reg      *registrar.Registrar
	calls    *call.Manager
	registry *call.Registry
	park     *call.ParkLot
	hunt     *hunt.Handler
	conf     *conference.Manager
	trunk    *trunk.Handler
	paging   *paging.Handler
	bridge   call.BridgePair
	features router.FeatureCodes
	dg       *diago.Diago
	testDialer call.Dialer // when set, used instead of dg for outbound INVITE (tests)
	testRingCaller func(ctx context.Context, in *diago.DialogServerSession) error
	ua       *sipgo.UserAgent
	log      *slog.Logger
}

func New(cfg *config.Config, cfgDir, extDir string, exts map[string]*config.Extension, st *store.Store, log *slog.Logger) (*Server, error) {
	if log == nil {
		log = slog.Default()
	}
	ua, err := sipgo.NewUA(sipgo.WithUserAgent(cfg.Server.Realm))
	if err != nil {
		return nil, err
	}
	srv, err := sipgo.NewServer(ua, sipgo.WithServerLogger(log))
	if err != nil {
		ua.Close()
		return nil, err
	}
	reg := registrar.New(cfg.Server.Realm, cfg.Server, exts, log)
	reg.Attach(srv)

	extHost := cfg.ExternalHost()
	bindHost := cfg.SIPBindHost()
	if bindHost != cfg.Server.BindHost {
		log.Info("sip bind host", "configured", cfg.Server.BindHost, "using", bindHost, "reason", "external_host set while bind_host is all-interfaces")
	}
	voiceCodecs, err := mediacodecs.VoiceCodecs(cfg.Media.Codecs)
	if err != nil {
		ua.Close()
		return nil, fmt.Errorf("media codecs: %w", err)
	}
	// Prefer our configured codec order in SDP answers.
	diagomedia.SDPCodecPreferLocalOrder = 1

	mediaIP := net.ParseIP(extHost)
	dg := diago.NewDiago(ua,
		diago.WithServer(srv),
		diago.WithLogger(log),
		diago.WithMediaConfig(diago.MediaConfig{Codecs: voiceCodecs}),
		diago.WithTransport(diago.Transport{
			Transport:       cfg.Server.Transport,
			BindHost:        bindHost,
			BindPort:        cfg.Server.BindPort,
			ExternalHost:    extHost,
			MediaExternalIP: mediaIP,
		}),
	)

	mohDir := cfg.Server.MOHDir
	if !strings.HasPrefix(mohDir, "/") {
		mohDir = cfgDir + "/" + mohDir
	}

	registry := call.NewRegistry()
	parkLot := call.NewParkLot()
	bridge := call.BridgePair{Log: log, Registry: registry}
	bridge.MOHDir = mohDir

	s := &Server{
		cfg:      cfg,
		cfgDir:   cfgDir,
		extDir:   extDir,
		exts:     exts,
		store:    st,
		reg:      reg,
		calls:    call.NewManager(cfg.Limits.MaxCalls),
		registry: registry,
		park:     parkLot,
		hunt:     hunt.NewHandler(reg, log, &bridge),
		conf:     conference.NewManager(log),
		trunk:    trunk.NewHandler(st, reg, cfg, log),
		paging:   paging.NewHandler(reg, log),
		bridge:   bridge,
		features: cfg.Features.FeatureCodes(),
		dg:       dg,
		ua:       ua,
		log:      log,
	}
	s.bridge.RecordCall = s.recordCall
	dg.HandleFunc(s.handleInvite)
	return s, nil
}

func (s *Server) Diago() *diago.Diago { return s.dg }

func (s *Server) inviteDialer() call.Dialer {
	if s.testDialer != nil {
		return s.testDialer
	}
	return s.dg
}

func (s *Server) ringCaller(ctx context.Context, in *diago.DialogServerSession) error {
	if s.testRingCaller != nil {
		return s.testRingCaller(ctx, in)
	}
	return call.RingCaller(ctx, in)
}

func (s *Server) ReloadExtensions(exts map[string]*config.Extension) {
	s.exts = exts
	s.reg.UpdateExtensions(exts)
}

func (s *Server) ReloadConfig(cfg *config.Config) {
	s.cfg = cfg
	s.features = cfg.Features.FeatureCodes()
	s.trunk = trunk.NewHandler(s.store, s.reg, cfg, s.log)
}

func (s *Server) Run(ctx context.Context) error {
	client, err := sipgo.NewClient(s.ua)
	if err != nil {
		return err
	}
	s.reg.RunBackground(ctx, client)
	return s.dg.Serve(ctx, s.handleInvite)
}

func (s *Server) Close() error {
	s.ua.Close()
	return nil
}

func (s *Server) mohDir() string {
	p := s.cfg.Server.MOHDir
	if strings.HasPrefix(p, "/") {
		return p
	}
	return s.cfgDir + "/" + p
}

func (s *Server) connectOpts(from, to string) call.ConnectOpts {
	video := false
	if e, ok := s.exts[from]; ok && e.VideoEnabled {
		video = true
	}
	if e, ok := s.exts[to]; ok && e.VideoEnabled {
		video = true
	}
	return call.ConnectOpts{
		VideoEnabled: video,
		ExternalIP:   s.cfg.ExternalHost(),
		CallerExt:    from,
		CalleeExt:    to,
	}
}

// handleInvite is the main SIP entry point for outbound dials from registered
// extensions and inbound trunk calls. Order: transfer-complete intercept → consult
// link → trunk inbound → feature star codes → normal dial plan.
func (s *Server) handleInvite(in *diago.DialogServerSession) {
	ctx := in.Context()
	dial := in.ToUser()
	from := in.FromUser()
	callerName := displayName(in.InviteRequest)
	s.log.Debug("invite received", "from", from, "to", dial, "caller_registered", s.reg.IsRegistered(from))

	// Complete attended transfer: dial target extension after *77.
	if ac := s.registry.FindByExtension(from); ac != nil && ac.TransferReady {
		if r := router.RouteDial(dial, s.features); r.Kind == router.KindExtension {
			s.handleTransferComplete(ctx, in, from, ac, r.Target)
			return
		}
	}

	// Link consult calls for attended transfer completion.
	if existing := s.registry.ByExtension(from); existing != nil && existing.In != nil && existing.In.ID != in.ID {
		s.registry.SetConsult(from, in)
	}

	// Inbound from external trunk (caller not a registered extension)
	if !s.reg.IsRegistered(from) {
		for _, t := range s.cfg.EnabledTrunks() {
			if s.tryInboundTrunk(ctx, in, t.ID, dial, callerName) {
				return
			}
		}
	}

	route := router.RouteDial(dial, s.features)

	// Star-code features (handled before normal routing).
	switch route.Kind {
	case router.KindRedial, router.KindCallReturn, router.KindTransfer, router.KindPark, router.KindParkRetrieve,
		router.KindDNDActivate, router.KindDNDDeactivate:
		s.handleFeature(ctx, in, from, route)
		return
	}

	if ext, ok := s.exts[dial]; ok && !ext.Enabled {
		_ = in.Respond(sip.StatusTemporarilyUnavailable, "Disabled", nil)
		return
	}

	sess, err := s.calls.TryAcquire(in.ID, from, dial, callerName, s.exts)
	if err != nil {
		_ = in.Respond(sip.StatusBusyHere, "Busy Here", nil)
		return
	}
	defer s.calls.Release(in.ID)

	host := s.cfg.ExternalHost()
	headers := call.OutboundHeaders(callerName, from, host)
	opts := s.connectOpts(from, route.Target)
	opts.Headers = headers

	switch route.Kind {
	case router.KindExtension:
		s.bridgeToExtension(ctx, in, route.Target, opts)
	case router.KindHunt:
		s.handleHunt(ctx, in, route.Target, opts)
	case router.KindConference:
		s.handleConference(ctx, in, route.Target)
	case router.KindPaging:
		s.handlePaging(ctx, in, route.Target)
	case router.KindTrunk:
		opts.CalleeExt = route.Target
		if t := s.cfg.TrunkByPrefix(route.Prefix); t != nil {
			s.logCall(from, route.Target, callerName, "outbound-trunk", t.Name, t.Prefix)
		}
		_ = s.trunk.Outbound(ctx, s.inviteDialer(), in, route.Prefix, route.Rest, opts, s.mohDir(), &s.bridge)
	default:
		_ = in.Respond(sip.StatusNotFound, "Not Found", nil)
	}
	_ = sess
}

func (s *Server) tryInboundTrunk(ctx context.Context, in *diago.DialogServerSession, trunkID int, dial, callerName string) bool {
	route, err := s.store.GetTrunkRoute(trunkID)
	if err != nil || route == nil {
		return false
	}
	t := s.cfg.TrunkByID(trunkID)
	trunkName, trunkPrefix := "", ""
	if t != nil {
		trunkName, trunkPrefix = t.Name, t.Prefix
	}
	target := route.RouteTarget
	if route.RouteType == "all" {
		target = "all-extensions"
	}
	s.logCall(dial, target, callerName, "inbound-trunk", trunkName, trunkPrefix)
	switch route.RouteType {
	case "extension":
		if route.RouteTarget == "" {
			return false
		}
		host := s.cfg.ExternalHost()
		headers := call.OutboundHeaders(callerName, dial, host)
		opts := s.connectOpts(dial, route.RouteTarget)
		opts.Headers = headers
		_ = s.bridgeToExtension(ctx, in, route.RouteTarget, opts)
		return true
	case "group":
		if route.RouteTarget == "" {
			return false
		}
		host := s.cfg.ExternalHost()
		headers := call.OutboundHeaders(callerName, dial, host)
		opts := s.connectOpts(dial, route.RouteTarget)
		opts.Headers = headers
		_ = s.handleHunt(ctx, in, route.RouteTarget, opts)
		return true
	case "all":
		exts := s.reg.RegisteredExtensions()
		if len(exts) == 0 {
			_ = in.Respond(sip.StatusTemporarilyUnavailable, "No Extensions", nil)
			return true
		}
		host := s.cfg.ExternalHost()
		headers := call.OutboundHeaders(callerName, dial, host)
		opts := call.ConnectOpts{ExternalIP: host, CallerExt: dial, Headers: headers}
		_ = s.hunt.Run(ctx, s.inviteDialer(), in, exts, "simultaneous", 25, headers, opts, s.mohDir())
		return true
	}
	return false
}

func displayName(req *sip.Request) string {
	if req == nil || req.From() == nil {
		return ""
	}
	return req.From().DisplayName
}

func (s *Server) bridgeToExtension(ctx context.Context, in *diago.DialogServerSession, ext string, opts call.ConnectOpts) error {
	if e, ok := s.exts[ext]; ok {
		if !e.Enabled {
			_ = in.Respond(sip.StatusTemporarilyUnavailable, "Disabled", nil)
			return fmt.Errorf("disabled")
		}
		if e.DND {
			return s.ringCaller(ctx, in)
		}
		if h := call.CallerNameHeader(e.DisplayName); h != nil && len(opts.Headers) == 0 {
			opts.Headers = call.OutboundHeaders(e.DisplayName, in.FromUser(), s.cfg.ExternalHost())
		}
	}
	uri, dest, transport, ok := s.reg.DialTarget(ext)
	if !ok {
		s.log.Warn("call to unregistered extension", "from", in.FromUser(), "to", ext)
		_ = in.Respond(sip.StatusTemporarilyUnavailable, "Unregistered", nil)
		return fmt.Errorf("unregistered")
	}
	s.log.Debug("bridge to extension", "from", in.FromUser(), "to", ext, "contact", uri.String(), "dest", dest)
	opts.CalleeExt = ext
	opts.DialDestination = dest
	opts.DialTransport = transport
	return s.bridge.Connect(ctx, s.inviteDialer(), in, uri, opts, s.mohDir())
}

func (s *Server) handleHunt(ctx context.Context, in *diago.DialogServerSession, number string, opts call.ConnectOpts) error {
	g, err := s.store.GetHuntGroupByNumber(number)
	if err != nil || g == nil || !g.Enabled {
		_ = in.Respond(sip.StatusNotFound, "Not Found", nil)
		return err
	}
	members, err := s.store.HuntMembers(g.ID)
	if err != nil {
		_ = in.Respond(sip.StatusInternalServerError, "Error", nil)
		return err
	}
	members = s.filterDND(members)
	if len(members) == 0 {
		return s.ringCaller(ctx, in)
	}
	return s.hunt.Run(ctx, s.inviteDialer(), in, members, g.Strategy, g.RingTimeoutSeconds, opts.Headers, opts, s.mohDir())
}

func (s *Server) handleConference(ctx context.Context, in *diago.DialogServerSession, number string) error {
	c, err := s.store.GetConferenceByNumber(number)
	if err != nil || c == nil || !c.Enabled {
		_ = in.Respond(sip.StatusNotFound, "Not Found", nil)
		return err
	}
	return s.conf.HandleJoin(ctx, in, c)
}

func (s *Server) handlePaging(ctx context.Context, in *diago.DialogServerSession, code string) error {
	code = strings.TrimPrefix(code, "*")
	g, err := s.store.GetPagingByCode(code)
	if err != nil || g == nil || !g.Enabled {
		_ = in.Respond(sip.StatusNotFound, "Not Found", nil)
		return err
	}
	members, err := s.store.PagingMembers(g.ID)
	if err != nil {
		_ = in.Respond(sip.StatusInternalServerError, "Error", nil)
		return err
	}
	return s.paging.Page(ctx, s.dg, in, g, members)
}

func (s *Server) Stats() string {
	return fmt.Sprintf("active_calls=%d registered=%d", s.calls.Active(), len(s.reg.RegisteredExtensions()))
}

func (s *Server) RegisteredExtensions() []string {
	return s.reg.RegisteredExtensions()
}
