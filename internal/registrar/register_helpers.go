package registrar

import (
	"strconv"
	"strings"

	"github.com/emiago/sipgo/sip"
)

// extractRegisterUser resolves the AOR from REGISTER (To, From, Contact, Request-URI).
func extractRegisterUser(req *sip.Request) string {
	if req == nil {
		return ""
	}
	if to := req.To(); to != nil && to.Address.User != "" {
		return to.Address.User
	}
	if from := req.From(); from != nil && from.Address.User != "" {
		return from.Address.User
	}
	if c := req.Contact(); c != nil && c.Address.User != "" {
		return c.Address.User
	}
	if req.Recipient.User != "" {
		return req.Recipient.User
	}
	return ""
}

func bindingDestFromContact(ext string, contact sip.ContactHeader) (string, bool) {
	_, dest, _ := bindingToDialTarget(ext, Binding{Contact: contact})
	return dest, dest != ""
}

func contactKey(uri sip.Uri) string {
	host := strings.ToLower(strings.TrimSpace(uri.Host))
	port := uri.Port
	if port == 0 {
		port = 5060
	}
	user := strings.ToLower(strings.TrimSpace(uri.User))
	return user + "@" + host + ":" + strconv.Itoa(port)
}

func contactMatchesStored(ext string, stored, incoming sip.ContactHeader, source string, preserve bool) bool {
	normalized := rewriteContactForNAT(incoming, source, preserve)
	if contactKey(stored.Address) == contactKey(normalized.Address) {
		return true
	}
	storedDest, ok := bindingDestFromContact(ext, stored)
	if !ok {
		return false
	}
	if source != "" && destinationsMatch(storedDest, source) {
		return true
	}
	return destinationsMatch(storedDest, normalized.Address.HostPort())
}

func isWildcardContact(contact sip.ContactHeader) bool {
	return contact.Address.Wildcard || strings.TrimSpace(contact.Address.String()) == "*"
}

func buildRegisterOK(req *sip.Request, stored sip.ContactHeader, expirySec int) *sip.Response {
	okRes := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	if expirySec <= 0 {
		return okRes
	}
	c := stored.Clone()
	if c.Params == nil {
		c.Params = sip.NewParams()
	}
	c.Params.Add("expires", strconv.Itoa(expirySec))
	okRes.AppendHeader(c)
	exp := sip.ExpiresHeader(float64(expirySec))
	okRes.AppendHeader(&exp)
	return okRes
}
