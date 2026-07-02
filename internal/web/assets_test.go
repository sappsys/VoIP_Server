package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebStaticAssets(t *testing.T) {
	s, _, _ := testWebServer(t)
	h := s.Handler()

	cases := []struct {
		path    string
		ctype   string
		contain string
	}{
		{"/web/favicon.svg", "image/svg+xml", "<svg"},
		{"/web/logo.svg", "image/svg+xml", "VoIP PBX"},
		{"/web/icon-192.png", "image/png", ""},
		{"/web/icon-512.png", "image/png", ""},
		{"/web/apple-touch-icon.png", "image/png", ""},
		{"/web/manifest.webmanifest", "application/manifest+json", "VoIP PBX"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.ctype) {
				t.Fatalf("content-type=%q want prefix %q", ct, tc.ctype)
			}
			if tc.contain != "" && !strings.Contains(rr.Body.String(), tc.contain) {
				t.Fatalf("body missing %q", tc.contain)
			}
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/web/missing.png", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing asset status=%d", rr.Code)
	}

	rr2 := httptest.NewRecorder()
	bad := httptest.NewRequest(http.MethodGet, "/web/", nil)
	bad.URL.Path = "/web/../favicon.svg"
	s.handleWebStatic(rr2, bad)
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("path traversal status=%d", rr2.Code)
	}
}
