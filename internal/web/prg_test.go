package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sappsys/VoIP_Server/internal/pbx"
)

func TestMutationHandlersRedirect(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "extensions")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	s, _, _ := testWebServer(t)
	s.extDir = extDir
	cookie := loginAs(t, s, "admin")
	h := s.Handler()

	cases := []struct {
		path   string
		body   string
		expect string
	}{
		{"/reload", "", "/"},
		{"/extensions/save", "extension=201&password=secret&display_name=Test", "/extensions"},
		{"/hunt/delete", "id=1", "/hunt"},
		{"/conferences/save", "number=601&name=Room&pin=1234&max_participants=8", "/conferences"},
		{"/conferences/delete", "id=1", "/conferences"},
		{"/paging/save", "code=80&name=All&mode=unicast&channel=0&members=101", "/paging"},
		{"/paging/delete", "id=1", "/paging"},
		{"/trunks/route", "trunk_id=1&route_type=all&route_target=", "/trunks"},
		{"/users/save", "username=ops&password=secret&role=user", "/users"},
		{"/users/delete", "id=99", "/users"},
		{"/phonebook/save", "name=Bob&number=202&label=", "/phonebook"},
		{"/phonebook/delete", "id=1", "/phonebook"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(cookie)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusSeeOther {
				t.Fatalf("status=%d want 303", rr.Code)
			}
			if loc := rr.Header().Get("Location"); loc != tc.expect {
				t.Fatalf("location=%q want %q", loc, tc.expect)
			}
			if strings.Contains(rr.Body.String(), `<div class="app-shell">`) {
				t.Fatalf("response must not contain full page layout")
			}
		})
	}
}

func TestExtensionSaveRedirectsWithoutRenderingLayout(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "extensions")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	s, _, _ := testWebServer(t)
	s.extDir = extDir
	cookie := loginAs(t, s, "admin")

	req := httptest.NewRequest(http.MethodPost, "/extensions/save",
		strings.NewReader("extension=201&password=secret&display_name=Test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	s.handleExtensionSave(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want 303", rr.Code)
	}
	if rr.Header().Get("Location") != "/extensions" {
		t.Fatalf("location=%q", rr.Header().Get("Location"))
	}
}

func TestFormsUsePostNotHTMX(t *testing.T) {
	files := []string{"server.go", "phonebook.go", "ui.go"}
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := string(data)
		if strings.Contains(src, "hx-post") || strings.Contains(src, "hx-target") {
			t.Fatalf("%s must not use hx-post/hx-target on forms", name)
		}
	}
}

func TestNonStatusPagesOmitHTMX(t *testing.T) {
	s, _, _ := testWebServer(t)
	cookie := loginAs(t, s, "viewer")

	req := httptest.NewRequest(http.MethodGet, "/extensions", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if strings.Contains(rr.Body.String(), "htmx.org") {
		t.Fatal("htmx should not load on non-status pages")
	}
}

func TestStatusFragmentIsPartialHTML(t *testing.T) {
	s, _, _ := testWebServer(t)
	body := s.statusLiveHTML(pbx.StatusReport{}, nil, "")
	if strings.Contains(body, "<!DOCTYPE html>") || strings.Contains(body, "app-shell") {
		t.Fatalf("fragment must not include layout:\n%s", body)
	}
	if !strings.Contains(body, `class="panel"`) {
		t.Fatal("fragment should include panel content")
	}
}

func TestStatusPageIncludesHTMX(t *testing.T) {
	page := strings.Replace(layout, "{{CONTENT}}", `<div id="status-live"></div>`, 1)
	page = strings.Replace(page, "{{VERSION}}", "test", 1)
	page = strings.Replace(page, "{{EXTRA_HEAD}}", htmxScript, 1)
	page = strings.Replace(page, "{{ADMIN_NAV}}", "", 1)
	if !strings.Contains(page, "htmx.org") {
		t.Fatal("status layout should include htmx when extra head set")
	}
}
