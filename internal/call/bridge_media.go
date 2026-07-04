package call

import (
	"context"
	"log/slog"
	"time"

	"github.com/emiago/diago"
)

const holdInboundRTPWait = 2 * time.Second

// waitForInboundRTP waits for a fresh inbound RTP packet after hold media setup.
// Do not trust ReadRTPFromAddr — it may be stale from the pre-hold bridge.
func waitForInboundRTP(ctx context.Context, sess diago.DialogSession, timeout time.Duration) bool {
	if sess == nil {
		return false
	}
	m := sess.Media()
	if m == nil {
		return false
	}
	ms := m.MediaSession()
	if ms == nil {
		return false
	}
	if timeout <= 0 {
		timeout = holdInboundRTPWait
	}
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 1600)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		readUntil := time.Now().Add(100 * time.Millisecond)
		if readUntil.After(deadline) {
			readUntil = deadline
		}
		if n, err := ms.ReadRTPRawDeadline(buf, readUntil); err == nil && n > 0 {
			return true
		}
	}
	return false
}

func clearSessionMediaHooks(d diago.DialogSession) {
	if d == nil {
		return
	}
	m := d.Media()
	if m == nil {
		return
	}
	m.SetAudioReader(nil)
	m.SetAudioWriter(nil)
}

func stopSessionMediaRTP(d diago.DialogSession) {
	if d == nil {
		return
	}
	m := d.Media()
	if m == nil || m.MediaSession() == nil {
		return
	}
	_ = m.StopRTP(3, 0)
}

func startSessionMediaRTP(d diago.DialogSession) {
	if d == nil {
		return
	}
	m := d.Media()
	if m == nil || m.MediaSession() == nil {
		return
	}
	// Clear read and write deadlines (StopRTP(3) may have expired both during bridge/hold teardown).
	_ = m.StartRTP(3, 0)
}

func resetDialogLegRTP(d diago.DialogSession) {
	if d == nil {
		return
	}
	switch s := d.(type) {
	case *diago.DialogServerSession:
		if s.RTPPacketWriter != nil {
			s.RTPPacketWriter.ResetTimestamp()
		}
	case *diago.DialogClientSession:
		if s.RTPPacketWriter != nil {
			s.RTPPacketWriter.ResetTimestamp()
		}
	}
}

// PrepareConferenceLeg resets media hooks and ensures RTP is running before mixer/MOH.
func PrepareConferenceLeg(sess diago.DialogSession) {
	if sess == nil {
		return
	}
	switch s := sess.(type) {
	case *diago.DialogServerSession:
		s.SetAudioReader(nil)
		s.SetAudioWriter(nil)
		EnsureSessionDTMFCodec(s)
	case *diago.DialogClientSession:
		s.SetAudioReader(nil)
		s.SetAudioWriter(nil)
		EnsureClientDTMFCodec(s)
	default:
		clearSessionMediaHooks(sess)
	}
	startSessionMediaRTP(sess)
	resetDialogLegRTP(sess)
}

// prepareLegsForBridge resets media hooks and RTP state before starting a PCM bridge
// or after tearing down hold/MOH playback.
func prepareLegsForBridge(a, c diago.DialogSession) {
	if a == nil || c == nil {
		return
	}
	prepareBridgeLegs(a, c)
	clearSessionMediaHooks(a)
	clearSessionMediaHooks(c)
	startSessionMediaRTP(a)
	startSessionMediaRTP(c)
	resetDialogLegRTP(a)
	resetDialogLegRTP(c)
	if prepareLegsForBridgeHook != nil {
		prepareLegsForBridgeHook(a, c)
	}
}

// prepareLegsForBridgeHook is set by tests to verify bridge/hold media sequencing (REQ-HOLD-6).
var prepareLegsForBridgeHook func(a, c diago.DialogSession)

// BridgeLegsPCM bridges two answered dialog legs exclusively via PCM transcoding.
func BridgeLegsPCM(log *slog.Logger, a, b diago.DialogSession) (func() error, error) {
	bp := &BridgePair{Log: log}
	stop, _, err := bp.startTranscodingBridge(a, b)
	return stop, err
}
