package call

import (
	"testing"

	diagomedia "github.com/emiago/diago/media"
)

func TestTelephoneEventPTs(t *testing.T) {
	ms := &diagomedia.MediaSession{
		Codecs: []diagomedia.Codec{
			diagomedia.CodecAudioUlaw,
			diagomedia.CodecTelephoneEvent8000,
			{Name: "telephone-event", PayloadType: 95, SampleRate: 8000},
		},
	}
	pts := BuildTelephoneEventPTSet(ms, []byte(sampleInviteSDP), diagomedia.CodecTelephoneEvent8000)
	if !pts[101] || !pts[95] {
		t.Fatalf("pts missing 101/95: %v", pts)
	}
	if !pts[96] || !pts[127] {
		t.Fatalf("pts missing dynamic range: %v", pts)
	}
	if pts[0] {
		t.Fatal("PCMU pt=0 must not be treated as DTMF")
	}
}

func TestTelephoneEventPTsRespectsAudioCodecPT(t *testing.T) {
	ms := &diagomedia.MediaSession{
		Codecs: []diagomedia.Codec{
			{PayloadType: 96, SampleRate: 48000, Name: "opus", NumChannels: 2},
			diagomedia.CodecTelephoneEvent8000,
		},
	}
	pts := BuildTelephoneEventPTSet(ms, nil, diagomedia.CodecAudioUlaw)
	if pts[96] {
		t.Fatal("opus pt=96 must not be treated as DTMF")
	}
	if !pts[101] || !pts[97] {
		t.Fatalf("expected 101 and 97 as DTMF pts, got %v", pts)
	}
}

func TestRFC2833DecoderDurationFallback(t *testing.T) {
	var dec rfc2833Decoder
	start := diagomedia.DTMFEncode(diagomedia.DTMFEvent{
		Event:    3,
		Duration: 160,
	})
	if _, ok := dec.processPayload(start, true); ok {
		t.Fatal("start packet should not emit digit")
	}
	payload := diagomedia.DTMFEncode(diagomedia.DTMFEvent{
		Event:    3,
		Duration: 700,
	})
	if d, ok := dec.processPayload(payload, false); !ok || d != '3' {
		t.Fatalf("got %q ok=%v want 3", d, ok)
	}
}

func TestIsDynamicPayloadType(t *testing.T) {
	if !isDynamicPayloadType(96) || !isDynamicPayloadType(127) {
		t.Fatal("96 and 127 should be dynamic")
	}
	if isDynamicPayloadType(95) || isDynamicPayloadType(128) {
		t.Fatal("95 and 128 should not be dynamic")
	}
}
