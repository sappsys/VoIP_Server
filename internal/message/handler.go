package message

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
	"github.com/sappsys/VoIP_Server/internal/registrar"
	"github.com/sappsys/VoIP_Server/internal/router"
	"github.com/sappsys/VoIP_Server/internal/store"
)

const relayTimeout = 10 * time.Second

// Handler relays SIP MESSAGE requests between registered extensions.
type Handler struct {
	reg      *registrar.Registrar
	store    *store.Store
	mu       sync.RWMutex
	exts     map[string]*config.Extension
	features router.FeatureCodes
	domain   string
	client   *sipgo.Client
	log      *slog.Logger
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
		log:      log,
	}
}

func (h *Handler) SetStore(st *store.Store) {
	h.store = st
}

func (h *Handler) Attach(server *sipgo.Server) {
	server.OnMessage(h.handleMessage)
}

func (h *Handler) SetClient(client *sipgo.Client) {
	h.client = client
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

func (h *Handler) handleMessage(req *sip.Request, tx sip.ServerTransaction) {
	from := extensionFromHeader(req.From())
	to := extensionFromHeader(req.To())
	source := req.Source()
	h.log.Debug("message received", "from", from, "to", to, "source", source, "bytes", len(req.Body()))

	if from == "" || to == "" {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil))
		return
	}
	if !h.reg.SenderAuthorized(from, source) {
		h.log.Debug("message rejected", "from", from, "reason", "unauthorized")
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
		return
	}
	if !h.extensionEnabled(from) {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil))
		return
	}

	route := router.RouteDial(to, h.features)
	if route.Kind != router.KindExtension {
		h.log.Debug("message rejected", "to", to, "reason", "not_extension")
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Not Found", nil))
		return
	}
	target := route.Target
	if !h.extensionEnabled(target) {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Not Found", nil))
		return
	}
	if h.extensionDND(target) {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusTemporarilyUnavailable, "Do Not Disturb", nil))
		return
	}

	uri, dest, _, ok := h.reg.DialTarget(target)
	if !ok {
		if h.store != nil {
			ct := contentType(req)
			if err := h.store.EnqueueOfflineMessage(target, from, ct, req.Body()); err == nil {
				h.log.Info("message queued offline", "from", from, "to", target, "bytes", len(req.Body()))
				_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusAccepted, "Accepted", nil))
				return
			} else {
				h.log.Warn("offline message queue failed", "from", from, "to", target, "error", err)
			}
		}
		h.log.Debug("message rejected", "to", target, "reason", "unregistered")
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusTemporarilyUnavailable, "Unregistered", nil))
		return
	}

	client := h.client
	if client == nil {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusServiceUnavailable, "Service Unavailable", nil))
		return
	}

	out := buildForwardRequest(req, uri)
	out.SetDestination(dest)

	ctx, cancel := context.WithTimeout(context.Background(), relayTimeout)
	defer cancel()

	outTx, err := client.TransactionRequest(ctx, out)
	if err != nil {
		h.log.Warn("message relay failed", "from", from, "to", target, "dest", dest, "error", err)
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusServiceUnavailable, "Service Unavailable", nil))
		return
	}

	select {
	case res := <-outTx.Responses():
		if res == nil {
			_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusGatewayTimeout, "Gateway Timeout", nil))
			return
		}
		h.log.Info("message relayed", "from", from, "to", target, "status", res.StatusCode, "bytes", len(req.Body()))
		resp := sip.NewResponseFromRequest(req, res.StatusCode, res.Reason, nil)
		if ct := res.GetHeader("Content-Type"); ct != nil {
			resp.AppendHeader(ct)
		}
		if body := res.Body(); len(body) > 0 {
			resp.SetBody(body)
		}
		_ = tx.Respond(resp)
	case <-ctx.Done():
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusGatewayTimeout, "Gateway Timeout", nil))
	}
}

func buildForwardRequest(in *sip.Request, recipient sip.Uri) *sip.Request {
	out := sip.NewRequest(sip.MESSAGE, recipient)
	if from := in.From(); from != nil {
		cp := *from
		cp.Address = *from.Address.Clone()
		if from.Params != nil {
			cp.Params = from.Params.Clone()
		}
		out.AppendHeader(&cp)
	}
	if to := in.To(); to != nil {
		cp := *to
		cp.Address = *to.Address.Clone()
		if to.Params != nil {
			cp.Params = to.Params.Clone()
		}
		out.AppendHeader(&cp)
	}
	for _, name := range []string{"Content-Type", "Content-Encoding", "Content-Language"} {
		if h := in.GetHeader(name); h != nil {
			out.AppendHeader(h)
		}
	}
	if body := in.Body(); len(body) > 0 {
		out.SetBody(body)
	}
	return out
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

func (h *Handler) extensionDND(ext string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	e, ok := h.exts[ext]
	return ok && e.DND
}

// OnRecipientOnline delivers queued messages when an extension registers.
func (h *Handler) OnRecipientOnline(ext string) {
	if h.store == nil {
		return
	}
	n, err := h.DeliverPending(ext)
	if err != nil {
		h.log.Warn("offline message flush", "extension", ext, "error", err)
		return
	}
	if n > 0 {
		h.log.Info("offline messages delivered", "extension", ext, "count", n)
	}
}

// DeliverPending relays stored messages to a now-online extension.
func (h *Handler) DeliverPending(recipient string) (int, error) {
	pending, err := h.store.ListPendingOfflineMessages(recipient)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, msg := range pending {
		if h.relayStored(msg) {
			if err := h.store.MarkOfflineMessageDelivered(msg.ID); err != nil {
				return delivered, err
			}
			delivered++
		}
	}
	return delivered, nil
}

func (h *Handler) relayStored(msg store.OfflineMessage) bool {
	client := h.client
	if client == nil {
		return false
	}
	uri, dest, _, ok := h.reg.DialTarget(msg.Recipient)
	if !ok {
		return false
	}
	req := sip.NewRequest(sip.MESSAGE, uri)
	req.AppendHeader(&sip.FromHeader{Address: sip.Uri{User: msg.Sender, Host: h.domain}})
	req.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: msg.Recipient, Host: h.domain}})
	req.AppendHeader(sip.NewHeader("Content-Type", msg.ContentType))
	req.SetBody(msg.Body)
	req.SetDestination(dest)

	ctx, cancel := context.WithTimeout(context.Background(), relayTimeout)
	defer cancel()
	outTx, err := client.TransactionRequest(ctx, req)
	if err != nil {
		return false
	}
	select {
	case res := <-outTx.Responses():
		return res != nil && res.StatusCode >= 200 && res.StatusCode < 300
	case <-ctx.Done():
		return false
	}
}

func contentType(req *sip.Request) string {
	if h := req.GetHeader("Content-Type"); h != nil && h.Value() != "" {
		return h.Value()
	}
	return "text/plain"
}
