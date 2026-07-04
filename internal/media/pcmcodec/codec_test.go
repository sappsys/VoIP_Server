package pcmcodec

import (
	"testing"

	diagomedia "github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/media"
)

func TestG711RoundTrip(t *testing.T) {
	for _, c := range []diagomedia.Codec{diagomedia.CodecAudioUlaw, diagomedia.CodecAudioAlaw} {
		h, err := New(c)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		samples := make([]int16, 160)
		for i := range samples {
			samples[i] = int16(i * 100)
		}
		pkt, err := h.Encode(samples)
		if err != nil || len(pkt) != 160 {
			t.Fatalf("%s encode: pkt=%d err=%v", c.Name, len(pkt), err)
		}
		out, err := h.Decode(pkt)
		if err != nil || len(out) != 160 {
			t.Fatalf("%s decode: len=%d err=%v", c.Name, len(out), err)
		}
	}
}

func TestG729RoundTrip(t *testing.T) {
	c := media.VoiceCodecTable()[media.CodecG729]
	h, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]int16, 160)
	pkt, err := h.Encode(samples)
	if err != nil || len(pkt) != 20 {
		t.Fatalf("encode len=%d err=%v", len(pkt), err)
	}
	out, err := h.Decode(pkt)
	if err != nil || len(out) != 160 {
		t.Fatalf("decode len=%d err=%v", len(out), err)
	}
}

func TestG726RoundTrip(t *testing.T) {
	c := media.VoiceCodecTable()[media.CodecG72632]
	h, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]int16, 160)
	pkt, err := h.Encode(samples)
	if err != nil || len(pkt) == 0 {
		t.Fatalf("encode len=%d err=%v", len(pkt), err)
	}
	out, err := h.Decode(pkt)
	if err != nil || len(out) != 160 {
		t.Fatalf("decode len=%d err=%v", len(out), err)
	}
}

func TestG722RoundTrip(t *testing.T) {
	c := media.VoiceCodecTable()[media.CodecG722]
	h, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]int16, 160)
	pkt, err := h.Encode(samples)
	if err != nil || len(pkt) == 0 {
		t.Fatalf("encode len=%d err=%v", len(pkt), err)
	}
	out, err := h.Decode(pkt)
	if err != nil || len(out) != 160 {
		t.Fatalf("decode len=%d err=%v", len(out), err)
	}
}

func TestG723Unsupported(t *testing.T) {
	c := media.VoiceCodecTable()[media.CodecG72363]
	if Supported(c) {
		t.Fatal("g723 should not be supported yet")
	}
}
