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

	"github.com/sappsys/VoIP_Server/internal/store"
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
	admin := s.isAdmin(r)
	editID := queryEditID(r)
	var editing *store.PhonebookEntry
	if editID > 0 {
		editing, _ = s.store.GetPhonebookEntry(editID)
	}

	var rows string
	for _, e := range entries {
		actions := ""
		if admin {
			actions = rowActions(
				editLink(fmt.Sprintf("/phonebook?edit=%d", e.ID)),
				deleteForm("/phonebook/delete", "id", fmt.Sprintf("%d", e.ID)),
			)
		}
		rows += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td>`,
			html.EscapeString(e.Name), html.EscapeString(e.Number), html.EscapeString(e.Label))
		if admin {
			rows += fmt.Sprintf(`<td>%s</td>`, actions)
		}
		rows += `</tr>`
	}
	tableHeaders := th("Name", "Number", "Label")
	if admin {
		tableHeaders = th("Name", "Number", "Label", "Actions")
	}

	nameVal := ""
	numberVal := ""
	labelVal := ""
	hiddenID := ""
	if editing != nil {
		nameVal = editing.Name
		numberVal = editing.Number
		labelVal = editing.Label
		hiddenID = hiddenField("id", fmt.Sprintf("%d", editing.ID))
	}

	content := pageHeader("Remote phonebook", "Served at <code>/phonebook/directory.xml</code> for IP phones (Yealink/Grandstream compatible).") +
		panel("Entries", dataTable(tableHeaders, rows)) +
		adminOnly(admin, editPanel(editPanelTitle(editing != nil, "Add entry", "Edit entry"), formPost("/phonebook/save",
			hiddenID+
				field("Name", fmt.Sprintf(`<input name="name" placeholder="Contact name" required%s>`, valAttr(nameVal)))+
				field("Number", fmt.Sprintf(`<input name="number" placeholder="101 or 302@host" required%s>`, valAttr(numberVal)))+
				field("Label", fmt.Sprintf(`<input name="label" placeholder="label (optional)"%s>`, valAttr(labelVal)))+
				formActions(fmt.Sprintf(`<button type="submit">%s</button>`, html.EscapeString(editSubmitLabel(editing != nil)+" entry"))),
		)))
	s.render(w, r, content)
}

func (s *Server) handlePhonebookSave(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
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
	redirectTo(w, r, "/phonebook")
}

func (s *Server) handlePhonebookDelete(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) || !s.requireAdmin(w, r) {
		return
	}
	var id int64
	fmt.Sscan(r.FormValue("id"), &id)
	_ = s.store.DeletePhonebookEntry(id)
	redirectTo(w, r, "/phonebook")
}
