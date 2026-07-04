#!/usr/bin/env bash
#
# Deploy voip-server to /opt/voip-server and manage the systemd service.
#
# Usage:
#   sudo ./deploy/deploy.sh install              # build + install + start
#   sudo ./deploy/deploy.sh update              # stop, replace binary, start
#   sudo ./deploy/deploy.sh update --no-build   # update from bin/voip-server-*
#   sudo ./deploy/deploy.sh start
#   sudo ./deploy/deploy.sh stop
#   sudo ./deploy/deploy.sh restart
#   sudo ./deploy/deploy.sh status
#
# Install options:
#   --install-dir PATH   Target directory (default: /opt/voip-server)
#   --binary PATH        Use this binary instead of building
#   --no-build           Skip build; require bin/voip-server-<os>-<arch> or --binary
#   --no-start           Install only; do not enable/start the service
#
set -euo pipefail

INSTALL_DIR="/opt/voip-server"
SERVICE_NAME="voip-server"
RUN_USER="voip-server"
RUN_GROUP="voip-server"
NO_BUILD=0
NO_START=0
BINARY=""

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

usage() {
	cat <<'EOF'
Deploy voip-server to /opt/voip-server and manage the systemd service.

Usage:
  sudo ./deploy/deploy.sh <command> [options]

Commands:
  install    Install or upgrade files and systemd unit (default)
  update     Stop service, replace binary only, start service
  start      Enable and start the service
  stop       Stop the service
  restart    Restart the service
  status     Show service status

Install / update options:
  --install-dir PATH   Target directory (default: /opt/voip-server)
  --binary PATH        Use this binary instead of building
  --no-build           Use existing bin/voip-server-<os>-<arch>
  --no-start           (install only) Do not enable/start after install

Examples:
  sudo ./deploy/deploy.sh install
  sudo ./deploy/deploy.sh update
  sudo ./deploy/deploy.sh update --no-build
  sudo ./deploy/deploy.sh restart
EOF
}

log() { printf '>> %s\n' "$*"; }
die() { printf '!! %s\n' "$*" >&2; exit 1; }

require_root() {
	[ "$(id -u)" -eq 0 ] || die "run as root: sudo $0 $*"
}

require_systemd() {
	command -v systemctl >/dev/null 2>&1 || die "systemctl not found; is this a systemd host?"
}

parse_binary_options() {
	while [ $# -gt 0 ]; do
		case "$1" in
		--install-dir)
			shift
			INSTALL_DIR="${1:?--install-dir requires a path}"
			;;
		--binary)
			shift
			BINARY="${1:?--binary requires a path}"
			;;
		--no-build) NO_BUILD=1 ;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			die "unknown option: $1 (try --help)"
			;;
		esac
		shift
	done
}

parse_install_options() {
	while [ $# -gt 0 ]; do
		case "$1" in
		--install-dir)
			shift
			INSTALL_DIR="${1:?--install-dir requires a path}"
			;;
		--binary)
			shift
			BINARY="${1:?--binary requires a path}"
			;;
		--no-build) NO_BUILD=1 ;;
		--no-start) NO_START=1 ;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			die "unknown install option: $1 (try --help)"
			;;
		esac
		shift
	done
}

host_platform() {
	HOST_OS="$(go env GOOS 2>/dev/null || uname -s | tr '[:upper:]' '[:lower:]')"
	HOST_ARCH="$(go env GOARCH 2>/dev/null || uname -m)"
	case "$HOST_ARCH" in
	x86_64) HOST_ARCH="amd64" ;;
	aarch64 | arm64) HOST_ARCH="arm64" ;;
	i686 | i386) HOST_ARCH="386" ;;
	esac
}

resolve_binary() {
	host_platform
	if [ -n "$BINARY" ]; then
		[ -f "$BINARY" ] || die "binary not found: $BINARY"
		echo "$BINARY"
		return
	fi
	local built="${REPO_ROOT}/bin/voip-server-${HOST_OS}-${HOST_ARCH}"
	if [ "$HOST_OS" = "windows" ]; then
		built="${built}.exe"
	fi
	if [ -f "$built" ]; then
		echo "$built"
		return
	fi
	if [ "$NO_BUILD" -eq 1 ]; then
		die "no binary at $built; run ./build.sh or pass --binary PATH"
	fi
	log "building voip-server for ${HOST_OS}/${HOST_ARCH}"
	( cd "$REPO_ROOT" && "./build.sh" "${HOST_OS}/${HOST_ARCH}" )
	[ -f "$built" ] || die "build failed: missing $built"
	echo "$built"
}

ensure_user() {
	if ! getent group "$RUN_GROUP" >/dev/null 2>&1; then
		log "creating group $RUN_GROUP"
		groupadd --system "$RUN_GROUP"
	fi
	if ! getent passwd "$RUN_USER" >/dev/null 2>&1; then
		log "creating user $RUN_USER"
		useradd --system --gid "$RUN_GROUP" --home-dir "$INSTALL_DIR" \
			--no-create-home --shell /usr/sbin/nologin "$RUN_USER"
	fi
}

install_tree() {
	local fresh=0
	if [ ! -d "$INSTALL_DIR" ]; then
		fresh=1
		log "creating $INSTALL_DIR"
		mkdir -p "$INSTALL_DIR"
	fi

	mkdir -p "$INSTALL_DIR"/{data,extensions,phonebook,assets/moh,assets/sounds}

	log "installing binary"
	install -m 0755 "$(resolve_binary)" "$INSTALL_DIR/voip-server"

	if [ "$fresh" -eq 1 ] || [ ! -f "$INSTALL_DIR/config.toml" ]; then
		log "installing default config.toml"
		install -m 0640 "$REPO_ROOT/config.example.toml" "$INSTALL_DIR/config.toml"
	else
		log "keeping existing config.toml"
	fi

	if [ -d "$REPO_ROOT/assets/sounds" ]; then
		log "syncing assets/sounds"
		if command -v rsync >/dev/null 2>&1; then
			rsync -a --delete "$REPO_ROOT/assets/sounds/" "$INSTALL_DIR/assets/sounds/"
		else
			rm -rf "$INSTALL_DIR/assets/sounds"
			cp -a "$REPO_ROOT/assets/sounds" "$INSTALL_DIR/assets/"
		fi
	fi

	if [ -f "$REPO_ROOT/assets/moh/moh.wav" ]; then
		log "installing assets/moh/moh.wav"
		install -m 0644 "$REPO_ROOT/assets/moh/moh.wav" "$INSTALL_DIR/assets/moh/moh.wav"
	fi

	if [ -f "$REPO_ROOT/extensions/example.toml" ]; then
		install -m 0640 "$REPO_ROOT/extensions/example.toml" "$INSTALL_DIR/extensions/example.toml"
	fi

	if [ "$fresh" -eq 1 ] || [ -z "$(find "$INSTALL_DIR/extensions" -maxdepth 1 -name '*.toml' ! -name 'example.toml' -print -quit)" ]; then
		for f in "$REPO_ROOT"/extensions/*.toml; do
			[ -f "$f" ] || continue
			base="$(basename "$f")"
			[ "$base" = "example.toml" ] && continue
			if [ ! -f "$INSTALL_DIR/extensions/$base" ]; then
				log "installing extensions/$base"
				install -m 0640 "$f" "$INSTALL_DIR/extensions/$base"
			fi
		done
	fi
}

install_systemd() {
	local unit="/etc/systemd/system/${SERVICE_NAME}.service"
	log "installing systemd unit -> $unit"
	sed \
		-e "s|/opt/voip-server|${INSTALL_DIR}|g" \
		-e "s|^User=.*|User=${RUN_USER}|" \
		-e "s|^Group=.*|Group=${RUN_GROUP}|" \
		"$REPO_ROOT/deploy/voip-server.service" >"$unit"
	systemctl daemon-reload
}

fix_permissions() {
	log "setting ownership on $INSTALL_DIR"
	chown -R "${RUN_USER}:${RUN_GROUP}" "$INSTALL_DIR"
	chmod 0750 "$INSTALL_DIR"
	chmod 0750 "$INSTALL_DIR/data" "$INSTALL_DIR/extensions"
	chmod 0640 "$INSTALL_DIR/config.toml" 2>/dev/null || true
}

cmd_update() {
	parse_binary_options "$@"
	command -v go >/dev/null 2>&1 || [ -n "$BINARY" ] || [ "$NO_BUILD" -eq 1 ] || \
		die "go not found; install Go, pass --binary, or use --no-build"

	[ -d "$INSTALL_DIR" ] || die "not installed at $INSTALL_DIR; run: $0 install"
	[ -f "$INSTALL_DIR/voip-server" ] || die "missing $INSTALL_DIR/voip-server; run: $0 install"

	require_systemd
	local src
	src="$(resolve_binary)"

	if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
		log "stopping ${SERVICE_NAME}"
		systemctl stop "$SERVICE_NAME"
	fi

	log "installing binary -> $INSTALL_DIR/voip-server"
	install -m 0755 "$src" "$INSTALL_DIR/voip-server"
	chown "${RUN_USER}:${RUN_GROUP}" "$INSTALL_DIR/voip-server"

	log "starting ${SERVICE_NAME}"
	systemctl start "$SERVICE_NAME"

	VERSION="$("$INSTALL_DIR/voip-server" -version 2>/dev/null || echo unknown)"
	log "updated — ${SERVICE_NAME} ${VERSION}"
	systemctl --no-pager status "$SERVICE_NAME" || true
}

cmd_install() {
	parse_install_options "$@"
	command -v go >/dev/null 2>&1 || [ -n "$BINARY" ] || [ "$NO_BUILD" -eq 1 ] || \
		die "go not found; install Go, pass --binary, or use --no-build"

	ensure_user
	install_tree
	install_systemd
	fix_permissions

	if [ "$NO_START" -eq 1 ]; then
		log "skipping start (--no-start)"
	else
		cmd_start
	fi

	VERSION="$("$INSTALL_DIR/voip-server" -version 2>/dev/null || echo unknown)"
	log "done — ${SERVICE_NAME} ${VERSION} at ${INSTALL_DIR}"
	log "edit ${INSTALL_DIR}/config.toml then: $0 restart"
}

cmd_start() {
	require_systemd
	log "starting ${SERVICE_NAME}"
	systemctl enable "$SERVICE_NAME" 2>/dev/null || true
	systemctl start "$SERVICE_NAME"
	systemctl --no-pager status "$SERVICE_NAME" || true
}

cmd_stop() {
	require_systemd
	log "stopping ${SERVICE_NAME}"
	systemctl stop "$SERVICE_NAME"
	systemctl --no-pager status "$SERVICE_NAME" || true
}

cmd_restart() {
	require_systemd
	log "restarting ${SERVICE_NAME}"
	systemctl restart "$SERVICE_NAME"
	systemctl --no-pager status "$SERVICE_NAME" || true
}

cmd_status() {
	require_systemd
	systemctl --no-pager status "$SERVICE_NAME" || true
}

main() {
	if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ] || [ "${1:-}" = "help" ]; then
		usage
		exit 0
	fi

	local cmd="${1:-install}"
	if [ $# -gt 0 ]; then
		shift
	fi

	case "$cmd" in
	install) require_root; cmd_install "$@" ;;
	update) require_root; cmd_update "$@" ;;
	start) require_root; cmd_start ;;
	stop) require_root; cmd_stop ;;
	restart) require_root; cmd_restart ;;
	status) cmd_status ;;
	-h | --help | help)
		usage
		;;
	*)
		die "unknown command: $cmd (try: install | update | start | stop | restart | status)"
		;;
	esac
}

main "$@"
