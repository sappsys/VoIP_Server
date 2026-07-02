#!/usr/bin/env bash
# Tier 3 — hunt, conference, registrar, trunk, web (incl. auth), version.
#
# Usage:
#   ./utils/test-tier3.sh
#   ./utils/test-tier3.sh -v
#
set -euo pipefail
# shellcheck source=test-common.sh
source "$(dirname "$0")/test-common.sh"

echo ">> Tier 3: hunt, conference, registrar, trunk, web, version"
run_go_test "$@" \
  ./internal/hunt/... \
  ./internal/conference/... \
  ./internal/registrar/... \
  ./internal/trunk/... \
  ./internal/web/... \
  ./internal/version/...
