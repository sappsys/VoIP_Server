package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// DefaultMaxCallsPerExtension is applied when an extension omits max_simultaneous_calls
// and when [limits] max_calls_per_extension is unset.
const DefaultMaxCallsPerExtension = 5

type Config struct {
	Server   ServerConfig   `toml:"server"`
	NAT      NATConfig      `toml:"nat"`
	Logging  LoggingConfig  `toml:"logging"`
	Media    MediaConfig    `toml:"media"`
	Limits   LimitsConfig   `toml:"limits"`
	Web      WebConfig      `toml:"web"`
	Database DatabaseConfig `toml:"database"`
	Paths    PathsConfig    `toml:"paths"`
	Features FeaturesConfig `toml:"features"`
	Sounds   SoundsConfig   `toml:"sounds"`
	Tones    TonesConfig    `toml:"tones"`
	Trunks   []TrunkConfig  `toml:"trunks"`
	Users    []WebUser      `toml:"web.users"`
}

type ServerConfig struct {
	BindHost     string   `toml:"bind_host"`
	BindPort     int      `toml:"bind_port"`
	Transport    string   `toml:"transport"`  // udp, tcp, or comma-separated (e.g. "udp,tcp")
	Transports   []string `toml:"transports"` // optional list; overrides transport when set
	ExternalHost string `toml:"external_host"`
	Realm        string `toml:"realm"`
	MOHDir       string `toml:"moh_dir"`
	MOHFile      string `toml:"moh_file"` // deprecated: parent directory used when moh_dir is empty
	// OptionsKeepaliveSeconds sends SIP OPTIONS to registered phones (0 = disabled).
	OptionsKeepaliveSeconds int `toml:"options_keepalive_seconds"`
	// TrunkKeepaliveSeconds sends SIP OPTIONS to enabled trunks with keepalive=options (0 = disabled).
	TrunkKeepaliveSeconds int `toml:"trunk_keepalive_seconds"`
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
	MaxCalls               int `toml:"max_calls"`
	MaxExtensions          int `toml:"max_extensions"`
	MaxCallsPerExtension   int `toml:"max_calls_per_extension"`
	DialTimeoutSeconds     int `toml:"dial_timeout_seconds"`
	RingTimeoutSeconds     int `toml:"ring_timeout_seconds"`
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

// SoundsConfig configures voice prompt WAV files played to callers.
// Dir is the base directory; the other fields are filenames relative to Dir
// (or absolute paths). Empty filename disables that prompt.
type SoundsConfig struct {
	Dir         string `toml:"dir"`
	Busy        string `toml:"busy"`         // deprecated: busy uses [tones] region busy tone
	WrongNumber string `toml:"wrong_number"` // invalid/unknown number dialed
	ConfPIN     string `toml:"conf_pin"`     // prompt for conference PIN
	ConfPINBad  string `toml:"conf_pin_bad"` // conference PIN incorrect
	Unavailable string `toml:"unavailable"`  // extension unregistered/unavailable
	Extension   string `toml:"extension"`    // prompt for extension digits (park retrieve)
}

type TrunkConfig struct {
	ID        int    `toml:"id"`
	Name      string `toml:"name"`
	Prefix    string `toml:"prefix"`
	Server    string `toml:"server"`
	Username  string `toml:"username"`
	Password  string `toml:"password"`
	Transport string `toml:"transport"`
	// Keepalive: options (default), register, or off.
	Keepalive string `toml:"keepalive"`
	// KeepaliveSeconds is the OPTIONS ping interval for NAT/liveness (default 30).
	KeepaliveSeconds int `toml:"keepalive_seconds"`
	// RegisterExpirySeconds is the REGISTER Expires sent to the provider (default 3600).
	RegisterExpirySeconds int  `toml:"register_expiry_seconds"`
	Enabled               bool `toml:"enabled"`
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
	if cfg.Server.Transport == "" && len(cfg.Server.Transports) == 0 {
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
	if cfg.Server.TrunkKeepaliveSeconds == 0 {
		cfg.Server.TrunkKeepaliveSeconds = 30
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
	if cfg.Limits.MaxCallsPerExtension == 0 {
		cfg.Limits.MaxCallsPerExtension = DefaultMaxCallsPerExtension
	}
	if cfg.Limits.DialTimeoutSeconds == 0 {
		cfg.Limits.DialTimeoutSeconds = 15
	}
	if cfg.Limits.RingTimeoutSeconds == 0 {
		cfg.Limits.RingTimeoutSeconds = 30
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
	setSoundDefaults(&cfg.Sounds)
	setFeatureDefaults(&cfg.Features)
	setNATDefaults(cfg)
	cfg.Tones.ApplyDefaults()
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
		if cfg.Trunks[i].KeepaliveSeconds == 0 {
			cfg.Trunks[i].KeepaliveSeconds = cfg.Server.TrunkKeepaliveSeconds
		}
		if cfg.Trunks[i].RegisterExpirySeconds == 0 {
			cfg.Trunks[i].RegisterExpirySeconds = 3600
		}
	}
}

func (c *Config) Validate() error {
	if err := validateSIPTransports(c.Server.SIPTransports()); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	if err := c.validateNAT(); err != nil {
		return err
	}
	if err := c.validateFeatures(); err != nil {
		return err
	}
	if err := c.Tones.Validate(); err != nil {
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
		mode, err := NormalizeTrunkKeepalive(t.Keepalive)
		if err != nil {
			return fmt.Errorf("trunk %q: %w", t.Name, err)
		}
		if mode == "register" && t.Username == "" {
			return fmt.Errorf("trunk %q: keepalive register requires username", t.Name)
		}
		if t.KeepaliveSeconds < 0 {
			return fmt.Errorf("trunk %q: keepalive_seconds must be >= 0", t.Name)
		}
		if t.RegisterExpirySeconds < 0 {
			return fmt.Errorf("trunk %q: register_expiry_seconds must be >= 0", t.Name)
		}
		if t.RegisterExpirySeconds > 0 && t.RegisterExpirySeconds < c.Server.RegisterMinExpiry {
			return fmt.Errorf("trunk %q: register_expiry_seconds must be >= %d", t.Name, c.Server.RegisterMinExpiry)
		}
	}
	return nil
}

func (c *Config) ResolvePath(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(filepath.Dir(c.Database.Path), "..", rel)
}

func LoadExtensions(dir string, defaultMaxSimCalls int) (map[string]*Extension, error) {
	if defaultMaxSimCalls <= 0 {
		defaultMaxSimCalls = DefaultMaxCallsPerExtension
	}
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
			ext.MaxSimultaneousCalls = defaultMaxSimCalls
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

// NormalizeTrunkKeepalive returns options (default), register, or off.
func NormalizeTrunkKeepalive(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "options":
		return "options", nil
	case "register":
		return "register", nil
	case "off", "none", "disabled":
		return "off", nil
	default:
		return "", fmt.Errorf("keepalive must be options, register, or off")
	}
}

// KeepaliveInterval returns the OPTIONS ping interval for a trunk.
func (t *TrunkConfig) KeepaliveInterval() time.Duration {
	sec := t.KeepaliveSeconds
	if sec <= 0 {
		sec = 30
	}
	return time.Duration(sec) * time.Second
}

// RegisterExpiry returns the REGISTER lifetime requested for a trunk.
func (t *TrunkConfig) RegisterExpiry() time.Duration {
	sec := t.RegisterExpirySeconds
	if sec <= 0 {
		sec = 3600
	}
	return time.Duration(sec) * time.Second
}
