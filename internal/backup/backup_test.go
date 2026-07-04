package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/store"
)

func TestExportRestoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	extDir := filepath.Join(dir, "extensions")
	dbPath := filepath.Join(dir, "data", "pbx.db")
	_ = os.MkdirAll(extDir, 0o755)
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)

	cfgToml := `bind_host = "127.0.0.1"
bind_port = 5060
`
	if err := os.WriteFile(cfgPath, []byte(`[server]
`+cfgToml+`
[database]
path = "data/pbx.db"

[paths]
extensions_dir = "extensions"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "101.toml"), []byte(`extension = "101"
display_name = "Alice"
password = "secret"
enabled = true
`), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateHuntGroup("Sales", "500", "simultaneous", 20); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	layout := LayoutFrom(dir, cfgPath, cfg)

	st2, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := Export(layout, ExportOptions{}, st2.SnapshotTo)
	_ = st2.Close()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	restoreDir := filepath.Join(dir, "restore")
	_ = os.MkdirAll(restoreDir, 0o755)
	restoreCfgPath := filepath.Join(restoreDir, "config.toml")
	if err := os.WriteFile(restoreCfgPath, []byte(`[server]
bind_host = "0.0.0.0"
bind_port = 5060

[database]
path = "data/pbx.db"

[paths]
extensions_dir = "extensions"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	restoreCfg, err := config.LoadConfig(restoreCfgPath)
	if err != nil {
		t.Fatal(err)
	}
	restoreLayout := LayoutFrom(restoreDir, restoreCfgPath, restoreCfg)
	restoreLayout.ExtensionsDir = filepath.Join(restoreDir, "extensions")
	restoreLayout.DatabasePath = filepath.Join(restoreDir, "data", "pbx.db")

	files, err := Unpack(bytes.NewReader(archive), MaxArchiveBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := Restore(restoreLayout, files); err != nil {
		t.Fatalf("restore: %v", err)
	}

	restoredCfg, err := os.ReadFile(restoreLayout.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(restoredCfg, []byte(`bind_host = "127.0.0.1"`)) {
		t.Fatalf("config not restored: %s", restoredCfg)
	}
	exts, err := config.LoadExtensions(restoreLayout.ExtensionsDir, 0)
	if err != nil || exts["101"] == nil {
		t.Fatalf("extensions not restored: %v", err)
	}
	rst, err := store.Open(restoreLayout.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer rst.Close()
	groups, err := rst.ListHuntGroups()
	if err != nil || len(groups) != 1 || groups[0].Number != "500" {
		t.Fatalf("db not restored: %+v err=%v", groups, err)
	}
}

func TestLayoutFromRelativePaths(t *testing.T) {
	cfg := &config.Config{
		Server:   config.ServerConfig{MOHDir: "assets/moh"},
		Database: config.DatabaseConfig{Path: "data/pbx.db"},
		Paths:    config.PathsConfig{ExtensionsDir: "extensions", PhonebookDir: "phonebook"},
	}
	layout := LayoutFrom("/opt/voip", "/opt/voip/config.toml", cfg)
	if layout.DatabaseRel != "data/pbx.db" {
		t.Fatalf("DatabaseRel=%q", layout.DatabaseRel)
	}
	if layout.ExtensionsRel != "extensions" {
		t.Fatalf("ExtensionsRel=%q", layout.ExtensionsRel)
	}
}
