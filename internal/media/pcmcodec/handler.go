package pcmcodec

import (
	"fmt"
	"time"

	diagomedia "github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/media"
)

// HubSampleRate is the internal PCM sample rate used for transcoding bridges.
const HubSampleRate = 8000

// Handler decodes RTP payloads to 8 kHz mono PCM and encodes back.
type Handler struct {
	codec       diagomedia.Codec
	hubFrame    int // PCM samples per bridge frame at HubSampleRate
	encodeFrame int // encoded frame size hint
	decode      decodeFn
	encode      encodeFn
	state       any
}

type decodeFn func(h *Handler, payload []byte) ([]int16, error)
type encodeFn func(h *Handler, samples []int16) ([]byte, error)

// Supported reports whether transcoding is implemented for this codec.
func Supported(c diagomedia.Codec) bool {
	_, err := New(c)
	return err == nil
}

// New builds a codec handler for the negotiated codec.
func New(c diagomedia.Codec) (*Handler, error) {
	h := &Handler{codec: c}
	switch {
	case c.PayloadType == 0 && c.Name == "PCMU":
		h.hubFrame = samplesPerFrame(c)
		h.decode = decodePCMU
		h.encode = encodePCMU
	case c.PayloadType == 8 && c.Name == "PCMA":
		h.hubFrame = samplesPerFrame(c)
		h.decode = decodePCMA
		h.encode = encodePCMA
	case c.Name == "G722" || c.PayloadType == 9:
		// G.722 RTP clock is 8 kHz but PCM hub is 8 kHz mono; one 20 ms RTP
		// frame decodes to 80 hub samples (wideband resampled), not 160.
		h.hubFrame = samplesPerFrame(c) / 2
		if h.hubFrame == 0 {
			h.hubFrame = 80
		}
		st, err := newG722State()
		if err != nil {
			return nil, err
		}
		h.state = st
		h.decode = decodeG722
		h.encode = encodeG722
	case c.Name == "G729" || c.PayloadType == 18:
		h.hubFrame = 160 // two 10 ms G.729 frames
		st, err := newG729State()
		if err != nil {
			return nil, err
		}
		h.state = st
		h.decode = decodeG729
		h.encode = encodeG729
	case isG726(c):
		bps, err := g726Bits(c)
		if err != nil {
			return nil, err
		}
		h.hubFrame = samplesPerFrame(c)
		st, err := newG726State(bps)
		if err != nil {
			return nil, err
		}
		h.state = st
		h.decode = decodeG726
		h.encode = encodeG726
	default:
		return nil, fmt.Errorf("codec %s pt=%d not supported for transcoding", c.Name, c.PayloadType)
	}
	return h, nil
}

// HubFrameSamples returns PCM samples per bridge tick at 8 kHz.
func (h *Handler) HubFrameSamples() int {
	if h == nil || h.hubFrame == 0 {
		return 160
	}
	return h.hubFrame
}

func (h *Handler) Decode(payload []byte) ([]int16, error) {
	if h == nil || h.decode == nil {
		return nil, fmt.Errorf("codec handler not initialized")
	}
	return h.decode(h, payload)
}

func (h *Handler) Encode(samples []int16) ([]byte, error) {
	if h == nil || h.encode == nil {
		return nil, fmt.Errorf("codec handler not initialized")
	}
	return h.encode(h, samples)
}

func samplesPerFrame(c diagomedia.Codec) int {
	dur := c.SampleDur
	if dur <= 0 {
		dur = 20 * time.Millisecond
	}
	rate := int(c.SampleRate)
	if rate <= 0 {
		rate = HubSampleRate
	}
	return rate * int(dur.Milliseconds()) / 1000
}

func isG726(c diagomedia.Codec) bool {
	switch c.Name {
	case "G726-16", "G726-24", "G726-32", "G726-40", "G726":
		return true
	default:
		return false
	}
}

func g726Bits(c diagomedia.Codec) (int, error) {
	switch c.Name {
	case "G726-16":
		return 2, nil
	case "G726-24":
		return 3, nil
	case "G726-32", "G726":
		return 4, nil
	case "G726-40":
		return 5, nil
	default:
		if c.PayloadType == 2 {
			return 4, nil
		}
		return 0, fmt.Errorf("unknown G.726 variant %q", c.Name)
	}
}

// CodecID returns the config table ID for a negotiated codec when known.
func CodecID(c diagomedia.Codec) string {
	switch {
	case c.PayloadType == 0:
		return media.CodecPCMU
	case c.PayloadType == 8:
		return media.CodecPCMA
	case c.Name == "G722" || c.PayloadType == 9:
		return media.CodecG722
	case c.Name == "G729" || c.PayloadType == 18:
		return media.CodecG729
	case c.Name == "G726-32" || (c.PayloadType == 2 && c.Name == "G726-32"):
		return media.CodecG72632
	case c.Name == "G726-16":
		return media.CodecG72616
	case c.Name == "G726-24":
		return media.CodecG72624
	case c.Name == "G726-40":
		return media.CodecG72640
	default:
		return c.Name
	}
}
