package registrar

import (
	"strings"
	"time"
)

// SenderAuthorized reports whether extension is registered and the packet source
// matches a live binding (prevents spoofed From on MESSAGE).
func (r *Registrar) SenderAuthorized(extension, source string) bool {
	if extension == "" || source == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := r.bindings[extension]
	now := time.Now()
	for _, b := range list {
		if !b.Expires.After(now) {
			continue
		}
		_, dest, _ := bindingToDialTarget(extension, b)
		if destinationsMatch(dest, source) {
			return true
		}
	}
	return false
}

func destinationsMatch(a, b string) bool {
	return strings.EqualFold(normalizeDest(a), normalizeDest(b))
}

func normalizeDest(dest string) string {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return ""
	}
	if !strings.Contains(dest, ":") {
		return dest + ":5060"
	}
	return dest
}
