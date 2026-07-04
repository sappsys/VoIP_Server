package pcmcodec

import (
	"encoding/binary"

	diaudio "github.com/emiago/diago/audio"
)

func decodePCMU(_ *Handler, payload []byte) ([]int16, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	pcm := make([]byte, len(payload)*2)
	if _, err := diaudio.DecodeUlawTo(pcm, payload); err != nil {
		return nil, err
	}
	return bytesLEToSamples(pcm), nil
}

func encodePCMU(_ *Handler, samples []int16) ([]byte, error) {
	pcm := samplesToBytesLE(samples)
	out := make([]byte, len(samples))
	if _, err := diaudio.EncodeUlawTo(out, pcm); err != nil {
		return nil, err
	}
	return out, nil
}

func decodePCMA(_ *Handler, payload []byte) ([]int16, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	pcm := make([]byte, len(payload)*2)
	if _, err := diaudio.DecodeAlawTo(pcm, payload); err != nil {
		return nil, err
	}
	return bytesLEToSamples(pcm), nil
}

func encodePCMA(_ *Handler, samples []int16) ([]byte, error) {
	pcm := samplesToBytesLE(samples)
	out := make([]byte, len(samples))
	if _, err := diaudio.EncodeAlawTo(out, pcm); err != nil {
		return nil, err
	}
	return out, nil
}

func bytesLEToSamples(pcm []byte) []int16 {
	out := make([]int16, len(pcm)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}
	return out
}

func samplesToBytesLE(samples []int16) []byte {
	out := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(s))
	}
	return out
}

func resampleLinear(in []int16, inRate, outRate int) []int16 {
	if len(in) == 0 || inRate <= 0 || outRate <= 0 {
		return nil
	}
	if inRate == outRate {
		out := make([]int16, len(in))
		copy(out, in)
		return out
	}
	outLen := len(in) * outRate / inRate
	if outLen == 0 {
		return nil
	}
	out := make([]int16, outLen)
	last := len(in) - 1
	for i := range out {
		srcPos := float64(i) * float64(inRate) / float64(outRate)
		j := int(srcPos)
		if j >= last {
			out[i] = in[last]
			continue
		}
		frac := srcPos - float64(j)
		out[i] = int16(float64(in[j])*(1-frac) + float64(in[j+1])*frac)
	}
	return out
}
