package call

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	diaudio "github.com/emiago/diago/audio"
	diagomedia "github.com/emiago/diago/media"
)

// wavPCMInfo describes decoded PCM from a WAV file header.
type wavPCMInfo struct {
	sampleRate   int
	numChannels  int
	bitsPerSample int
}

func readWavPCMInfo(path string) (wavPCMInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return wavPCMInfo{}, err
	}
	defer f.Close()

	wr := diaudio.NewWavReader(f)
	if err := wr.ReadHeaders(); err != nil {
		return wavPCMInfo{}, err
	}
	return wavPCMInfo{
		sampleRate:    int(wr.SampleRate),
		numChannels:   int(wr.NumChannels),
		bitsPerSample: int(wr.BitsPerSample),
	}, nil
}

func wavMatchesCodec(info wavPCMInfo, codec diagomedia.Codec) bool {
	return info.bitsPerSample == 16 &&
		info.sampleRate == int(codec.SampleRate) &&
		info.numChannels == codec.NumChannels
}

// loadWavPCMForCodec reads a WAV file and returns 16-bit little-endian mono PCM
// at the codec sample rate, resampling/downmixing when needed.
func loadWavPCMForCodec(path string, codec diagomedia.Codec) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	wr := diaudio.NewWavReader(f)
	if err := wr.ReadHeaders(); err != nil {
		return nil, fmt.Errorf("wav headers: %w", err)
	}
	if wr.BitsPerSample != 16 {
		return nil, fmt.Errorf("wav bit depth %d: need 16-bit PCM", wr.BitsPerSample)
	}

	raw, err := io.ReadAll(wr)
	if err != nil {
		return nil, fmt.Errorf("wav pcm read: %w", err)
	}
	samples, err := pcmBytesToInt16(raw)
	if err != nil {
		return nil, err
	}

	mono, err := downmixToMono(samples, int(wr.NumChannels))
	if err != nil {
		return nil, err
	}

	outRate := int(codec.SampleRate)
	inRate := int(wr.SampleRate)
	if inRate != outRate {
		mono = resampleLinear(mono, inRate, outRate)
	}
	return int16ToPCMBytes(mono), nil
}

func pcmBytesToInt16(raw []byte) ([]int16, error) {
	if len(raw)%2 != 0 {
		return nil, fmt.Errorf("pcm length %d is not even", len(raw))
	}
	out := make([]int16, len(raw)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(raw[i*2 : i*2+2]))
	}
	return out, nil
}

func int16ToPCMBytes(samples []int16) []byte {
	out := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(s))
	}
	return out
}

func downmixToMono(samples []int16, channels int) ([]int16, error) {
	switch channels {
	case 1:
		return samples, nil
	case 2:
		if len(samples)%2 != 0 {
			return nil, fmt.Errorf("stereo pcm length %d is odd", len(samples))
		}
		mono := make([]int16, len(samples)/2)
		for i := 0; i < len(mono); i++ {
			l := int32(samples[i*2])
			r := int32(samples[i*2+1])
			mono[i] = int16((l + r) / 2)
		}
		return mono, nil
	default:
		return nil, fmt.Errorf("unsupported channel count %d", channels)
	}
}

// resampleLinear converts PCM to outRate using linear interpolation.
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

// openAudioPlaybackReader returns a reader and mime type suitable for AudioPlayback.Play.
// WAV and MP3 are decoded/resampled to the negotiated codec rate. Native 8 kHz mono WAVs
// stream as audio/wav; others are normalized to PCM and must use audio/pcm.
func openAudioPlaybackReader(path string, codec diagomedia.Codec) (io.Reader, string, error) {
	if isMP3Path(path) {
		pcm, err := loadMP3PCMForCodec(path, codec)
		if err != nil {
			return nil, "", err
		}
		return bytes.NewReader(pcm), "audio/pcm", nil
	}
	return openWavPlaybackReader(path, codec)
}

func isMP3Path(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".mp3"
}

// openWavPlaybackReader returns a reader and mime type for WAV prompts.
func openWavPlaybackReader(path string, codec diagomedia.Codec) (io.Reader, string, error) {
	info, err := readWavPCMInfo(path)
	if err != nil {
		return nil, "", err
	}
	if wavMatchesCodec(info, codec) {
		f, err := os.Open(path)
		if err != nil {
			return nil, "", err
		}
		return f, "audio/wav", nil
	}

	pcm, err := loadWavPCMForCodec(path, codec)
	if err != nil {
		return nil, "", err
	}
	return bytes.NewReader(pcm), "audio/pcm", nil
}
