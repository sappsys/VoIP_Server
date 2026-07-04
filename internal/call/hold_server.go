package call

import (
	"context"
	"log/slog"

	"github.com/sappsys/VoIP_Server/internal/media/audiobridge"
)

// StopBridge tears down active RTP bridging between call legs.
func (ac *ActiveCall) StopBridge() {
	if ac == nil {
		return
	}
	ac.holdMu.Lock()
	defer ac.holdMu.Unlock()
	ac.stopBridgeLocked()
}

// setBridgeStop stores the bridge teardown func under the hold lock so it is
// visible to concurrent hold detection goroutines.
func (ac *ActiveCall) setBridgeStop(stop func() error, stats *audiobridge.RelayStats) {
	if ac == nil {
		return
	}
	ac.holdMu.Lock()
	ac.bridgeStop = stop
	ac.relayStats = stats
	ac.holdMu.Unlock()
}

// HoldRemoteWithMOH stops the bridge and plays hold music to the party that is not
// the holder (attended transfer arming, park, etc.).
func (ac *ActiveCall) HoldRemoteWithMOH(ctx context.Context, holderExt, mohDir string, log *slog.Logger) {
	if ac == nil {
		return
	}
	ac.holdMu.Lock()
	defer ac.holdMu.Unlock()

	ac.stopBridgeLocked()
	ac.stopHoldPlaybackLocked()

	if mohDir == "" {
		ac.HoldActive = true
		ac.HolderExt = holderExt
		return
	}

	holdCtx, cancel := context.WithCancel(ctx)
	ac.holdCancel = cancel
	player := &holdPlayer{}
	ac.holdPlayer = player

	if holderExt == ac.CallerExt && ac.Out != nil {
		player.goMOH(holdCtx, ac.Out.Context(), mohDir, log, ac.Out, ac.Out.PlaybackControlCreate)
	} else if holderExt == ac.CalleeExt && ac.In != nil {
		player.goMOH(holdCtx, ac.In.Context(), mohDir, log, ac.In, ac.In.PlaybackControlCreate)
	}

	ac.HoldActive = true
	ac.HolderExt = holderExt
	if log != nil {
		log.Info("remote party on hold with moh", "holder", holderExt, "caller", ac.CallerExt, "callee", ac.CalleeExt)
	}
}

func (ac *ActiveCall) stopBridgeLocked() {
	if ac.bridgeStop != nil {
		stop := ac.bridgeStop
		ac.bridgeStop = nil
		ac.relayStats = nil
		_ = stop()
	}
}

func (ac *ActiveCall) stopHoldPlaybackLocked() {
	if ac.holdCancel != nil {
		ac.holdCancel()
		ac.holdCancel = nil
	}
	if ac.holdPlayer != nil {
		ac.holdPlayer.stopAndWait()
		ac.holdPlayer = nil
	}
}
