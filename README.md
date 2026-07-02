# VoIP PBX Server

**Version: v0.11alpha**

Go SIP PBX using [sipgo](https://github.com/emiago/sipgo) and [diago](https://github.com/emiago/diago).

Repository: [sappsys/VoIP_Server](https://github.com/sappsys/VoIP_Server)

## Features

- Extension registration (digest auth) via `extensions/*.toml`
- Extension-to-extension calls with caller display name (`P-Asserted-Identity`)
- Call waiting and per-extension simultaneous call limits
- Hunt groups 500–599 (simultaneous / sequential) — web UI + SQLite
- Conferences 600–699 with PIN IVR and `BridgeMix` — web UI + SQLite
- SIP trunks in `config.toml` (prefix 90–99 outbound); inbound routing via web UI
- Paging `*80`–`*99` — unicast intercom (default) or multicast per group
- Hold with music-on-hold (WAV loop), blind transfer (SIP REFER), attended transfer
- H.264 video passthrough when `video_enabled` on extensions (requires `external_host`)
- Configurable star codes — redial, call return, transfer, park, DND (see below)
- Remote phonebook (Yealink/Grandstream XML) — web UI to manage, served at `/phonebook/directory.xml`
- **Status page** — live extensions, active calls, call audit log (internal + trunk in/out), auto-refreshes every 5s
- HTMX web admin (default login `admin` / `admin`)

See [OPERATIONS.md](OPERATIONS.md) for star codes, Yealink setup, and troubleshooting.

## Quick start

```bash
cp config.example.toml config.toml
mkdir -p assets data extensions
./build.sh
./voip-server -config config.toml
```

`build.sh` produces a portable, statically linked binary for your current
platform in `bin/` and creates a `./voip-server` symlink pointing at it.

| Service | Default |
|---------|---------|
| SIP | UDP `0.0.0.0:5060` |
| Web UI | `http://localhost:7030` |

Place WAV files in the MOH directory (`assets/moh` by default). Tracks play in alphanumeric filename order and loop. Use 8 kHz mono WAV for best phone compatibility.

## Numbering plan

| Range | Use |
|-------|-----|
| 100–499 | Extensions |
| 500–599 | Hunt groups |
| 600–699 | Conferences (PIN) |
| 90–99 + digits | Outbound via trunk prefix |
| *80–*99 | Paging groups (web-managed) |

## Configuration

| File / store | Purpose |
|--------------|---------|
| `config.toml` | Server, limits, web, trunks, **feature star codes** |
| `extensions/<ext>.toml` | Per-phone auth, DND state, video, call waiting |
| `data/pbx.db` (SQLite) | Hunt, conferences, paging, trunk inbound routes, call history, web users |

### Feature star codes (`[features]` in config.toml)

Default codes (all configurable):

| Code | Feature |
|------|---------|
| `*66` | Redial last number you dialed |
| `*69` | Call return — dial last caller |
| `*77` | Attended transfer — hold other party, then dial target ext |
| `*85` | Park call (slot = your extension number) |
| `*86<ext>` | Retrieve parked call (e.g. `*86101`) |
| `*78` | DND on |
| `*79` | DND off |

Example:

```toml
[features]
redial = "*66"
call_return = "*69"
transfer = "*77"
park = "*85"
park_retrieve = "*86"
dnd_activate = "*78"
dnd_deactivate = "*79"
```

Values may be written with or without `*`. Codes must be unique. Reload via web UI or restart after changes.

### Extension fields (`extensions/<ext>.toml`)

| Field | Description |
|-------|-------------|
| `extension` | Extension number (required) |
| `display_name` | Caller ID name |
| `password` | SIP digest password (required) |
| `enabled` | Allow registration and calls (default true) |
| `call_waiting` | Allow one extra call when at `max_simultaneous_calls` |
| `max_simultaneous_calls` | Per-extension active call limit (default 4) |
| `video_enabled` | Offer H.264 video passthrough on calls |
| `voicemail` | Reserved for future use |
| `dnd` | Do not disturb (usually toggled via `*78` / `*79`) |

### Trunks

Outbound: prefix `90`–`99` in `config.toml` `[[trunks]]`. Inbound routing: web UI → Trunks page → SQLite.

Set `external_host` in `config.toml` to your PBX LAN/WAN IP for correct SDP (required for video and some phones).

### Voice codecs

The PBX advertises these codecs in SDP (configurable via `[media] codecs` in `config.toml`):

| Codec | Config ID | Notes |
|-------|-----------|--------|
| G.711 μ-law | `PCMU` | Fully supported |
| G.711 A-law | `PCMA` | Fully supported |
| G.722 | `G722` | Wideband; both legs must agree |
| G.729 / G.729A | `G729` | Both legs must agree |
| G.723 6.3 kbps | `G723-63` | Payload type 4 |
| G.723 5.3 kbps | `G723-53` | Dynamic payload type 104 |
| G.726 32 kbps | `G726-32` | Payload type 2 (RFC 3551) |
| G.726 16/24/40 kbps | `G726-16` / `G726-24` / `G726-40` | Dynamic payload types |

Calls are bridged by relaying RTP between legs. **Both parties must negotiate the same codec** — there is no transcoding. G.711 (PCMU/PCMA) is listed first by default for maximum compatibility. DTMF (`telephone-event`) is added automatically unless already configured.

## Status page

Open **Status** in the web admin (or `/status`). The page shows:

- Which extensions are registered and who they are in a call with
- Active and connecting calls, plus parked slots
- Per-extension last-dialed / last-caller history
- Call audit log with filters for internal, inbound trunk, and outbound trunk calls

Live sections refresh every 5 seconds. The current release version is shown in the nav bar and on the status page.

## Remote phonebook

A shared remote phonebook is served for IP phones (Yealink, Grandstream, and other `IPPhoneDirectory` XML clients):

```
http://<web-ip>:<web-port>/phonebook/directory.xml
e.g. http://192.168.1.100:7030/phonebook/directory.xml
```

- Manage entries (create / edit / delete) in the web UI under **Phonebook**.
- Entries are stored in SQLite; each has a name, number, and optional label.
- Numbers accept extensions (`101`), `user@host` (`302@192.168.2.121`), or SIP URIs (`sip:301@host`).
- The XML endpoint is **public** (no login), matching how phones fetch remote phonebooks.
- Static XML files dropped into `phonebook_dir` (default `phonebook/`) are also served at `/phonebook/<name>.xml`.

On a Yealink phone: **Directory → Remote Phone Book**, set the URL above.

## Project layout

```
cmd/voip-server/     Main entry
internal/config/     TOML loading, feature codes
internal/store/      SQLite persistence
internal/pbx/        SIP server, routing, star-code handlers
internal/call/       B2BUA bridge, registry, park, MOH, DND ring
internal/router/     Dial plan
internal/web/        Admin UI + remote phonebook XML
extensions/          Per-extension config
phonebook/           Optional static remote-phonebook XML files
deploy/              systemd unit
assets/              MOH audio
```

## Development

```bash
go test ./...
go test ./... -cover
go test -tags=integration ./test/integration/...
./build.sh
```

### Building portable binaries

`build.sh` builds CGO-free, statically linked binaries (`-s -w -trimpath`) that
run without a specific glibc/musl and stamps the version via `-ldflags`. All
output goes to `bin/`.

```bash
./build.sh                 # current OS/arch (default) + ./voip-server symlink
./build.sh all             # full cross-compile matrix
./build.sh linux/amd64 windows/amd64 darwin/arm64   # explicit targets
./build.sh --list          # list supported matrix targets
```

Supported matrix: Linux (386/amd64/arm64/arm), Windows (386/amd64/arm64),
macOS (amd64/arm64). Here `386` is 32-bit x86 (i386/i686), `amd64` is x86-64/x64,
and `arm64` is 64-bit ARM. A `./voip-server` symlink in the project root always
points at the binary for the current host.

### Test tiers

| Tier | Scope | Location |
|------|--------|----------|
| 1 | Unit: router, config, store, call, media | `internal/*/*_test.go` |
| 2 | PBX handlers with mock dialer / ring hooks | `internal/pbx/*_test.go` |
| 3 | Hunt, conference, registrar, trunk, web auth | respective `internal/*/_test.go` |
| 4 | Live SIP (REGISTER, OPTIONS) | `test/integration/` (`-tags=integration`) |

## systemd

See [deploy/voip-server.service](deploy/voip-server.service).
