package registrar

import (
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
)

type Binding struct {
	Contact sip.ContactHeader
	Expires time.Time
}

type Registrar struct {
	realm    string
	exts     map[string]*config.Extension
	digest   *diago.DigestAuthServer
	mu       sync.RWMutex
	bindings map[string][]Binding
	log      *slog.Logger
}

func New(realm string, exts map[string]*config.Extension, log *slog.Logger) *Registrar {
	if log == nil {
		log = slog.Default()
	}
	return &Registrar{
		realm:    realm,
		exts:     exts,
		digest:   diago.NewDigestServer(),
		bindings: make(map[string][]Binding),
		log:      log,
	}
}

func (r *Registrar) UpdateExtensions(exts map[string]*config.Extension) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exts = exts
}

func (r *Registrar) Attach(server *sipgo.Server) {
	server.OnRegister(r.handleRegister)
}

func (r *Registrar) handleRegister(req *sip.Request, tx sip.ServerTransaction) {
	user := req.To().Address.User
	if user == "" {
		if from := req.From().Address.User; from != "" {
			user = from
		}
	}

	r.mu.RLock()
	ext, ok := r.exts[user]
	r.mu.RUnlock()
	if !ok || !ext.Enabled {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Not Found", nil))
		return
	}

	auth := diago.DigestAuth{Username: user, Password: ext.Password, Realm: r.realm}
	res, err := r.digest.AuthorizeRequest(req, auth)
	if err != nil || res.StatusCode != sip.StatusOK {
		if res != nil {
			_ = tx.Respond(res)
		}
		return
	}

	expiry := 3600 * time.Second
	if h := req.GetHeader("Expires"); h != nil {
		if sec, err := strconv.Atoi(h.Value()); err == nil && sec >= 0 {
			expiry = time.Duration(sec) * time.Second
		}
	}
	contact := req.Contact()
	if contact == nil {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil))
		return
	}

	if expiry == 0 {
		r.removeBinding(user, *contact)
	} else {
		r.addBinding(user, Binding{Contact: *contact.Clone(), Expires: time.Now().Add(expiry)})
	}

	okRes := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	if expiry > 0 {
		exp := sip.ExpiresHeader(expiry.Seconds())
		okRes.AppendHeader(&exp)
	}
	_ = tx.Respond(okRes)
}

func (r *Registrar) addBinding(user string, b Binding) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.bindings[user]
	updated := list[:0]
	for _, existing := range list {
		if existing.Contact.Address.String() != b.Contact.Address.String() {
			updated = append(updated, existing)
		}
	}
	updated = append(updated, b)
	r.bindings[user] = updated
}

func (r *Registrar) removeBinding(user string, contact sip.ContactHeader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.bindings[user]
	out := list[:0]
	target := contact.Address.String()
	for _, b := range list {
		if b.Contact.Address.String() != target {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		delete(r.bindings, user)
	} else {
		r.bindings[user] = out
	}
}

func (r *Registrar) ContactURI(extension string) (sip.Uri, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := r.bindings[extension]
	now := time.Now()
	for _, b := range list {
		if b.Expires.After(now) {
			uri := b.Contact.Address
			return uri, true
		}
	}
	return sip.Uri{}, false
}

func (r *Registrar) IsRegistered(extension string) bool {
	_, ok := r.ContactURI(extension)
	return ok
}

func (r *Registrar) RegisteredExtensions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.registeredExtensionsLocked(time.Now())
}

// RegisterForTest seeds a contact binding without a SIP REGISTER (tests only).
func (r *Registrar) RegisterForTest(ext string, uri sip.Uri) {
	r.addBinding(ext, Binding{
		Contact: sip.ContactHeader{Address: uri},
		Expires: time.Now().Add(24 * time.Hour),
	})
}

func (r *Registrar) registeredExtensionsLocked(now time.Time) []string {
	var out []string
	for ext, list := range r.bindings {
		for _, b := range list {
			if b.Expires.After(now) {
				out = append(out, ext)
				break
			}
		}
	}
	return out
}
