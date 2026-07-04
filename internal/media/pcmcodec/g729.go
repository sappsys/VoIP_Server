package pcmcodec

import (
	"fmt"

	g729 "github.com/hunydev/g729"
)

const (
	g729FrameSamples = 80
	g729FrameBytes   = 10
)

type g729State struct {
	dec *g729.Decoder
	enc *g729.Encoder
}

func newG729State() (*g729State, error) {
	return &g729State{
		dec: g729.NewDecoder(),
		enc: g729.NewEncoder(),
	}, nil
}

func decodeG729(h *Handler, payload []byte) ([]int16, error) {
	st, ok := h.state.(*g729State)
	if !ok || st.dec == nil {
		return nil, fmt.Errorf("g729 decoder unavailable")
	}
	if len(payload) == 0 {
		return nil, nil
	}
	if len(payload)%g729FrameBytes != 0 {
		return nil, fmt.Errorf("g729 payload len %d not multiple of %d", len(payload), g729FrameBytes)
	}
	frames := len(payload) / g729FrameBytes
	out := make([]int16, 0, frames*g729FrameSamples)
	framePCM := make([]int16, g729FrameSamples)
	for i := 0; i < frames; i++ {
		chunk := payload[i*g729FrameBytes : (i+1)*g729FrameBytes]
		if err := st.dec.DecodeFrame(chunk, framePCM); err != nil {
			return nil, err
		}
		out = append(out, framePCM...)
	}
	return out, nil
}

func encodeG729(h *Handler, samples []int16) ([]byte, error) {
	st, ok := h.state.(*g729State)
	if !ok || st.enc == nil {
		return nil, fmt.Errorf("g729 encoder unavailable")
	}
	if len(samples) == 0 {
		return nil, nil
	}
	if len(samples)%g729FrameSamples != 0 {
		return nil, fmt.Errorf("g729 pcm len %d not multiple of %d", len(samples), g729FrameSamples)
	}
	frames := len(samples) / g729FrameSamples
	out := make([]byte, 0, frames*g729FrameBytes)
	frameBits := make([]byte, g729FrameBytes)
	for i := 0; i < frames; i++ {
		chunk := samples[i*g729FrameSamples : (i+1)*g729FrameSamples]
		if err := st.enc.EncodeFrame(chunk, frameBits); err != nil {
			return nil, err
		}
		out = append(out, frameBits...)
	}
	return out, nil
}
