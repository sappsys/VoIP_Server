// Package call implements B2BUA call bridging, active-call tracking, parking,
// music-on-hold, blind/attended transfer helpers, and DND ringing behavior.
//
// BridgePair connects two SIP dialog legs (caller server session + callee client
// session). Registry tracks in-flight calls for star-code features such as
// transfer (*77), park (*85), and consult linking during attended transfer.
package call
