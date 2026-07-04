package audiobridge

import (
	"testing"

	diagomedia "github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/media/pcmcodec"
)

// REQ-BRIDGE-1: same negotiated codec must not transcode.
// REQ-BRIDGE-2: relay path uses WriteSamples (no playback clock / ClockDisable).
// REQ-BRIDGE-3: G.722 hub frame is 80 samples at 8 kHz (one RTP packet).

type recordingSampleWriter struct {
	writes     int
	lastMarker bool
}

func (r *recordingSampleWriter) WriteSamples(payload []byte, _ uint32, marker bool, _ uint8) (int, error) {
	r.writes++
	r.lastMarker = marker
	return len(payload), nil
}

func TestREQ_BRIDGE_SameCodecNoTranscode(t *testing.T) {
	if NeedsTranscoding(diagomedia.CodecAudioUlaw, diagomedia.CodecAudioUlaw) {
		t.Fatal("REQ-BRIDGE-1: PCMU↔PCMU must use passthrough")
	}
	if !NeedsTranscoding(diagomedia.CodecAudioUlaw, diagomedia.CodecAudioAlaw) {
		t.Fatal("different codecs must transcode")
	}
}

func TestREQ_BRIDGE_RelayUsesWriteSamples(t *testing.T) {
	rec := &recordingSampleWriter{}
	w := newRTPRelayWriterForTest(rec, diagomedia.CodecAudioUlaw)

	if _, err := w.Write([]byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte{4}); err != nil {
		t.Fatal(err)
	}
	if rec.writes != 2 {
		t.Fatalf("REQ-BRIDGE-2: expected WriteSamples twice, got %d", rec.writes)
	}
	if rec.lastMarker {
		t.Fatal("second packet must not repeat marker bit")
	}
}

func TestREQ_BRIDGE_FirstPacketHasMarker(t *testing.T) {
	rec := &recordingSampleWriter{}
	w := newRTPRelayWriterForTest(rec, diagomedia.CodecAudioUlaw)
	if _, err := w.Write([]byte{0xff}); err != nil {
		t.Fatal(err)
	}
	if !rec.lastMarker {
		t.Fatal("first relayed packet must set RTP marker bit")
	}
}

func TestREQ_BRIDGE_G722HubFrame80Samples(t *testing.T) {
	h, err := pcmcodec.New(diagomedia.Codec{PayloadType: 9, SampleRate: 8000, Name: "G722"})
	if err != nil {
		t.Fatal(err)
	}
	if got := h.HubFrameSamples(); got != 80 {
		t.Fatalf("REQ-BRIDGE-3: G722 hub frame=%d want 80", got)
	}
}
