#!/usr/bin/env bash
# Tier 1 — core unit tests: router, config, store, call, media.
#
# Usage:
#   ./utils/test-tier1.sh
#   ./utils/test-tier1.sh -v
#   ./utils/test-tier1.sh -count=1 -cover
#
set -euo pipefail
# shellcheck source=test-common.sh
source "$(dirname "$0")/test-common.sh"

echo ">> Tier 1: router, config, store, call, media"
run_go_test "$@" \
  ./internal/router/... \
  ./internal/config/... \
  ./internal/store/... \
  ./internal/call/... \
  ./internal/media/...
