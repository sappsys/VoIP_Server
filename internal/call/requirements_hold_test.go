package call

import (
	"net"
	"testing"

	diagomedia "github.com/emiago/diago/media"
	"github.com/emiago/diago/media/sdp"
)

// REQ-HOLD-3: phone-initiated hold is negotiated recvonly (we receive, phone stopped sending).
// REQ-HOLD-4: server sendonly re-INVITE (dial tone) is NOT phone hold.
// REQ-HOLD-5: inactive remote media is treated as hold.

func TestREQ_HOLD_PhoneHoldIsRecvonly(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeRecvonly)
	if !remoteHoldMode(ms) {
		t.Fatalf("REQ-HOLD-3: recvonly must be hold, dir=%q", negotiatedMediaDirection(ms))
	}
}

func TestREQ_HOLD_ServerDialToneSendonlyIsNotHold(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendonly)
	if remoteHoldMode(ms) {
		t.Fatalf("REQ-HOLD-4: sendonly is server dial-tone re-INVITE, not phone hold, dir=%q", negotiatedMediaDirection(ms))
	}
}

func TestREQ_HOLD_SendRecvIsNotHold(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	if remoteHoldMode(ms) {
		t.Fatal("REQ-HOLD: active call sendrecv must not be hold")
	}
}

func TestREQ_HOLD_InactiveIsHold(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeInactive)
	if !remoteHoldMode(ms) {
		t.Fatal("REQ-HOLD-5: inactive must be hold")
	}
}

func TestREQ_HOLD_HolderReleasedOnSendRecv(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeRecvonly)
	if holderPhoneReleased(ms, true) {
		t.Fatal("still recvonly — phone still on hold")
	}
	ms = mediaSessionWithLocalDirection(t, sdp.ModeSendonly)
	if holderPhoneReleased(ms, true) {
		t.Fatal("REQ-HOLD-4: server sendonly dial tone must not count as phone unhold")
	}
	ms = mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	if !holderPhoneReleased(ms, true) {
		t.Fatal("REQ-HOLD: phone unhold (sendrecv) must release holder")
	}
}

func mediaSessionWithLocalDirection(t *testing.T, mode string) *diagomedia.MediaSession {
	t.Helper()
	ms, err := diagomedia.NewMediaSession(net.ParseIP("127.0.0.1"), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ms.Close() })
	if err := ms.InitWithSDP(localAudioSDP(mode)); err != nil {
		t.Fatal(err)
	}
	return ms
}

func localAudioSDP(mode string) []byte {
	return []byte("v=0\r\n" +
		"o=- 1 2 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"c=IN IP4 127.0.0.1\r\n" +
		"t=0 0\r\n" +
		"m=audio 5004 RTP/AVP 0\r\n" +
		"a=rtpmap:0 PCMU/8000\r\n" +
		"a=" + mode + "\r\n")
}
