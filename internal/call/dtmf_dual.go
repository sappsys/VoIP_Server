package call

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	diagomedia "github.com/emiago/diago/media"
	diaudio "github.com/emiago/diago/audio"
)

// dualDTMFCollector detects RFC 2833 telephone-event packets and in-band DTMF
// tones from decoded G.711 audio on the same RTP stream.
type dualDTMFCollector struct {
	reader       io.Reader
	packetReader *diagomedia.RTPPacketReader
	mediaSession *diagomedia.MediaSession
	audioCodec   diagomedia.Codec
	dtmfCodec    diagomedia.Codec
	pcmDec       diaudio.PCMDecoder
	pcmScratch   []byte
	inband       *inbandDetector
	rfc          rfc2833Decoder
	onDTMF       func(dtmf rune) error
}

func newDualDTMFCollector(
	audioCodec, dtmfCodec diagomedia.Codec,
	packetReader *diagomedia.RTPPacketReader,
	reader io.Reader,
	mediaSession *diagomedia.MediaSession,
) (*dualDTMFCollector, error) {
	d := &dualDTMFCollector{
		reader:       reader,
		packetReader: packetReader,
		mediaSession: mediaSession,
		audioCodec:   audioCodec,
		dtmfCodec:    dtmfCodec,
		pcmScratch:   make([]byte, audioCodec.SamplesPCM(16)*2),
	}
	if err := d.pcmDec.Init(audioCodec); err != nil {
		return nil, err
	}
	if inbandSupportedCodec(audioCodec) {
		d.inband = newInbandDetector()
	}
	return d, nil
}

func (d *dualDTMFCollector) Read(buf []byte) (int, error) {
	n, err := d.reader.Read(buf)
	if err != nil {
		return n, err
	}

	hdr := d.packetReader.PacketHeader
	payload := buf[:n]

	if hdr.PayloadType == d.dtmfCodec.PayloadType {
		marker := hdr.Marker || d.rfc.lastTimestamp != hdr.Timestamp
		if digit, ok := d.rfc.processPayload(payload, marker); ok && d.onDTMF != nil {
			if err := d.onDTMF(digit); err != nil {
				return n, err
			}
		}
		d.rfc.lastTimestamp = hdr.Timestamp
		return n, nil
	}

	if d.inband != nil && hdr.PayloadType == d.audioCodec.PayloadType {
		need := len(payload) * 2
		if len(d.pcmScratch) < need {
			d.pcmScratch = make([]byte, need)
		}
		nn, decErr := d.pcmDec.DecoderTo(d.pcmScratch, payload)
		if decErr == nil && nn > 0 && d.onDTMF != nil {
			for _, digit := range d.inband.FeedPCM(d.pcmScratch[:nn]) {
				if err := d.onDTMF(digit); err != nil {
					return n, err
				}
			}
		}
	}
	return n, nil
}

func (d *dualDTMFCollector) listen(ctx context.Context, onDTMF func(dtmf rune) error) error {
	d.onDTMF = onDTMF
	buf := make([]byte, diagomedia.RTPBufSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, err := d.readDeadline(buf, 250*time.Millisecond); err != nil {
			if errors.Is(err, errDTMFComplete) {
				return nil
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			return err
		}
	}
}

func (d *dualDTMFCollector) readDeadline(buf []byte, dur time.Duration) (int, error) {
	if d.mediaSession != nil && dur > 0 {
		d.mediaSession.StopRTP(1, dur)
		defer d.mediaSession.StartRTP(2)
	}
	return d.Read(buf)
}

func audioCodecFromSession(ms *diagomedia.MediaSession) (diagomedia.Codec, bool) {
	if ms == nil {
		return diagomedia.Codec{}, false
	}
	if c, ok := diagomedia.CodecAudioFromList(ms.CommonCodecs()); ok {
		return c, true
	}
	return diagomedia.CodecAudioFromList(ms.Codecs)
}
