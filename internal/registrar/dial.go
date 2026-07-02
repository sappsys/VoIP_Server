package registrar

import (
	"time"

	"github.com/emiago/sipgo/sip"
)

// DialTarget returns the SIP URI, UDP/TCP destination (host:port), and transport for outbound INVITEs.
func (r *Registrar) DialTarget(extension string) (sip.Uri, string, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := r.bindings[extension]
	now := time.Now()
	for _, b := range list {
		if b.Expires.After(now) {
			uri, dest, transport := bindingToDialTarget(extension, b)
			return uri, dest, transport, true
		}
	}
	r.log.Debug("contact lookup miss", "extension", extension)
	return sip.Uri{}, "", "", false
}

func (r *Registrar) ContactURI(extension string) (sip.Uri, bool) {
	uri, _, _, ok := r.DialTarget(extension)
	return uri, ok
}

func bindingToDialTarget(extension string, b Binding) (sip.Uri, string, string) {
	uri := b.Contact.Address
	if uri.User == "" {
		uri.User = extension
	}
	transport := ""
	if b.Contact.Params != nil {
		if uri.UriParams == nil {
			uri.UriParams = sip.NewParams()
		}
		if t, ok := b.Contact.Params.Get("transport"); ok && t != "" {
			uri.UriParams.Add("transport", t)
			transport = t
		}
	}
	if transport == "" && uri.UriParams != nil {
		if t, ok := uri.UriParams.Get("transport"); ok {
			transport = t
		}
	}
	dest := uri.HostPort()
	return uri, dest, transport
}
