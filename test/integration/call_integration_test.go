//go:build integration

package integration_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/pbx"
	"github.com/sappsys/VoIP_Server/internal/store"
)

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

func newPhoneUA(t *testing.T) *diago.Diago {
	t.Helper()
	ua, err := sipgo.NewUA()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ua.Close() })
	client, err := sipgo.NewClient(ua, sipgo.WithClientHostname("127.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	return diago.NewDiago(ua,
		diago.WithClient(client),
		diago.WithTransport(diago.Transport{
			Transport: "udp",
			BindHost:  "127.0.0.1",
			BindPort:  0,
		}),
	)
}

func startTestPBXTwoExt(t *testing.T) (int, func()) {
	t.Helper()
	port := freeUDPPort(t)
	dir := t.TempDir()
	extDir := filepath.Join(dir, "extensions")
	dbPath := filepath.Join(dir, "pbx.db")

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	ext101 := &config.Extension{Extension: "101", DisplayName: "Alice", Password: "secret", Enabled: true}
	ext102 := &config.Extension{Extension: "102", DisplayName: "Bob", Password: "secret", Enabled: true}
	for _, e := range []*config.Extension{ext101, ext102} {
		if err := config.SaveExtension(extDir, e); err != nil {
			t.Fatal(err)
		}
	}
	exts := map[string]*config.Extension{"101": ext101, "102": ext102}

	cfg := &config.Config{
		Server: config.ServerConfig{
			BindHost:     "127.0.0.1",
			BindPort:     port,
			Transport:    "udp",
			Realm:        "test.local",
			ExternalHost: "127.0.0.1",
		},
		Database: config.DatabaseConfig{Path: dbPath},
		Paths:    config.PathsConfig{ExtensionsDir: extDir},
		Features: config.FeaturesConfig{
			Redial: "*66", CallReturn: "*69", Transfer: "*77", Park: "*85",
			ParkRetrieve: "*86", DNDActivate: "*78", DNDDeactivate: "*79",
		},
		Limits: config.LimitsConfig{MaxCalls: 10},
	}

	srv, err := pbx.New(cfg, dir, extDir, exts, st, nil)
	if err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Run(runCtx) }()
	time.Sleep(200 * time.Millisecond)

	cleanup := func() {
		cancel()
		_ = srv.Close()
		_ = st.Close()
	}
	return port, cleanup
}

func TestExtensionToExtensionInvite(t *testing.T) {
	port, cleanup := startTestPBXTwoExt(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	phone101 := newPhoneUA(t)
	phone102 := newPhoneUA(t)
	registerExtension(t, phone101, port, "101", "secret")
	registerExtension(t, phone102, port, "102", "secret")

	answered := make(chan struct{}, 1)
	go func() {
		_ = phone102.Serve(ctx, func(in *diago.DialogServerSession) {
			_ = in.Trying()
			_ = in.Ringing()
			if err := in.Answer(); err != nil {
				t.Errorf("answer: %v", err)
				return
			}
			answered <- struct{}{}
			<-in.Context().Done()
		})
	}()
	time.Sleep(100 * time.Millisecond)

	recipient := sip.Uri{User: "102", Host: "127.0.0.1", Port: port}
	out, err := phone101.Invite(ctx, recipient, diago.InviteOptions{})
	if err != nil {
		t.Fatalf("invite 101->102 via PBX: %v", err)
	}
	defer out.Close()

	select {
	case <-answered:
	case <-time.After(10 * time.Second):
		t.Fatal("callee 102 did not receive/answer INVITE from PBX")
	}
}
