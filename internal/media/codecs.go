package media

import (
	"fmt"
	"strings"
	"time"

	diagomedia "github.com/emiago/diago/media"
)

// Voice codec identifiers used in config and SDP negotiation.
const (
	CodecPCMU    = "PCMU"
	CodecPCMA    = "PCMA"
	CodecG722    = "G722"
	CodecG723    = "G723"
	CodecG72353  = "G723-53"
	CodecG72363  = "G723-63"
	CodecG729    = "G729"
	CodecG72616  = "G726-16"
	CodecG72624  = "G726-24"
	CodecG72632  = "G726-32"
	CodecG72640  = "G726-40"
	CodecDTMF    = "telephone-event"
)

// DefaultVoiceCodecIDs is the preferred negotiation order for bridged calls.
var DefaultVoiceCodecIDs = []string{
	CodecPCMU,
	CodecPCMA,
	CodecG722,
	CodecG729,
	CodecG72363,
	CodecG72353,
	CodecG72632,
	CodecG72616,
	CodecG72624,
	CodecG72640,
	CodecDTMF,
}

var voiceCodecTable = map[string]diagomedia.Codec{
	CodecPCMU:   diagomedia.CodecAudioUlaw,
	CodecPCMA:   diagomedia.CodecAudioAlaw,
	CodecG722:   {PayloadType: 9, SampleRate: 8000, SampleDur: 20 * time.Millisecond, NumChannels: 1, Name: "G722"},
	CodecG723:   {PayloadType: 4, SampleRate: 8000, SampleDur: 30 * time.Millisecond, NumChannels: 1, Name: "G723"},
	CodecG72363: {PayloadType: 4, SampleRate: 8000, SampleDur: 30 * time.Millisecond, NumChannels: 1, Name: "G723"},
	CodecG72353: {PayloadType: 104, SampleRate: 8000, SampleDur: 30 * time.Millisecond, NumChannels: 1, Name: "G723"},
	CodecG729:   {PayloadType: 18, SampleRate: 8000, SampleDur: 10 * time.Millisecond, NumChannels: 1, Name: "G729"},
	CodecG72632: {PayloadType: 2, SampleRate: 8000, SampleDur: 20 * time.Millisecond, NumChannels: 1, Name: "G726-32"},
	CodecG72616: {PayloadType: 111, SampleRate: 8000, SampleDur: 20 * time.Millisecond, NumChannels: 1, Name: "G726-16"},
	CodecG72624: {PayloadType: 112, SampleRate: 8000, SampleDur: 20 * time.Millisecond, NumChannels: 1, Name: "G726-24"},
	CodecG72640: {PayloadType: 113, SampleRate: 8000, SampleDur: 20 * time.Millisecond, NumChannels: 1, Name: "G726-40"},
	CodecDTMF:   diagomedia.CodecTelephoneEvent8000,
}

// VoiceCodecs resolves configured codec IDs to diago media codecs for SDP/RTP.
// Unknown IDs are skipped. DTMF is appended when absent so feature codes keep working.
func VoiceCodecs(ids []string) ([]diagomedia.Codec, error) {
	if len(ids) == 0 {
		ids = DefaultVoiceCodecIDs
	}
	seenPT := map[uint8]bool{}
	var out []diagomedia.Codec
	hasDTMF := false

	for _, raw := range ids {
		id := normalizeCodecID(raw)
		if id == "" {
			continue
		}
		c, ok := voiceCodecTable[id]
		if !ok {
			return nil, fmt.Errorf("unknown codec %q", raw)
		}
		if id == CodecDTMF {
			hasDTMF = true
		}
		if seenPT[c.PayloadType] {
			continue
		}
		seenPT[c.PayloadType] = true
		out = append(out, c)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no valid codecs configured")
	}
	if !hasDTMF {
		dtmf := diagomedia.CodecTelephoneEvent8000
		if !seenPT[dtmf.PayloadType] {
			out = append(out, dtmf)
		}
	}
	return out, nil
}

func normalizeCodecID(id string) string {
	id = strings.TrimSpace(strings.ToUpper(id))
	switch id {
	case "G711U", "G711ULAW", "ULAW", "G.711U", "G.711MU":
		return CodecPCMU
	case "G711A", "G711ALAW", "ALAW", "G.711A":
		return CodecPCMA
	case "G.722":
		return CodecG722
	case "G.729", "G729A", "G.729A":
		return CodecG729
	case "G.723", "G723.1":
		return CodecG72363
	case "G723(5.3KBPS)", "G723-5.3", "G723-53K", "G723-53KBPS":
		return CodecG72353
	case "G723(6.3KBPS)", "G723-6.3", "G723-63K", "G723-63KBPS":
		return CodecG72363
	case "G726(16KBPS)", "G726-16K":
		return CodecG72616
	case "G726(24KBPS)", "G726-24K":
		return CodecG72624
	case "G726(32KBPS)", "G726-32K":
		return CodecG72632
	case "G726(40KBPS)", "G726-40K":
		return CodecG72640
	case "DTMF", "TELEPHONE-EVENT":
		return CodecDTMF
	default:
		return id
	}
}

// SupportedVoiceCodecIDs returns sorted known codec IDs (for docs/UI).
func SupportedVoiceCodecIDs() []string {
	ids := make([]string, 0, len(voiceCodecTable))
	for id := range voiceCodecTable {
		if id == CodecG723 {
			continue // alias of G723-63
		}
		ids = append(ids, id)
	}
	sortStrings(ids)
	return ids
}

func sortStrings(s []string) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
