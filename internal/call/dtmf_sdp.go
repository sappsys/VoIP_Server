package call

import (
	"github.com/emiago/diago"
	diagomedia "github.com/emiago/diago/media"
)

// sdpBodiesForDTMF returns INVITE and answered SDP bodies for telephone-event discovery.
func sdpBodiesForDTMF(in *diago.DialogServerSession) [][]byte {
	if in == nil || in.DialogServerSession == nil {
		return nil
	}
	var bodies [][]byte
	if in.InviteRequest != nil {
		if b := in.InviteRequest.Body(); len(b) > 0 {
			bodies = append(bodies, b)
		}
	}
	if ms := in.MediaSession(); ms != nil {
		if b := ms.LocalSDP(); len(b) > 0 {
			bodies = append(bodies, b)
		}
	}
	return bodies
}

// telephoneEventsFromSDPs collects telephone-event codecs from multiple SDP bodies.
func telephoneEventsFromSDPs(bodies ...[]byte) []diagomedia.Codec {
	seen := make(map[uint8]bool)
	var out []diagomedia.Codec
	for _, body := range bodies {
		for _, c := range telephoneEventsFromSDP(body) {
			if seen[c.PayloadType] {
				continue
			}
			seen[c.PayloadType] = true
			out = append(out, c)
		}
	}
	return out
}

// buildTelephoneEventPTSetFromSDPs builds the RFC2833 PT map from session state and SDP.
func buildTelephoneEventPTSetFromSDPs(ms *diagomedia.MediaSession, sdpBodies [][]byte, primary diagomedia.Codec) map[uint8]bool {
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
	for _, body := range sdpBodies {
		addTE(telephoneEventsFromSDP(body))
	}
	audioPTs := audioPayloadTypes(ms, primary)
	for pt := uint8(DynamicPayloadTypeMin); pt <= DynamicPayloadTypeMax; pt++ {
		if !audioPTs[pt] {
			pts[pt] = true
		}
	}
	return pts
}
