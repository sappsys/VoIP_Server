package call

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media/sdp"
)

// REQ-HOLD-7: on hold, holder hears dial tone and remote party hears MOH.
// REQ-HOLD-8: on unhold, prepareLegsForBridge runs before bridge restart (REQ-HOLD-6b).

func TestREQ_HOLD_EnterStartsDialToneAndMOH(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "moh.wav"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var started sync.Map
	holdPlaybackHook = func(kind string) {
		started.Store(kind, true)
	}
	t.Cleanup(func() { holdPlaybackHook = nil })

	testEnableHolderSendMedia = func(*holdController, context.Context, bool) bool { return true }
	t.Cleanup(func() { testEnableHolderSendMedia = nil })
	testEnableHeldPartyMOHMedia = func(*holdController, context.Context, bool) bool { return true }
	t.Cleanup(func() { testEnableHeldPartyMOHMedia = nil })

	msIn := mediaSessionWithLocalDirection(t, sdp.ModeRecvonly)
	msOut := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	in := testServerDialog("holder")
	out := &diago.DialogClientSession{}
	in.InitMediaSession(msIn, nil, nil)
	out.InitMediaSession(msOut, nil, nil)

	ac := &ActiveCall{CallerExt: "111", CalleeExt: "110"}
	h := &holdController{
		b:      &BridgePair{},
		ctx:    t.Context(),
		ac:     ac,
		mohDir: dir,
		in:     in,
		out:    out,
	}

	// Patch playback on legs by wrapping — enter uses leg methods; call enter and verify hooks.
	h.enter(true)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, ok := started.Load("dial_tone"); ok {
			if _, ok := started.Load("moh"); ok {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, ok := started.Load("dial_tone"); !ok {
		t.Fatal("REQ-HOLD-7: holder must hear dial tone on hold")
	}
	if _, ok := started.Load("moh"); !ok {
		t.Fatal("REQ-HOLD-7: remote party must hear MOH on hold")
	}
	if !ac.HoldActive {
		t.Fatal("enter must mark call on hold")
	}
}
