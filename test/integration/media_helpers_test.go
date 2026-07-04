//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/diago/media"
)

const rtpFrameSize = 160
const minBridgeRelayBytes int64 = 320
const minMOHBytes int64 = 320

// readRTPBytes accumulates at least minBytes from the leg's audio reader or times out.
// Use only when the PCM bridge is stopped (e.g. MOH during hold) — never while the
// bridge is also reading the same leg.
func readRTPBytes(ctx context.Context, sess diago.DialogSession, minBytes int) error {
	r, err := sess.Media().AudioReader()
	if err != nil {
		return err
	}
	buf := make([]byte, media.RTPBufSize)
	total := 0
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := r.Read(buf)
		if n > 0 {
			total += n
			if total >= minBytes {
				return nil
			}
		}
		if err != nil && err != io.EOF {
			return err
		}
	}
	if total >= minBytes {
		return nil
	}
	return fmt.Errorf("read %d bytes, need >= %d", total, minBytes)
}

// writeToneFrames sends non-silent PCM frames toward the PBX/server peer.
func writeToneFrames(ctx context.Context, sess diago.DialogSession) context.CancelFunc {
	w, err := sess.Media().AudioWriter()
	if err != nil {
		return func() {}
	}
	toneCtx, cancel := context.WithCancel(ctx)
	go func() {
		frame := make([]byte, rtpFrameSize)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-toneCtx.Done():
				return
			case <-ticker.C:
				for j := range frame {
					frame[j] = byte((i + j) % 251)
				}
				i++
				if _, err := w.Write(frame); err != nil {
					return
				}
			}
		}
	}()
	return cancel
}

func waitHoldMOHBytes(ctx context.Context, pbx *testPBX, callerExt, calleeExt string, min int64) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	for time.Now().Before(deadline) {
		n, ok := pbx.Srv.HoldMOHBytesSent(callerExt, calleeExt)
		if ok && n >= min {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	n, _ := pbx.Srv.HoldMOHBytesSent(callerExt, calleeExt)
	return fmt.Errorf("hold MOH bytes sent %d, need >= %d", n, min)
}

func bridgedRelayDelta(pbx *testPBX, callerExt, calleeExt string) (callerToCallee, calleeToCaller int64, ok bool) {
	toCallee, toCaller, ok := pbx.Srv.BridgedRelayBytes(callerExt, calleeExt)
	return toCallee, toCaller, ok
}

func waitBridgeRelay(ctx context.Context, pbx *testPBX, callerExt, calleeExt string, baselineCallerToCallee, baselineCalleeToCaller int64, wantCallerToCallee bool, minDelta int64) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(8 * time.Second)
	}
	for time.Now().Before(deadline) {
		toCallee, toCaller, ok := bridgedRelayDelta(pbx, callerExt, calleeExt)
		if !ok {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if wantCallerToCallee && toCallee-baselineCallerToCallee >= minDelta {
			return nil
		}
		if !wantCallerToCallee && toCaller-baselineCalleeToCaller >= minDelta {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	toCallee, toCaller, _ := bridgedRelayDelta(pbx, callerExt, calleeExt)
	if wantCallerToCallee {
		return fmt.Errorf("caller→callee relay delta %d, need >= %d (total %d)", toCallee-baselineCallerToCallee, minDelta, toCallee)
	}
	return fmt.Errorf("callee→caller relay delta %d, need >= %d (total %d)", toCaller-baselineCalleeToCaller, minDelta, toCaller)
}

// assertTwoWayBridgeRelay verifies the production PCM bridge relayed RTP both ways
// after injecting tone on each handset (observed via server bridge counters).
func assertTwoWayBridgeRelay(t *testing.T, ctx context.Context, pbx *testPBX, callerLeg, calleeLeg diago.DialogSession) {
	t.Helper()

	baseToCallee, baseToCaller, ok := bridgedRelayDelta(pbx, "111", "110")
	if !ok {
		t.Fatal("bridged call not found for relay stats")
	}

	stopCaller := writeToneFrames(ctx, callerLeg)
	defer stopCaller()
	if err := waitBridgeRelay(ctx, pbx, "111", "110", baseToCallee, baseToCaller, true, minBridgeRelayBytes); err != nil {
		t.Fatalf("bridge did not relay caller→callee audio: %v", err)
	}

	midToCallee, midToCaller, ok := bridgedRelayDelta(pbx, "111", "110")
	if !ok {
		t.Fatal("bridged call lost during relay check")
	}

	stopCallee := writeToneFrames(ctx, calleeLeg)
	defer stopCallee()
	if err := waitBridgeRelay(ctx, pbx, "111", "110", midToCallee, midToCaller, false, minBridgeRelayBytes); err != nil {
		t.Fatalf("bridge did not relay callee→caller audio: %v", err)
	}
}

// assertTwoWayRTP verifies tone injected on each leg is received on the peer handset.
// Used for conference mixer paths where the server mixer is the sole reader.
func assertTwoWayRTP(t *testing.T, ctx context.Context, a, b diago.DialogSession, minBytes int) {
	t.Helper()

	toneA := writeToneFrames(ctx, a)
	defer toneA()
	readCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := readRTPBytes(readCtx, b, minBytes); err != nil {
		t.Fatalf("peer B did not receive RTP from A: %v", err)
	}

	toneB := writeToneFrames(ctx, b)
	defer toneB()
	readCtx2, cancel2 := context.WithTimeout(ctx, 8*time.Second)
	defer cancel2()
	if err := readRTPBytes(readCtx2, a, minBytes); err != nil {
		t.Fatalf("peer A did not receive RTP from B: %v", err)
	}
}
