package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionSetAndRead(t *testing.T) {
	s := NewSessions("test-secret-key")
	rr := httptest.NewRecorder()
	s.Set(rr, "admin")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rr.Result().Cookies() {
		req.AddCookie(c)
	}
	if user := s.Username(req); user != "admin" {
		t.Fatalf("username=%q want admin", user)
	}
}

func TestSessionClear(t *testing.T) {
	s := NewSessions("test-secret-key")
	setRR := httptest.NewRecorder()
	s.Set(setRR, "admin")

	clearRR := httptest.NewRecorder()
	s.Clear(clearRR)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range clearRR.Result().Cookies() {
		req.AddCookie(c)
	}
	if user := s.Username(req); user != "" {
		t.Fatalf("expected empty user after clear, got %q", user)
	}
}

func TestSessionTamperRejected(t *testing.T) {
	s := NewSessions("test-secret-key")
	rr := httptest.NewRecorder()
	s.Set(rr, "admin")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rr.Result().Cookies() {
		c.Value = c.Value + "tampered"
		req.AddCookie(c)
	}
	if user := s.Username(req); user != "" {
		t.Fatal("tampered cookie should be rejected")
	}
}

func TestSessionWrongSecret(t *testing.T) {
	s1 := NewSessions("secret-a")
	s2 := NewSessions("secret-b")
	rr := httptest.NewRecorder()
	s1.Set(rr, "admin")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rr.Result().Cookies() {
		req.AddCookie(c)
	}
	if user := s2.Username(req); user != "" {
		t.Fatal("wrong secret should not validate cookie")
	}
}
