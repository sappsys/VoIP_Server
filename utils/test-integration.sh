#!/usr/bin/env bash
# Tier 4 — live SIP integration tests (requires localhost UDP).
#
# Usage:
#   ./utils/test-integration.sh
#   ./utils/test-integration.sh -v
#
set -euo pipefail
# shellcheck source=test-common.sh
source "$(dirname "$0")/test-common.sh"

echo ">> Tier 4: SIP integration"
run_go_test -tags=integration "$@" ./test/integration/...
