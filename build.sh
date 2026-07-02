#!/usr/bin/env bash
#
# build.sh - build portable, static voip-server binaries.
#
# Binaries are CGO-free (CGO_ENABLED=0) so they do not depend on a specific
# glibc/musl and run on any machine of the target OS/arch. They are stripped
# (-s -w) and built with -trimpath for reproducibility.
#
# Usage:
#   ./build.sh                 Build for the current OS/arch (default).
#   ./build.sh all             Build the full cross-compile matrix.
#   ./build.sh linux/amd64 ... Build one or more explicit GOOS/GOARCH targets.
#   ./build.sh --list          List supported matrix targets and exit.
#
# All binaries are written to ./bin. A symlink named "voip-server"
# (or "voip-server.exe" on Windows hosts) is created in the project root
# pointing at the binary for the current host OS/arch.
#
set -euo pipefail

cd "$(dirname "$0")"

PKG="./cmd/voip-server"
BIN_DIR="bin"
APP="voip-server"

# Cross-compile matrix: linux, windows, and macOS on i386/x86/x64/arm.
# Notes:
#   - 386      => i386 / i686 (32-bit x86)
#   - amd64    => x86-64 / x64
#   - arm64    => 64-bit ARM (Apple Silicon, ARM servers)
#   - darwin/386 is unsupported by modern Go, so it is intentionally omitted.
MATRIX=(
  "linux/386"
  "linux/amd64"
  "linux/arm64"
  "linux/arm"
  "windows/386"
  "windows/amd64"
  "windows/arm64"
  "darwin/amd64"
  "darwin/arm64"
)

VERSION="$(go run "$PKG" -version 2>/dev/null || echo dev)"
LDFLAGS="-s -w -X github.com/sappsys/VoIP_Server/internal/version.Version=${VERSION}"

build_one() {
  local goos="$1" goarch="$2"
  local out="${BIN_DIR}/${APP}-${goos}-${goarch}"
  if [ "$goos" = "windows" ]; then
    out="${out}.exe"
  fi

  echo ">> building ${goos}/${goarch} -> ${out}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "$out" "$PKG"
}

# Path (relative to bin/) of the binary matching the given host os/arch.
host_binary_name() {
  local goos="$1" goarch="$2"
  local name="${APP}-${goos}-${goarch}"
  [ "$goos" = "windows" ] && name="${name}.exe"
  echo "$name"
}

link_current() {
  local goos="$1" goarch="$2"
  local target
  target="$(host_binary_name "$goos" "$goarch")"
  local link="${APP}"
  [ "$goos" = "windows" ] && link="${APP}.exe"

  if [ ! -f "${BIN_DIR}/${target}" ]; then
    return 0
  fi

  # Prefer a relative symlink so the project tree stays portable.
  rm -f "$link"
  if ln -s "${BIN_DIR}/${target}" "$link" 2>/dev/null; then
    echo ">> symlinked ./${link} -> ${BIN_DIR}/${target}"
  else
    # Filesystems without symlink support (e.g. some Windows setups): copy.
    cp -f "${BIN_DIR}/${target}" "$link"
    echo ">> copied ${BIN_DIR}/${target} -> ./${link} (symlinks unsupported)"
  fi
}

HOST_OS="$(go env GOOS)"
HOST_ARCH="$(go env GOARCH)"

if [ "${1:-}" = "--list" ]; then
  printf '%s\n' "${MATRIX[@]}"
  exit 0
fi

mkdir -p "$BIN_DIR"

TARGETS=()
if [ "$#" -eq 0 ]; then
  TARGETS=("${HOST_OS}/${HOST_ARCH}")
elif [ "${1:-}" = "all" ]; then
  TARGETS=("${MATRIX[@]}")
else
  TARGETS=("$@")
fi

for t in "${TARGETS[@]}"; do
  goos="${t%%/*}"
  goarch="${t##*/}"
  if [ -z "$goos" ] || [ -z "$goarch" ] || [ "$goos" = "$goarch" ]; then
    echo "!! invalid target '$t' (expected GOOS/GOARCH, e.g. linux/amd64)" >&2
    exit 2
  fi
  build_one "$goos" "$goarch"
done

# Always (re)point the project-root symlink at the current host binary if we
# built it in this run.
for t in "${TARGETS[@]}"; do
  if [ "$t" = "${HOST_OS}/${HOST_ARCH}" ]; then
    link_current "$HOST_OS" "$HOST_ARCH"
    break
  fi
done

echo ">> done (version ${VERSION})"
