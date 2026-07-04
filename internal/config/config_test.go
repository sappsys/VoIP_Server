package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigAndExtensionsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	extDir := filepath.Join(dir, "extensions")
	_ = os.MkdirAll(extDir, 0o755)

	cfgToml := `
[server]
realm = "lab.local"
bind_port = 15070

[features]
redial = "66"
park_retrieve = "88"

[paths]
extensions_dir = "extensions"
`
	if err := os.WriteFile(cfgPath, []byte(cfgToml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Features.Redial != "*66" {
		t.Fatalf("redial normalize: %q", cfg.Features.Redial)
	}
	if cfg.Features.ParkRetrieve != "*88" {
		t.Fatalf("park retrieve: %q", cfg.Features.ParkRetrieve)
	}

	ext := &Extension{
		Extension: "201", DisplayName: "Test", Password: "pw",
		Enabled: true, DND: true, VideoEnabled: true,
	}
	if err := SaveExtension(extDir, ext); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadExtensions(extDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	e := loaded["201"]
	if e == nil || !e.DND || !e.VideoEnabled {
		t.Fatalf("reload: %+v", e)
	}
}

func TestTrunkByPrefix(t *testing.T) {
	cfg := &Config{
		Trunks: []TrunkConfig{
			{Prefix: "90", Enabled: true, Name: "A"},
			{Prefix: "91", Enabled: false, Name: "B"},
		},
	}
	if cfg.TrunkByPrefix("90") == nil {
		t.Fatal("expected trunk 90")
	}
	if cfg.TrunkByPrefix("91") != nil {
		t.Fatal("disabled trunk should be nil")
	}
}

func TestValidateDuplicateTrunkPrefix(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{RegisterMinExpiry: 60, RegisterMaxExpiry: 3600},
		Trunks: []TrunkConfig{{Prefix: "90"}, {Prefix: "90"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate prefix error")
	}
}

func TestTrunkDefaultsFromLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	toml := `
[server]
trunk_keepalive_seconds = 25

[[trunks]]
name = "PSTN"
prefix = "9"
server = "gw.example.com"
`
	if err := os.WriteFile(path, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	tr := cfg.Trunks[0]
	if tr.KeepaliveSeconds != 25 {
		t.Fatalf("keepalive_seconds=%d", tr.KeepaliveSeconds)
	}
	if tr.RegisterExpirySeconds != 3600 {
		t.Fatalf("register_expiry_seconds=%d", tr.RegisterExpirySeconds)
	}
}

func TestMOHDirDefaults(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)
	if cfg.Server.MOHDir != "assets/moh" {
		t.Fatalf("default moh_dir=%q", cfg.Server.MOHDir)
	}
}

func TestMOHDirFromLegacyFile(t *testing.T) {
	cfg := &Config{Server: ServerConfig{MOHFile: "custom/hold/track.wav"}}
	setDefaults(cfg)
	if cfg.Server.MOHDir != "custom/hold" {
		t.Fatalf("legacy moh_file dir=%q", cfg.Server.MOHDir)
	}
}

func TestMediaCodecsPassthrough(t *testing.T) {
	cfg := &Config{Media: MediaConfig{Codecs: []string{"PCMU", "G729"}}}
	setDefaults(cfg)
	if len(cfg.Media.Codecs) != 2 {
		t.Fatalf("codecs=%v", cfg.Media.Codecs)
	}
}
