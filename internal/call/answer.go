package call

import (
	"github.com/emiago/diago"
)

// AnswerSession answers the inbound leg and aligns telephone-event with the
// remote SDP payload type (e.g. PT 95). Call this instead of in.Answer().
func AnswerSession(in *diago.DialogServerSession) error {
	if err := in.Answer(); err != nil {
		return err
	}
	EnsureSessionDTMFCodec(in)
	return nil
}

// AnswerSessionOptions is like AnswerSession but passes diago.AnswerOptions.
func AnswerSessionOptions(in *diago.DialogServerSession, opts diago.AnswerOptions) error {
	if err := in.AnswerOptions(opts); err != nil {
		return err
	}
	EnsureSessionDTMFCodec(in)
	return nil
}
