// Package pbx wires the SIP server (sipgo/diago), registrar, dial plan, B2BUA
// bridges, hunt/conference/trunk/paging handlers, and star-code feature handlers.
//
// INVITEs from registered extensions are routed by number; inbound trunk calls
// use SQLite trunk routes. Star codes (redial, call return, transfer, park,
// DND) are handled before normal routing when matched in the dial plan.
package pbx
