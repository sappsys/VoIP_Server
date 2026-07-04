package audiobridge

import (
	"testing"

	diagomedia "github.com/emiago/diago/media"
)

// REQ-BRIDGE-5: mixed-codec deployments must transcode common PBX codecs through PCM hub.
// REQ-BRIDGE-6: all two-party calls use PCM bridge path (low latency relay when same codec).

func TestREQ_BRIDGE_CommonCodecsTranscode(t *testing.T) {
	pairs := [][2]diagomedia.Codec{
		{diagomedia.CodecAudioUlaw, diagomedia.CodecAudioAlaw},
		{diagomedia.CodecAudioUlaw, {PayloadType: 9, SampleRate: 8000, Name: "G722"}},
		{diagomedia.CodecAudioAlaw, {PayloadType: 18, SampleRate: 8000, Name: "G729"}},
	}
	for _, p := range pairs {
		if !NeedsTranscoding(p[0], p[1]) {
			t.Fatalf("REQ-BRIDGE-5: %s↔%s must transcode", p[0].Name, p[1].Name)
		}
		if !CanTranscode(p[0]) || !CanTranscode(p[1]) {
			t.Fatalf("REQ-BRIDGE-5: %s and %s must be supported", p[0].Name, p[1].Name)
		}
	}
}

func TestREQ_BRIDGE_SameCodecPassthroughLowLatency(t *testing.T) {
	if NeedsTranscoding(diagomedia.CodecAudioUlaw, diagomedia.CodecAudioUlaw) {
		t.Fatal("REQ-BRIDGE-6: same codec must use passthrough relay")
	}
}
