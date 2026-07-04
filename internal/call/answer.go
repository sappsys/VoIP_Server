package call

import (
	"github.com/emiago/diago"
	diagomedia "github.com/emiago/diago/media"
)

func prepareServerAnswer(in *diago.DialogServerSession, opts diago.AnswerOptions) diago.AnswerOptions {
	inviteSDP := in.InviteRequest.Body()

	if ms := in.MediaSession(); ms != nil {
		patchMediaSessionDTMFCodec(ms, inviteSDP, defaultDTMFFallback())
	}

	switch {
	case len(opts.Codecs) > 0:
		opts.Codecs = alignCodecsWithRemoteDTMF(opts.Codecs, inviteSDP)
	case len(defaultVoiceCodecs) > 0:
		opts.Codecs = alignCodecsWithRemoteDTMF(defaultVoiceCodecs, inviteSDP)
	}
	return opts
}

func defaultDTMFFallback() diagomedia.Codec {
	return diagomedia.CodecTelephoneEvent8000
}

// AnswerSession answers the inbound leg and aligns telephone-event with the
// remote SDP payload type (e.g. PT 95). Call this instead of in.Answer().
func AnswerSession(in *diago.DialogServerSession) error {
	return AnswerSessionOptions(in, diago.AnswerOptions{})
}

// AnswerSessionOptions is like AnswerSession but passes diago.AnswerOptions.
func AnswerSessionOptions(in *diago.DialogServerSession, opts diago.AnswerOptions) error {
	opts = prepareServerAnswer(in, opts)
	return in.AnswerOptions(opts)
}
