package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Logging  LoggingConfig  `toml:"logging"`
	Media    MediaConfig    `toml:"media"`
	Limits   LimitsConfig   `toml:"limits"`
	Web      WebConfig      `toml:"web"`
	Database DatabaseConfig `toml:"database"`
	Paths    PathsConfig    `toml:"paths"`
	Features FeaturesConfig `toml:"features"`
	Trunks   []TrunkConfig  `toml:"trunks"`
	Users    []WebUser      `toml:"web.users"`
}

type ServerConfig struct {
	BindHost     string `toml:"bind_host"`
	BindPort     int    `toml:"bind_port"`
	Transport    string `toml:"transport"`
	ExternalHost string `toml:"external_host"`
	Realm        string `toml:"realm"`
	MOHDir       string `toml:"moh_dir"`
	MOHFile      string `toml:"moh_file"` // deprecated: parent directory used when moh_dir is empty
	// OptionsKeepaliveSeconds sends SIP OPTIONS to registered phones (0 = disabled).
	OptionsKeepaliveSeconds int `toml:"options_keepalive_seconds"`
	// RegisterMinExpiry rejects shorter REGISTER Expires values (seconds).
	RegisterMinExpiry int `toml:"register_min_expiry"`
	// RegisterMaxExpiry caps REGISTER Expires (seconds).
	RegisterMaxExpiry int `toml:"register_max_expiry"`
	// PreserveContactHost keeps the Contact host from the phone instead of rewriting private IPs to the packet source.
	PreserveContactHost bool `toml:"preserve_contact_host"`
}

// MediaConfig controls SDP codec offers for SIP calls.
type MediaConfig struct {
	// Codecs is an ordered list of codec IDs (e.g. PCMU, G722, G729).
	// Empty uses the built-in default set.
	Codecs []string `toml:"codecs"`
}

type LimitsConfig struct {
	MaxCalls      int `toml:"max_calls"`
	MaxExtensions int `toml:"max_extensions"`
}

type WebConfig struct {
	Listen          string `toml:"listen"`
	SessionSecret   string `toml:"session_secret"`
	SessionTTLHours int    `toml:"session_ttl_hours"`
}

type WebUser struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
	Role     string `toml:"role"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type PathsConfig struct {
	ExtensionsDir string `toml:"extensions_dir"`
	// PhonebookDir holds static remote-phonebook XML files served under /phonebook/.
	// The database-backed directory.xml is always available regardless of this setting.
	PhonebookDir string `toml:"phonebook_dir"`
}

type TrunkConfig struct {
	ID        int    `toml:"id"`
	Name      string `toml:"name"`
	Prefix    string `toml:"prefix"`
	Server    string `toml:"server"`
	Username  string `toml:"username"`
	Password  string `toml:"password"`
	Transport string `toml:"transport"`
	Enabled   bool   `toml:"enabled"`
}

type Extension struct {
	Extension            string `toml:"extension"`
	DisplayName          string `toml:"display_name"`
	Password             string `toml:"password"`
	Enabled              bool   `toml:"enabled"`
	CallWaiting          bool   `toml:"call_waiting"`
	MaxSimultaneousCalls int    `toml:"max_simultaneous_calls"`
	VideoEnabled         bool   `toml:"video_enabled"`
	Voicemail            bool   `toml:"voicemail"`
	DND                  bool   `toml:"dnd"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, err
	}
	setDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func setDefaults(cfg *Config) {
	if cfg.Server.BindHost == "" {
		cfg.Server.BindHost = "0.0.0.0"
	}
	if cfg.Server.BindPort == 0 {
		cfg.Server.BindPort = 5060
	}
	if cfg.Server.Transport == "" {
		cfg.Server.Transport = "udp"
	}
	if cfg.Server.Realm == "" {
		cfg.Server.Realm = "voip.local"
	}
	if cfg.Server.RegisterMinExpiry == 0 {
		cfg.Server.RegisterMinExpiry = 60
	}
	if cfg.Server.RegisterMaxExpiry == 0 {
		cfg.Server.RegisterMaxExpiry = 3600
	}
	if cfg.Server.OptionsKeepaliveSeconds == 0 {
		cfg.Server.OptionsKeepaliveSeconds = 30
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Server.MOHDir == "" {
		if cfg.Server.MOHFile != "" {
			cfg.Server.MOHDir = filepath.Dir(cfg.Server.MOHFile)
		} else {
			cfg.Server.MOHDir = "assets/moh"
		}
	}
	if cfg.Limits.MaxCalls == 0 {
		cfg.Limits.MaxCalls = 200
	}
	if cfg.Limits.MaxExtensions == 0 {
		cfg.Limits.MaxExtensions = 400
	}
	if cfg.Web.Listen == "" {
		cfg.Web.Listen = "0.0.0.0:7030"
	}
	if cfg.Web.SessionSecret == "" {
		cfg.Web.SessionSecret = "change-me-in-production"
	}
	if cfg.Web.SessionTTLHours == 0 {
		cfg.Web.SessionTTLHours = 24
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "data/pbx.db"
	}
	if cfg.Paths.ExtensionsDir == "" {
		cfg.Paths.ExtensionsDir = "extensions"
	}
	if cfg.Paths.PhonebookDir == "" {
		cfg.Paths.PhonebookDir = "phonebook"
	}
	setFeatureDefaults(&cfg.Features)
	if len(cfg.Users) == 0 {
		cfg.Users = []WebUser{{Username: "admin", Password: "admin", Role: "admin"}}
	}
	for i := range cfg.Trunks {
		if cfg.Trunks[i].Transport == "" {
			cfg.Trunks[i].Transport = "udp"
		}
		if cfg.Trunks[i].ID == 0 {
			cfg.Trunks[i].ID = i + 1
		}
	}
}

func (c *Config) Validate() error {
	if err := c.validateFeatures(); err != nil {
		return err
	}
	if c.Server.RegisterMaxExpiry < c.Server.RegisterMinExpiry {
		return fmt.Errorf("server.register_max_expiry must be >= register_min_expiry")
	}
	if len(c.Trunks) > 10 {
		return fmt.Errorf("maximum 10 trunks allowed")
	}
	seen := map[string]bool{}
	for _, t := range c.Trunks {
		if t.Prefix == "" {
			return fmt.Errorf("trunk %q: prefix required", t.Name)
		}
		if seen[t.Prefix] {
			return fmt.Errorf("duplicate trunk prefix %q", t.Prefix)
		}
		seen[t.Prefix] = true
	}
	return nil
}

func (c *Config) ResolvePath(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(filepath.Dir(c.Database.Path), "..", rel)
}

func LoadExtensions(dir string) (map[string]*Extension, error) {
	out := make(map[string]*Extension)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var ext Extension
		if _, err := toml.Decode(string(data), &ext); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if ext.Extension == "" {
			return nil, fmt.Errorf("%s: missing extension", path)
		}
		if ext.MaxSimultaneousCalls == 0 {
			ext.MaxSimultaneousCalls = 4
		}
		if ext.DisplayName == "" {
			ext.DisplayName = ext.Extension
		}
		if !strings.Contains(string(data), "enabled") {
			ext.Enabled = true
		}
		if !strings.Contains(string(data), "call_waiting") {
			ext.CallWaiting = true
		}
		out[ext.Extension] = &ext
	}
	return out, nil
}

func SaveExtension(dir string, ext *Extension) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, ext.Extension+".toml")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(ext)
}

func DeleteExtensionFile(dir, ext string) error {
	return os.Remove(filepath.Join(dir, ext+".toml"))
}

func (c *Config) TrunkByPrefix(prefix string) *TrunkConfig {
	for i := range c.Trunks {
		t := &c.Trunks[i]
		if t.Enabled && t.Prefix == prefix {
			return t
		}
	}
	return nil
}

func (c *Config) TrunkByID(id int) *TrunkConfig {
	for i := range c.Trunks {
		t := &c.Trunks[i]
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (c *Config) EnabledTrunks() []TrunkConfig {
	var out []TrunkConfig
	for _, t := range c.Trunks {
		if t.Enabled {
			out = append(out, t)
		}
	}
	return out
}
