package presence

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/registrar"
	"github.com/sappsys/VoIP_Server/internal/router"
)

const notifyTimeout = 5 * time.Second

// StateResolver returns live presence for an extension.
type StateResolver func(ext string) State

// Handler implements SIP SUBSCRIBE/NOTIFY presence (RFC 3856 / RFC 3265).
type Handler struct {
	reg      *registrar.Registrar
	mu       sync.RWMutex
	exts     map[string]*config.Extension
	features router.FeatureCodes
	domain   string
	resolve  StateResolver
	subs          *SubscriptionStore
	client        *sipgo.Client
	notifyClient  *sipgo.Client
	notifyUA      *sipgo.UserAgent
	log           *slog.Logger
}

func New(reg *registrar.Registrar, exts map[string]*config.Extension, features router.FeatureCodes, domain string, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	if domain == "" {
		domain = "localhost"
	}
	return &Handler{
		reg:      reg,
		exts:     exts,
		features: features,
		domain:   domain,
		subs:     NewSubscriptionStore(),
		log:      log,
	}
}

func (h *Handler) Attach(server *sipgo.Server) {
	server.OnSubscribe(h.handleSubscribe)
}

func (h *Handler) SetClient(client *sipgo.Client) {
	h.client = client
}

// InitNotifySender creates a dedicated UA for outbound NOTIFY to avoid Call-ID
// collisions with SUBSCRIBE dialogs on the main PBX user agent.
func (h *Handler) InitNotifySender(ctx context.Context) error {
	if h.notifyClient != nil {
		return nil
	}
	ua, err := sipgo.NewUA()
	if err != nil {
		return err
	}
	client, err := sipgo.NewClient(ua, sipgo.WithClientHostname(h.domain))
	if err != nil {
		ua.Close()
		return err
	}
	h.notifyUA = ua
	h.notifyClient = client
	go func() {
		<-ctx.Done()
		ua.Close()
	}()
	return nil
}

func (h *Handler) SetStateResolver(fn StateResolver) {
	h.resolve = fn
}

func (h *Handler) UpdateExtensions(exts map[string]*config.Extension) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.exts = exts
}

func (h *Handler) UpdateFeatures(features router.FeatureCodes) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.features = features
}

func (h *Handler) RunBackground(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if removed := h.subs.PruneExpired(); removed > 0 {
				h.log.Debug("presence subscription gc", "removed", removed)
			}
		}
	}
}

func (h *Handler) NotifyExtension(ext string) {
	for _, sub := range h.subs.ForWatched(ext) {
		h.notifySubscription(sub, h.stateFor(ext), "active", time.Until(sub.Expires))
	}
}

func (h *Handler) NotifyExtensions(exts ...string) {
	seen := make(map[string]struct{}, len(exts))
	for _, ext := range exts {
		if ext == "" {
			continue
		}
		if _, ok := seen[ext]; ok {
			continue
		}
		seen[ext] = struct{}{}
		h.NotifyExtension(ext)
	}
}

func (h *Handler) handleSubscribe(req *sip.Request, tx sip.ServerTransaction) {
	watcher := extensionFromHeader(req.From())
	watched := extensionFromHeader(req.To())
	source := req.Source()
	h.log.Debug("subscribe request", "watcher", watcher, "watched", watched, "source", source)

	if watcher == "" || watched == "" {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil))
		return
	}
	if !h.reg.SenderAuthorized(watcher, source) {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
		return
	}
	if !h.extensionEnabled(watcher) {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
		return
	}

	event := eventValue(req)
	if event != "" && event != "presence" {
		_ = tx.Respond(sip.NewResponseFromRequest(req, 489, "Bad Event", nil))
		return
	}

	route := router.RouteDial(watched, h.features)
	if route.Kind != router.KindExtension || !h.extensionEnabled(watched) {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Not Found", nil))
		return
	}

	expiry := parseSubscribeExpiry(req)
	callID := ""
	if cid := req.CallID(); cid != nil {
		callID = cid.Value()
	}
	fromTag := ""
	if from := req.From(); from != nil && from.Params != nil {
		fromTag, _ = from.Params.Get("tag")
	}
	if callID == "" || fromTag == "" {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil))
		return
	}

	if expiry == 0 {
		if sub := h.subs.Remove(callID, fromTag); sub != nil {
			h.notifySubscription(sub, h.stateFor(watched), "terminated;reason=deactivated", 0)
		}
		res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
		res.AppendHeader(sip.NewHeader("Expires", "0"))
		_ = tx.Respond(res)
		return
	}
	if expiry > defaultMaxExpiry {
		expiry = defaultMaxExpiry
	}
	if expiry < time.Minute {
		expiry = time.Minute
	}

	contact := req.Contact()
	if contact == nil {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil))
		return
	}

	notifyDest := req.Source()
	if notifyDest == "" {
		notifyDest = contact.Address.HostPort()
	}
	localToTag := sip.GenerateTagN(16)
	sub := &Subscription{
		CallID:        callID,
		WatcherFrom:   fromTag,
		LocalToTag:    localToTag,
		Watcher:       watcher,
		Watched:       watched,
		Event:         "presence",
		Expires:       time.Now().Add(expiry),
		NotifyContact: contact.Address,
		NotifyDest:    notifyDest,
	}
	h.subs.Upsert(sub)

	res := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	if to := res.To(); to != nil {
		if to.Params == nil {
			to.Params = sip.NewParams()
		}
		to.Params.Add("tag", localToTag)
	}
	res.AppendHeader(sip.NewHeader("Expires", strconv.Itoa(int(expiry.Seconds()))))
	_ = tx.Respond(res)

	h.notifySubscription(sub, h.stateFor(watched), "active", expiry)
}

func (h *Handler) notifySubscription(sub *Subscription, state State, subState string, expiresIn time.Duration) {
	client := h.notifyClient
	if client == nil {
		client = h.client
	}
	if client == nil {
		return
	}

	sub.CSeq++
	recipient := sub.NotifyContact
	dest := sub.NotifyDest
	if uri, dialDest, _, ok := h.reg.DialTarget(sub.Watcher); ok {
		recipient = uri
		dest = dialDest
	}
	req := sip.NewRequest(sip.NOTIFY, recipient)
	req.SetDestination(dest)

	from := &sip.FromHeader{
		Address: sip.Uri{User: sub.Watched, Host: h.domain},
		Params:  sip.NewParams(),
	}
	from.Params.Add("tag", sub.LocalToTag)
	to := &sip.ToHeader{
		Address: sip.Uri{User: sub.Watcher, Host: h.domain},
		Params:  sip.NewParams(),
	}
	to.Params.Add("tag", sub.WatcherFrom)
	req.AppendHeader(from)
	req.AppendHeader(to)
	req.AppendHeader(sip.NewHeader("Call-ID", sub.CallID))
	req.AppendHeader(&sip.CSeqHeader{SeqNo: sub.CSeq, MethodName: sip.NOTIFY})
	req.AppendHeader(sip.NewHeader("Event", sub.Event))
	if expiresIn > 0 && strings.HasPrefix(subState, "active") {
		subState = fmt.Sprintf("active;expires=%d", int(expiresIn.Seconds()))
	}
	req.AppendHeader(sip.NewHeader("Subscription-State", subState))
	req.AppendHeader(sip.NewHeader("Content-Type", "application/pidf+xml"))
	entity := entityURI(sub.Watched, h.domain)
	req.SetBody(pidfXML(entity, sub.Watched, state.Basic, state.DisplayName))

	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	if _, err := client.Do(ctx, req); err != nil {
		h.log.Debug("presence notify failed", "watcher", sub.Watcher, "watched", sub.Watched, "error", err)
		return
	}
	h.log.Debug("presence notify ok", "watcher", sub.Watcher, "watched", sub.Watched, "basic", state.Basic)
}

func (h *Handler) stateFor(ext string) State {
	if h.resolve != nil {
		return h.resolve(ext)
	}
	if h.reg.IsRegistered(ext) {
		return State{Basic: BasicOpen}
	}
	return State{Basic: BasicClosed}
}

func parseSubscribeExpiry(req *sip.Request) time.Duration {
	if h := req.GetHeader("Expires"); h != nil {
		if sec, err := strconv.Atoi(strings.TrimSpace(h.Value())); err == nil && sec >= 0 {
			return time.Duration(sec) * time.Second
		}
	}
	return time.Hour
}

func eventValue(req *sip.Request) string {
	if h := req.GetHeader("Event"); h != nil {
		return strings.TrimSpace(strings.Split(h.Value(), ";")[0])
	}
	return ""
}

func extensionFromHeader(hdr sip.Header) string {
	switch h := hdr.(type) {
	case *sip.FromHeader:
		return h.Address.User
	case *sip.ToHeader:
		return h.Address.User
	default:
		return ""
	}
}

func (h *Handler) extensionEnabled(ext string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	e, ok := h.exts[ext]
	return ok && e.Enabled
}
