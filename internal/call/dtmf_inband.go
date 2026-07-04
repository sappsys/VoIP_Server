package call

import (
	"encoding/binary"
	"math"

	"github.com/CyCoreSystems/goertzel"
	diagomedia "github.com/emiago/diago/media"
)

const (
	inbandSampleRate   = goertzel.RateTelephony
	inbandBlockSamples = 160 // 20 ms at 8 kHz, matches common G.711 RTP frames
	inbandMinActive    = 2   // 40 ms tone before accepting a digit
	inbandMinGap       = 2   // 40 ms silence before the next digit
)

// DTMF row/column frequencies (ITU-T Q.23).
var inbandDTMFFreqs = goertzel.DTMFFrequencies

// digit at [row][col] for low freq rows 697-941 and high freq cols 1209-1633.
var inbandDTMFDigit = [4][4]rune{
	{'1', '2', '3', 'A'},
	{'4', '5', '6', 'B'},
	{'7', '8', '9', 'C'},
	{'*', '0', '#', 'D'},
}

type goertzelBin struct {
	coeff float64
	q1    float64
	q2    float64
}

func newGoertzelBin(freq, sampleRate float64, blockSize int) goertzelBin {
	n := float64(blockSize)
	k := math.Floor(0.5 + (n*freq)/sampleRate)
	w := (2.0 * math.Pi / n) * k
	return goertzelBin{coeff: 2.0 * math.Cos(w)}
}

func (g *goertzelBin) reset() {
	g.q1, g.q2 = 0, 0
}

func (g *goertzelBin) feed(sample int16) {
	q := g.coeff*g.q1 - g.q2 + float64(sample)
	g.q2 = g.q1
	g.q1 = q
}

func (g *goertzelBin) magnitude2() float64 {
	return g.q1*g.q1 + g.q2*g.q2 - g.q1*g.q2*g.coeff
}

// inbandDetector decodes DTMF digits from 16-bit little-endian PCM at 8 kHz.
type inbandDetector struct {
	bins         [8]goertzelBin
	blockSize    int
	sampleInBlk  int
	pending      rune
	pendingBlks  int
	gapBlks      int
	emittedPress bool
}

func newInbandDetector() *inbandDetector {
	d := &inbandDetector{
		blockSize: inbandBlockSamples,
	}
	for i, freq := range inbandDTMFFreqs {
		d.bins[i] = newGoertzelBin(freq, inbandSampleRate, inbandBlockSamples)
	}
	return d
}

func (d *inbandDetector) FeedPCM(lpcm []byte) []rune {
	var out []rune
	for i := 0; i+1 < len(lpcm); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(lpcm[i : i+2]))
		for bi := range d.bins {
			d.bins[bi].feed(sample)
		}
		d.sampleInBlk++
		if d.sampleInBlk < d.blockSize {
			continue
		}
		if digit, ok := d.finishBlock(); ok {
			out = append(out, digit)
		}
	}
	return out
}

func (d *inbandDetector) finishBlock() (rune, bool) {
	defer func() {
		d.sampleInBlk = 0
		for i := range d.bins {
			d.bins[i].reset()
		}
	}()

	mags := [8]float64{}
	for i := range d.bins {
		mags[i] = d.bins[i].magnitude2()
	}

	digit, present := detectInbandDigit(mags)
	if present {
		d.gapBlks = 0
		if digit == d.pending {
			d.pendingBlks++
		} else {
			d.pending = digit
			d.pendingBlks = 1
			d.emittedPress = false
		}
		if d.pendingBlks >= inbandMinActive && !d.emittedPress {
			d.emittedPress = true
			return digit, true
		}
		return 0, false
	}

	d.gapBlks++
	if d.gapBlks >= inbandMinGap {
		d.pending = 0
		d.pendingBlks = 0
		d.emittedPress = false
	}
	return 0, false
}

func detectInbandDigit(mags [8]float64) (rune, bool) {
	lowIdx, lowMag := bestMagnitude(mags[0:4])
	highIdx, highMag := bestMagnitude(mags[4:8])
	if lowMag < goertzel.ToneThreshold || highMag < goertzel.ToneThreshold {
		return 0, false
	}

	// Reject weak secondary tones (simple twist/level guard).
	if secondBestMagnitude(mags[0:4]) > lowMag/4 {
		return 0, false
	}
	if secondBestMagnitude(mags[4:8]) > highMag/4 {
		return 0, false
	}

	if lowIdx < 4 && highIdx < 4 {
		return inbandDTMFDigit[lowIdx][highIdx], true
	}
	return 0, false
}

func bestMagnitude(mags []float64) (idx int, mag float64) {
	for i, m := range mags {
		if m > mag {
			mag = m
			idx = i
		}
	}
	return idx, mag
}

func secondBestMagnitude(mags []float64) float64 {
	var first, second float64
	for _, m := range mags {
		if m > first {
			second = first
			first = m
		} else if m > second {
			second = m
		}
	}
	return second
}

// rfc2833Decoder tracks RFC 4733 telephone-event packets.
type rfc2833Decoder struct {
	lastTimestamp uint32
	lastEv        diagomedia.DTMFEvent
	emitted       bool
}

func (r *rfc2833Decoder) processPayload(payload []byte, marker bool) (rune, bool) {
	ev := diagomedia.DTMFEvent{}
	if err := diagomedia.DTMFDecode(payload, &ev); err != nil {
		return 0, false
	}

	if ev.EndOfEvent {
		if r.emitted {
			return 0, false
		}
		if r.lastEv.Duration == 0 {
			return 0, false
		}
		if r.lastEv.Event != ev.Event {
			return 0, false
		}
		digit := diagomedia.DTMFToRune(ev.Event)
		r.lastEv = diagomedia.DTMFEvent{}
		r.emitted = true
		if digit != 0 {
			return digit, true
		}
		return 0, false
	}

	if marker || r.lastEv.Event != ev.Event {
		r.lastEv = ev
		r.emitted = false
		return 0, false
	}

	// Some endpoints never set the end bit; accept after ~20 ms of tone.
	if !r.emitted && ev.Duration >= 320 && r.lastEv.Event == ev.Event {
		digit := diagomedia.DTMFToRune(ev.Event)
		r.emitted = true
		if digit != 0 {
			return digit, true
		}
	}
	r.lastEv = ev
	return 0, false
}

func inbandSupportedCodec(c diagomedia.Codec) bool {
	switch c.PayloadType {
	case 0, 8: // PCMU, PCMA
		return c.SampleRate == 8000
	default:
		return false
	}
}
