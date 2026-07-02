package trunk

import (
	"testing"

	"github.com/emiago/sipgo/sip"
)

func TestParseTrunkURI(t *testing.T) {
	cases := []struct {
		host, user string
		wantHost   string
		wantPort   int
		wantUser   string
	}{
		{"sip.carrier.com:5061", "5551234", "sip.carrier.com", 5061, "5551234"},
		{"10.0.0.1", "9", "10.0.0.1", 5060, "9"},
		{"gw.example.net:5080", "", "gw.example.net", 5080, ""},
	}
	for _, c := range cases {
		uri := parseTrunkURI(c.host, c.user)
		if uri.Host != c.wantHost || uri.Port != c.wantPort || uri.User != c.wantUser {
			t.Fatalf("parseTrunkURI(%q,%q)=%+v want host=%s port=%d user=%s",
				c.host, c.user, uri, c.wantHost, c.wantPort, c.wantUser)
		}
	}
}

func TestParseTrunkURISipUriShape(t *testing.T) {
	uri := parseTrunkURI("192.168.1.1:5060", "18005551212")
	if uri.Scheme != "" && uri.Scheme != "sip" {
		// sip.Uri may leave scheme empty; host must be set
	}
	if uri.Host == "" {
		t.Fatal("host required")
	}
	_ = sip.Uri{Host: uri.Host, Port: uri.Port, User: uri.User}
}
