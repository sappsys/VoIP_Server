package registrar

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
)

var (
	errDigestNoChallenge = errors.New("digest: no challenge")
	errDigestBadCreds    = errors.New("digest: bad credentials")
)

type authOutcome int

const (
	authOK authOutcome = iota
	authChallenge
	authDenied
)

type authDecision struct {
	outcome authOutcome
	res     *sip.Response
	reason  string
}

type digestChallengeEntry struct {
	challenge digest.Challenge
	timer     *time.Timer
}

// registerDigestAuth validates REGISTER digest credentials with long-lived nonces
// and always re-challenges on stale or missing nonces (many phones reuse cached nonces).
type registerDigestAuth struct {
	mu       sync.Mutex
	cache    map[string]*digestChallengeEntry
	nonceTTL time.Duration
	log      *slog.Logger
}

func newRegisterDigestAuth(nonceTTL time.Duration, log *slog.Logger) *registerDigestAuth {
	if nonceTTL <= 0 {
		nonceTTL = time.Hour
	}
	return &registerDigestAuth{
		cache:    make(map[string]*digestChallengeEntry),
		nonceTTL: nonceTTL,
		log:      log,
	}
}

func (d *registerDigestAuth) authorize(req *sip.Request, realm, username, password string) authDecision {
	if req == nil {
		return authDecision{outcome: authDenied, reason: "nil request"}
	}
	h := req.GetHeader("Authorization")
	if h == nil {
		res, err := d.issueChallenge(req, realm)
		if err != nil {
			return authDecision{
				outcome: authDenied,
				res:     sip.NewResponseFromRequest(req, sip.StatusInternalServerError, "Internal Server Error", nil),
				reason:  "nonce generation failed",
			}
		}
		return authDecision{outcome: authChallenge, res: res, reason: "no authorization"}
	}

	cred, err := digest.ParseCredentials(h.Value())
	if err != nil {
		return authDecision{
			outcome: authDenied,
			res:     sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil),
			reason:  "malformed authorization",
		}
	}

	if cred.Username != "" && cred.Username != username {
		return d.challengeOrDeny(req, realm, "username mismatch")
	}

	d.mu.Lock()
	entry, exists := d.cache[cred.Nonce]
	d.mu.Unlock()
	if !exists {
		return d.challengeOrDeny(req, realm, "stale nonce")
	}

	digCred, err := digest.Digest(&entry.challenge, digest.Options{
		Method:   req.Method.String(),
		URI:      cred.URI,
		Username: username,
		Password: password,
	})
	if err != nil {
		return authDecision{
			outcome: authDenied,
			res:     sip.NewResponseFromRequest(req, sip.StatusForbidden, "Forbidden", nil),
			reason:  "digest algorithm unsupported",
		}
	}
	if cred.Response != digCred.Response {
		return d.challengeOrDeny(req, realm, "bad credentials")
	}

	// Successful auth; keep nonce valid for refresh cycles.
	d.touchNonce(cred.Nonce)
	return authDecision{outcome: authOK, reason: "ok"}
}

func (d *registerDigestAuth) challengeOrDeny(req *sip.Request, realm, reason string) authDecision {
	res, err := d.issueChallenge(req, realm)
	if err != nil {
		return authDecision{
			outcome: authDenied,
			res:     sip.NewResponseFromRequest(req, sip.StatusInternalServerError, "Internal Server Error", nil),
			reason:  reason,
		}
	}
	return authDecision{outcome: authChallenge, res: res, reason: reason}
}

func (d *registerDigestAuth) issueChallenge(req *sip.Request, realm string) (*sip.Response, error) {
	nonce, err := generateDigestNonce()
	if err != nil {
		return nil, err
	}
	chal := digest.Challenge{
		Realm:     realm,
		Nonce:     nonce,
		Algorithm: "MD5",
	}
	res := sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Unauthorized", nil)
	res.AppendHeader(sip.NewHeader("WWW-Authenticate", chal.String()))

	d.mu.Lock()
	if old, ok := d.cache[nonce]; ok && old.timer != nil {
		old.timer.Stop()
	}
	entry := &digestChallengeEntry{challenge: chal}
	entry.timer = time.AfterFunc(d.nonceTTL, func() {
		d.mu.Lock()
		delete(d.cache, nonce)
		d.mu.Unlock()
	})
	d.cache[nonce] = entry
	d.mu.Unlock()
	return res, nil
}

func (d *registerDigestAuth) touchNonce(nonce string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	entry, ok := d.cache[nonce]
	if !ok || entry.timer == nil {
		return
	}
	entry.timer.Stop()
	entry.timer = time.AfterFunc(d.nonceTTL, func() {
		d.mu.Lock()
		delete(d.cache, nonce)
		d.mu.Unlock()
	})
}

func generateDigestNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
