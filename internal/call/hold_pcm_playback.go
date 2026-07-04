package call

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/emiago/diago"
	diagomedia "github.com/emiago/diago/media"
	"github.com/sappsys/VoIP_Server/internal/media/audiobridge"
	"github.com/sappsys/VoIP_Server/internal/media/pcmcodec"
	"github.com/sappsys/VoIP_Server/internal/media/tones"
)

const holdRTPRetryDelay = 50 * time.Millisecond

// holdMediaViaPCMCodec reports whether hold MOH/dial tone should use pcmcodec +
// WriteSamples instead of diago AudioPlayback (survives RTP session replacement).
func holdMediaViaPCMCodec(sess diago.DialogSession) bool {
	if sess == nil {
		return false
	}
	m := sess.Media()
	if m == nil || m.MediaSession() == nil {
		return false
	}
	codec, ok := audiobridge.NegotiatedCodec(m.MediaSession())
	if !ok {
		return false
	}
	return pcmcodec.Supported(codec)
}

// toneViaPCMCodec is an alias kept for tests; prefer holdMediaViaPCMCodec.
func toneViaPCMCodec(sess diago.DialogSession) bool {
	return holdMediaViaPCMCodec(sess)
}

func rtpPacketWriterForSession(sess diago.DialogSession) *diagomedia.RTPPacketWriter {
	if sess == nil {
		return nil
	}
	switch s := sess.(type) {
	case *diago.DialogServerSession:
		return s.RTPPacketWriter
	case *diago.DialogClientSession:
		return s.RTPPacketWriter
	default:
		return nil
	}
}

func holdCodecHandler(sess diago.DialogSession) (diagomedia.Codec, *pcmcodec.Handler, *diagomedia.RTPPacketWriter, bool) {
	m := sess.Media()
	if m == nil || m.MediaSession() == nil {
		return diagomedia.Codec{}, nil, nil, false
	}
	codec, ok := audiobridge.NegotiatedCodec(m.MediaSession())
	if !ok {
		return diagomedia.Codec{}, nil, nil, false
	}
	handler, err := pcmcodec.New(codec)
	if err != nil {
		return codec, nil, nil, false
	}
	pw := rtpPacketWriterForSession(sess)
	if pw == nil {
		return codec, nil, nil, false
	}
	return codec, handler, pw, true
}

func runPCMCodecToneLoop(
	ctx context.Context,
	callDone context.Context,
	sess diago.DialogSession,
	tone tones.Tone,
	log *slog.Logger,
	bytesSent *atomic.Int64,
) {
	defer recoverHoldPlayback("pcm_tone", log)

	gen := tones.NewGenerator(tone)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		if ctxDone(callDone, ctx) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if !pcmToneCanSend(sess) {
			continue
		}
		codec, handler, pw, ok := holdCodecHandler(sess)
		if !ok {
			time.Sleep(holdRTPRetryDelay)
			continue
		}
		frameSamples := handler.HubFrameSamples()
		pcmBuf := make([]byte, frameSamples*2)
		if _, err := io.ReadFull(gen, pcmBuf); err != nil {
			return
		}
		samples := make([]int16, frameSamples)
		for i := 0; i < frameSamples; i++ {
			samples[i] = int16(binary.LittleEndian.Uint16(pcmBuf[i*2 : i*2+2]))
		}
		payload, err := handler.Encode(samples)
		if err != nil {
			time.Sleep(holdRTPRetryDelay)
			continue
		}
		if _, err := pw.WriteSamples(payload, codec.SampleTimestamp(), false, codec.PayloadType); err != nil {
			if log != nil {
				log.Debug("hold tone rtp retry", "codec", codec.Name, "error", err)
			}
			time.Sleep(holdRTPRetryDelay)
			continue
		}
		if bytesSent != nil {
			bytesSent.Add(int64(len(payload)))
		}
	}
}

// runPCMCodecMOHLoop streams MOH WAV tracks (or generated tone) via pcmcodec RTP writes.
// Re-reads the RTP writer each frame so MOH survives held-party SDP re-INVITEs.
func runPCMCodecMOHLoop(
	ctx context.Context,
	callDone context.Context,
	sess diago.DialogSession,
	tracks []string,
	fallback tones.Tone,
	log *slog.Logger,
	bytesSent *atomic.Int64,
) {
	defer recoverHoldPlayback("pcm_moh", log)

	tone := fallback
	if len(tone.Frequencies) == 0 {
		tone = fallbackMOHTone(tones.DefaultProfile())
	}

	for {
		if ctxDone(callDone, ctx) {
			return
		}
		if !pcmToneCanSend(sess) {
			time.Sleep(holdRTPRetryDelay)
			continue
		}

		played := false
		for _, path := range tracks {
			if ctxDone(callDone, ctx) {
				return
			}
			if streamMOHWAVPCM(ctx, callDone, sess, path, log, bytesSent) {
				played = true
				break
			}
			if log != nil {
				log.Debug("moh wav pcm stream failed, trying next track", "path", path)
			}
		}
		if played {
			continue
		}
		runPCMCodecToneLoop(ctx, callDone, sess, tone, log, bytesSent)
		return
	}
}

func streamMOHWAVPCM(
	ctx context.Context,
	callDone context.Context,
	sess diago.DialogSession,
	path string,
	log *slog.Logger,
	bytesSent *atomic.Int64,
) bool {
	codec, handler, _, ok := holdCodecHandler(sess)
	if !ok {
		return false
	}
	pcmBytes, err := loadWavPCMForCodec(path, codec)
	if err != nil {
		if log != nil {
			log.Debug("moh wav load failed", "path", path, "error", err)
		}
		return false
	}
	samples, err := pcmBytesToInt16(pcmBytes)
	if err != nil {
		return false
	}

	frameSamples := handler.HubFrameSamples()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	offset := 0
	for {
		if ctxDone(callDone, ctx) {
			return true
		}
		if !pcmToneCanSend(sess) {
			time.Sleep(holdRTPRetryDelay)
			continue
		}
		select {
		case <-ctx.Done():
			return true
		case <-ticker.C:
		}

		codec, handler, pw, ok := holdCodecHandler(sess)
		if !ok {
			time.Sleep(holdRTPRetryDelay)
			continue
		}
		if offset+frameSamples > len(samples) {
			return true
		}
		frame := samples[offset : offset+frameSamples]
		offset += frameSamples
		payload, err := handler.Encode(frame)
		if err != nil {
			time.Sleep(holdRTPRetryDelay)
			continue
		}
		if _, err := pw.WriteSamples(payload, codec.SampleTimestamp(), false, codec.PayloadType); err != nil {
			if log != nil {
				log.Debug("moh wav rtp retry", "path", path, "error", err)
			}
			time.Sleep(holdRTPRetryDelay)
			continue
		}
		if bytesSent != nil {
			bytesSent.Add(int64(len(payload)))
		}
	}
}

func ctxDone(callDone, ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
	}
	if callDone != nil {
		select {
		case <-callDone.Done():
			return true
		default:
		}
	}
	return false
}

func pcmToneCanSend(sess diago.DialogSession) bool {
	if sess == nil {
		return false
	}
	m := sess.Media()
	if m == nil || m.MediaSession() == nil {
		return false
	}
	return legCanSendMedia(m.MediaSession())
}
