//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

type sipEndpoint struct {
	client *sipgo.Client
	dg     *diago.Diago
	port   int
}

func newSIPEndpoint(t *testing.T, onRequest func(*sip.Request, sip.ServerTransaction)) *sipEndpoint {
	t.Helper()
	ua, err := sipgo.NewUA()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ua.Close() })

	srv, err := sipgo.NewServer(ua)
	if err != nil {
		t.Fatal(err)
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	_ = conn.Close()

	client, err := sipgo.NewClient(ua, sipgo.WithClientHostname("127.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}

	dg := diago.NewDiago(ua,
		diago.WithServer(srv),
		diago.WithClient(client),
		diago.WithTransport(diago.Transport{
			Transport: "udp",
			BindHost:  "127.0.0.1",
			BindPort:  port,
		}),
	)
	if onRequest != nil {
		srv.OnMessage(onRequest)
		srv.OnNotify(onRequest)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.ListenAndServe(ctx, "udp", fmt.Sprintf("127.0.0.1:%d", port)) }()
	time.Sleep(50 * time.Millisecond)

	return &sipEndpoint{client: client, dg: dg, port: port}
}

func TestExtensionToExtensionMessage(t *testing.T) {
	port, cleanup := startTestPBXTwoExt(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var mu sync.Mutex
	var gotBody string
	ep102 := newSIPEndpoint(t, func(req *sip.Request, tx sip.ServerTransaction) {
		mu.Lock()
		gotBody = string(req.Body())
		mu.Unlock()
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil))
	})
	ep101 := newSIPEndpoint(t, nil)

	registerExtension(t, ep102.dg, port, "102", "secret")
	registerExtension(t, ep101.dg, port, "101", "secret")
	time.Sleep(100 * time.Millisecond)

	recipient := sip.Uri{User: "102", Host: "127.0.0.1", Port: port}
	req := sip.NewRequest(sip.MESSAGE, recipient)
	req.AppendHeader(&sip.FromHeader{Address: sip.Uri{User: "101", Host: "127.0.0.1", Port: ep101.port}})
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
