package registrar

import (
	"testing"

	"github.com/emiago/sipgo/sip"
)

func TestRewriteContactUsesSignalingSource(t *testing.T) {
	contact := sip.ContactHeader{Address: sip.Uri{User: "101", Host: "192.168.1.50", Port: 5060}}
	got := rewriteContactForNAT(contact, "203.0.113.10:51234", false)
	if got.Address.Host != "203.0.113.10" || got.Address.Port != 51234 {
		t.Fatalf("rewritten contact: %+v", got.Address)
	}
}

func TestRewriteContactPreservesWhenSame(t *testing.T) {
	contact := sip.ContactHeader{Address: sip.Uri{User: "101", Host: "203.0.113.5", Port: 5060}}
	got := rewriteContactForNAT(contact, "203.0.113.5:5060", false)
	if got.Address.Host != "203.0.113.5" || got.Address.Port != 5060 {
		t.Fatalf("unchanged contact: %+v", got.Address)
	}
}

func TestRewriteContactPreserveFlag(t *testing.T) {
	contact := sip.ContactHeader{Address: sip.Uri{User: "101", Host: "192.168.1.50", Port: 5060}}
	got := rewriteContactForNAT(contact, "203.0.113.10:51234", true)
	if got.Address.Host != "192.168.1.50" {
		t.Fatalf("preserve flag ignored: %+v", got.Address)
	}
}
