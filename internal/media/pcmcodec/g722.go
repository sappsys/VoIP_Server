package pcmcodec

import (
	"fmt"

	g722 "github.com/gotranspile/g722"
)

type g722State struct {
	dec *g722.Decoder
	enc *g722.Encoder
}

func newG722State() (*g722State, error) {
	return &g722State{
		dec: g722.NewDecoder(g722.Rate64000, 0),
		enc: g722.NewEncoder(g722.Rate64000, 0),
	}, nil
}

func decodeG722(h *Handler, payload []byte) ([]int16, error) {
	st, ok := h.state.(*g722State)
	if !ok || st.dec == nil {
		return nil, fmt.Errorf("g722 decoder unavailable")
	}
	if len(payload) == 0 {
		return nil, nil
	}
	wide := make([]int16, len(payload)*2)
	n := st.dec.Decode(wide, payload)
	wide = wide[:n]
	// G.722 carries 16 kHz audio; bridge hub is 8 kHz.
	return resampleLinear(wide, 16000, HubSampleRate), nil
}

func encodeG722(h *Handler, samples []int16) ([]byte, error) {
	st, ok := h.state.(*g722State)
	if !ok || st.enc == nil {
		return nil, fmt.Errorf("g722 encoder unavailable")
	}
	wide := resampleLinear(samples, HubSampleRate, 16000)
	if len(wide) == 0 {
		return nil, nil
	}
	out := make([]byte, len(wide)/2+1)
	n := st.enc.Encode(out, wide)
	return out[:n], nil
}
