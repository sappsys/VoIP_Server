#!/usr/bin/env bash
# Full test suite: unit tests then integration tests.
# Extra flags are passed to both runs (e.g. -v, -count=1).
#
# Usage:
#   ./utils/test-all.sh
#   ./utils/test-all.sh -v -count=1
#
set -euo pipefail
DIR="$(dirname "$0")"

"$DIR/test-unit.sh" "$@"
"$DIR/test-integration.sh" "$@"

echo ">> all tests passed"
