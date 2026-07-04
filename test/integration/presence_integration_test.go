//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

func TestPresenceSubscribeNotify(t *testing.T) {
	port, cleanup := startTestPBXTwoExt(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var mu sync.Mutex
	var gotPIDF string
	ep101 := newSIPEndpoint(t, port, "101", "secret", func(req *sip.Request, tx sip.ServerTransaction) {
		if req.Method == sip.NOTIFY {
			mu.Lock()
			gotPIDF = string(req.Body())
			mu.Unlock()
			_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil))
		}
	})
	ep102 := newSIPEndpoint(t, port, "102", "secret", nil)

	ep102.h.register()
	ep101.h.register()
	time.Sleep(100 * time.Millisecond)

	recipient := sip.Uri{User: "102", Host: "127.0.0.1", Port: port}
	req := sip.NewRequest(sip.SUBSCRIBE, recipient)
	req.AppendHeader(&sip.FromHeader{Address: sip.Uri{User: "101", Host: "127.0.0.1"}, Params: sip.NewParams()})
	req.From().Params.Add("tag", "subfrom")
	req.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: "102", Host: "127.0.0.1", Port: port}})
	req.AppendHeader(sip.NewHeader("Call-ID", "presence-call-1"))
	req.AppendHeader(sip.NewHeader("Event", "presence"))
	req.AppendHeader(sip.NewHeader("Expires", "3600"))
	req.AppendHeader(&sip.ContactHeader{Address: sip.Uri{User: "101", Host: "127.0.0.1"}})
	req.SetDestination(fmt.Sprintf("127.0.0.1:%d", port))

	tx, err := ep101.client.TransactionRequest(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	var res *sip.Response
	select {
	case res = <-tx.Responses():
	case <-ctx.Done():
		t.Fatal("subscribe timeout")
	}
	if res == nil || res.StatusCode != sip.StatusOK {
		t.Fatalf("subscribe status=%v", statusOrZero(res))
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		body := gotPIDF
		mu.Unlock()
		if body != "" {
			if !contains(body, "<basic>open</basic>") {
				t.Fatalf("pidf=%q", body)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no NOTIFY received")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestOfflineMessageDeliveredOnRegister(t *testing.T) {
	port, cleanup := startTestPBXTwoExt(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var mu sync.Mutex
	var gotBody string
	ep102 := newSIPEndpoint(t, port, "102", "secret", func(req *sip.Request, tx sip.ServerTransaction) {
		if req.Method == sip.MESSAGE {
			mu.Lock()
			gotBody = string(req.Body())
			mu.Unlock()
			_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil))
		}
	})
	ep101 := newSIPEndpoint(t, port, "101", "secret", nil)
	ep101.h.register()
	time.Sleep(100 * time.Millisecond)

	recipient := sip.Uri{User: "102", Host: "127.0.0.1", Port: port}
	msg := sip.NewRequest(sip.MESSAGE, recipient)
	msg.AppendHeader(&sip.FromHeader{Address: sip.Uri{User: "101", Host: "127.0.0.1"}})
	msg.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: "102", Host: "127.0.0.1", Port: port}})
	msg.AppendHeader(sip.NewHeader("Content-Type", "text/plain"))
	msg.SetBody([]byte("see you later"))
	msg.SetDestination(fmt.Sprintf("127.0.0.1:%d", port))

	tx, err := ep101.client.TransactionRequest(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case res := <-tx.Responses():
		if res == nil || res.StatusCode != sip.StatusAccepted {
			t.Fatalf("queue status=%v", statusOrZero(res))
		}
	case <-ctx.Done():
		t.Fatal("message timeout")
	}

	ep102.h.register()

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		body := gotBody
		mu.Unlock()
		if body == "see you later" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("offline delivery body=%q", body)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
