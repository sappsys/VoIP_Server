package registrar

import (
	"net"
	"strconv"
	"strings"

	"github.com/emiago/sipgo/sip"
)

// rewriteContactForNAT stores the packet source for return routing (symmetric SIP).
// Phones often put a private or incorrect address in Contact; the source address is
// where responses and outbound requests must be sent.
func rewriteContactForNAT(contact sip.ContactHeader, source string, preserve bool) sip.ContactHeader {
	if preserve || source == "" {
		return contact
	}
	srcHost, srcPort := parseHostPort(source)
	if srcHost == "" || srcPort == 0 {
		return contact
	}
	addr := contact.Address
	out := contact
	out.Address = addr
	if addr.Host != srcHost || addr.Port != srcPort {
		out.Address.Host = srcHost
		out.Address.Port = srcPort
	}
	return out
}

func parseHostPort(source string) (string, int) {
	if source == "" {
		return "", 0
	}
	host, portStr, err := net.SplitHostPort(source)
	if err != nil {
		return strings.TrimSpace(source), 5060
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port == 0 {
		port = 5060
	}
	return host, port
}

func isPrivateIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}
