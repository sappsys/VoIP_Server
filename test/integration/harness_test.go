//go:build integration

package integration_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/pbx"
	"github.com/sappsys/VoIP_Server/internal/store"
)

// pbxOptions configures the integration PBX for a specific requirement scenario.
type pbxOptions struct {
	Extensions  map[string]string // ext -> password
	MOHDir      string            // absolute; when empty a temp MOH dir with a tone wav is created
	Region      string            // tones region (uk/eu/usa); default uk
	BusySeconds int               // busy tone duration; default 5
	Conferences []confSpec        // conference rooms to create
	SoundsDir   string            // absolute sounds dir for PIN prompts (optional)
	Codecs      []string          // media codec order (optional)
	MaxCalls       int // global call limit (0 = default 10)
	ExtMaxSimCalls int // per-extension max simultaneous calls (0 = default 5)
}

type confSpec struct {
	Number string
	PIN    string
	Max    int
}

type testPBX struct {
	Srv  *pbx.Server
	Port int
	Dir  string
}

// startPBX starts a PBX server with the given options and returns it plus a cleanup.
func startPBX(t *testing.T, opts pbxOptions) *testPBX {
	t.Helper()
	port := freeUDPPort(t)
	dir := t.TempDir()
	extDir := filepath.Join(dir, "extensions")
	dbPath := filepath.Join(dir, "pbx.db")

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if len(opts.Extensions) == 0 {
		opts.Extensions = map[string]string{"110": "andy", "111": "andy"}
	}
	exts := map[string]*config.Extension{}
	for ext, pw := range opts.Extensions {
		e := &config.Extension{Extension: ext, DisplayName: "Ext " + ext, Password: pw, Enabled: true}
		if opts.ExtMaxSimCalls > 0 {
			e.MaxSimultaneousCalls = opts.ExtMaxSimCalls
		}
		if err := config.SaveExtension(extDir, e); err != nil {
			t.Fatal(err)
		}
		exts[ext] = e
	}

	mohDir := opts.MOHDir
	if mohDir == "" {
		mohDir = filepath.Join(dir, "moh")
		if err := os.MkdirAll(mohDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeToneWAV(t, filepath.Join(mohDir, "moh.wav"))
	}

	region := opts.Region
	if region == "" {
		region = "uk"
	}
	busy := opts.BusySeconds
	if busy == 0 {
		busy = 5
	}
	maxCalls := opts.MaxCalls
	if maxCalls == 0 {
		maxCalls = 10
	}

	cfg := &config.Config{
		Server: config.ServerConfig{
			BindHost:     "127.0.0.1",
			BindPort:     port,
			Transport:    "udp",
			Realm:        "test.local",
			ExternalHost: "127.0.0.1",
			MOHDir:       mohDir,
		},
		Database: config.DatabaseConfig{Path: dbPath},
		Paths:    config.PathsConfig{ExtensionsDir: extDir},
		Features: config.FeaturesConfig{
			Redial: "*66", CallReturn: "*69", Transfer: "*77", Park: "*85",
			ParkRetrieve: "*86", DNDActivate: "*78", DNDDeactivate: "*79",
		},
		Tones:  config.TonesConfig{Region: region, BusySeconds: busy},
		Media:  config.MediaConfig{Codecs: opts.Codecs},
		Limits: config.LimitsConfig{MaxCalls: maxCalls},
	}
	if opts.SoundsDir != "" {
		cfg.Sounds.Dir = opts.SoundsDir
	}

	for _, c := range opts.Conferences {
		max := c.Max
		if max == 0 {
			max = 8
		}
		if err := st.CreateConference("Room "+c.Number, c.Number, c.PIN, max); err != nil {
			t.Fatal(err)
		}
	}

	srv, err := pbx.New(cfg, dir, extDir, exts, st, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = srv.Run(runCtx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
		}
		_ = srv.Close()
	})
	time.Sleep(250 * time.Millisecond)

	return &testPBX{Srv: srv, Port: port, Dir: dir}
}

// handset is a simulated SIP phone with separate client and server diago stacks.
// Splitting them avoids concurrent Serve+Invite on one diago instance (race-free
// under go test -race).
type handset struct {
	t         *testing.T
	clientDg  *diago.Diago
	serverDg  *diago.Diago
	sipClient *sipgo.Client // server-stack client (register, MESSAGE, SUBSCRIBE)
	sipServer *sipgo.Server
	ext       string
	pw        string
	port      int

	inbound atomic.Value // func(*diago.DialogServerSession)
}

func newHandset(t *testing.T, pbxPort int, ext, password string, codecs ...media.Codec) *handset {
	t.Helper()
	bgCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	clientDg, _ := newClientDiago(t, ext, codecs)
	serverDg, sipSrv, sipCli := newServerDiago(t, ext)

	h := &handset{
		t: t, clientDg: clientDg, serverDg: serverDg,
		sipClient: sipCli, sipServer: sipSrv,
		ext: ext, pw: password, port: pbxPort,
	}
	h.inbound.Store((func(*diago.DialogServerSession))(nil))

	// Client listener: handles in-dialog re-INVITE/BYE on outbound legs only.
	if err := clientDg.ServeBackground(bgCtx, h.rejectInbound); err != nil {
		t.Fatalf("client listen %s: %v", ext, err)
	}
	// Server listener: handles fresh INVITEs to this extension's registered contact.
	if err := serverDg.ServeBackground(bgCtx, h.dispatchInbound); err != nil {
		t.Fatalf("server listen %s: %v", ext, err)
	}
	return h
}

func newClientDiago(t *testing.T, ext string, codecs []media.Codec) (*diago.Diago, *sipgo.Client) {
	t.Helper()
	ua, err := sipgo.NewUA(sipgo.WithUserAgent(ext))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ua.Close() })
	client, err := sipgo.NewClient(ua, sipgo.WithClientHostname("127.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	opts := []diago.DiagoOption{
		diago.WithClient(client),
		diago.WithTransport(diago.Transport{
			Transport: "udp",
			BindHost:  "127.0.0.1",
			BindPort:  0,
		}),
	}
	if len(codecs) > 0 {
		opts = append(opts, diago.WithMediaConfig(diago.MediaConfig{Codecs: codecs}))
	}
	return diago.NewDiago(ua, opts...), client
}

func newServerDiago(t *testing.T, ext string) (*diago.Diago, *sipgo.Server, *sipgo.Client) {
	t.Helper()
	ua, err := sipgo.NewUA(sipgo.WithUserAgent(ext))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ua.Close() })
	srv, err := sipgo.NewServer(ua)
	if err != nil {
		t.Fatal(err)
	}
	client, err := sipgo.NewClient(ua, sipgo.WithClientHostname("127.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	dg := diago.NewDiago(ua,
		diago.WithServer(srv),
		diago.WithClient(client),
		diago.WithTransport(diago.Transport{
			Transport: "udp",
			BindHost:  "127.0.0.1",
			BindPort:  0,
		}),
	)
	return dg, srv, client
}

func (h *handset) rejectInbound(in *diago.DialogServerSession) {
	_ = in.Respond(sip.StatusBusyHere, "Busy", nil)
}

func (h *handset) dispatchInbound(in *diago.DialogServerSession) {
	if fn, ok := h.inbound.Load().(func(*diago.DialogServerSession)); ok && fn != nil {
		fn(in)
		return
	}
	h.rejectInbound(in)
}

func (h *handset) register() {
	h.t.Helper()
	registerExtension(h.t, h.serverDg, h.port, h.ext, h.pw)
}

func (h *handset) uri(target string) sip.Uri {
	return sip.Uri{User: target, Host: "127.0.0.1", Port: h.port}
}

// setInbound installs the handler for fresh INVITEs arriving at this extension.
func (h *handset) setInbound(fn func(*diago.DialogServerSession)) {
	h.inbound.Store(fn)
}

// serveAnswer answers incoming calls and signals on the answered channel.
func (h *handset) serveAnswer(ctx context.Context, answered chan<- *diago.DialogServerSession, hold bool) {
	h.setInbound(func(in *diago.DialogServerSession) {
		_ = in.Trying()
		_ = in.Ringing()
		if err := in.Answer(); err != nil {
			return
		}
		select {
		case answered <- in:
		default:
		}
		if hold {
			<-in.Context().Done()
		}
	})
	_ = ctx // inbound handler runs until handset listener context is cancelled
}

// serveRingingAnswer rings for delay before answering (ringback tests).
func (h *handset) serveRingingAnswer(delay time.Duration, answered chan<- *diago.DialogServerSession) {
	h.setInbound(func(in *diago.DialogServerSession) {
		_ = in.Trying()
		_ = in.Ringing()
		time.Sleep(delay)
		if err := in.Answer(); err != nil {
			return
		}
		select {
		case answered <- in:
		default:
		}
		<-in.Context().Done()
	})
}

// invite places a call to target via the client stack and returns the client leg.
func (h *handset) invite(ctx context.Context, target string, onResp func(*sip.Response) error) (*diago.DialogClientSession, error) {
	return h.clientDg.Invite(ctx, h.uri(target), diago.InviteOptions{OnResponse: onResp})
}

// inviteServer places a call via the server-stack client. Use for star codes (*77, etc.)
// while a media call is active on the client stack — avoids blocking a second client Invite.
func (h *handset) inviteServer(ctx context.Context, target string) (*diago.DialogClientSession, error) {
	return h.serverDg.Invite(ctx, h.uri(target), diago.InviteOptions{
		Username: h.ext,
		Password: h.pw,
	})
}

func registerExtension(t *testing.T, dg *diago.Diago, port int, ext, password string) {
	t.Helper()
	ctx := context.Background()
	recipient := sip.Uri{User: ext, Host: "127.0.0.1", Port: port}
	rtx, err := dg.RegisterTransaction(ctx, recipient, diago.RegisterOptions{
		Username: ext,
		Password: password,
		Expiry:   120 * time.Second,
	})
	if err != nil {
		t.Fatalf("register tx %s: %v", ext, err)
	}
	if err := rtx.Register(ctx); err != nil {
		t.Fatalf("register %s: %v", ext, err)
	}
}

// sendDTMF keys in digits as RFC 2833 telephone-events over the given leg.
func sendDTMF(sess diago.DialogSession, digits string) error {
	dw := diago.DTMFWriter{}
	if _, err := sess.Media().AudioWriter(diago.WithAudioWriterDTMF(&dw)); err != nil {
		return err
	}
	for _, d := range digits {
		if err := dw.WriteDTMF(d); err != nil {
			return err
		}
		time.Sleep(120 * time.Millisecond)
	}
	return nil
}

// pumpAudio keeps call setup alive by writing RTP toward the PBX. Real phones send
// media; they do not attach a second reader on the same leg. While the production
// PCM bridge is active, two-way audio is verified via server bridge relay counters
// (assertTwoWayBridgeRelay), not by reading RTP on handset legs in parallel.
func pumpAudio(ctx context.Context, sess diago.DialogSession) func() {
	pumpCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w, err := sess.Media().AudioWriter()
		if err != nil {
			return
		}
		frame := make([]byte, 160)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-pumpCtx.Done():
				return
			case <-ticker.C:
				if _, err := w.Write(frame); err != nil {
					return
				}
			}
		}
	}()
	return func() {
		cancel()
		wg.Wait()
	}
}

// waitFor polls fn until it returns true or the deadline expires.
func waitFor(d time.Duration, fn func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fn()
}

// writeToneWAV writes a minimal valid 8 kHz mono 16-bit PCM WAV with a short tone.
func writeToneWAV(t *testing.T, path string) {
	t.Helper()
	const sampleRate = 8000
	const seconds = 1
	nSamples := sampleRate * seconds
	dataLen := nSamples * 2
	var buf []byte
	put16 := func(v uint16) { buf = append(buf, byte(v), byte(v>>8)) }
	put32 := func(v uint32) { buf = append(buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)) }

	buf = append(buf, []byte("RIFF")...)
	put32(uint32(36 + dataLen))
	buf = append(buf, []byte("WAVE")...)
	buf = append(buf, []byte("fmt ")...)
	put32(16)
	put16(1) // PCM
	put16(1) // mono
	put32(sampleRate)
	put32(sampleRate * 2)
	put16(2)
	put16(16)
	buf = append(buf, []byte("data")...)
	put32(uint32(dataLen))
	for i := 0; i < nSamples; i++ {
		s := int16((i % 100) * 100)
		buf = append(buf, byte(uint16(s)), byte(uint16(s)>>8))
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

func testLogger() *slog.Logger {
	if os.Getenv("VOIP_TEST_DEBUG") == "" {
		return nil
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
