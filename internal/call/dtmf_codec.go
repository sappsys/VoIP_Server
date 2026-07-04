package call

import (
	diagomedia "github.com/emiago/diago/media"
	"github.com/emiago/diago/media/sdp"
	"github.com/emiago/diago"
)

// RFC 3551 dynamic payload types may carry telephone-event (RFC 4733).
const (
	DynamicPayloadTypeMin = 96
	DynamicPayloadTypeMax = 127
)

func isDynamicPayloadType(pt uint8) bool {
	return pt >= DynamicPayloadTypeMin && pt <= DynamicPayloadTypeMax
}

// alignCodecsWithRemoteDTMF returns a copy of local with telephone-event payload
// types taken from remoteSDP so SDP negotiation matches the phone (e.g. PT 95).
func alignCodecsWithRemoteDTMF(local []diagomedia.Codec, remoteSDP []byte) []diagomedia.Codec {
	remotes := telephoneEventsFromSDP(remoteSDP)
	if len(remotes) == 0 || len(local) == 0 {
		return local
	}
	out := make([]diagomedia.Codec, 0, len(local)+len(remotes))
	seenPT := make(map[uint8]bool)
	for _, c := range local {
		if c.Name == "telephone-event" {
			continue
		}
		if !seenPT[c.PayloadType] {
			out = append(out, c)
			seenPT[c.PayloadType] = true
		}
	}
	for _, remote := range remotes {
		if !seenPT[remote.PayloadType] {
			out = append(out, remote)
			seenPT[remote.PayloadType] = true
		}
	}
	return out
}

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

// BuildTelephoneEventPTSet returns RTP payload types to decode as RFC 2833 DTMF.
// All dynamic PTs 96–127 are accepted unless already used by a non-DTMF codec.
func BuildTelephoneEventPTSet(ms *diagomedia.MediaSession, inviteSDP []byte, primary diagomedia.Codec) map[uint8]bool {
	pts := make(map[uint8]bool)
	addTE := func(codecs []diagomedia.Codec) {
		for _, c := range codecs {
			if c.Name == "telephone-event" {
				pts[c.PayloadType] = true
			}
		}
	}
	if primary.Name == "telephone-event" {
		pts[primary.PayloadType] = true
	}
	if ms != nil {
		addTE(ms.Codecs)
		addTE(ms.CommonCodecs())
	}
	for _, c := range telephoneEventsFromSDP(inviteSDP) {
		pts[c.PayloadType] = true
	}
	audioPTs := audioPayloadTypes(ms, primary)
	for pt := uint8(DynamicPayloadTypeMin); pt <= DynamicPayloadTypeMax; pt++ {
		if !audioPTs[pt] {
			pts[pt] = true
		}
	}
	return pts
}

func audioPayloadTypes(ms *diagomedia.MediaSession, primary diagomedia.Codec) map[uint8]bool {
	pts := make(map[uint8]bool)
	add := func(codecs []diagomedia.Codec) {
		for _, c := range codecs {
			if c.Name != "telephone-event" {
				pts[c.PayloadType] = true
			}
		}
	}
	if ms != nil {
		add(ms.Codecs)
		add(ms.CommonCodecs())
	}
	if primary.Name != "telephone-event" {
		pts[primary.PayloadType] = true
	}
	return pts
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
	primary := fallback
	for _, body := range sdpBodiesForDTMF(in) {
		primary = patchMediaSessionDTMFCodec(ms, body, primary)
	}
	return primary
}

// EnsureClientDTMFCodec aligns the outbound leg with telephone-event from the
// remote 200 OK SDP. Without this, bridged calls log unsupported pt=95 warnings
// when the callee sends RFC 2833 DTMF.
func EnsureClientDTMFCodec(out *diago.DialogClientSession) diagomedia.Codec {
	fallback := diagomedia.CodecTelephoneEvent8000
	if out == nil {
		return fallback
	}
	ms := out.MediaSession()
	if ms == nil {
		return fallback
	}
	var sdpBody []byte
	if out.DialogClientSession != nil {
		if out.InviteResponse != nil {
			sdpBody = out.InviteResponse.Body()
		}
		if len(sdpBody) == 0 && out.InviteRequest != nil {
			sdpBody = out.InviteRequest.Body()
		}
	}
	return patchMediaSessionDTMFCodec(ms, sdpBody, fallback)
}

func patchMediaSessionDTMFCodec(ms *diagomedia.MediaSession, inviteSDP []byte, fallback diagomedia.Codec) diagomedia.Codec {
	remotes := telephoneEventsFromSDP(inviteSDP)
	primary := dtmfPrimaryCodec(remotes, fallback)

	kept := make([]diagomedia.Codec, 0, len(ms.Codecs)+len(remotes)+1)
	seenPT := make(map[uint8]bool)
	for _, c := range ms.Codecs {
		if c.Name == "telephone-event" {
			continue
		}
		if !seenPT[c.PayloadType] {
			kept = append(kept, c)
			seenPT[c.PayloadType] = true
		}
	}
	for _, remote := range remotes {
		if !seenPT[remote.PayloadType] {
			kept = append(kept, remote)
			seenPT[remote.PayloadType] = true
		}
	}
	if len(remotes) == 0 {
		if !seenPT[fallback.PayloadType] {
			kept = append(kept, fallback)
		}
	} else if !seenPT[primary.PayloadType] {
		kept = append(kept, primary)
	}
	ms.Codecs = kept
	return primary
}

func dtmfPrimaryCodec(remotes []diagomedia.Codec, fallback diagomedia.Codec) diagomedia.Codec {
	if len(remotes) > 0 {
		return remotes[0]
	}
	return fallback
}

func dtmfCodecFromList(codecs []diagomedia.Codec, fallback diagomedia.Codec) diagomedia.Codec {
	for _, c := range codecs {
		if c.Name == "telephone-event" {
			return c
		}
	}
	return fallback
}
