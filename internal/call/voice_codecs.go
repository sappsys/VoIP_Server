package call

import diagomedia "github.com/emiago/diago/media"

// defaultVoiceCodecs is the server's configured codec list (set at PBX startup).
var defaultVoiceCodecs []diagomedia.Codec

// SetDefaultVoiceCodecs records the negotiated voice codec list so inbound answers
// can align telephone-event with the remote SDP before media is established.
func SetDefaultVoiceCodecs(codecs []diagomedia.Codec) {
	defaultVoiceCodecs = append([]diagomedia.Codec(nil), codecs...)
}
