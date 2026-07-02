//go:build integration

package integration_test

import (
	"context"
	"net"
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

func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	_ = conn.Close()
	return port
}

func startTestPBX(t *testing.T) (*pbx.Server, int) {
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

	ext := &config.Extension{Extension: "101", DisplayName: "Alice", Password: "secret", Enabled: true}
	if err := config.SaveExtension(extDir, ext); err != nil {
		t.Fatal(err)
	}
	exts := map[string]*config.Extension{"101": ext}

	cfg := &config.Config{
		Server: config.ServerConfig{
			BindHost:     "127.0.0.1",
			BindPort:     port,
			Transport:    "udp",
			Realm:        "test.local",
			ExternalHost: "127.0.0.1",
			MOHDir:       "",
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
	t.Cleanup(func() { _ = srv.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Run(ctx) }()
	time.Sleep(150 * time.Millisecond)

	return srv, port
}

func TestSIPRegisterExtension(t *testing.T) {
	srv, port := startTestPBX(t)
	ctx := context.Background()

	ua, err := sipgo.NewUA()
	if err != nil {
		t.Fatal(err)
	}
	defer ua.Close()

	client, err := sipgo.NewClient(ua, sipgo.WithClientHostname("127.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}

	dg := diago.NewDiago(ua,
		diago.WithClient(client),
		diago.WithTransport(diago.Transport{
			Transport: "udp",
			BindHost:  "127.0.0.1",
			BindPort:  0,
		}),
	)

	recipient := sip.Uri{User: "101", Host: "127.0.0.1", Port: port}
	rtx, err := dg.RegisterTransaction(ctx, recipient, diago.RegisterOptions{
		Username: "101",
		Password: "secret",
		Expiry:   120 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rtx.Register(ctx); err != nil {
		t.Fatalf("register: %v", err)
	}

	found := false
	for _, ext := range srv.RegisteredExtensions() {
		if ext == "101" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("extension 101 not registered, got %v", srv.RegisteredExtensions())
	}
}
