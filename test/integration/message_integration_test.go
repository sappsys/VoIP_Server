//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

type sipEndpoint struct {
	client *sipgo.Client
	h      *handset
}

func newSIPEndpoint(t *testing.T, pbxPort int, ext, password string, onRequest func(*sip.Request, sip.ServerTransaction)) *sipEndpoint {
	t.Helper()
	h := newHandset(t, pbxPort, ext, password)
	if onRequest != nil {
		h.sipServer.OnMessage(onRequest)
		h.sipServer.OnNotify(onRequest)
	}
	return &sipEndpoint{client: h.sipClient, h: h}
}

func TestExtensionToExtensionMessage(t *testing.T) {
	port, cleanup := startTestPBXTwoExt(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var mu sync.Mutex
	var gotBody string
	ep102 := newSIPEndpoint(t, port, "102", "secret", func(req *sip.Request, tx sip.ServerTransaction) {
		mu.Lock()
		gotBody = string(req.Body())
		mu.Unlock()
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil))
	})
	ep101 := newSIPEndpoint(t, port, "101", "secret", nil)

	ep102.h.register()
	ep101.h.register()
	time.Sleep(100 * time.Millisecond)

	recipient := sip.Uri{User: "102", Host: "127.0.0.1", Port: port}
	req := sip.NewRequest(sip.MESSAGE, recipient)
	req.AppendHeader(&sip.FromHeader{Address: sip.Uri{User: "101", Host: "127.0.0.1"}})
	req.AppendHeader(&sip.ToHeader{Address: sip.Uri{User: "102", Host: "127.0.0.1", Port: port}})
	req.AppendHeader(sip.NewHeader("Content-Type", "text/plain"))
	req.SetBody([]byte("hello bob"))
	req.SetDestination(fmt.Sprintf("127.0.0.1:%d", port))

	tx, err := ep101.client.TransactionRequest(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	var res *sip.Response
	select {
	case res = <-tx.Responses():
	case <-ctx.Done():
		t.Fatal("timeout waiting for MESSAGE response")
	}
	if res == nil || res.StatusCode != sip.StatusOK {
		t.Fatalf("response status=%v", statusOrZero(res))
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		body := gotBody
		mu.Unlock()
		if body == "hello bob" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("recipient body=%q", body)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func statusOrZero(res *sip.Response) int {
	if res == nil {
		return 0
	}
	return res.StatusCode
}
