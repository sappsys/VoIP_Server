# Music on hold (not bundled)

This directory is intentionally empty in the public repository. Hold music is
**not** shipped with VoIP Server — you must add your own royalty-cleared WAV
files after clone or deploy.

## Setup

1. Copy one or more `.wav` files into this directory (or set `moh_dir` in
   `config.toml` to another path).
2. Files play in alphanumeric filename order and loop.
3. Prefer **8 kHz mono** PCM WAV for best phone compatibility (the server can
   resample stereo or other rates, but mono 8 kHz is the most reliable).

## Licensing

Do **not** commit third-party hold music (e.g. commercial tracks or downloads
with unclear redistribution terms) to a public fork. Use music you own, music
explicitly licensed for redistribution, or silence until you add your own files.

The server runs without MOH files; hold and conference features log a warning
and continue without music until tracks are present.
