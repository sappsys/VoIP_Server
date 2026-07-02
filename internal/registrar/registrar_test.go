package registrar

import (
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/sappsys/VoIP_Server/internal/config"
)

func TestRegisterForTestAndContactURI(t *testing.T) {
	exts := map[string]*config.Extension{
		"101": {Extension: "101", Password: "secret", Enabled: true},
	}
	r := New("lab.local", exts, nil)
	r.RegisterForTest("101", sip.Uri{User: "101", Host: "10.0.0.5", Port: 5060})

	uri, ok := r.ContactURI("101")
	if !ok || uri.Host != "10.0.0.5" || uri.Port != 5060 {
		t.Fatalf("contact: %+v ok=%v", uri, ok)
	}
	if !r.IsRegistered("101") {
		t.Fatal("expected registered")
	}
}

func TestRegisteredExtensionsList(t *testing.T) {
	exts := map[string]*config.Extension{
		"101": {Extension: "101", Enabled: true},
		"102": {Extension: "102", Enabled: true},
	}
	r := New("lab.local", exts, nil)
	r.RegisterForTest("101", sip.Uri{User: "101", Host: "127.0.0.1", Port: 5060})
	r.RegisterForTest("102", sip.Uri{User: "102", Host: "127.0.0.1", Port: 5061})

	list := r.RegisteredExtensions()
	if len(list) != 2 {
		t.Fatalf("registered=%v", list)
	}
}

func TestExpiredBindingIgnored(t *testing.T) {
	r := New("lab.local", map[string]*config.Extension{}, nil)
	r.mu.Lock()
	r.bindings["101"] = []Binding{{
		Contact: sip.ContactHeader{Address: sip.Uri{User: "101", Host: "127.0.0.1"}},
		Expires: time.Now().Add(-time.Hour),
	}}
	r.mu.Unlock()
	if r.IsRegistered("101") {
		t.Fatal("expired binding should not register")
	}
}

func TestUpdateExtensions(t *testing.T) {
	r := New("lab.local", map[string]*config.Extension{}, nil)
	exts := map[string]*config.Extension{
		"201": {Extension: "201", Enabled: true},
	}
	r.UpdateExtensions(exts)
	r.mu.RLock()
	_, ok := r.exts["201"]
	r.mu.RUnlock()
	if !ok {
		t.Fatal("extensions not updated")
	}
}
