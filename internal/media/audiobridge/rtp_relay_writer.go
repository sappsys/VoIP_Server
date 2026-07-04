package audiobridge

import (
	"io"

	"github.com/emiago/diago"
	diagomedia "github.com/emiago/diago/media"
)

// rtpSampleWriter is the subset of RTPPacketWriter used for low-latency relay.
type rtpSampleWriter interface {
	WriteSamples(payload []byte, sampleRateTimestamp uint32, marker bool, payloadType uint8) (int, error)
}

// rtpRelayWriter sends RTP using WriteSamples, bypassing diago's playback clock in
// Write() (which blocks ~20 ms per packet and queues seconds of audio on bridges).
// ClockDisable() must not be used — diago panics on nil clockTicker in Write().
type rtpRelayWriter struct {
	w      rtpSampleWriter
	codec  diagomedia.Codec
	marker bool
}

func rtpPacketWriterForLeg(leg SessionLeg) *diagomedia.RTPPacketWriter {
	if leg == nil {
		return nil
	}
	switch s := leg.(type) {
	case *diago.DialogServerSession:
		return s.RTPPacketWriter
	case *diago.DialogClientSession:
		return s.RTPPacketWriter
	default:
		return nil
	}
}

// audioRelayWriter returns a low-latency writer for bridging/transcoding.
func audioRelayWriter(leg SessionLeg, codec diagomedia.Codec) (io.Writer, error) {
	if pw := rtpPacketWriterForLeg(leg); pw != nil {
		return &rtpRelayWriter{w: pw, codec: codec, marker: true}, nil
	}
	return leg.AudioWriter()
}

// newRTPRelayWriterForTest exposes relay construction for unit tests.
func newRTPRelayWriterForTest(w rtpSampleWriter, codec diagomedia.Codec) io.Writer {
	return &rtpRelayWriter{w: w, codec: codec, marker: true}
}

func (r *rtpRelayWriter) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	marker := r.marker
	r.marker = false
	return r.w.WriteSamples(b, r.codec.SampleTimestamp(), marker, r.codec.PayloadType)
}
