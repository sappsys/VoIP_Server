# Operations Guide

Day-to-day use of the VoIP PBX: star codes, phones, and troubleshooting.

## Star codes

All codes are configurable in `config.toml` under `[features]`. Defaults below.

### Making calls

| Dial | Action |
|------|--------|
| `*66` | **Redial** — calls the last number you dialed |
| `*69` | **Call return** — calls back whoever last called you |

History is stored in SQLite (`extension_call_history`).

### Transferring calls

| Dial | Action |
|------|--------|
| `*77` | **Attended transfer** — puts the other party on hold (MOH), then dial the target extension to complete |
| `*85` | **Park** — parks the call; slot is your extension number |
| `*86<ext>` | **Retrieve** — connect to a call parked by extension `<ext>` (e.g. `*86101`) |

**Attended transfer flow**

1. You are on a call with party B.
2. Dial `*77` (you may get a brief confirmation; other party hears hold music).
3. Dial extension C (e.g. `103`).
4. B is bridged to C; your legs drop.

**Park flow**

1. On a call, dial `*85`.
2. Other party is on hold with MOH at your extension slot (e.g. 101).
3. Anyone dials `*86101` to pick up that parked call.

Blind transfer is also supported via the phone’s transfer key + SIP REFER.

### Do not disturb

| Dial | Action |
|------|--------|
| `*78` | **DND on** — callers hear ringing; your phone never rings |
| `*79` | **DND off** |

DND state is saved in `extensions/<your-ext>.toml` (`dnd = true/false`).

Hunt groups skip members who are on DND. If every member is on DND, the caller rings with no answer.

### Paging

Dial `*<code>` where code is 80–99 (configured in web UI → Paging). Default mode is unicast intercom to group members.

---

## Yealink T42G setup

1. **Account → Register** — Server host = PBX IP, port 5060, transport UDP.
2. **Username** = extension number, **Password** = from `extensions/<ext>.toml`.
3. Enable **Call Waiting** on the phone if you use multiple lines.
4. Program star codes as speed dials or dial them directly:
   - Speed dial `*66` Redial, `*69` Call return
   - During a call: `*77` transfer, `*85` park
   - Idle: `*78` / `*79` DND, `*86xxx` retrieve

**Recommended**

- Set PBX `external_host` in `config.toml` to the IP phones use to reach the server.
- Open UDP 5060 (SIP) and RTP range on the firewall.
- Place `.wav` files in the MOH directory (`assets/moh` by default); they play in alphanumeric order.

---

## Web admin

URL: `http://<server>:7030` (default `admin` / `admin`).

| Page | Purpose |
|------|---------|
| Dashboard | Active calls, registered extensions |
| Extensions | Create/edit extension TOML files |
| Hunt / Conferences / Paging | SQLite-managed features |
| Trunks | Inbound route targets (credentials in `config.toml`) |
| Reload | Re-read `config.toml` and extensions without restart |

Change `session_secret` and default passwords before production.

---

## Voice codecs

Configured in `config.toml` under `[media] codecs` (preference order). Defaults include:

`PCMU`, `PCMA`, `G722`, `G729`, `G723-63`, `G723-53`, `G726-32`, `G726-16`, `G726-24`, `G726-40`

| Symptom | Check |
|---------|--------|
| No audio on bridged call | Both phones must negotiate the **same** codec; no transcoding is performed |
| Phone only offers G.729 | Ensure callee also supports G.729, or move `PCMU`/`PCMA` higher in `[media] codecs` |
| G.723 modes | Use `G723-63` (6.3 kbps, PT 4) and `G723-53` (5.3 kbps, dynamic PT 104) |

---

## Troubleshooting

| Symptom | Check |
|---------|--------|
| Phone won’t register | Extension exists, `enabled = true`, password matches, UDP 5060 reachable |
| One-way audio | Set `external_host`; verify RTP/firewall |
| No hold music | MOH directory exists with `.wav` files; `moh_dir` in `config.toml` |
| Redial / return empty | At least one prior call to populate SQLite history |
| Park retrieve fails | Slot = parker’s extension; dial `*86` + that number |
| DND still rings phone | Confirm `*78` saved (`dnd = true` in extension TOML); reload extensions |
| Video not working | Both extensions `video_enabled = true`; both offer H.264; `external_host` set |
| Star code does nothing | Verify `[features]` in `config.toml`; use Reload or restart |

### Logs

Run in foreground for SIP traces:

```bash
./voip-server -config config.toml
```

Look for `dnd updated`, `call parked`, `transfer complete`, `park retrieve failed`.

### Database

SQLite path: `data/pbx.db` (default). Call history table: `extension_call_history`.

### Backup and restore

**Web UI (admin):** **Backup** in the nav bar — download a `.tar.gz` archive or upload one to restore.

The archive includes:

- `config.toml` (server, trunks, features, web users seed)
- `extensions/*.toml` (SIP extension credentials and settings)
- SQLite database (`data/pbx.db` by default) — hunt groups, conferences, paging, trunk routes, phonebook DB, web users, call log
- Optional: MOH WAV files and static phonebook XML

Restore replaces those files and reloads the running PBX configuration (no restart required for SIP/extensions; restart recommended after trunk or listen-address changes).

Manual database backup before upgrades:

```bash
cp data/pbx.db data/pbx.db.bak
```

---

## Capacity defaults

| Limit | Default |
|-------|---------|
| Max concurrent calls | 200 (system-wide `[limits] max_calls`) |
| Max extensions | 400 |
| Per-extension calls | 5 (default via `[limits] max_calls_per_extension`; override per extension) |

Adjust in `config.toml` `[limits]` and per-extension `max_simultaneous_calls`.
