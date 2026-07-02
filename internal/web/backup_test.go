package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBackupPageRequiresAdmin(t *testing.T) {
	s, _, _ := testWebServer(t)
	h := s.Handler()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/backup", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("unauth status=%d", rr.Code)
	}

	cookie := loginAs(t, s, "admin")
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/backup", nil)
	req.AddCookie(cookie)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Download backup") {
		t.Fatal("missing backup UI")
	}
}
