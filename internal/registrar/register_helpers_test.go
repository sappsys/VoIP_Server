package registrar

import (
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
)

func TestExtractRegisterUser(t *testing.T) {
	req := sip.NewRequest(sip.REGISTER, sip.Uri{User: "111", Host: "pbx.local"})
	req.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: "111", Host: "pbx.local"}})
	if got := extractRegisterUser(req); got != "111" {
		t.Fatalf("user=%q", got)
	}

	req2 := sip.NewRequest(sip.REGISTER, sip.Uri{Host: "pbx.local"})
	req2.AppendHeader(&sip.FromHeader{Address: sip.Uri{User: "222", Host: "pbx.local"}})
	if got := extractRegisterUser(req2); got != "222" {
		t.Fatalf("from user=%q", got)
	}
}

func TestBuildRegisterOKIncludesContact(t *testing.T) {
	req := sip.NewRequest(sip.REGISTER, sip.Uri{User: "111", Host: "pbx.local"})
	stored := sip.ContactHeader{Address: sip.Uri{User: "111", Host: "203.0.113.10", Port: 5060}}
	res := buildRegisterOK(req, stored, 3600)
	if res.StatusCode != sip.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}
	contact := res.Contact()
	if contact == nil {
		t.Fatal("missing Contact in 200 OK")
	}
	if contact.Address.Host != "203.0.113.10" || contact.Address.Port != 5060 {
		t.Fatalf("contact=%s", contact.Address.String())
	}
	exp, ok := contact.Params.Get("expires")
	if !ok || exp != "3600" {
		t.Fatalf("contact expires=%q ok=%v", exp, ok)
	}
}

func TestNATRegisterAndOptionsRefresh(t *testing.T) {
	exts := map[string]*config.Extension{
		"111": {Extension: "111", Password: "andy", Enabled: true},
	}
	r := New("lab.local", config.ServerConfig{}, exts, nil)

	private := sip.ContactHeader{Address: sip.Uri{User: "111", Host: "192.168.1.50", Port: 5060}}
	publicSource := "203.0.113.10:5060"
	stored := rewriteContactForNAT(private, publicSource, false)
	r.addBinding("111", publicSource, Binding{
		Contact: stored,
		Expires: time.Now().Add(30 * time.Second),
		granted: 60 * time.Second,
	})
	if !r.IsRegistered("111") {
		t.Fatal("expected registered after add")
	}

	// Phone OPTIONS keepalive still advertises private Contact; packet source is public.
	r.touchBinding("111", private, publicSource)
	r.mu.RLock()
	exp := r.bindings["111"][0].Expires
	r.mu.RUnlock()
	if time.Until(exp) < 50*time.Second {
		t.Fatalf("binding not refreshed, expires in %v", time.Until(exp))
	}

	// OPTIONS without Contact matches by source only.
	r.touchBindingBySource("111", publicSource)
	r.mu.RLock()
	exp2 := r.bindings["111"][0].Expires
	r.mu.RUnlock()
	if time.Until(exp2) < 50*time.Second {
		t.Fatalf("source refresh failed, expires in %v", time.Until(exp2))
	}
}

func TestContactMatchesStoredBySource(t *testing.T) {
	ext := "111"
	stored := sip.ContactHeader{Address: sip.Uri{User: "111", Host: "203.0.113.10", Port: 5060}}
	incoming := sip.ContactHeader{Address: sip.Uri{User: "111", Host: "192.168.1.50", Port: 5060}}
	if !contactMatchesStored(ext, stored, incoming, "203.0.113.10:5060", false) {
		t.Fatal("expected NAT source match")
	}
}
