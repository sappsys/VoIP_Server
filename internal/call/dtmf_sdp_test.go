package call

import (
	"testing"

	diagomedia "github.com/emiago/diago/media"
)

func TestTelephoneEventsFromSDPsMergesPT95(t *testing.T) {
	invite := []byte(sampleInviteSDP)
	answer := []byte(`v=0
o=- 1 2 IN IP4 192.168.105.240
s=-
c=IN IP4 192.168.105.240
t=0 0
m=audio 10000 RTP/AVP 9 95
a=rtpmap:9 G722/8000
a=rtpmap:95 telephone-event/8000
a=fmtp:95 0-16
`)
	got := telephoneEventsFromSDPs(invite, answer)
	if len(got) != 1 || got[0].PayloadType != 95 {
		t.Fatalf("events=%v", got)
	}
}

func TestBuildTelephoneEventPTSetFromSDPsIncludes95(t *testing.T) {
	ms := &diagomedia.MediaSession{
		Codecs: []diagomedia.Codec{
			{Name: "G722", PayloadType: 9, SampleRate: 8000},
		},
	}
	pts := buildTelephoneEventPTSetFromSDPs(ms, [][]byte{[]byte(sampleInviteSDP)}, diagomedia.CodecTelephoneEvent8000)
	if !pts[95] {
		t.Fatalf("missing pt 95: %v", pts)
	}
}
