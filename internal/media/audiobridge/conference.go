package audiobridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/emiago/diago/audio"
	diagomedia "github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/media/pcmcodec"
)

const conferenceFrameSamples = 160 // 20 ms at 8 kHz

// FanoutTranscoded reads PCM from src and writes transcoded audio to each destination leg.
func FanoutTranscoded(ctx context.Context, log *slog.Logger, src SessionLeg, dsts []SessionLeg) error {
	if src == nil || len(dsts) == 0 {
		return nil
	}
	srcCodec, ok := NegotiatedCodec(src.MediaSession())
	if !ok || !CanTranscode(srcCodec) {
		return fmt.Errorf("fanout: source codec not transcodable")
	}
	rawR, err := src.AudioReader()
	if err != nil {
		return err
	}
	pcmR, err := pcmcodec.NewPCMReader(srcCodec, rawR)
	if err != nil {
		return err
	}

	type encodedLeg struct {
		w *pcmcodec.PCMWriter
	}
	var out []encodedLeg
	for _, d := range dsts {
		if d == nil {
			continue
		}
		codec, ok := NegotiatedCodec(d.MediaSession())
		if !ok || !CanTranscode(codec) {
			return fmt.Errorf("fanout: destination codec %v not transcodable", codec.Name)
		}
		relayW, err := audioRelayWriter(d, codec)
		if err != nil {
			return err
		}
		pcmW, err := pcmcodec.NewPCMWriter(codec, relayW)
		if err != nil {
			return err
		}
		out = append(out, encodedLeg{w: pcmW})
	}
	if len(out) == 0 {
		return nil
	}

	buf := make([]byte, diagomedia.RTPBufSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := pcmR.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			for _, leg := range out {
				if _, werr := leg.w.Write(chunk); werr != nil && log != nil {
					log.Debug("fanout write ended", "error", werr)
				}
			}
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// ConferenceMixer mixes multiple legs in PCM with per-leg transcoding.
type ConferenceMixer struct {
	log  *slog.Logger
	mu   sync.Mutex
	legs map[string]*confLeg
	run  context.CancelFunc
	wg   sync.WaitGroup
}

type confLeg struct {
	id       string
	leg      SessionLeg
	pcmW     *pcmcodec.PCMWriter
	cancel   context.CancelFunc
	latest   []int16
	latestMu sync.Mutex
}

func NewConferenceMixer(log *slog.Logger) *ConferenceMixer {
	return &ConferenceMixer{log: log, legs: make(map[string]*confLeg)}
}

func (m *ConferenceMixer) RemoveAll() {
	m.mu.Lock()
	legs := m.legs
	m.legs = make(map[string]*confLeg)
	cancel := m.run
	m.run = nil
	m.mu.Unlock()

	for _, l := range legs {
		if l.cancel != nil {
			l.cancel()
		}
		stopLegMedia(l.leg)
	}
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()
	for _, l := range legs {
		resetLegMedia(l.leg)
	}
}

func resetLegMedia(leg SessionLeg) {
	clearSessionLegMedia(leg)
	if pw := rtpPacketWriterForLeg(leg); pw != nil {
		pw.ResetTimestamp()
	}
}

func (m *ConferenceMixer) Add(leg SessionLeg) error {
	if leg == nil {
		return fmt.Errorf("conference: nil leg")
	}
	id := legID(leg)
	codec, ok := NegotiatedCodec(leg.MediaSession())
	if !ok || !CanTranscode(codec) {
		return fmt.Errorf("conference: codec not transcodable")
	}
	relayW, err := audioRelayWriter(leg, codec)
	if err != nil {
		return err
	}
	pcmW, err := pcmcodec.NewPCMWriter(codec, relayW)
	if err != nil {
		return err
	}
	rawR, err := leg.AudioReader()
	if err != nil {
		return err
	}
	pcmR, err := pcmcodec.NewPCMReader(codec, rawR)
	if err != nil {
		return err
	}

	readCtx, readCancel := context.WithCancel(context.Background())
	cl := &confLeg{
		id:     id,
		leg:    leg,
		pcmW:   pcmW,
		cancel: readCancel,
		latest: make([]int16, conferenceFrameSamples),
	}

	m.mu.Lock()
	if _, exists := m.legs[id]; exists {
		m.mu.Unlock()
		readCancel()
		return nil
	}
	m.legs[id] = cl
	if len(m.legs) >= 2 && m.run == nil {
		mixCtx, mixCancel := context.WithCancel(context.Background())
		m.run = mixCancel
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.mixLoop(mixCtx)
		}()
	}
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.readLoop(readCtx, cl, pcmR)
	}()
	return nil
}

func (m *ConferenceMixer) readLoop(ctx context.Context, cl *confLeg, pcmR *pcmcodec.PCMReader) {
	defer cl.cancel()
	buf := make([]byte, conferenceFrameSamples*4)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := readWithContext(ctx, pcmR, buf)
		if n > 0 {
			samples := pcmcodecBytesToSamples(buf[:n])
			if len(samples) == 0 {
				continue
			}
			cl.latestMu.Lock()
			if len(samples) >= conferenceFrameSamples {
				copy(cl.latest, samples[len(samples)-conferenceFrameSamples:])
			} else {
				off := conferenceFrameSamples - len(samples)
				copy(cl.latest, cl.latest[len(samples):])
				copy(cl.latest[off:], samples)
			}
			cl.latestMu.Unlock()
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			return
		}
	}
}

func (m *ConferenceMixer) mixLoop(ctx context.Context) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	mixBuf := make([]byte, conferenceFrameSamples*2)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			legs := make([]*confLeg, 0, len(m.legs))
			for _, l := range m.legs {
				legs = append(legs, l)
			}
			m.mu.Unlock()
			if len(legs) < 2 {
				return
			}
			for _, target := range legs {
				for i := range mixBuf {
					mixBuf[i] = 0
				}
				for _, src := range legs {
					if src.id == target.id {
						continue
					}
					src.latestMu.Lock()
					frame := append([]int16(nil), src.latest...)
					src.latestMu.Unlock()
					if len(frame) != conferenceFrameSamples {
						continue
					}
					frameBytes := make([]byte, conferenceFrameSamples*2)
					pcmcodecSamplesToBytes(frame, frameBytes)
					audio.PCMMix(mixBuf, mixBuf, frameBytes)
				}
				if _, err := target.pcmW.Write(mixBuf); err != nil && m.log != nil {
					m.log.Debug("conference mix write ended", "leg", target.id, "error", err)
				}
			}
		}
	}
}

func legID(leg SessionLeg) string {
	switch s := leg.(type) {
	case interface{ Id() string }:
		return s.Id()
	default:
		return fmt.Sprintf("%p", leg)
	}
}
