package registrar

import (
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
)

func TestRegisterDigestChallengeThenOK(t *testing.T) {
	d := newRegisterDigestAuth(time.Minute, nil)
	req := sip.NewRequest(sip.REGISTER, sip.Uri{User: "110", Host: "voip.local"})
	req.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: "110", Host: "voip.local"}})

	first := d.authorize(req, "voip.local", "110", "andy")
	if first.outcome != authChallenge || first.res.StatusCode != sip.StatusUnauthorized {
		t.Fatalf("first=%+v status=%d", first, first.res.StatusCode)
	}
	www := first.res.GetHeader("WWW-Authenticate")
	if www == nil {
		t.Fatal("missing WWW-Authenticate")
	}
	chal, err := digest.ParseChallenge(www.Value())
	if err != nil {
		t.Fatal(err)
	}
	cred, err := digest.Digest(chal, digest.Options{
		Method:   "REGISTER",
		URI:      "sip:110@voip.local",
		Username: "110",
		Password: "andy",
	})
	if err != nil {
		t.Fatal(err)
	}
	req2 := req.Clone()
	req2.AppendHeader(sip.NewHeader("Authorization", cred.String()))

	second := d.authorize(req2, "voip.local", "110", "andy")
	if second.outcome != authOK {
		t.Fatalf("second=%+v", second)
	}
}

func TestRegisterDigestStaleNonceReChallenges(t *testing.T) {
	d := newRegisterDigestAuth(time.Millisecond, nil)
	req := sip.NewRequest(sip.REGISTER, sip.Uri{User: "110", Host: "voip.local"})
	req.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: "110", Host: "voip.local"}})

	first := d.authorize(req, "voip.local", "110", "andy")
	chal, _ := digest.ParseChallenge(first.res.GetHeader("WWW-Authenticate").Value())
	cred, _ := digest.Digest(chal, digest.Options{
		Method: "REGISTER", URI: "sip:110@voip.local", Username: "110", Password: "andy",
	})
	time.Sleep(5 * time.Millisecond)

	req2 := req.Clone()
	req2.AppendHeader(sip.NewHeader("Authorization", cred.String()))
	stale := d.authorize(req2, "voip.local", "110", "andy")
	if stale.outcome != authChallenge || stale.reason != "stale nonce" {
		t.Fatalf("stale=%+v", stale)
	}
	if stale.res.GetHeader("WWW-Authenticate") == nil {
		t.Fatal("expected fresh WWW-Authenticate on stale nonce")
	}
}

func TestRegisterDigestBadPasswordReChallenges(t *testing.T) {
	d := newRegisterDigestAuth(time.Minute, nil)
	req := sip.NewRequest(sip.REGISTER, sip.Uri{User: "110", Host: "voip.local"})
	req.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: "110", Host: "voip.local"}})

	first := d.authorize(req, "voip.local", "110", "andy")
	chal, _ := digest.ParseChallenge(first.res.GetHeader("WWW-Authenticate").Value())
	cred, _ := digest.Digest(chal, digest.Options{
		Method: "REGISTER", URI: "sip:110@voip.local", Username: "110", Password: "wrong",
	})
	req2 := req.Clone()
	req2.AppendHeader(sip.NewHeader("Authorization", cred.String()))
	bad := d.authorize(req2, "voip.local", "110", "andy")
	if bad.outcome != authChallenge || bad.reason != "bad credentials" {
		t.Fatalf("bad=%+v", bad)
	}
}
