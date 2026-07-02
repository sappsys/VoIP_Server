package call

import (
	diagomedia "github.com/emiago/diago/media"
	"github.com/emiago/diago/media/sdp"
	"github.com/emiago/diago"
)

// telephoneEventsFromSDP returns all telephone-event codecs in an SDP body.
func telephoneEventsFromSDP(sdpBytes []byte) []diagomedia.Codec {
	if len(sdpBytes) == 0 {
		return nil
	}
	sd := sdp.SessionDescription{}
	if err := sdp.Unmarshal(sdpBytes, &sd); err != nil {
		return nil
	}
	md, err := sd.MediaDescription("audio")
	if err != nil {
		return nil
	}
	codecs := make([]diagomedia.Codec, len(md.Formats))
	n, err := diagomedia.CodecsFromSDPRead(md.Formats, sd.Values("a"), codecs)
	if err != nil && n == 0 {
		return nil
	}
	var out []diagomedia.Codec
	for _, c := range codecs[:n] {
		if c.Name == "telephone-event" {
			out = append(out, c)
		}
	}
	return out
}

// EnsureSessionDTMFCodec aligns MediaSession.Codecs with the remote telephone-event
// payload type from the INVITE SDP. Phones often negotiate DTMF on a dynamic PT
// (e.g. 95) while diago defaults to 101; without this patch RTP packets on the
// remote PT are dropped and logged as unsupported.
func EnsureSessionDTMFCodec(in *diago.DialogServerSession) diagomedia.Codec {
	fallback := diagomedia.CodecTelephoneEvent8000
	if in == nil {
		return fallback
	}
	ms := in.MediaSession()
	if ms == nil {
		return fallback
	}
	return patchMediaSessionDTMFCodec(ms, in.InviteRequest.Body(), fallback)
}

func patchMediaSessionDTMFCodec(ms *diagomedia.MediaSession, inviteSDP []byte, fallback diagomedia.Codec) diagomedia.Codec {
	remotes := telephoneEventsFromSDP(inviteSDP)
	if len(remotes) == 0 {
		return dtmfCodecFromList(ms.Codecs, fallback)
	}
	primary := remotes[0]

	seenPT := make(map[uint8]bool, len(ms.Codecs))
	for _, c := range ms.Codecs {
		seenPT[c.PayloadType] = true
	}
	for _, remote := range remotes {
		if !seenPT[remote.PayloadType] {
			ms.Codecs = append(ms.Codecs, remote)
			seenPT[remote.PayloadType] = true
		}
	}
	for i, c := range ms.Codecs {
		if c.Name == "telephone-event" {
			ms.Codecs[i] = primary
			seenPT[primary.PayloadType] = true
		}
	}
	if !seenPT[primary.PayloadType] {
		ms.Codecs = append(ms.Codecs, primary)
	}
	return primary
}

func dtmfCodecFromList(codecs []diagomedia.Codec, fallback diagomedia.Codec) diagomedia.Codec {
	for _, c := range codecs {
		if c.Name == "telephone-event" {
			return c
		}
	}
	return fallback
}
