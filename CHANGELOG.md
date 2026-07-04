# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html) with
an `alpha` or `beta` prerelease suffix.

## [Unreleased]

## [v0.2.0beta] - 2026-07-04

[Release](https://github.com/sappsys/VoIP_Server/releases/tag/v0.2.0beta)

First public **beta** release. Supersedes prior alpha tags (`v0.1.3alpha`–`v0.1.6alpha`), which were removed from GitHub.

### Added

- Third-party notices in `LICENSE` (Go modules, vendored diago/MPL-2.0, Asterisk CC BY-SA pointers)
- `assets/moh/README.md` — MOH is user-supplied; not published in the repo

### Changed

- Public-repo licensing: hold music and unclear third-party audio are not bundled; Asterisk English prompts remain with bundled LICENSE/CREDITS files
- Deploy installs root `LICENSE` and MOH README alongside assets

### Included from prior alpha work

- Go SIP PBX with hold/MOH, PCM bridge, conferences, hunt groups, web admin, integration tests (`REQUIREMENTS.md`)
- Conference hold no longer triggers solo MOH when two+ participants are admitted (REQ-CONF-5)
- Phone-initiated hold with dial tone + MOH on first press; unhold codec/bridge fixes

## [v0.1.6alpha] - 2026-07-04

(Superseded by v0.2.0beta — release removed from GitHub.)

### Fixed

- Conference hold no longer starts Music on Hold when two or more participants are admitted; held phones remain in the room (REQ-CONF-5)
- Conference mixer refreshes on hold/unhold media updates without treating hold as a participant leaving

## [v0.1.5alpha] - 2026-07-04

[Release](https://github.com/sappsys/VoIP_Server/releases/tag/v0.1.5alpha)

### Added

- Phone-initiated hold: holder hears dial tone, held party hears MOH on first press (real IP phone / unregistered caller path)
- PCM transcoding bridge with requirement tests for hold, transfer, conference, and mixed-codec calls
- `REQUIREMENTS.md` traceability for hold, bridge, and signalling behaviour
- Symmetric RTP (`RTPNAT`) for unregistered callers and NAT-learned media paths
- Regional dial/busy/ring tone profiles via `[tones]` in `config.toml`
- MP3 voice prompt playback where supported

### Fixed

- Hold entry taking 7+ seconds: dial tone on recvonly phone-hold leg without a racing server re-INVITE
- Hold release during slow entry leaving MOH running or activating hold after the user already released
- Corrupted audio after unhold: re-INVITE codec reorder (e.g. G722 before PCMU) no longer breaks the PCM bridge
- Pre-hold bridge codecs restored before bridge restart on unhold
- MOH and dial tone via pcmcodec RTP path for G722 and post-hold SDP churn

## [v0.1.4alpha] - 2026-07-02

[Release](https://github.com/sappsys/VoIP_Server/releases/tag/v0.1.4alpha)

### Added

- SIP instant messaging (`MESSAGE`) relay between registered extensions (text/plain and other content types)
- SIP presence via `SUBSCRIBE`/`NOTIFY` with PIDF XML (`open` / `closed` / `busy`)
- Offline message queue in SQLite, delivered when the recipient registers
- Admin backup/restore of `config.toml`, extensions, SQLite database, and optional MOH/phonebook files
- Trunk keepalive: per-trunk SIP `OPTIONS` pings (`keepalive_seconds`, default 30s) and optional `keepalive = "register"` with `register_expiry_seconds` (default 3600s); OPTIONS use the bound SIP socket for NAT pinholes
- Conference MOH: music on hold plays when only one participant is in a conference room; mixing starts when a second person joins
- MOH fallback finds `assets/moh.wav` when `assets/moh/` is empty; conference answers before PIN collection
- Configurable voice prompts via `[sounds]` in `config.toml`: busy (no call waiting), invalid/unknown number, conference PIN request, conference PIN incorrect (with re-request, up to 3 attempts), extension unavailable, and extension prompt for park retrieve (`*86` without slot)
- Dual DTMF detection for PIN/extension entry: RFC 2833 (`telephone-event`) and in-band G.711 tones
- WAV resampling for MOH/prompts: non-8 kHz stereo files are converted automatically for G.711 playback

### Fixed

- Conference PIN rejected when phones send DTMF on negotiated `telephone-event` PT (e.g. 95) instead of default 101; codec list patched at answer time
- PIN and extension digit entry require `#` to submit (timeout without `#` no longer accepted)
- MOH sounding like noise: resampled PCM now encoded to G.711 (`audio/pcm`) instead of sent as raw RTP
- MOH playback loop calling `Play()` repeatedly on the same file, causing cutouts and early stop
- Outbound hold MOH using raw WAV bytes without decode/encode

## [v0.1.3alpha] - 2026-07-02

[Release](https://github.com/sappsys/VoIP_Server/releases/tag/v0.1.3alpha)

### Added

- Configurable log level via `[logging] level` in `config.toml` (`debug`, `info`, `warn`, `error`)
- SIP OPTIONS keepalive for registered extensions (`options_keepalive_seconds`, default 30s)
- `DialTarget()` for outbound INVITE routing (URI, destination host:port, transport from Contact)
- NAT-aware contact rewrite when packet source differs from Contact host/port
- Cancel button on web Edit forms (extensions, hunt, conferences, paging, users, phonebook)
- `utils/test-*.sh` tiered test runners and extension-to-extension integration test
- `SIPBindHost()` — uses `external_host` when `bind_host` is all-interfaces (Docker/multi-homed fix)

### Changed

- Symmetric SIP routing for extension-to-extension, hunt, paging, and transfer outbound legs
- Outbound INVITE uses `SetDestination()` with registered contact dial targets
- Registrar expiry bounds, binding GC, and debug logging for REGISTER/OPTIONS
- `config.example.toml` documents `external_host` for multi-homed and Docker hosts

### Fixed

- Extension-to-extension calls failing with `bind: address already in use` when `bind_host = 0.0.0.0` and Docker bridge was selected for outbound SIP
- Phones appearing offline / calls not connecting due to NAT contact and outbound routing issues
- RTP/media bind IP now follows `external_host` instead of auto-picked docker0 on wildcard bind

## [v0.11alpha] - 2026-07-02

[Release](https://github.com/sappsys/VoIP_Server/releases/tag/v0.11alpha)

### Added

- Dark themed web control panel with panels, stat cards, and aligned form layouts
- Admin vs view-only (`user`) web roles with server-side enforcement
- Per-row **Edit** actions for extensions, hunt groups, conferences, paging groups, web users, and phonebook entries
- VoIP favicon, logo, and PWA manifest assets under `/web/*`
- `build.sh` for portable CGO-free binaries (current arch + full cross-compile matrix)
- Web tests for PRG redirects, role-based access, edit flows, and static assets

### Changed

- Default web UI listen port from `8080` to `7030`
- Form saves use POST + redirect (PRG) instead of HTMX full-page swaps, fixing duplicate layouts
- Phonebook admin uses the same Edit + bottom form pattern as other artifact pages
- README documents portable builds and updated quick-start paths

### Fixed

- Duplicate page headings when saving settings via the web UI
- View-only users no longer see admin mutation controls

## [v0.10alpha] - 2026-07-02

[Release](https://github.com/sappsys/VoIP_Server/releases/tag/v0.10alpha)

### Added

- Initial tagged alpha release of the Go SIP PBX (sipgo / diago)
- Extension registration with digest auth via `extensions/*.toml`
- Extension-to-extension calls, call waiting, and per-extension simultaneous call limits
- Hunt groups (500–599), conferences with PIN IVR (600–699), and paging (`*80`–`*99`)
- SIP trunks with outbound prefix routing and inbound routing via web UI + SQLite
- Music on hold from looping WAV playlists; blind and attended transfer; call park
- Configurable star codes (redial, call return, transfer, park, DND)
- Remote Yealink/Grandstream phonebook at `/phonebook/directory.xml`
- Web admin UI with live **Status** page (registered extensions, active calls, parked calls)
- Call audit log with filters for internal, inbound trunk, and outbound trunk calls
- Codec negotiation: PCMU, PCMA, G722, G723, G729, G726
- Portable statically linked release binaries for Linux, Windows, and macOS
