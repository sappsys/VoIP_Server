package call

import (
	"encoding/binary"
	"math"
	"testing"

	diagomedia "github.com/emiago/diago/media"
)

func TestDetectInbandDigit(t *testing.T) {
	for _, tc := range []struct {
		digit rune
		low   int
		high  int
	}{
		{'1', 0, 0},
		{'5', 1, 1},
		{'0', 3, 1},
		{'#', 3, 2},
	} {
		mags := [8]float64{}
		mags[tc.low] = goertzelToneMagnitude(inbandDTMFFreqs[tc.low])
		mags[4+tc.high] = goertzelToneMagnitude(inbandDTMFFreqs[4+tc.high])
		got, ok := detectInbandDigit(mags)
		if !ok {
			t.Fatalf("digit %q not detected", tc.digit)
		}
		if got != tc.digit {
			t.Fatalf("digit %q got %q", tc.digit, got)
		}
	}
}

func TestInbandDetectorFeedPCM(t *testing.T) {
	d := newInbandDetector()
	var digits []rune
	for range 6 {
		pcm := synthesizeDTMF('4', 80)
		for _, digit := range d.FeedPCM(pcm) {
			digits = append(digits, digit)
		}
		pcm = make([]byte, inbandBlockSamples*2*inbandMinGap)
		for _, digit := range d.FeedPCM(pcm) {
			digits = append(digits, digit)
		}
	}
	if len(digits) == 0 {
		t.Fatal("expected at least one in-band digit")
	}
	if digits[0] != '4' {
		t.Fatalf("first digit=%q want 4", digits[0])
	}
}

func TestRFC2833Decoder(t *testing.T) {
	var dec rfc2833Decoder
	events := diagomedia.RTPDTMFEncode8000('7')
	var got []rune
	for i, ev := range events {
		payload := diagomedia.DTMFEncode(ev)
		marker := i == 0
		if d, ok := dec.processPayload(payload, marker); ok {
			got = append(got, d)
		}
	}
	if len(got) != 1 || got[0] != '7' {
		t.Fatalf("got %q want 7", got)
	}
}

// synthesizeDTMF generates little-endian 16-bit PCM for a DTMF digit.
func synthesizeDTMF(digit rune, durationMs int) []byte {
	lowFreq, highFreq := dtmfFreqsForDigit(digit)
	samples := inbandSampleRate * float64(durationMs) / 1000
	buf := make([]byte, int(samples)*2)
	for i := 0; i < int(samples); i++ {
		t := float64(i) / inbandSampleRate
		v := 0.45*math.Sin(2*math.Pi*lowFreq*t) + 0.45*math.Sin(2*math.Pi*highFreq*t)
		sample := int16(v * 32767)
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(sample))
	}
	return buf
}

func dtmfFreqsForDigit(digit rune) (low, high float64) {
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			if inbandDTMFDigit[row][col] == digit {
				return inbandDTMFFreqs[row], inbandDTMFFreqs[4+col]
			}
		}
	}
	return inbandDTMFFreqs[0], inbandDTMFFreqs[4]
}

func goertzelToneMagnitude(freq float64) float64 {
	bin := newGoertzelBin(freq, inbandSampleRate, inbandBlockSamples)
	for i := 0; i < inbandBlockSamples; i++ {
		t := float64(i) / inbandSampleRate
		sample := int16(0.5 * 32767 * math.Sin(2*math.Pi*freq*t))
		bin.feed(sample)
	}
	return bin.magnitude2()
}
