package registrar

import (
	"context"
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
	// granted is the server-accepted registration lifetime (used to refresh on OPTIONS).
	granted time.Duration
}

type Registrar struct {
	realm               string
	exts                map[string]*config.Extension
	digest              *diago.DigestAuthServer
	mu                  sync.RWMutex
	bindings            map[string][]Binding
	log                 *slog.Logger
	minExpiry           time.Duration
	maxExpiry           time.Duration
	preserveContactHost bool
	optionsKeepalive    time.Duration
}

func New(realm string, srv config.ServerConfig, exts map[string]*config.Extension, log *slog.Logger) *Registrar {
	if log == nil {
		log = slog.Default()
	}
	minExpiry := time.Duration(srv.RegisterMinExpiry) * time.Second
	if srv.RegisterMinExpiry <= 0 {
		minExpiry = 60 * time.Second
	}
	maxExpiry := time.Duration(srv.RegisterMaxExpiry) * time.Second
	if srv.RegisterMaxExpiry <= 0 {
		maxExpiry = 3600 * time.Second
	}
	keepalive := time.Duration(srv.OptionsKeepaliveSeconds) * time.Second
	if srv.OptionsKeepaliveSeconds <= 0 {
		keepalive = 30 * time.Second
	}
	return &Registrar{
		realm:               realm,
		exts:                exts,
		digest:              diago.NewDigestServer(),
		bindings:            make(map[string][]Binding),
		log:                 log,
		minExpiry:           minExpiry,
		maxExpiry:           maxExpiry,
		preserveContactHost: srv.PreserveContactHost,
		optionsKeepalive:    keepalive,
	}
}

func (r *Registrar) UpdateExtensions(exts map[string]*config.Extension) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exts = exts
}

func (r *Registrar) Attach(server *sipgo.Server) {
	server.OnRegister(r.handleRegister)
	server.OnOptions(r.handleOptions)
}

// RunBackground starts registration maintenance (OPTIONS keepalive and expiry GC).
func (r *Registrar) RunBackground(ctx context.Context, client *sipgo.Client) {
	if client != nil && r.optionsKeepalive > 0 {
		go r.runOptionsKeepalive(ctx, client)
	}
	go r.runExpiryGC(ctx)
}

func (r *Registrar) handleRegister(req *sip.Request, tx sip.ServerTransaction) {
	user := req.To().Address.User
	if user == "" {
		if from := req.From().Address.User; from != "" {
			user = from
		}
	}
	source := req.Source()
	r.log.Debug("register request", "user", user, "source", source)

	r.mu.RLock()
	ext, ok := r.exts[user]
	r.mu.RUnlock()
	if !ok || !ext.Enabled {
		r.log.Debug("register rejected", "user", user, "reason", "unknown_or_disabled")
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Not Found", nil))
		return
	}

	auth := diago.DigestAuth{Username: user, Password: ext.Password, Realm: r.realm}
	res, err := r.digest.AuthorizeRequest(req, auth)
	if err != nil || res.StatusCode != sip.StatusOK {
		r.log.Debug("register auth failed", "user", user, "status", statusCode(res))
		if res != nil {
			_ = tx.Respond(res)
		}
		return
	}

	expiry := r.parseRequestedExpiry(req)
	contact := req.Contact()
	if contact == nil {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil))
		return
	}

	if expiry == 0 {
		r.removeBinding(user, *contact)
		r.log.Info("register removed", "user", user, "contact", contact.Address.String())
	} else {
		if expiry < r.minExpiry {
			r.log.Debug("register expiry too short", "user", user, "requested", expiry, "min", r.minExpiry)
			res := sip.NewResponseFromRequest(req, sip.StatusIntervalToBrief, "Interval Too Brief", nil)
			min := sip.NewHeader("Min-Expires", strconv.Itoa(int(r.minExpiry.Seconds())))
			res.AppendHeader(min)
			_ = tx.Respond(res)
			return
		}
		if expiry > r.maxExpiry {
			expiry = r.maxExpiry
		}
		if expiry <= 0 {
			expiry = r.minExpiry
		}
		stored := rewriteContactForNAT(*contact.Clone(), source, r.preserveContactHost)
		r.addBinding(user, Binding{
			Contact: stored,
			Expires: time.Now().Add(expiry),
			granted: expiry,
		})
		r.log.Info("register ok", "user", user, "contact", stored.Address.String(), "expires_sec", int(expiry.Seconds()), "source", source)
	}

	okRes := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	if expiry > 0 {
		exp := sip.ExpiresHeader(expiry.Seconds())
		okRes.AppendHeader(&exp)
	}
	_ = tx.Respond(okRes)
}

func (r *Registrar) handleOptions(req *sip.Request, tx sip.ServerTransaction) {
	user := ""
	if req.From() != nil {
		user = req.From().Address.User
	}
	r.log.Debug("options request", "user", user, "source", req.Source())
	if user != "" {
		if contact := req.Contact(); contact != nil {
			r.touchBinding(user, *contact)
		} else {
			r.touchBindingByUser(user)
		}
	}
	res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	allow := sip.NewHeader("Allow", "INVITE, ACK, CANCEL, BYE, OPTIONS, REGISTER, REFER, NOTIFY, INFO, UPDATE")
	res.AppendHeader(allow)
	_ = tx.Respond(res)
}

func (r *Registrar) parseRequestedExpiry(req *sip.Request) time.Duration {
	expiry := 3600 * time.Second
	if contact := req.Contact(); contact != nil && contact.Params != nil {
		if v, ok := contact.Params.Get("expires"); ok {
			if sec, err := strconv.Atoi(v); err == nil && sec >= 0 {
				expiry = time.Duration(sec) * time.Second
			}
		}
	}
	if h := req.GetHeader("Expires"); h != nil {
		if sec, err := strconv.Atoi(h.Value()); err == nil && sec >= 0 {
			expiry = time.Duration(sec) * time.Second
		}
	}
	return expiry
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

func (r *Registrar) touchBinding(user string, contact sip.ContactHeader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.bindings[user]
	target := contact.Address.String()
	for i, b := range list {
		if b.Contact.Address.String() != target {
			continue
		}
		refresh := b.granted
		if refresh <= 0 {
			refresh = r.maxExpiry
		}
		list[i].Expires = time.Now().Add(refresh)
		r.bindings[user] = list
		r.log.Debug("register refreshed via options", "user", user, "contact", target, "expires_sec", int(refresh.Seconds()))
		return
	}
}

func (r *Registrar) touchBindingByUser(user string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.bindings[user]
	for i := range list {
		refresh := list[i].granted
		if refresh <= 0 {
			refresh = r.maxExpiry
		}
		list[i].Expires = time.Now().Add(refresh)
	}
	if len(list) > 0 {
		r.bindings[user] = list
		r.log.Debug("register refreshed via options", "user", user, "bindings", len(list))
	}
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
		granted: 24 * time.Hour,
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

func (r *Registrar) runExpiryGC(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed := r.pruneExpired()
			if removed > 0 {
				r.log.Debug("registration gc", "removed_bindings", removed)
			}
		}
	}
}

func (r *Registrar) pruneExpired() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	removed := 0
	for user, list := range r.bindings {
		out := list[:0]
		for _, b := range list {
			if b.Expires.After(now) {
				out = append(out, b)
			} else {
				removed++
				r.log.Debug("registration expired", "user", user, "contact", b.Contact.Address.String())
			}
		}
		if len(out) == 0 {
			delete(r.bindings, user)
		} else {
			r.bindings[user] = out
		}
	}
	return removed
}

func (r *Registrar) runOptionsKeepalive(ctx context.Context, client *sipgo.Client) {
	ticker := time.NewTicker(r.optionsKeepalive)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sendOptionsKeepalive(ctx, client)
		}
	}
}

func (r *Registrar) sendOptionsKeepalive(ctx context.Context, client *sipgo.Client) {
	snapshot := r.snapshotBindings()
	if len(snapshot) == 0 {
		return
	}
	r.log.Debug("options keepalive cycle", "bindings", len(snapshot))
	for ext, list := range snapshot {
		for _, b := range list {
			uri := b.Contact.Address
			req := sip.NewRequest(sip.OPTIONS, uri)
			req.SetDestination(uri.HostPort())
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			tx, err := client.TransactionRequest(pingCtx, req)
			cancel()
			if err != nil {
				r.log.Debug("options keepalive failed", "extension", ext, "uri", uri.String(), "error", err)
				continue
			}
			select {
			case res := <-tx.Responses():
				if res != nil && res.StatusCode >= 200 && res.StatusCode < 300 {
					r.touchBinding(ext, b.Contact)
					r.log.Debug("options keepalive ok", "extension", ext, "uri", uri.String(), "status", res.StatusCode)
				} else {
					code := 0
					if res != nil {
						code = res.StatusCode
					}
					r.log.Debug("options keepalive bad status", "extension", ext, "uri", uri.String(), "status", code)
				}
			case <-time.After(5 * time.Second):
				r.log.Debug("options keepalive timeout", "extension", ext, "uri", uri.String())
			}
		}
	}
}

func (r *Registrar) snapshotBindings() map[string][]Binding {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	out := make(map[string][]Binding, len(r.bindings))
	for ext, list := range r.bindings {
		var live []Binding
		for _, b := range list {
			if b.Expires.After(now) {
				live = append(live, b)
			}
		}
		if len(live) > 0 {
			out[ext] = live
		}
	}
	return out
}

func statusCode(res *sip.Response) int {
	if res == nil {
		return 0
	}
	return res.StatusCode
}
