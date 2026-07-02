#!/usr/bin/env bash
# Shared helpers for utils/test-*.sh — run from anywhere; repo root is resolved automatically.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

run_go_test() {
  echo ">> go test $*"
  go test "$@"
}
