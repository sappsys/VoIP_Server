package call

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emiago/diago"
	diagomedia "github.com/emiago/diago/media"
)

// REQ-MOH-1: MOH must use PlaybackControlCreate (stoppable), not PlaybackCreate.
// REQ-MOH-2: Stop() must end playback goroutines.

func TestREQ_MOH_UsesPlaybackControlCreate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "moh.wav"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var createCalls atomic.Int32
	create := func() (diago.AudioPlaybackControl, error) {
		createCalls.Add(1)
		return diago.NewAudioPlaybackControl(
			diago.NewAudioPlayback(io.Discard, diagomedia.CodecAudioUlaw),
		), nil
	}

	sess := newMOHSession(context.Background(), context.Background(), dir, nil, create)
	if sess == nil {
		t.Fatal("expected session")
	}
	deadline := time.Now().Add(2 * time.Second)
	for createCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if createCalls.Load() != 1 {
		t.Fatalf("REQ-MOH-1: PlaybackControlCreate must be invoked, got %d calls", createCalls.Load())
	}
	sess.Stop()
}

func TestREQ_MOH_StopEndsGoroutine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "moh.wav"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	create := func() (diago.AudioPlaybackControl, error) {
		return diago.NewAudioPlaybackControl(
			diago.NewAudioPlayback(io.Discard, diagomedia.CodecAudioUlaw),
		), nil
	}

	parent, cancel := context.WithCancel(context.Background())
	sess := newMOHSession(parent, context.Background(), dir, nil, create)
	if sess == nil {
		t.Fatal("expected session")
	}

	done := make(chan struct{})
	go func() {
		sess.player.wg.Wait()
		close(done)
	}()

	sess.Stop()
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("REQ-MOH-2: MOH goroutine still running after Stop()")
	}
}

func TestMOHSessionStopIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "moh.wav"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	create := func() (diago.AudioPlaybackControl, error) {
		return diago.NewAudioPlaybackControl(
			diago.NewAudioPlayback(io.Discard, diagomedia.CodecAudioUlaw),
		), nil
	}
	sess := newMOHSession(context.Background(), context.Background(), dir, nil, create)
	sess.Stop()
	sess.Stop()
}

func TestREQ_HOLD_PlayerStopAndWait(t *testing.T) {
	player := &holdPlayer{}
	ctrl := diago.NewAudioPlaybackControl(
		diago.NewAudioPlayback(io.Discard, diagomedia.CodecAudioUlaw),
	)
	player.track(&ctrl)
	player.stopAndWait()
}
