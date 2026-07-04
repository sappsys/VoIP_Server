package nat

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/pion/stun/v3"
)

func TestSTUNServerBinding(t *testing.T) {
	stunLn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stunPort := stunLn.LocalAddr().(*net.UDPAddr).Port
	_ = stunLn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = (&STUNServer{}).Run(ctx, "127.0.0.1", stunPort)
	}()
	time.Sleep(50 * time.Millisecond)

	client, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	req, err := stun.Build(stun.TransactionID, stun.BindingRequest)
	if err != nil {
		t.Fatal(err)
	}
	serverAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort("127.0.0.1", strconv.Itoa(stunPort)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.WriteTo(req.Raw, serverAddr); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1500)
	n, _, err := client.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	res := &stun.Message{Raw: buf[:n]}
	if err := res.Decode(); err != nil {
		t.Fatal(err)
	}
	if res.Type != stun.BindingSuccess {
		t.Fatalf("type=%v want BindingSuccess", res.Type)
	}
	var mapped stun.XORMappedAddress
	if err := mapped.GetFrom(res); err != nil {
		t.Fatal(err)
	}
	local := client.LocalAddr().(*net.UDPAddr)
	if mapped.Port != local.Port {
		t.Fatalf("mapped port=%d want %d", mapped.Port, local.Port)
	}
}
