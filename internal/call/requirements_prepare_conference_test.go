package call

import (
	"testing"

	"github.com/emiago/diago/media/sdp"
)

// REQ-CONF-3: conference legs must be RTP-ready before mixer attach.
func TestREQ_CONF_PrepareConferenceLegResetsHooks(t *testing.T) {
	ms := mediaSessionWithLocalDirection(t, sdp.ModeSendrecv)
	in := testServerDialog("conf")
	in.InitMediaSession(ms, nil, nil)

	PrepareConferenceLeg(in)
	// Second call must not panic (idempotent prep before mixer/MOH handoff).
	PrepareConferenceLeg(in)
}

func TestREQ_CONF_PrepareConferenceLegNilSafe(t *testing.T) {
	PrepareConferenceLeg(nil)
}
