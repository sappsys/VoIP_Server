package web

import (
	"net/http"
	"strings"

	"github.com/sappsys/VoIP_Server/internal/store"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

func NormalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case RoleUser:
		return RoleUser
	default:
		return RoleAdmin
	}
}

func (s *Server) currentUser(r *http.Request) *store.WebUser {
	username := s.sessions.Username(r)
	if username == "" {
		return nil
	}
	u, err := s.store.GetWebUserByUsername(username)
	if err != nil || u == nil {
		return nil
	}
	u.Role = NormalizeRole(u.Role)
	return u
}

func (s *Server) isAdmin(r *http.Request) bool {
	u := s.currentUser(r)
	return u != nil && u.Role == RoleAdmin
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.isAdmin(r) {
		return true
	}
	http.Error(w, "admin access required", http.StatusForbidden)
	return false
}
