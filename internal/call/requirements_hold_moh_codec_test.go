package call

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/emiago/diago"
	diagomedia "github.com/emiago/diago/media"
	"github.com/emiago/diago/media/sdp"
	"github.com/sappsys/VoIP_Server/internal/media"
	"github.com/sappsys/VoIP_Server/internal/media/tones"
)

// REQ-HOLD-2: MOH must be audible on non-PCMU/PCMA legs (G722 uses generated tone).
func TestREQ_HOLD_MOHSkipsWAVForG722(t *testing.T) {
	g722 := media.VoiceCodecTable()[media.CodecG722]
	if wavPlaybackCodecSupported(g722) {
		t.Fatal("REQ-HOLD-2: G722 must not use raw WAV path (diago cannot encode)")
	}
}

func TestREQ_HOLD_MOHStartsToneFallbackForG722(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "moh.wav"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	var started sync.Map
	holdPlaybackHook = func(kind string) { started.Store(kind, true) }
	t.Cleanup(func() { holdPlaybackHook = nil })

	g722 := media.VoiceCodecTable()[media.CodecG722]
	msOut := g722MediaSession(t, sdp.ModeSendrecv)
	out := &diago.DialogClientSession{}
	rtpW := diagomedia.NewRTPPacketWriter(nil, g722)
	out.InitMediaSession(msOut, nil, rtpW)

	create := func() (diago.AudioPlaybackControl, error) {
		return diago.NewAudioPlaybackControl(
			diago.NewAudioPlayback(io.Discard, g722),
		), nil
	}

	player := &holdPlayer{}
	ok := player.startMOH(context.Background(), context.Background(), dir, tones.DefaultProfile(), nil, out, create)
	if !ok {
		t.Fatal("REQ-HOLD-2: MOH must start via tone fallback on G722")
	}
	if _, ok := started.Load("moh"); !ok {
		t.Fatal("REQ-HOLD-2: MOH playback hook must fire")
	}
	player.stopAndWait()
}

func g722MediaSession(t *testing.T, mode string) *diagomedia.MediaSession {
	t.Helper()
	ms, err := diagomedia.NewMediaSession(net.ParseIP("127.0.0.1"), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ms.Close() })
	sdpBody := "v=0\r\n" +
		"o=- 1 2 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"c=IN IP4 127.0.0.1\r\n" +
		"t=0 0\r\n" +
		"m=audio 5004 RTP/AVP 9\r\n" +
		"a=rtpmap:9 G722/8000\r\n" +
		"a=" + mode + "\r\n"
	if err := ms.InitWithSDP([]byte(sdpBody)); err != nil {
		t.Fatal(err)
	}
	return ms
}
