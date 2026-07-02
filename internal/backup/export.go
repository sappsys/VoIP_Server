package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ExportOptions struct {
	IncludeMOH       bool
	IncludePhonebook bool
}

type snapshotFunc func(dest string) error

// Export builds a tar.gz backup archive for the given layout.
func Export(layout Layout, opts ExportOptions, snapshotDB snapshotFunc) ([]byte, error) {
	files := make(map[string][]byte)

	addFile := func(arcPath string, srcPath string) error {
		data, err := os.ReadFile(srcPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		files[filepath.ToSlash(arcPath)] = data
		return nil
	}

	addDir := func(arcPrefix, srcDir string, filter func(name string) bool) error {
		info, err := os.Stat(srcDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !info.IsDir() {
			return nil
		}
		return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if filter != nil && !filter(d.Name()) {
				return nil
			}
			rel, err := filepath.Rel(srcDir, path)
			if err != nil {
				return err
			}
			arcPath := filepath.ToSlash(filepath.Join(arcPrefix, rel))
			return addFile(arcPath, path)
		})
	}

	if err := addFile(layout.ConfigRel, layout.ConfigPath); err != nil {
		return nil, err
	}
	if err := addDir(layout.ExtensionsRel, layout.ExtensionsDir, func(name string) bool {
		return strings.HasSuffix(strings.ToLower(name), ".toml")
	}); err != nil {
		return nil, err
	}

	if snapshotDB != nil {
		tmp, err := os.CreateTemp("", "voip-db-*.db")
		if err != nil {
			return nil, err
		}
		tmpPath := tmp.Name()
		_ = tmp.Close()
		defer os.Remove(tmpPath)
		if err := snapshotDB(tmpPath); err != nil {
			return nil, err
		}
		if err := addFile(layout.DatabaseRel, tmpPath); err != nil {
			return nil, err
		}
	} else if err := addFile(layout.DatabaseRel, layout.DatabasePath); err != nil {
		return nil, err
	}

	if opts.IncludeMOH {
		_ = addDir(layout.MOHRel, layout.MOHDir, func(name string) bool {
			return strings.HasSuffix(strings.ToLower(name), ".wav")
		})
	}
	if opts.IncludePhonebook {
		_ = addDir(layout.PhonebookRel, layout.PhonebookDir, func(name string) bool {
			return strings.HasSuffix(strings.ToLower(name), ".xml")
		})
	}

	manifest := newManifest(layout, opts)
	for path, data := range files {
		manifest.Files = append(manifest.Files, FileEntry{
			Path:   path,
			SHA256: hashBytes(data),
			Size:   int64(len(data)),
		})
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	files["manifest.json"] = manifestBytes

	return packTarGz(files)
}

func packTarGz(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	now := time.Now()
	for _, name := range paths {
		data := files[name]
		hdr := &tar.Header{
			Name:    name,
			Mode:    0o644,
			Size:    int64(len(data)),
			ModTime: now,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unpack reads a tar.gz archive into a path→content map (max total size enforced).
func Unpack(r io.Reader, maxBytes int64) (map[string][]byte, error) {
	if maxBytes <= 0 {
		maxBytes = MaxArchiveBytes
	}
	limited := &io.LimitedReader{R: r, N: maxBytes + 1}
	gz, err := gzip.NewReader(limited)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	files := make(map[string][]byte)
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.ToSlash(hdr.Name)
		if strings.Contains(name, "..") {
			return nil, ErrInvalidManifest
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		total += int64(len(data))
		if total > maxBytes {
			return nil, ErrArchiveTooLarge
		}
		files[name] = data
	}
	if limited.N <= 0 {
		return nil, ErrArchiveTooLarge
	}
	return files, nil
}
