package call

import (
	"github.com/emiago/diago"
)

// prepareBridgeLegs aligns DTMF payload types and codecs before bridging.
func prepareBridgeLegs(a, c diago.DialogSession) {
	switch s := a.(type) {
	case *diago.DialogServerSession:
		EnsureSessionDTMFCodec(s)
	case *diago.DialogClientSession:
		EnsureClientDTMFCodec(s)
	}
	switch s := c.(type) {
	case *diago.DialogServerSession:
		EnsureSessionDTMFCodec(s)
	case *diago.DialogClientSession:
		EnsureClientDTMFCodec(s)
	}
}
