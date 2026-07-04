package pcmcodec

import (
	"fmt"

	g726 "github.com/Distortions81/g726"
)

type g726State struct {
	bps    g726.BitsPerSample
	dec    *g726.Decoder
	enc    *g726.Encoder
}

func newG726State(bits int) (*g726State, error) {
	bps := g726.BitsPerSample(bits)
	dec, err := g726.NewDecoder(bps)
	if err != nil {
		return nil, err
	}
	enc, err := g726.NewEncoder(bps)
	if err != nil {
		return nil, err
	}
	return &g726State{bps: bps, dec: dec, enc: enc}, nil
}

func decodeG726(h *Handler, payload []byte) ([]int16, error) {
	st, ok := h.state.(*g726State)
	if !ok || st.dec == nil {
		return nil, fmt.Errorf("g726 decoder unavailable")
	}
	if len(payload) == 0 {
		return nil, nil
	}
	return st.dec.Decode(payload)
}

func encodeG726(h *Handler, samples []int16) ([]byte, error) {
	st, ok := h.state.(*g726State)
	if !ok || st.enc == nil {
		return nil, fmt.Errorf("g726 encoder unavailable")
	}
	if len(samples) == 0 {
		return nil, nil
	}
	return st.enc.Encode(samples)
}
