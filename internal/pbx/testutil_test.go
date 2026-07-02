package pbx

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/call"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/hunt"
	"github.com/sappsys/VoIP_Server/internal/registrar"
	"github.com/sappsys/VoIP_Server/internal/store"
)

// mockDialer records outbound INVITE attempts for test assertions.
type mockDialer struct {
	mu      sync.Mutex
	invites []string
	err     error
}

func (m *mockDialer) Invite(ctx context.Context, recipient sip.Uri, opts diago.InviteOptions) (*diago.DialogClientSession, error) {
	m.mu.Lock()
	m.invites = append(m.invites, recipient.User)
	m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return &diago.DialogClientSession{}, nil
}

func (m *mockDialer) count(user string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, u := range m.invites {
		if u == user {
			n++
		}
	}
	return n
}

func (m *mockDialer) total() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.invites)
}

func newTestServerLight(t *testing.T) (*Server, *store.Store, *mockDialer) {
	t.Helper()
	dir := t.TempDir()
	extDir := filepath.Join(dir, "extensions")
	dbPath := filepath.Join(dir, "pbx.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ext101 := &config.Extension{Extension: "101", DisplayName: "Alice", Password: "secret", Enabled: true}
	ext102 := &config.Extension{Extension: "102", DisplayName: "Bob", Password: "secret", Enabled: true}
	ext103 := &config.Extension{Extension: "103", DisplayName: "Carol", Password: "secret", Enabled: true, DND: true}
	for _, e := range []*config.Extension{ext101, ext102, ext103} {
		if err := config.SaveExtension(extDir, e); err != nil {
			t.Fatal(err)
		}
	}
	exts := map[string]*config.Extension{"101": ext101, "102": ext102, "103": ext103}

	cfg := &config.Config{
		Server:   config.ServerConfig{Realm: "test.local", BindHost: "127.0.0.1", BindPort: 0, MOHDir: ""},
		Database: config.DatabaseConfig{Path: dbPath},
		Paths:    config.PathsConfig{ExtensionsDir: extDir},
		Features: config.FeaturesConfig{
			Redial: "*66", CallReturn: "*69", Transfer: "*77", Park: "*85",
			ParkRetrieve: "*86", DNDActivate: "*78", DNDDeactivate: "*79",
		},
		Limits: config.LimitsConfig{MaxCalls: 50},
	}

	reg := registrar.New(cfg.Server.Realm, exts, nil)
	reg.RegisterForTest("101", sip.Uri{User: "101", Host: "127.0.0.1", Port: 5060})
	reg.RegisterForTest("102", sip.Uri{User: "102", Host: "127.0.0.1", Port: 5060})

	registry := call.NewRegistry()
	bridge := call.BridgePair{Registry: registry}
	bridge.RecordCall = func(caller, callee string) {
		_ = st.SetLastDialed(caller, callee)
		_ = st.SetLastCaller(callee, caller)
	}

	md := &mockDialer{}
	srv := &Server{
		cfg:        cfg,
		cfgDir:     dir,
		extDir:     extDir,
		exts:       exts,
		store:      st,
		reg:        reg,
		calls:      call.NewManager(cfg.Limits.MaxCalls),
		registry:   registry,
		park:       call.NewParkLot(),
		hunt:       hunt.NewHandler(reg, nil, &bridge),
		bridge:     bridge,
		features:   cfg.Features.FeatureCodes(),
		testDialer: md,
		testRingCaller: func(ctx context.Context, in *diago.DialogServerSession) error {
			return ctx.Err()
		},
	}
	return srv, st, md
}

func testInviteDialog(id, from, to string) *diago.DialogServerSession {
	req := sip.NewRequest(sip.INVITE, sip.Uri{User: to, Host: "test.local"})
	req.AppendHeader(&sip.FromHeader{Address: sip.Uri{User: from, Host: "test.local"}})
	req.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: to, Host: "test.local"}})
	return &diago.DialogServerSession{
		DialogServerSession: &sipgo.DialogServerSession{
			Dialog: sipgo.Dialog{ID: id, InviteRequest: req},
		},
	}
}
