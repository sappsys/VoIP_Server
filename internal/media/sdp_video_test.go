package media_test

import (
	"strings"
	"testing"

	"github.com/sappsys/VoIP_Server/internal/media"
)

const sampleOffer = `v=0
o=- 0 0 IN IP4 192.168.1.10
s=-
c=IN IP4 192.168.1.10
t=0 0
m=audio 5004 RTP/AVP 0 8
a=rtpmap:0 PCMU/8000
m=video 5006 RTP/AVP 96
a=rtpmap:96 H264/90000
a=fmtp:96 profile-level-id=42e01f
`

func TestHasVideo(t *testing.T) {
	if !media.HasVideo([]byte(sampleOffer)) {
		t.Fatal("expected video in offer")
	}
	if media.HasVideo([]byte("v=0\r\nm=audio 9 RTP/AVP 0\r\n")) {
		t.Fatal("unexpected video")
	}
}

func TestAppendVideo(t *testing.T) {
	audioSDP := []byte("v=0\r\no=- 0 0 IN IP4 10.0.0.1\r\ns=-\r\nt=0 0\r\nm=audio 4000 RTP/AVP 0\r\n")
	merged, err := media.AppendVideo(audioSDP, []byte(sampleOffer), "10.0.0.1", 4010)
	if err != nil {
		t.Fatal(err)
	}
	body := string(merged)
	if !strings.Contains(body, "m=video 4010") {
		t.Fatalf("missing video line: %s", body)
	}
}
