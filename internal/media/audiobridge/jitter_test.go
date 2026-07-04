package audiobridge

import (
	"testing"

	diagomedia "github.com/emiago/diago/media"
)

func TestNeedsTranscoding(t *testing.T) {
	if !NeedsTranscoding(diagomedia.CodecAudioUlaw, diagomedia.CodecAudioAlaw) {
		t.Fatal("PCMU vs PCMA should need transcoding")
	}
	if NeedsTranscoding(diagomedia.CodecAudioUlaw, diagomedia.CodecAudioUlaw) {
		t.Fatal("same codec should not need transcoding")
	}
}

func TestCanTranscodeG722(t *testing.T) {
	c := diagomedia.Codec{PayloadType: 9, SampleRate: 8000, Name: "G722"}
	if !CanTranscode(c) {
		t.Fatal("g722 should transcode")
	}
}

func TestPCMJitterBufferPrefill(t *testing.T) {
	j := newPCMJitterBuffer()
	if j.popFrame(160) != nil {
		t.Fatal("expected empty before prefill")
	}
	samples := make([]int16, minPCMSamples)
	j.push(samples)
	if j.popFrame(160) == nil {
		t.Fatal("expected frame after prefill")
	}
}
