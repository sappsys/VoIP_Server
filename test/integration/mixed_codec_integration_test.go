//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media"
)

// REQ-BRIDGE-5 / REQ-AUDIO-LATENCY: a call between two handsets that negotiate
// different codecs (PCMA vs PCMU) must bridge through the PCM transcoding hub
// with two-way audio and stay up (low-latency relay, no clock stall/panic).
func TestREQ_BRIDGE_MixedCodecCall(t *testing.T) {
	// PBX offers a broad codec set so each leg can settle on its single codec.
	pbx := startPBX(t, pbxOptions{
		Extensions: map[string]string{"110": "andy", "111": "andy"},
		Codecs:     []string{"PCMU", "PCMA", "G722"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// Caller only speaks PCMA; callee only speaks PCMU -> forces transcoding.
	caller := newHandset(t, pbx.Port, "111", "andy", media.CodecAudioAlaw)
	callee := newHandset(t, pbx.Port, "110", "andy", media.CodecAudioUlaw)
	caller.register()
	callee.register()

	answered := make(chan *diago.DialogServerSession, 1)
	callee.serveAnswer(ctx, answered, true)

	out, err := caller.invite(ctx, "110", nil)
	if err != nil {
		t.Fatalf("mixed-codec invite 111(PCMA)->110(PCMU): %v", err)
	}
	defer out.Close()

	var calleeLeg *diago.DialogServerSession
	select {
	case calleeLeg = <-answered:
	case <-time.After(6 * time.Second):
		t.Fatal("callee did not answer mixed-codec call")
	}

	audioCtx, audioCancel := context.WithCancel(ctx)
	defer audioCancel()
	stopOut := pumpAudio(audioCtx, out)
	stopCallee := pumpAudio(audioCtx, calleeLeg)
	defer func() { stopOut(); stopCallee() }()

	// The transcoding bridge must stay healthy while audio flows in both directions.
	time.Sleep(1 * time.Second)
	if out.Context().Err() != nil {
		t.Fatal("REQ-BRIDGE-5: caller leg dropped during mixed-codec call (transcode bridge stall)")
	}
	if calleeLeg.Context().Err() != nil {
		t.Fatal("REQ-BRIDGE-5: callee leg dropped during mixed-codec call (transcode bridge stall)")
	}
}

// REQ-BRIDGE-6 / REQ-AUDIO-LATENCY: when both legs share the same codec the call
// bridges via passthrough and stays up.
func TestREQ_BRIDGE_SameCodecPassthrough(t *testing.T) {
	pbx := startPBX(t, pbxOptions{
		Extensions: map[string]string{"110": "andy", "111": "andy"},
		Codecs:     []string{"PCMU", "PCMA", "G722"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	caller := newHandset(t, pbx.Port, "111", "andy", media.CodecAudioUlaw)
	callee := newHandset(t, pbx.Port, "110", "andy", media.CodecAudioUlaw)
	caller.register()
	callee.register()

	answered := make(chan *diago.DialogServerSession, 1)
	callee.serveAnswer(ctx, answered, true)

	out, err := caller.invite(ctx, "110", nil)
	if err != nil {
		t.Fatalf("same-codec invite: %v", err)
	}
	defer out.Close()

	var calleeLeg *diago.DialogServerSession
	select {
	case calleeLeg = <-answered:
	case <-time.After(6 * time.Second):
		t.Fatal("callee did not answer same-codec call")
	}

	audioCtx, audioCancel := context.WithCancel(ctx)
	defer audioCancel()
	stopOut := pumpAudio(audioCtx, out)
	stopCallee := pumpAudio(audioCtx, calleeLeg)
	defer func() { stopOut(); stopCallee() }()

	time.Sleep(1 * time.Second)
	if out.Context().Err() != nil || calleeLeg.Context().Err() != nil {
		t.Fatal("REQ-BRIDGE-6: a leg dropped during same-codec passthrough call")
	}
}
