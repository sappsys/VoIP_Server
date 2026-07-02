package backup

import (
	"os"
	"path/filepath"
	"strings"
)

func validateArchive(files map[string][]byte) (*Manifest, error) {
	raw, ok := files["manifest.json"]
	if !ok {
		return nil, ErrInvalidManifest
	}
	manifest, err := parseManifest(raw)
	if err != nil {
		return nil, err
	}
	for _, f := range manifest.Files {
		data, ok := files[f.Path]
		if !ok {
			return nil, ErrMissingFile
		}
		if hashBytes(data) != f.SHA256 {
			return nil, ErrChecksumMismatch
		}
	}
	if _, ok := files[manifest.ConfigFile]; !ok {
		return nil, ErrMissingFile
	}
	if _, ok := files[manifest.DatabasePath]; !ok {
		return nil, ErrMissingFile
	}
	return manifest, nil
}

// Restore writes archive contents into layout paths, replacing existing files.
func Restore(layout Layout, files map[string][]byte) error {
	manifest, err := validateArchive(files)
	if err != nil {
		return err
	}

	writeFile := func(dest string, data []byte) error {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	}

	if data, ok := files[manifest.ConfigFile]; ok {
		if err := writeFile(layout.ConfigPath, data); err != nil {
			return err
		}
	}
	if data, ok := files[manifest.DatabasePath]; ok {
		if err := writeFile(layout.DatabasePath, data); err != nil {
			return err
		}
	}

	if err := cleanDir(layout.ExtensionsDir, ".toml"); err != nil {
		return err
	}
	extPrefix := filepath.ToSlash(manifest.ExtensionsDir)
	for path, data := range files {
		if !strings.HasPrefix(path, extPrefix) {
			continue
		}
		rel := strings.TrimPrefix(path, extPrefix)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || !strings.HasSuffix(strings.ToLower(rel), ".toml") {
			continue
		}
		dest := filepath.Join(layout.ExtensionsDir, filepath.FromSlash(rel))
		if err := writeFile(dest, data); err != nil {
			return err
		}
	}

	if manifest.IncludeMOH && manifest.MOHDir != "" {
		if err := cleanDir(layout.MOHDir, ".wav"); err != nil {
			return err
		}
		mohPrefix := filepath.ToSlash(manifest.MOHDir)
		for path, data := range files {
			if !strings.HasPrefix(path, mohPrefix) {
				continue
			}
			rel := strings.TrimPrefix(path, mohPrefix)
			rel = strings.TrimPrefix(rel, "/")
			if rel == "" {
				continue
			}
			dest := filepath.Join(layout.MOHDir, filepath.FromSlash(rel))
			if err := writeFile(dest, data); err != nil {
				return err
			}
		}
	}

	if manifest.IncludePhonebook && manifest.PhonebookDir != "" {
		if err := cleanDir(layout.PhonebookDir, ".xml"); err != nil {
			return err
		}
		pbPrefix := filepath.ToSlash(manifest.PhonebookDir)
		for path, data := range files {
			if !strings.HasPrefix(path, pbPrefix) {
				continue
			}
			rel := strings.TrimPrefix(path, pbPrefix)
			rel = strings.TrimPrefix(rel, "/")
			if rel == "" {
				continue
			}
			dest := filepath.Join(layout.PhonebookDir, filepath.FromSlash(rel))
			if err := writeFile(dest, data); err != nil {
				return err
			}
		}
	}

	return nil
}

func cleanDir(dir string, ext string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dir, 0o755)
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ext) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}
