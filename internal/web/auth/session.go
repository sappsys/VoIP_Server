package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const cookieName = "voip_session"

type Sessions struct {
	secret []byte
	ttl    time.Duration
}

func NewSessions(secret string) *Sessions {
	return &Sessions{secret: []byte(secret), ttl: 24 * time.Hour}
}

func (s *Sessions) Set(w http.ResponseWriter, username string) {
	exp := time.Now().Add(s.ttl).Unix()
	payload := fmt.Sprintf("%s:%d", username, exp)
	sig := s.mac(payload)
	val := base64.StdEncoding.EncodeToString([]byte(payload + ":" + sig))
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: val, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func (s *Sessions) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
}

func (s *Sessions) Username(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(c.Value)
	if err != nil {
		return ""
	}
	parts := strings.SplitN(string(raw), ":", 3)
	if len(parts) != 3 {
		return ""
	}
	payload := parts[0] + ":" + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(s.mac(payload))) {
		return ""
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return ""
	}
	return parts[0]
}

func (s *Sessions) mac(payload string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(payload))
	return hex.EncodeToString(m.Sum(nil))
}
