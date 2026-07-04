# VoIP PBX — Requirements

This document captures the functional and non-functional requirements for the
VoIP PBX server, gathered from the original plan and the ongoing implementation
discussions. Each requirement has a stable `REQ-*` identifier that maps to unit
and integration tests (see [Traceability](#traceability)).

## Overview

The server is a SIP PBX built on:

- **sipgo** — SIP signalling (registrar, MESSAGE, presence, trunks).
- **diago** — dialogs, INVITE/Answer/Hold/Unhold, RTP readers/writers, playback.
- **Custom PCM media layer** — `internal/media/audiobridge` and
  `internal/media/pcmcodec` for bridging, conferencing, transcoding, and paging.

Two-party calls, hold/MOH, transfer, and conferencing all route audio through
the PCM bridge so that mixed-codec endpoints interoperate with low latency.

Reference deployment: extensions **110** and **111**, password `andy`, server on
the LAN. Extensions are numbered **100–499**, hunt groups **500–599**,
conference rooms **600–699**, paging `*80`–`*99`.

---

## 1. Phone-to-phone calls

### REQ-CALL-1 — Geographic ringback while ringing
While a call is placed and the remote endpoint is ringing (not yet answered),
the caller MUST hear a **geographically relevant ringing tone** so they know the
endpoint is ringing. The tone plan (UK / EU / USA) is selected from
`[tones] region` in config; the tone is played on early media (before 200 OK)
in response to SIP `180 Ringing` or `183 Session Progress`.

### REQ-CALL-2 — Busy tone then hangup when not connected
If the endpoint is not connected (unregistered, unreachable, INVITE fails, or
ring times out), the caller MUST hear a **geographically relevant busy tone for
5 seconds** (configurable via `[tones] busy_seconds`, default 5) and then the
call MUST be hung up.

### REQ-CALL-3 — Answered call bridges bidirectional audio
When the callee answers, the caller and callee MUST be connected with two-way
audio via the PCM bridge, and ringback MUST stop.

### REQ-CALL-4 — Symmetric teardown
When either party hangs up, the peer leg MUST be released (BYE) and media torn
down. No orphaned dialogs or bridges.

---

## 2. Hold, Music on Hold, transfer, conference-from-hold

### REQ-HOLD-1 — Holder hears dial tone
When a caller or recipient puts the call on hold, the party that initiated the
hold MUST hear a **dial tone** (so they can dial a third party).

### REQ-HOLD-2 — Remote party hears Music on Hold
The remote (held) party MUST hear **Music on Hold** while the call is on hold.

### REQ-HOLD-3 — Unhold restores audio
On exiting hold, two-way audio between the original parties MUST be restored.
The bridge is restarted only when both legs are `sendrecv`; media legs are
prepared (`prepareLegsForBridge`) before the PCM relay restarts.

### REQ-HOLD-4 — Hold direction semantics
- Phone-initiated hold is negotiated **recvonly** on our side (phone sends
  `a=sendonly`, we answer `recvonly`) — treated as hold.
- **inactive** media is treated as hold.
- A server-initiated **sendonly** re-INVITE (our dial-tone leg) is **not** phone
  hold.
- `sendrecv` is an active call, not hold.
- `c=0.0.0.0` with `sendrecv` is treated as hold.

### REQ-HOLD-5 — Holder release detection
When the holder's phone takes the call off hold (sends a new SDP that is not
recvonly), the server MUST detect release and drive the unhold flow.

### REQ-XFER-1 — Conference in a third party
If the party that put the call on hold dials another extension, they MAY
**conference in** that third person to the call.

### REQ-XFER-2 — Blind transfer
The holder MAY **blind transfer** the call by either:
- hanging up after dialling the target extension, or
- pressing the transfer key (SIP `REFER`).

The held party MUST be bridged to the transfer target via the PCM bridge.

### REQ-XFER-3 — Attended transfer completion
Attended transfer (via the `*77`-style transfer code, then dialling the target)
MUST bridge the held party to the dialled target and release the transferor.

---

## 3. Conference bridge

### REQ-CONF-PIN-1 — Immediate DTMF during PIN prompt
Phones calling into a conference bridge (600–699) are required to key in a PIN.
DTMF decoding MUST be **immediate**, working even during the "enter your PIN"
prompt. Both **RFC 2833** (`telephone-event`, including dynamic PT like 95) and
**in-band** DTMF MUST be decoded. The first digit stops the prompt playback.

### REQ-CONF-PIN-2 — Wrong PIN retries
A wrong PIN MUST replay a "PIN incorrect" prompt and allow retry, up to a
maximum number of attempts (3).

### REQ-CONF-1 — MOH stops before mixer
When the conference transitions to mixing, MOH MUST be stopped **before** the
mixer starts (no MOH mixed into live conference audio).

### REQ-CONF-2 — Sole participant hears MOH
If a participant is the **only** caller in the room — because they are first, or
because everyone else has left — they MUST hear Music on Hold.

### REQ-CONF-3 — Two or more participants use the mixer
With two or more participants, audio MUST be mixed (no MOH).

### REQ-CONF-4 — MOH restarts on 2→1
When a room drops from two participants back to one, MOH MUST restart for the
remaining participant.

### REQ-CONF-5 — Hold does not trigger conference MOH
When a participant puts their phone on hold during a conference, they remain
admitted in the room. With two or more admitted participants, the server MUST NOT
play Music on Hold to any party; the mixer MUST remain active (or be refreshed on
hold/unhold media updates).

---

## 4. Audio quality

### REQ-BRIDGE-1 — Same-codec passthrough
When both legs negotiate the **same** codec, audio MUST be relayed without
transcoding (passthrough).

### REQ-BRIDGE-2 — Low-latency relay (no playback clock)
The relay path MUST use `WriteSamples` (RTP relay writer) and MUST NOT be paced
by diago's playback clock. The audiobridge package MUST NOT call `ClockDisable()`
(it panics on a nil clock ticker in diago `Write()`).

### REQ-BRIDGE-3 — G.722 hub framing
G.722 hub frames are 80 samples at the 8 kHz RTP timestamp clock (one RTP
packet), not 160.

### REQ-BRIDGE-5 — Mixed-codec transcoding
Deployments MUST support a **mix of codecs**. Common PBX codecs (PCMU, PCMA,
G.722, G.729, G.726) MUST transcode through the PCM hub when the two legs differ.

### REQ-BRIDGE-6 — All two-party calls use the PCM bridge
Extension-to-extension, trunk, transfer, park, and hunt two-party calls MUST use
the PCM bridge path (passthrough when codecs match, transcode when they differ).

### REQ-AUDIO-LATENCY — Low latency
Audio MUST be low latency. The relay avoids the diago playback clock and extra
jitter buffering on the passthrough path; transcoding uses fixed 20 ms framing.

---

## 5. Reliability (basic PBX functionality)

### REQ-PBX-1 — Registration & keepalive
Extensions register with digest auth; NAT-aware contact rewriting and OPTIONS
keepalive keep bindings alive. Re-REGISTER refreshes bindings without dropping.

### REQ-PBX-2 — Call capacity
System-wide `[limits] max_calls` caps total concurrent calls across the PBX.
Per-extension `max_simultaneous_calls` (default from `[limits] max_calls_per_extension`, 5)
limits how many active calls each extension may have; `[limits] max_calls_per_extension`
is the default when an extension file omits its own limit. Calls over either limit get a busy tone.

### REQ-PBX-3 — Feature codes
Configurable star codes: redial, call return, transfer, park, park retrieve, DND
on/off. DND blocks inbound dialling.

### REQ-PBX-4 — Standard call features
Hunt groups, conferences, paging, trunks (inbound/outbound), park/retrieve,
blind/attended transfer, MOH.

### REQ-PBX-5 — No regressions
Hold/MOH/unhold, conference MOH↔mixer transitions, and dial tone MUST NOT
regress. These are guarded by requirement tests.

---

## Traceability

| Requirement | Unit tests | Integration tests |
|-------------|-----------|-------------------|
| REQ-CALL-1 | `requirements_signalling_test.go` (`TestREQ_CALL_GeographicRingProfilesDiffer`, `TestREQ_CALL_RingbackOnRingingResponses`, `TestREQ_CALL_BridgeUsesConfiguredTones`) | `call_flows_integration_test.go` (`TestREQ_CALL_RingbackWhileRinging`) |
| REQ-CALL-2 | `requirements_signalling_test.go` (`TestREQ_CALL_BusyDuration*`) | `call_flows_integration_test.go` (`TestREQ_CALL_BusyThenHangupWhenUnavailable`) |
| REQ-CALL-3 | `bridge_lifecycle_test.go` | `call_flows_integration_test.go` (`TestREQ_CALL_AnsweredCallHasTwoWayAudio`) |
| REQ-CALL-4 | `registry_test.go` | `call_flows_integration_test.go` (`TestREQ_CALL_HangupReleasesPeer`) |
| REQ-HOLD-1/2 | `requirements_hold_enter_test.go` (`TestREQ_HOLD_EnterStartsDialToneAndMOH`), `requirements_hold_moh_codec_test.go` | `hold_integration_test.go` (`TestREQ_HOLD_CallerHoldDialToneAndMOH`, `TestREQ_HOLD_CalleeHoldDialToneAndMOH`) |
| REQ-HOLD-3 | `requirements_bridge_test.go` (`TestREQ_HOLD_LeaveRestartsBridgeAfterPrepare`) | `hold_integration_test.go` (`TestREQ_HOLD_UnholdRestoresAudio`, `TestREQ_HOLD_UnholdRestoresRTP`) |
| REQ-HOLD-4/5 | `requirements_hold_test.go`, `requirements_hold_leave_test.go` | `hold_integration_test.go` (`TestREQ_HOLD_StableAfterDialToneReinvite`) |
| REQ-XFER-1 | `pbx/requirements_transfer_test.go` (`TestREQ_XFER_ConsultLegLinkedWhileOnHold`) | `transfer_integration_test.go` |
| REQ-XFER-2 | `pbx/requirements_transfer_test.go` (`TestREQ_XFER_BlindTransferUsesReferHandler`) | `transfer_integration_test.go` (`TestREQ_XFER_BlindTransferBridgesTarget`) |
| REQ-XFER-3 | `pbx/requirements_transfer_test.go` (`TestREQ_XFER_CompleteTransferBridgesHeldParty`) | `transfer_integration_test.go` (`TestREQ_XFER_AttendedTransferBridges`) |
| REQ-CONF-PIN-1 | `requirements_prompt_dtmf_test.go`, `conference/requirements_pin_test.go` | `conference_integration_test.go` (`TestREQ_CONF_PINThenSoloMOH`) |
| REQ-CONF-PIN-2 | `conference/requirements_pin_test.go` (`TestREQ_CONF_PIN_MaxAttempts`) | `conference_integration_test.go` (`TestREQ_CONF_WrongPINNotAdmitted`) |
| REQ-CONF-1..4 | `conference/reconcile_requirements_test.go`, `requirements_order_test.go`, `requirements_prepare_conference_test.go` | `conference_integration_test.go` (`TestREQ_CONF_PINThenSoloMOH`, `TestREQ_CONF_TwoParticipantsMixerAudio`, `TestREQ_CONF_DropToOneRestartsMOH`) |
| REQ-CONF-5 | `conference/reconcile_requirements_test.go` (`TestREQ_CONF_TwoAdmittedHoldDoesNotStartMOH`) | `conference_integration_test.go` (`TestREQ_CONF_HoldDoesNotStartMOH`) |
| REQ-BRIDGE-1/2/3 | `audiobridge/bridge_requirements_test.go` | — |
| REQ-BRIDGE-5/6 | `audiobridge/requirements_codec_test.go` | `mixed_codec_integration_test.go` (`TestREQ_BRIDGE_MixedCodecCall`, `TestREQ_BRIDGE_SameCodecPassthrough`) |
| REQ-AUDIO-LATENCY | `audiobridge/bridge_requirements_test.go`, `audiobridge/requirements_codec_test.go` | `mixed_codec_integration_test.go` (bridge liveness under transcoding/passthrough) |
| REQ-PBX-1 | `registrar/*_test.go` | `sip_integration_test.go` (`TestSIPRegisterExtension`) |
| REQ-PBX-2 | `manager_test.go` | `call_flows_integration_test.go` (`TestREQ_PBX_MaxCallsBusy`) |
| REQ-PBX-3 | `pbx/features_test.go` | `features_integration_test.go` (`TestREQ_PBX_DNDBlocksInboundDial`) |
| REQ-PBX-4 | package tests | `*_integration_test.go` |
| REQ-PBX-5 | requirement suites above | integration suites above |

## Running tests

```bash
# Unit tests (fast)
go test ./...

# Integration tests (simulated SIP handsets over UDP loopback)
go test -tags=integration ./test/integration/...
```

Integration tests spin up a real PBX server and drive it with simulated
`diago`/`sipgo` user agents (handsets) over `127.0.0.1`. When an integration
test fails, first determine whether it is a test harness issue (timing, SDP,
port) or a production code defect, and fix the correct layer — never weaken a
requirement assertion to make a test pass.

Note on `-race`: production packages are race-clean (`go test -race ./internal/...`).
The integration harness uses **separate client and server `diago` stacks per simulated
handset** so outbound calls and in-dialog re-INVITEs do not share one user agent.
Integration tests can be run with `-race`:

```bash
go test -race -tags=integration ./test/integration/...
```

Remaining races in the PBX itself were fixed (ringback teardown, `HoldActive` snapshots,
DTMF patching after answer). Stop audio pumps before `Unhold()` to avoid races inside
`diago`'s client re-INVITE path. Star-code dials during an active call use `inviteServer()`
(server stack), not a second client-stack `Invite`.
