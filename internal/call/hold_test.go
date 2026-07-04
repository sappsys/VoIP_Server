package call

import (
	"net"
	"testing"

	diagomedia "github.com/emiago/diago/media"
	"github.com/emiago/diago/media/sdp"
)

func TestRemoteHoldMode(t *testing.T) {
	tests := []struct {
		remoteMode string
		want       bool
	}{
		{sdp.ModeSendrecv, false},
		{sdp.ModeSendonly, true}, // phone hold: remote sendonly negotiates to local recvonly
		{sdp.ModeInactive, true},
	}
	for _, tc := range tests {
		ms := negotiatedHoldSession(t, tc.remoteMode)
		if got := remoteHoldMode(ms); got != tc.want {
			t.Fatalf("remote=%s got=%v want=%v dir=%q", tc.remoteMode, got, tc.want, negotiatedMediaDirection(ms))
		}
	}
}

func negotiatedHoldSession(t *testing.T, remoteMode string) *diagomedia.MediaSession {
	t.Helper()
	ms, err := diagomedia.NewMediaSession(net.ParseIP("127.0.0.1"), 0)
	if err != nil {
		t.Fatalf("NewMediaSession: %v", err)
	}
	t.Cleanup(func() { _ = ms.Close() })
	if err := ms.InitWithSDP(localAudioSDP(sdp.ModeSendrecv)); err != nil {
		t.Fatal(err)
	}
	if err := ms.RemoteSDP(holdRemoteSDP(remoteMode)); err != nil {
		t.Fatalf("remote=%s RemoteSDP: %v", remoteMode, err)
	}
	return ms
}

func TestRemoteHoldModeZeroConnection(t *testing.T) {
	ms := newTestMediaSession(t)
	if err := ms.RemoteSDP(holdRemoteSDP(sdp.ModeSendrecv)); err != nil {
		t.Fatal(err)
	}
	ms.Raddr = net.UDPAddr{IP: net.IPv4zero, Port: 9}
	if !remoteHoldMode(ms) {
		t.Fatal("expected hold for c=0.0.0.0 with sendrecv")
	}
}

func TestHolderPhoneReleased(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeRecvonly)
	if holderPhoneReleased(ms, false) {
		t.Fatal("expected false without phone hold flag")
	}
	if holderPhoneReleased(ms, true) {
		t.Fatal("recvonly should not be released while phone still on hold")
	}
	ms = mediaSessionWithLocalDirection(t, sdp.ModeSendonly)
	if holderPhoneReleased(ms, true) {
		t.Fatal("sendonly dial tone must not count as phone unhold")
	}
	ms = mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	if !holderPhoneReleased(ms, true) {
		t.Fatalf("expected released after phone sendrecv, dir=%q", negotiatedMediaDirection(ms))
	}
}

func newTestMediaSession(t *testing.T) *diagomedia.MediaSession {
	t.Helper()
	ms, err := diagomedia.NewMediaSession(net.ParseIP("127.0.0.1"), 0)
	if err != nil {
		t.Fatalf("NewMediaSession: %v", err)
	}
	t.Cleanup(func() { _ = ms.Close() })
	return ms
}

func holdRemoteSDP(mode string) []byte {
	return []byte("v=0\r\n" +
		"o=- 1 2 IN IP4 192.168.1.2\r\n" +
		"s=-\r\n" +
		"c=IN IP4 192.168.1.2\r\n" +
		"t=0 0\r\n" +
		"m=audio 5004 RTP/AVP 0 8\r\n" +
		"a=" + mode + "\r\n")
}
