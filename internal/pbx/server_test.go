package pbx

import (
	"path/filepath"
	"testing"

	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/store"
)

func testPBXServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	extDir := filepath.Join(dir, "extensions")
	dbPath := filepath.Join(dir, "pbx.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ext := &config.Extension{
		Extension: "101", DisplayName: "Alice", Password: "secret",
		Enabled: true, CallWaiting: true, MaxSimultaneousCalls: 4,
	}
	if err := config.SaveExtension(extDir, ext); err != nil {
		t.Fatal(err)
	}
	ext2 := &config.Extension{Extension: "102", DisplayName: "Bob", Password: "secret", Enabled: true}
	if err := config.SaveExtension(extDir, ext2); err != nil {
		t.Fatal(err)
	}
	exts := map[string]*config.Extension{"101": ext, "102": ext2}

	cfg := &config.Config{
		Server:   config.ServerConfig{Realm: "test.local", BindPort: 15060},
		Database: config.DatabaseConfig{Path: dbPath},
		Paths:    config.PathsConfig{ExtensionsDir: extDir},
		Features: config.FeaturesConfig{
			Redial: "*66", CallReturn: "*69", Transfer: "*77", Park: "*85",
			ParkRetrieve: "*86", DNDActivate: "*78", DNDDeactivate: "*79",
		},
	}

	srv, err := New(cfg, dir, extDir, exts, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv, st
}

func TestFilterDND(t *testing.T) {
	s := &Server{exts: map[string]*config.Extension{
		"101": {DND: true},
		"102": {},
		"103": {DND: true},
	}}
	got := s.filterDND([]string{"101", "102", "103", "104"})
	if len(got) != 2 || got[0] != "102" || got[1] != "104" {
		t.Fatalf("filterDND: %v", got)
	}
}

func TestSetDNDPersists(t *testing.T) {
	srv, _ := testPBXServer(t)
	if err := srv.setDND("101", true); err != nil {
		t.Fatal(err)
	}
	if !srv.exts["101"].DND {
		t.Fatal("memory not updated")
	}
	reloaded, err := config.LoadExtensions(srv.extDir, 0)
	if err != nil || !reloaded["101"].DND {
		t.Fatalf("file not persisted: %+v err=%v", reloaded["101"], err)
	}
	if err := srv.setDND("101", false); err != nil {
		t.Fatal(err)
	}
}

func TestRecordCall(t *testing.T) {
	srv, st := testPBXServer(t)
	srv.recordCall("101", "102")
	dialed, _ := st.GetLastDialed("101")
	caller, _ := st.GetLastCaller("102")
	if dialed != "102" || caller != "101" {
		t.Fatalf("history: dialed=%q caller=%q", dialed, caller)
	}
}
