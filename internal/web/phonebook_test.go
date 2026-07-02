package web

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/store"
)

func testWebServer(t *testing.T) (*Server, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	pbDir := filepath.Join(dir, "phonebook")
	if err := os.MkdirAll(pbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "pbx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := &config.Config{Web: config.WebConfig{SessionSecret: "secret"}}
	s := New(cfg, "", dir, pbDir, st, nil, nil)
	return s, st, pbDir
}

func TestPhonebookXMLYealinkFormat(t *testing.T) {
	s, st, _ := testWebServer(t)
	if _, err := st.CreatePhonebookEntry("Alice & Co", "101", "Office"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreatePhonebookEntry("Bob", "sip:102@host", ""); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/phonebook/directory.xml", nil)
	rr := httptest.NewRecorder()
	s.handlePhonebookStatic(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/xml") {
		t.Fatalf("content-type=%q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "IPPhoneDirectory") {
		t.Fatalf("root element missing:\n%s", body)
	}
	if !strings.Contains(body, "<Telephone label=\"Office\">101</Telephone>") {
		t.Fatalf("labeled entry missing:\n%s", body)
	}
	// Ampersand must be escaped for valid XML.
	if !strings.Contains(body, "Alice &amp; Co") {
		t.Fatalf("XML not escaped:\n%s", body)
	}

	// Must parse back as valid XML.
	var dir ipPhoneDirectory
	if err := xml.Unmarshal([]byte(body), &dir); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if len(dir.Entries) != 2 {
		t.Fatalf("entries=%d", len(dir.Entries))
	}
}

func TestPhonebookSaveAndDelete(t *testing.T) {
	s, st, _ := testWebServer(t)
	cookie := loginAs(t, s, "admin")

	form := strings.NewReader("name=Carol&number=103&label=Desk")
	req := httptest.NewRequest(http.MethodPost, "/phonebook/save", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	s.handlePhonebookSave(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("save status=%d", rr.Code)
	}

	list, _ := st.ListPhonebookEntries()
	if len(list) != 1 || list[0].Name != "Carol" {
		t.Fatalf("entry not saved: %+v", list)
	}

	delForm := strings.NewReader("id=" + itoa(list[0].ID))
	delReq := httptest.NewRequest(http.MethodPost, "/phonebook/delete", delForm)
	delReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	delReq.AddCookie(cookie)
	delRR := httptest.NewRecorder()
	s.handlePhonebookDelete(delRR, delReq)

	if remaining, _ := st.ListPhonebookEntries(); len(remaining) != 0 {
		t.Fatalf("expected empty after delete, got %d", len(remaining))
	}
}

func TestPhonebookSaveRequiresFields(t *testing.T) {
	s, _, _ := testWebServer(t)
	cookie := loginAs(t, s, "admin")
	form := strings.NewReader("name=&number=")
	req := httptest.NewRequest(http.MethodPost, "/phonebook/save", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	s.handlePhonebookSave(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestPhonebookStaticFile(t *testing.T) {
	s, _, pbDir := testWebServer(t)
	xmlData := `<?xml version="1.0"?><TestIPPhoneDirectory></TestIPPhoneDirectory>`
	if err := os.WriteFile(filepath.Join(pbDir, "custom.xml"), []byte(xmlData), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/phonebook/custom.xml", nil)
	rr := httptest.NewRecorder()
	s.handlePhonebookStatic(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "TestIPPhoneDirectory") {
		t.Fatalf("static file not served: %q", rr.Body.String())
	}
}

func TestPhonebookStaticPathTraversalBlocked(t *testing.T) {
	s, _, _ := testWebServer(t)
	req := httptest.NewRequest(http.MethodGet, "/phonebook/..%2f..%2fetc%2fpasswd", nil)
	// Simulate decoded path with slashes.
	req.URL.Path = "/phonebook/../../etc/passwd"
	rr := httptest.NewRecorder()
	s.handlePhonebookStatic(rr, req)
	if rr.Code == http.StatusOK {
		t.Fatalf("path traversal should be blocked, got %d", rr.Code)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
