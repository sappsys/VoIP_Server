package backup

import (
	"path/filepath"

	"github.com/sappsys/VoIP_Server/internal/config"
)

const FormatVersion = 1

// Layout holds resolved on-disk paths for backup and restore.
type Layout struct {
	CfgDir         string
	ConfigPath     string
	ExtensionsDir  string
	DatabasePath   string
	MOHDir         string
	PhonebookDir   string
	ConfigRel      string // path in archive, e.g. config.toml
	DatabaseRel    string // e.g. data/pbx.db
	ExtensionsRel  string // e.g. extensions
	MOHRel         string // e.g. assets/moh
	PhonebookRel   string // e.g. phonebook
}

// LayoutFrom resolves paths relative to the directory containing config.toml.
func LayoutFrom(cfgDir, cfgPath string, cfg *config.Config) Layout {
	rel := func(p string) string {
		if filepath.IsAbs(p) {
			if rel, err := filepath.Rel(cfgDir, p); err == nil && rel != ".." && !filepath.IsAbs(rel) {
				return filepath.ToSlash(rel)
			}
			return filepath.Base(p)
		}
		return filepath.ToSlash(p)
	}

	dbPath := cfg.Database.Path
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(cfgDir, dbPath)
	}
	extDir := cfg.Paths.ExtensionsDir
	if !filepath.IsAbs(extDir) {
		extDir = filepath.Join(cfgDir, extDir)
	}
	mohDir := cfg.Server.MOHDir
	if !filepath.IsAbs(mohDir) {
		mohDir = filepath.Join(cfgDir, mohDir)
	}
	pbDir := cfg.Paths.PhonebookDir
	if !filepath.IsAbs(pbDir) {
		pbDir = filepath.Join(cfgDir, pbDir)
	}

	return Layout{
		CfgDir:        cfgDir,
		ConfigPath:    cfgPath,
		ExtensionsDir: extDir,
		DatabasePath:  dbPath,
		MOHDir:        mohDir,
		PhonebookDir:  pbDir,
		ConfigRel:     filepath.Base(cfgPath),
		DatabaseRel:   rel(cfg.Database.Path),
		ExtensionsRel: rel(cfg.Paths.ExtensionsDir),
		MOHRel:        rel(cfg.Server.MOHDir),
		PhonebookRel:  rel(cfg.Paths.PhonebookDir),
	}
}
