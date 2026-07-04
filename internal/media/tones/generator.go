package tones

import (
	"encoding/binary"
	"io"
	"math"
)

const sampleRate = 8000.0

const defaultAmplitude = 0.35

// Generator streams 16-bit mono PCM at 8 kHz for a tone definition.
type Generator struct {
	tone   Tone
	sample uint64
	segIdx int
	segPos int
	segOn  bool
}

func NewGenerator(t Tone) *Generator {
	g := &Generator{tone: t}
	if len(t.Cadence) == 0 {
		g.segOn = true
	} else {
		g.segOn = t.Cadence[0].On
	}
	return g
}

func (g *Generator) Read(p []byte) (int, error) {
	for i := 0; i+1 < len(p); i += 2 {
		if !g.segOn {
			binary.LittleEndian.PutUint16(p[i:], 0)
		} else {
			t := float64(g.sample) / sampleRate
			var v float64
			n := len(g.tone.Frequencies)
			if n == 0 {
				v = 0
			} else {
				amp := defaultAmplitude / float64(n)
				for _, f := range g.tone.Frequencies {
					v += amp * math.Sin(2*math.Pi*f*t)
				}
			}
			if v > 1 {
				v = 1
			}
			if v < -1 {
				v = -1
			}
			binary.LittleEndian.PutUint16(p[i:], uint16(int16(v*32767)))
		}
		g.sample++
		g.advanceCadence()
	}
	return len(p), nil
}

func (g *Generator) advanceCadence() {
	if len(g.tone.Cadence) == 0 {
		return
	}
	g.segPos++
	segSamples := int(g.tone.Cadence[g.segIdx].Duration.Seconds() * sampleRate)
	if g.segPos < segSamples {
		return
	}
	g.segIdx = (g.segIdx + 1) % len(g.tone.Cadence)
	g.segPos = 0
	g.segOn = g.tone.Cadence[g.segIdx].On
}

// PlayDuration returns how long to play a cadenced tone for at least `cycles` repeats.
func PlayDuration(t Tone, cycles int) int {
	if cycles < 1 {
		cycles = 1
	}
	if len(t.Cadence) == 0 {
		return cycles
	}
	var cycle int
	for _, s := range t.Cadence {
		cycle += int(s.Duration.Milliseconds())
	}
	if cycle <= 0 {
		return cycles
	}
	return cycle * cycles
}

// Drain discards generated PCM (for tests).
func Drain(g io.Reader, nbytes int) error {
	buf := make([]byte, 4096)
	remaining := nbytes
	for remaining > 0 {
		n, err := g.Read(buf)
		if n > 0 {
			remaining -= n
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}
