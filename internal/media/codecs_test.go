package media

import (
	"testing"

	diagomedia "github.com/emiago/diago/media"
)

func TestDefaultVoiceCodecs(t *testing.T) {
	codecs, err := VoiceCodecs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(codecs) < 10 {
		t.Fatalf("expected full default set, got %d", len(codecs))
	}
	if codecs[0].Name != "PCMU" {
		t.Fatalf("first codec=%s want PCMU", codecs[0].Name)
	}
	last := codecs[len(codecs)-1]
	if last.Name != "telephone-event" {
		t.Fatalf("last codec=%s want telephone-event", last.Name)
	}
}

func TestVoiceCodecsDedupPayloadType(t *testing.T) {
	codecs, err := VoiceCodecs([]string{"PCMU", "G723", "G723-63", "G723-53"})
	if err != nil {
		t.Fatal(err)
	}
	g723 := 0
	for _, c := range codecs {
		if c.Name == "G723" {
			g723++
		}
	}
	if g723 != 2 {
		t.Fatalf("expected G723 5.3 and 6.3 variants, got %d G723 entries: %+v", g723, codecs)
	}
}

func TestVoiceCodecsUnknown(t *testing.T) {
	_, err := VoiceCodecs([]string{"PCMU", "VP9"})
	if err == nil {
		t.Fatal("expected unknown codec error")
	}
}

func TestNormalizeCodecID(t *testing.T) {
	cases := map[string]string{
		"g729a":        CodecG729,
		"G.711A":       CodecPCMA,
		"G723(5.3kbps)": CodecG72353,
		"G726(32kbps)": CodecG72632,
	}
	for in, want := range cases {
		if got := normalizeCodecID(in); got != want {
			t.Fatalf("normalizeCodecID(%q)=%q want %q", in, got, want)
		}
	}
}

func TestG726PayloadTypes(t *testing.T) {
	codecs, err := VoiceCodecs([]string{"G726-16", "G726-24", "G726-32", "G726-40"})
	if err != nil {
		t.Fatal(err)
	}
	pts := map[uint8]string{}
	for _, c := range codecs {
		pts[c.PayloadType] = c.Name
	}
	if pts[2] != "G726-32" {
		t.Fatalf("G726-32 pt=2 missing: %+v", pts)
	}
	if pts[111] != "G726-16" || pts[112] != "G726-24" || pts[113] != "G726-40" {
		t.Fatalf("dynamic G726 pts wrong: %+v", pts)
	}
}

func TestVoiceCodecsMatchesDiagoConstants(t *testing.T) {
	codecs, _ := VoiceCodecs([]string{"PCMU", "PCMA"})
	if codecs[0] != diagomedia.CodecAudioUlaw || codecs[1] != diagomedia.CodecAudioAlaw {
		t.Fatalf("G711 mismatch: %+v", codecs)
	}
}
