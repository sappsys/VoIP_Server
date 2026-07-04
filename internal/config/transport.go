package config

import (
	"fmt"
	"strings"
)

var allowedSIPTransports = map[string]bool{
	"udp": true,
	"tcp": true,
	"tls": true,
	"ws":  true,
	"wss": true,
}

// SIPTransports returns the SIP listener transports to bind (e.g. udp, tcp).
// Uses transports when set; otherwise transport (single value or comma-separated).
func (s *ServerConfig) SIPTransports() []string {
	if len(s.Transports) > 0 {
		return normalizeSIPTransports(s.Transports)
	}
	if s.Transport == "" {
		return []string{"udp"}
	}
	if strings.Contains(s.Transport, ",") {
		return normalizeSIPTransports(strings.Split(s.Transport, ","))
	}
	return []string{strings.ToLower(strings.TrimSpace(s.Transport))}
}

// DefaultSIPTransport is used for outbound signaling when the peer did not specify transport.
func (s *ServerConfig) DefaultSIPTransport() string {
	transports := s.SIPTransports()
	if len(transports) == 0 {
		return "udp"
	}
	return transports[0]
}

func normalizeSIPTransports(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		t := strings.ToLower(strings.TrimSpace(raw))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) == 0 {
		return []string{"udp"}
	}
	return out
}

func validateSIPTransports(transports []string) error {
	if len(transports) == 0 {
		return fmt.Errorf("at least one sip transport required")
	}
	for _, t := range transports {
		if !allowedSIPTransports[t] {
			return fmt.Errorf("unsupported sip transport %q (use udp, tcp, tls, ws, or wss)", t)
		}
	}
	return nil
}
