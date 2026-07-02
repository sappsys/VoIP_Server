package web

import (
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Yealink/Grandstream-compatible remote phonebook XML.
// Root element name must end in "IPPhoneDirectory".
type ipPhoneDirectory struct {
	XMLName xml.Name         `xml:"VoIPServerIPPhoneDirectory"`
	Entries []directoryEntry `xml:"DirectoryEntry"`
}

type directoryEntry struct {
	Name      string          `xml:"Name"`
	Telephone []telephoneElem `xml:"Telephone"`
}

type telephoneElem struct {
	Label  string `xml:"label,attr,omitempty"`
	Number string `xml:",chardata"`
}

// handlePhonebookXML serves the database-backed directory.xml (no auth: IP phones
// fetch this unauthenticated, same as any TFTP/HTTP remote phonebook server).
func (s *Server) handlePhonebookXML(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ListPhonebookEntries()
	if err != nil {
		http.Error(w, "phonebook unavailable", http.StatusInternalServerError)
		return
	}
	dir := ipPhoneDirectory{Entries: make([]directoryEntry, 0, len(entries))}
	for _, e := range entries {
		dir.Entries = append(dir.Entries, directoryEntry{
			Name:      e.Name,
			Telephone: []telephoneElem{{Label: e.Label, Number: e.Number}},
		})
	}
	body, err := xml.MarshalIndent(dir, "", "  ")
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
}

// handlePhonebookStatic serves any additional *.xml files placed in the phonebook
// directory, so operators can drop in hand-crafted phonebooks alongside the DB one.
func (s *Server) handlePhonebookStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/phonebook/")
	if name == "" || name == "directory.xml" {
		s.handlePhonebookXML(w, r)
		return
	}
	// Only allow simple .xml filenames from the configured directory.
	if s.phonebookDir == "" || strings.ContainsAny(name, "/\\") || !strings.HasSuffix(strings.ToLower(name), ".xml") {
		http.NotFound(w, r)
		return
	}
	clean := filepath.Base(filepath.Clean(name))
	full := filepath.Join(s.phonebookDir, clean)
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	modTime := time.Time{}
	if info, statErr := f.Stat(); statErr == nil {
		modTime = info.ModTime()
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	http.ServeContent(w, r, clean, modTime, f)
}

// handlePhonebook renders the admin UI to create/edit/delete entries.
func (s *Server) handlePhonebook(w http.ResponseWriter, r *http.Request) {
	entries, _ := s.store.ListPhonebookEntries()
	var rows string
	for _, e := range entries {
		rows += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td>
<form hx-post="/phonebook/save" hx-target="body" style="display:inline">
<input type="hidden" name="id" value="%d">
<input name="name" value="%s" required>
<input name="number" value="%s" required>
<input name="label" value="%s" placeholder="label">
<button>Update</button></form>
<form hx-post="/phonebook/delete" hx-target="body" style="display:inline">
<input type="hidden" name="id" value="%d"><button>Delete</button></form></td></tr>`,
			html.EscapeString(e.Name), html.EscapeString(e.Number), html.EscapeString(e.Label),
			e.ID, html.EscapeString(e.Name), html.EscapeString(e.Number), html.EscapeString(e.Label), e.ID)
	}
	content := `<h1>Remote phonebook</h1>
<p>Served at <code>/phonebook/directory.xml</code> for IP phones (Yealink/Grandstream compatible).</p>
<table><tr><th>Name</th><th>Number</th><th>Label</th><th>Edit / Delete</th></tr>` + rows + `</table>
<h2>Add entry</h2>
<form hx-post="/phonebook/save" hx-target="body">
<input name="name" placeholder="Contact name" required>
<input name="number" placeholder="101 or 302@host or sip:301@host" required>
<input name="label" placeholder="label (optional)">
<button>Add</button></form>`
	s.render(w, content)
}

func (s *Server) handlePhonebookSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	number := strings.TrimSpace(r.FormValue("number"))
	label := strings.TrimSpace(r.FormValue("label"))
	if name == "" || number == "" {
		http.Error(w, "name and number required", http.StatusBadRequest)
		return
	}
	var id int64
	fmt.Sscan(r.FormValue("id"), &id)
	if id > 0 {
		_ = s.store.UpdatePhonebookEntry(id, name, number, label)
	} else {
		_, _ = s.store.CreatePhonebookEntry(name, number, label)
	}
	s.handlePhonebook(w, r)
}

func (s *Server) handlePhonebookDelete(w http.ResponseWriter, r *http.Request) {
	var id int64
	fmt.Sscan(r.FormValue("id"), &id)
	_ = s.store.DeletePhonebookEntry(id)
	s.handlePhonebook(w, r)
}
