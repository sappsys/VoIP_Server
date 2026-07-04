package call

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	diagomedia "github.com/emiago/diago/media"
	diaudio "github.com/emiago/diago/audio"
	"github.com/sappsys/VoIP_Server/internal/media/pcmcodec"
)

// dualDTMFCollector detects RFC 2833 telephone-event packets and in-band DTMF
// tones from decoded G.711 audio on the same RTP stream.
type dualDTMFCollector struct {
	reader       io.Reader
	packetReader *diagomedia.RTPPacketReader
	mediaSession *diagomedia.MediaSession
	audioCodec   diagomedia.Codec
	dtmfPTs      map[uint8]bool
	pcmDec       diaudio.PCMDecoder
	pcmG722      *pcmcodec.Handler
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
	sdpBodies [][]byte,
) (*dualDTMFCollector, error) {
	if packetReader == nil {
		return nil, errors.New("dtmf: nil packet reader")
	}
	if reader == nil {
		reader = packetReader
	}
	d := &dualDTMFCollector{
		reader:       reader,
		packetReader: packetReader,
		mediaSession: mediaSession,
		audioCodec:   audioCodec,
		dtmfPTs:      buildTelephoneEventPTSetFromSDPs(mediaSession, sdpBodies, dtmfCodec),
		pcmScratch:   make([]byte, audioCodec.SamplesPCM(16)*2),
	}
	if inbandSupportedCodec(audioCodec) {
		if err := d.pcmDec.Init(audioCodec); err != nil {
			return nil, err
		}
		d.inband = newInbandDetector()
	} else if audioCodec.Name == "G722" || audioCodec.PayloadType == 9 {
		h, err := pcmcodec.New(audioCodec)
		if err != nil {
			return nil, err
		}
		d.pcmG722 = h
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

	if d.isDTMFPacket(hdr.PayloadType) {
		marker := hdr.Marker || d.rfc.lastTimestamp != hdr.Timestamp
		if digit, ok := d.rfc.processPayload(payload, marker); ok && d.onDTMF != nil {
			if err := d.onDTMF(digit); err != nil {
				return n, err
			}
		}
		d.rfc.lastTimestamp = hdr.Timestamp
		return n, nil
	}

	if d.inband != nil && d.isAudioPacket(hdr.PayloadType) {
		pcm := d.decodeAudioToPCM(payload)
		if len(pcm) > 0 && d.onDTMF != nil {
			for _, digit := range d.inband.FeedPCM(pcm) {
				if err := d.onDTMF(digit); err != nil {
					return n, err
				}
			}
		}
	}
	return n, nil
}

func (d *dualDTMFCollector) isAudioPacket(pt uint8) bool {
	if pt == d.audioCodec.PayloadType {
		return true
	}
	if d.audioCodec.Name == "G722" && pt == 9 {
		return true
	}
	return false
}

func (d *dualDTMFCollector) decodeAudioToPCM(payload []byte) []byte {
	if d.pcmG722 != nil {
		samples, err := d.pcmG722.Decode(payload)
		if err != nil || len(samples) == 0 {
			return nil
		}
		out := make([]byte, len(samples)*2)
		for i, s := range samples {
			out[i*2] = byte(s)
			out[i*2+1] = byte(s >> 8)
		}
		return out
	}
	if !inbandSupportedCodec(d.audioCodec) {
		return nil
	}
	need := len(payload) * 2
	if len(d.pcmScratch) < need {
		d.pcmScratch = make([]byte, need)
	}
	nn, decErr := d.pcmDec.DecoderTo(d.pcmScratch, payload)
	if decErr != nil || nn <= 0 {
		return nil
	}
	return d.pcmScratch[:nn]
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

func (d *dualDTMFCollector) isDTMFPacket(pt uint8) bool {
	return d.dtmfPTs[pt]
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
