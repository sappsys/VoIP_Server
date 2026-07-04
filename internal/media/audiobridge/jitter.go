package audiobridge

import (
	"sync"
)

// minPCMSamples is 20 ms of 16-bit mono audio at 8 kHz (one frame).
const minPCMSamples = 160

// pcmJitterBuffer holds decoded PCM until one frame is available.
type pcmJitterBuffer struct {
	mu     sync.Mutex
	buf    []int16
	ready  bool
	closed bool
}

func newPCMJitterBuffer() *pcmJitterBuffer {
	return &pcmJitterBuffer{buf: make([]int16, 0, minPCMSamples*4)}
}

func (j *pcmJitterBuffer) push(samples []int16) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return
	}
	j.buf = append(j.buf, samples...)
	if !j.ready && len(j.buf) >= minPCMSamples {
		j.ready = true
	}
}

func (j *pcmJitterBuffer) popFrame(frameSamples int) []int16 {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed || !j.ready || len(j.buf) < frameSamples {
		return nil
	}
	out := make([]int16, frameSamples)
	copy(out, j.buf[:frameSamples])
	j.buf = j.buf[frameSamples:]
	return out
}

func (j *pcmJitterBuffer) close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.closed = true
}

func (j *pcmJitterBuffer) drain() []int16 {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.buf) == 0 {
		return nil
	}
	out := append([]int16(nil), j.buf...)
	j.buf = j.buf[:0]
	return out
}
