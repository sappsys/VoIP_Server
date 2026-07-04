package registrar

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

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
	digest              *registerDigestAuth
	mu                  sync.RWMutex
	bindings            map[string][]Binding
	log                 *slog.Logger
	minExpiry           time.Duration
	maxExpiry           time.Duration
	preserveContactHost bool
	optionsKeepalive    time.Duration
	onBindingChange     func(extension string)
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
		digest:              newRegisterDigestAuth(maxExpiry, log),
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

func (r *Registrar) SetOnBindingChange(fn func(extension string)) {
	r.onBindingChange = fn
}

// RunBackground starts registration maintenance (OPTIONS keepalive and expiry GC).
func (r *Registrar) RunBackground(ctx context.Context, client *sipgo.Client) {
	if client != nil && r.optionsKeepalive > 0 {
		go r.runOptionsKeepalive(ctx, client)
	}
	go r.runExpiryGC(ctx)
}

func (r *Registrar) handleRegister(req *sip.Request, tx sip.ServerTransaction) {
	user := extractRegisterUser(req)
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

	decision := r.digest.authorize(req, r.realm, user, ext.Password)
	switch decision.outcome {
	case authOK:
		// continue below
	case authChallenge:
		r.log.Debug("register digest challenge", "user", user, "source", source, "reason", decision.reason)
		if decision.res != nil {
			_ = tx.Respond(decision.res)
		}
		return
	default:
		r.log.Info("register auth denied", "user", user, "source", source, "reason", decision.reason)
		if decision.res != nil {
			_ = tx.Respond(decision.res)
		}
		return
	}

	expiry := r.parseRequestedExpiry(req)
	contact := req.Contact()
	if contact == nil {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil))
		return
	}

	var stored sip.ContactHeader
	grantedSec := 0

	if expiry == 0 || isWildcardContact(*contact) {
		r.removeBinding(user, *contact, source)
		r.log.Info("register removed", "user", user, "contact", contact.Address.String(), "source", source)
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
		stored = rewriteContactForNAT(*contact.Clone(), source, r.preserveContactHost)
		grantedSec = int(expiry.Seconds())
		r.addBinding(user, source, Binding{
			Contact: stored,
			Expires: time.Now().Add(expiry),
			granted: expiry,
		})
		r.log.Info("register ok", "user", user, "contact", stored.Address.String(), "expires_sec", grantedSec, "source", source)
	}

	_ = tx.Respond(buildRegisterOK(req, stored, grantedSec))
}

func (r *Registrar) handleOptions(req *sip.Request, tx sip.ServerTransaction) {
	user := ""
	if req.From() != nil {
		user = req.From().Address.User
	}
	if user == "" {
		user = extractRegisterUser(req)
	}
	source := req.Source()
	r.log.Debug("options request", "user", user, "source", source)
	if user != "" {
		if contact := req.Contact(); contact != nil {
			r.touchBinding(user, *contact, source)
		} else {
			r.touchBindingBySource(user, source)
		}
	}
	res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	allow := sip.NewHeader("Allow", "INVITE, ACK, CANCEL, BYE, OPTIONS, REGISTER, REFER, NOTIFY, INFO, UPDATE, MESSAGE, SUBSCRIBE")
	allowEvents := sip.NewHeader("Allow-Events", "presence")
	res.AppendHeader(allow)
	res.AppendHeader(allowEvents)
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

func (r *Registrar) addBinding(user, source string, b Binding) {
	r.mu.Lock()
	dest, _ := bindingDestFromContact(user, b.Contact)
	list := r.bindings[user]
	out := make([]Binding, 0, len(list)+1)
	for _, existing := range list {
		if contactMatchesStored(user, existing.Contact, b.Contact, source, r.preserveContactHost) {
			continue
		}
		existingDest, ok := bindingDestFromContact(user, existing.Contact)
		if ok && dest != "" && destinationsMatch(existingDest, dest) {
			continue
		}
		out = append(out, existing)
	}
	out = append(out, b)
	r.bindings[user] = out
	r.mu.Unlock()
	r.fireBindingChange(user)
}

func (r *Registrar) removeBinding(user string, contact sip.ContactHeader, source string) {
	r.mu.Lock()
	if isWildcardContact(contact) {
		delete(r.bindings, user)
		r.mu.Unlock()
		r.fireBindingChange(user)
		return
	}
	list := r.bindings[user]
	out := list[:0]
	for _, b := range list {
		if contactMatchesStored(user, b.Contact, contact, source, r.preserveContactHost) {
			continue
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		delete(r.bindings, user)
	} else {
		r.bindings[user] = out
	}
	r.mu.Unlock()
	r.fireBindingChange(user)
}

func (r *Registrar) fireBindingChange(user string) {
	if r.onBindingChange != nil {
		r.onBindingChange(user)
	}
}

func (r *Registrar) touchBinding(user string, contact sip.ContactHeader, source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.bindings[user]
	if len(list) == 0 {
		return
	}
	for i, b := range list {
		if !contactMatchesStored(user, b.Contact, contact, source, r.preserveContactHost) {
			continue
		}
		refresh := b.granted
		if refresh <= 0 {
			refresh = r.maxExpiry
		}
		list[i].Expires = time.Now().Add(refresh)
		if !r.preserveContactHost && source != "" {
			list[i].Contact = rewriteContactForNAT(contact, source, false)
		}
		r.bindings[user] = list
		r.log.Debug("register refreshed via options", "user", user, "source", source, "expires_sec", int(refresh.Seconds()))
		return
	}
}

func (r *Registrar) touchBindingBySource(user, source string) {
	if source == "" {
		r.touchBindingByUser(user)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.bindings[user]
	for i, b := range list {
		dest, ok := bindingDestFromContact(user, b.Contact)
		if !ok || !destinationsMatch(dest, source) {
			continue
		}
		refresh := b.granted
		if refresh <= 0 {
			refresh = r.maxExpiry
		}
		list[i].Expires = time.Now().Add(refresh)
		r.bindings[user] = list
		r.log.Debug("register refreshed via options", "user", user, "source", source, "expires_sec", int(refresh.Seconds()))
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
	r.addBinding(ext, "", Binding{
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
	now := time.Now()
	removed := 0
	changed := make(map[string]struct{})
	for user, list := range r.bindings {
		out := list[:0]
		for _, b := range list {
			if b.Expires.After(now) {
				out = append(out, b)
			} else {
				removed++
				changed[user] = struct{}{}
				r.log.Debug("registration expired", "user", user, "contact", b.Contact.Address.String())
			}
		}
		if len(out) == 0 {
			delete(r.bindings, user)
		} else {
			r.bindings[user] = out
		}
	}
	r.mu.Unlock()
	for user := range changed {
		r.fireBindingChange(user)
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
					r.touchBinding(ext, b.Contact, uri.HostPort())
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

