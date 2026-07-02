#!/usr/bin/env bash
# Tier 2 — PBX handler tests (mock dialer / ring hooks).
#
# Usage:
#   ./utils/test-tier2.sh
#   ./utils/test-tier2.sh -v
#
set -euo pipefail
# shellcheck source=test-common.sh
source "$(dirname "$0")/test-common.sh"

echo ">> Tier 2: PBX handlers"
run_go_test "$@" ./internal/pbx/...
