package web

import (
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sappsys/VoIP_Server/internal/backup"
	"github.com/sappsys/VoIP_Server/internal/config"
)

func (s *Server) cfgDir() string {
	if s.cfgPath == "" {
		return "."
	}
	dir := filepath.Dir(s.cfgPath)
	if dir == "." {
		wd, err := os.Getwd()
		if err == nil {
			return wd
		}
	}
	return dir
}

func (s *Server) backupLayout() backup.Layout {
	return backup.LayoutFrom(s.cfgDir(), s.cfgPath, s.cfg)
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}
	notice := ""
	if r.URL.Query().Get("restored") == "1" {
		notice = `<p class="badge badge-online">Backup restored and configuration reloaded.</p>`
	}
	body := pageHeader("Backup & restore", "Export or restore the full PBX configuration: <code>config.toml</code>, extensions, SQLite database, and optional MOH WAV files and static phonebook XML.") +
		notice +
		panel("Download backup", fmt.Sprintf(`<p>Creates a <code>.tar.gz</code> archive with config, extensions, and the SQLite database.</p>
<form method="get" action="/backup/download" class="stack-form">
%s
%s
<p><button type="submit">Download backup</button></p>
</form>`, checkField("Include MOH WAV files", `<input type="checkbox" name="include_moh" value="1">`),
			checkField("Include static phonebook XML", `<input type="checkbox" name="include_phonebook" value="1">`))) +
		panel("Restore backup", `<p>Restore replaces <strong>config.toml</strong>, all extension files, and the database. Hunt groups, conferences, paging, trunk routes, phonebook entries, and web users come from the database backup.</p>
<form method="post" action="/backup/restore" enctype="multipart/form-data" class="stack-form">
<label>Backup archive (.tar.gz)</label>
<input type="file" name="archive" accept=".tar.gz,.tgz,application/gzip" required>
<p><button type="submit">Restore backup</button></p>
</form>`)
	s.render(w, r, body)
}

func (s *Server) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}
	opts := backup.ExportOptions{
		IncludeMOH:       r.URL.Query().Get("include_moh") == "1",
		IncludePhonebook: r.URL.Query().Get("include_phonebook") == "1",
	}
	layout := s.backupLayout()
	var snap func(string) error
	if s.store != nil {
		snap = s.store.SnapshotTo
	}
	data, err := backup.Export(layout, opts, snap)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		if s.log != nil {
			s.log.Warn("backup export", "error", err)
		}
		return
	}
	name := fmt.Sprintf("voip-backup-%s.tar.gz", time.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	_, _ = w.Write(data)
}

func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid upload", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("archive")
	if err != nil {
		http.Error(w, "missing archive", http.StatusBadRequest)
		return
	}
	defer file.Close()
	low := strings.ToLower(header.Filename)
	if !strings.HasSuffix(low, ".tar.gz") && !strings.HasSuffix(low, ".tgz") {
		http.Error(w, "expected .tar.gz archive", http.StatusBadRequest)
		return
	}

	files, err := backup.Unpack(file, backup.MaxArchiveBytes)
	if err != nil {
		http.Error(w, html.EscapeString(err.Error()), http.StatusBadRequest)
		return
	}

	layout := s.backupLayout()
	if s.store != nil {
		_ = s.store.Close()
	}
	if err := backup.Restore(layout, files); err != nil {
		if s.store != nil {
			_ = s.store.Reopen(layout.DatabasePath)
		}
		http.Error(w, html.EscapeString(err.Error()), http.StatusBadRequest)
		return
	}
	if s.store != nil {
		if err := s.store.Reopen(layout.DatabasePath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	cfg, err := config.LoadConfig(s.cfgPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.cfg = cfg
	if s.pbx != nil {
		s.pbx.ReloadConfig(cfg)
		exts, _ := config.LoadExtensions(s.extDir)
		s.pbx.ReloadExtensions(exts)
	}
	redirectTo(w, r, "/backup?restored=1")
}
