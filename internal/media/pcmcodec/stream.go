package pcmcodec

import (
	"io"

	diagomedia "github.com/emiago/diago/media"
)

// PCMReader decodes RTP payloads from src into 16-bit little-endian PCM at 8 kHz.
type PCMReader struct {
	Handler *Handler
	Source  io.Reader
	buf     []byte
	scratch []byte
}

func NewPCMReader(codec diagomedia.Codec, src io.Reader) (*PCMReader, error) {
	h, err := New(codec)
	if err != nil {
		return nil, err
	}
	return &PCMReader{
		Handler: h,
		Source:  src,
		buf:     make([]byte, diagomedia.RTPBufSize),
	}, nil
}

func (r *PCMReader) Read(p []byte) (int, error) {
	for len(r.scratch) == 0 {
		n, err := r.Source.Read(r.buf)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		samples, err := r.Handler.Decode(r.buf[:n])
		if err != nil {
			return 0, err
		}
		if len(samples) == 0 {
			continue
		}
		r.scratch = samplesToBytesLE(samples)
	}
	n := copy(p, r.scratch)
	r.scratch = r.scratch[n:]
	return n, nil
}

// PCMWriter encodes 16-bit little-endian PCM at 8 kHz into RTP payloads on dst.
type PCMWriter struct {
	Handler *Handler
	Writer  io.Writer
	pending []int16
}

func NewPCMWriter(codec diagomedia.Codec, dst io.Writer) (*PCMWriter, error) {
	h, err := New(codec)
	if err != nil {
		return nil, err
	}
	return &PCMWriter{
		Handler: h,
		Writer:  dst,
	}, nil
}

func (w *PCMWriter) Write(p []byte) (int, error) {
	if len(p)%2 != 0 {
		return 0, io.ErrShortWrite
	}
	samples := bytesLEToSamples(p)
	w.pending = append(w.pending, samples...)
	frame := w.Handler.HubFrameSamples()
	for len(w.pending) >= frame {
		chunk := w.pending[:frame]
		enc, err := w.Handler.Encode(chunk)
		if err != nil {
			return len(p), err
		}
		if len(enc) > 0 {
			if _, err := w.Writer.Write(enc); err != nil {
				return len(p), err
			}
		}
		w.pending = w.pending[frame:]
	}
	return len(p), nil
}
