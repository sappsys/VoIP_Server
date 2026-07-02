# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html) with
an `alpha` prerelease suffix.

## [Unreleased]

_No changes yet._

## [v0.1.3alpha] - 2026-07-02

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
