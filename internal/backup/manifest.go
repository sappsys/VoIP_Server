package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/sappsys/VoIP_Server/internal/version"
)

type Manifest struct {
	FormatVersion     int         `json:"format_version"`
	VoIPServerVersion string      `json:"voip_server_version"`
	CreatedAt         string      `json:"created_at"`
	ConfigFile        string      `json:"config_file"`
	DatabasePath      string      `json:"database_path"`
	ExtensionsDir     string      `json:"extensions_dir"`
	MOHDir            string      `json:"moh_dir,omitempty"`
	PhonebookDir      string      `json:"phonebook_dir,omitempty"`
	IncludeMOH        bool        `json:"include_moh"`
	IncludePhonebook  bool        `json:"include_phonebook"`
	Files             []FileEntry `json:"files"`
}

type FileEntry struct {
	Path string `json:"path"`
	SHA256 string `json:"sha256"`
	Size int64  `json:"size"`
}

func newManifest(layout Layout, opts ExportOptions) *Manifest {
	return &Manifest{
		FormatVersion:     FormatVersion,
		VoIPServerVersion: version.Version,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
		ConfigFile:        layout.ConfigRel,
		DatabasePath:      layout.DatabaseRel,
		ExtensionsDir:     layout.ExtensionsRel,
		MOHDir:            layout.MOHRel,
		PhonebookDir:      layout.PhonebookRel,
		IncludeMOH:        opts.IncludeMOH,
		IncludePhonebook:  opts.IncludePhonebook,
	}
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func parseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.FormatVersion != FormatVersion {
		return nil, ErrUnsupportedFormat
	}
	if m.ConfigFile == "" || m.DatabasePath == "" || m.ExtensionsDir == "" {
		return nil, ErrInvalidManifest
	}
	return &m, nil
}
