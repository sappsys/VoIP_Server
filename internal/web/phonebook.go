package web

import (
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/sappsys/VoIP_Server/internal/config"
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

// handlePhonebookXML serves the combined directory (static DB entries + extensions).
// No auth: IP phones fetch this unauthenticated, same as any TFTP/HTTP remote phonebook server.
func (s *Server) handlePhonebookXML(w http.ResponseWriter, r *http.Request) {
	dir, err := s.buildPhonebookDirectory()
	if err != nil {
		http.Error(w, "phonebook unavailable", http.StatusInternalServerError)
		return
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

func (s *Server) buildPhonebookDirectory() (ipPhoneDirectory, error) {
	static, err := s.store.ListPhonebookEntries()
	if err != nil {
		return ipPhoneDirectory{}, err
	}
	exts, err := s.loadExtensions()
	if err != nil {
		return ipPhoneDirectory{}, err
	}

	seen := make(map[string]bool, len(static))
	dir := ipPhoneDirectory{Entries: make([]directoryEntry, 0, len(static)+len(exts))}
	for _, e := range static {
		dir.Entries = append(dir.Entries, directoryEntry{
			Name:      e.Name,
			Telephone: []telephoneElem{{Label: e.Label, Number: e.Number}},
		})
		seen[phonebookNumberKey(e.Number)] = true
	}

	extList := make([]*config.Extension, 0, len(exts))
	for _, ext := range exts {
		if ext != nil && ext.Enabled {
			extList = append(extList, ext)
		}
	}
	slices.SortFunc(extList, func(a, b *config.Extension) int {
		return strings.Compare(strings.ToLower(extensionDisplayName(a)), strings.ToLower(extensionDisplayName(b)))
	})
	for _, ext := range extList {
		if seen[ext.Extension] {
			continue
		}
		dir.Entries = append(dir.Entries, directoryEntry{
			Name:      extensionDisplayName(ext),
			Telephone: []telephoneElem{{Label: "Extension", Number: ext.Extension}},
		})
	}

	sort.Slice(dir.Entries, func(i, j int) bool {
		return strings.ToLower(dir.Entries[i].Name) < strings.ToLower(dir.Entries[j].Name)
	})
	return dir, nil
}

func extensionDisplayName(ext *config.Extension) string {
	if ext == nil {
		return ""
	}
	if ext.DisplayName != "" {
		return ext.DisplayName
	}
	return ext.Extension
}

// phonebookNumberKey normalizes a dial string for duplicate detection against extensions.
func phonebookNumberKey(number string) string {
	n := strings.TrimSpace(number)
	n = strings.TrimPrefix(strings.ToLower(n), "sip:")
	if at := strings.Index(n, "@"); at >= 0 {
		n = n[:at]
	}
	return n
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

// handlePhonebook renders the admin UI to create/edit/delete static entries.
func (s *Server) handlePhonebook(w http.ResponseWriter, r *http.Request) {
	entries, _ := s.store.ListPhonebookEntries()
	exts, _ := s.loadExtensions()
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

	var extRows string
	extList := make([]*config.Extension, 0, len(exts))
	for _, ext := range exts {
		if ext != nil && ext.Enabled {
			extList = append(extList, ext)
		}
	}
	slices.SortFunc(extList, func(a, b *config.Extension) int {
		return strings.Compare(strings.ToLower(extensionDisplayName(a)), strings.ToLower(extensionDisplayName(b)))
	})
	for _, ext := range extList {
		extRows += fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(extensionDisplayName(ext)),
			html.EscapeString(ext.Extension),
			html.EscapeString("Extension"))
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

	content := pageHeader("Remote phonebook", "Served at <code>/phonebook/directory.xml</code> for IP phones (Yealink/Grandstream compatible). Static contacts are managed here; enabled extensions are included automatically.") +
		panel("Static entries", dataTable(tableHeaders, rows)) +
		panel("Extensions (automatic)", `<p class="sub">These are included in <code>directory.xml</code> from extension config. Edit them under <a href="/extensions">Extensions</a>.</p>`+
			dataTable(th("Name", "Number", "Label"), extRows)) +
		adminOnly(admin, editPanel(editPanelTitle(editing != nil, "Add entry", "Edit entry"), formPost("/phonebook/save",
			hiddenID+
				field("Name", fmt.Sprintf(`<input name="name" placeholder="Contact name" required%s>`, valAttr(nameVal)))+
				field("Number", fmt.Sprintf(`<input name="number" placeholder="101 or 302@host" required%s>`, valAttr(numberVal)))+
				field("Label", fmt.Sprintf(`<input name="label" placeholder="label (optional)"%s>`, valAttr(labelVal)))+
				editFormActions(editing != nil, "/phonebook",
					fmt.Sprintf(`<button type="submit">%s</button>`, html.EscapeString(editSubmitLabel(editing != nil)+" entry"))),
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
