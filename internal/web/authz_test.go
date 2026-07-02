package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/store"
)

func loginAs(t *testing.T, s *Server, username string) *http.Cookie {
	t.Helper()
	hash, err := store.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	role := RoleAdmin
	if username == "viewer" {
		role = RoleUser
	}
	if err := s.store.UpsertWebUser(username, hash, role); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"username": {username}, "password": {"secret"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("login status=%d", rr.Code)
	}
	resp := rr.Result()
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "voip_session" {
			return c
		}
	}
	t.Fatal("session cookie missing")
	return nil
}

func TestNormalizeRole(t *testing.T) {
	if NormalizeRole("user") != RoleUser {
		t.Fatal("user role")
	}
	if NormalizeRole("ADMIN") != RoleAdmin {
		t.Fatal("admin role")
	}
	if NormalizeRole("bogus") != RoleAdmin {
		t.Fatal("default admin")
	}
}

func TestViewOnlyCannotMutate(t *testing.T) {
	s, _, _ := testWebServer(t)
	cookie := loginAs(t, s, "viewer")
	h := s.Handler()

	req := httptest.NewRequest(http.MethodPost, "/extensions/save", strings.NewReader("extension=101&password=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("save status=%d want 403", rr.Code)
	}
}

func TestViewOnlyHidesUsersNav(t *testing.T) {
	s, _, _ := testWebServer(t)
	cookie := loginAs(t, s, "viewer")

	req := httptest.NewRequest(http.MethodGet, "/extensions", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	body := rr.Body.String()
	if strings.Contains(body, `href="/users"`) {
		t.Fatal("users nav should be hidden for view-only user")
	}
	if strings.Contains(body, "Save extension") {
		t.Fatal("extension form should be hidden for view-only user")
	}
}

func TestAdminUsersFormHasRoleDropdown(t *testing.T) {
	s, _, _ := testWebServer(t)
	cfg := &config.Config{Web: config.WebConfig{SessionSecret: "secret"}}
	s.cfg = cfg
	cookie := loginAs(t, s, "admin")

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	s.handleUsers(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `<select name="role"`) {
		t.Fatal("role dropdown missing")
	}
	if !strings.Contains(body, "User (view only)") {
		t.Fatal("user option missing")
	}
}
