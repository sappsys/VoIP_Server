package call

import (
	"errors"
	"fmt"
	"io"
	"os"

	diagomedia "github.com/emiago/diago/media"
	mp3 "github.com/hajimehoshi/go-mp3"
)

// loadMP3PCMForCodec decodes an MP3 file to 16-bit little-endian mono PCM at the
// negotiated codec sample rate.
func loadMP3PCMForCodec(path string, codec diagomedia.Codec) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec, err := mp3.NewDecoder(f)
	if err != nil {
		return nil, fmt.Errorf("mp3 decoder: %w", err)
	}

	raw, err := readAllMP3PCM(dec)
	if err != nil {
		return nil, err
	}
	samples, err := pcmBytesToInt16(raw)
	if err != nil {
		return nil, err
	}
	channels := 2
	if len(samples)%2 != 0 {
		channels = 1
	}
	mono, err := downmixToMono(samples, channels)
	if err != nil {
		return nil, err
	}
	samples = mono
	outRate := int(codec.SampleRate)
	inRate := dec.SampleRate()
	if inRate != outRate {
		samples = resampleLinear(samples, inRate, outRate)
	}
	return int16ToPCMBytes(samples), nil
}

func readAllMP3PCM(dec *mp3.Decoder) ([]byte, error) {
	const chunk = 32 * 1024
	var out []byte
	buf := make([]byte, chunk)
	for {
		n, err := dec.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, err
		}
	}
}
