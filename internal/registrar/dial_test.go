package registrar

import (
	"testing"

	"github.com/emiago/sipgo/sip"
)

func TestBindingToDialTargetTransport(t *testing.T) {
	params := sip.NewParams()
	params.Add("transport", "udp")
	b := Binding{
		Contact: sip.ContactHeader{
			Address: sip.Uri{User: "101", Host: "10.0.0.5", Port: 5060},
			Params:  params,
		},
	}
	uri, dest, transport := bindingToDialTarget("101", b)
	if uri.User != "101" || dest != "10.0.0.5:5060" || transport != "udp" {
		t.Fatalf("uri=%+v dest=%q transport=%q", uri, dest, transport)
	}
}

func TestBindingToDialTargetFillsUser(t *testing.T) {
	b := Binding{
		Contact: sip.ContactHeader{
			Address: sip.Uri{Host: "10.0.0.5", Port: 5062},
		},
	}
	uri, _, _ := bindingToDialTarget("202", b)
	if uri.User != "202" {
		t.Fatalf("user=%q", uri.User)
	}
}
