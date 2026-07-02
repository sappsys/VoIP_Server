package call

import (
	"testing"

	diagomedia "github.com/emiago/diago/media"
)

const sampleInviteSDP = `v=0
o=- 1 1 IN IP4 192.168.105.125
s=-
c=IN IP4 192.168.105.125
t=0 0
m=audio 65268 RTP/AVP 0 8 95
a=rtpmap:0 PCMU/8000
a=rtpmap:8 PCMA/8000
a=rtpmap:95 telephone-event/8000
a=fmtp:95 0-16
`

func TestTelephoneEventsFromSDP(t *testing.T) {
	codecs := telephoneEventsFromSDP([]byte(sampleInviteSDP))
	if len(codecs) != 1 {
		t.Fatalf("count=%d want 1", len(codecs))
	}
	if codecs[0].PayloadType != 95 {
		t.Fatalf("pt=%d want 95", codecs[0].PayloadType)
	}
}

func TestPatchMediaSessionDTMFCodec(t *testing.T) {
	ms := &diagomedia.MediaSession{
		Codecs: []diagomedia.Codec{
			diagomedia.CodecAudioUlaw,
			diagomedia.CodecTelephoneEvent8000,
		},
	}
	got := patchMediaSessionDTMFCodec(ms, []byte(sampleInviteSDP), diagomedia.CodecTelephoneEvent8000)
	if got.PayloadType != 95 {
		t.Fatalf("returned pt=%d want 95", got.PayloadType)
	}
	found95 := false
	for _, c := range ms.Codecs {
		if c.PayloadType == 95 {
			found95 = true
		}
	}
	if !found95 {
		t.Fatalf("codecs=%v missing pt 95", ms.Codecs)
	}
}

func TestDtmfCodecFromList(t *testing.T) {
	codecs := []diagomedia.Codec{
		diagomedia.CodecAudioUlaw,
		{Name: "telephone-event", PayloadType: 95, SampleRate: 8000},
	}
	got := dtmfCodecFromList(codecs, diagomedia.CodecTelephoneEvent8000)
	if got.PayloadType != 95 {
		t.Fatalf("pt=%d want 95", got.PayloadType)
	}
}
