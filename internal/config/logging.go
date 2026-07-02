package config

import (
	"log/slog"
	"os"
	"strings"
)

// LoggingConfig controls application log output.
type LoggingConfig struct {
	// Level is one of: debug, info, warn, error (default info).
	Level string `toml:"level"`
}

// ParseLogLevel maps a config string to slog levels. Unknown values default to info.
func ParseLogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger builds the process-wide slog logger from config.
func NewLogger(cfg LoggingConfig) *slog.Logger {
	level := ParseLogLevel(cfg.Level)
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
