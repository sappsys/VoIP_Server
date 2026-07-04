//go:build integration

package integration_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/pbx"
	"github.com/sappsys/VoIP_Server/internal/store"
)

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

	phone101 := newHandset(t, port, "101", "secret")
	phone102 := newHandset(t, port, "102", "secret")
	phone101.register()
	phone102.register()

	answered := make(chan *diago.DialogServerSession, 1)
	phone102.serveAnswer(ctx, answered, true)

	out, err := phone101.invite(ctx, "102", nil)
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
