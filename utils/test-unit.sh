#!/usr/bin/env bash
# All unit/package tests (everything except integration).
# Equivalent to: go test ./...
#
# Usage:
#   ./utils/test-unit.sh
#   ./utils/test-unit.sh -cover
#
set -euo pipefail
# shellcheck source=test-common.sh
source "$(dirname "$0")/test-common.sh"

echo ">> Unit tests (all packages)"
run_go_test "$@" ./...
