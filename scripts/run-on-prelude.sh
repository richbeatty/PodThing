#!/bin/sh
# Temporary takeover: retain Prelude's OS and restore its app on exit.
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
case "$(uname -m)" in
    aarch64|arm64) binary=./podthing-linux-arm64 ;;
    armv7l|armv8l) binary=./podthing-linux-arm ;;
    *) printf '%s\n' 'Unsupported CPU. Run uname -m and include it in the test report.' >&2; exit 1 ;;
esac
[ "$(id -u)" = 0 ] || { printf '%s\n' 'Run as root to access USB and hardware inputs.' >&2; exit 1; }
[ -x "$binary" ] || { printf '%s\n' "Missing $binary; extract the complete test archive." >&2; exit 1; }
command -v systemctl >/dev/null || { printf '%s\n' 'This launcher requires a systemd-based Prelude image.' >&2; exit 1; }
# Check display compatibility before stopping a working application.
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/weston}"
export WAYLAND_DISPLAY="${WAYLAND_DISPLAY:-wayland-1}"
[ -S "$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY" ] || { printf '%s\n' 'No Weston socket found. This test build targets current mainline/Yocto Prelude; see DEVICE-TEST.md for older versions.' >&2; exit 1; }
restore_prelde=0
child=
cleanup() {
    trap - EXIT INT TERM
    if [ -n "$child" ]; then kill "$child" 2>/dev/null || true; wait "$child" 2>/dev/null || true; fi
    if [ "$restore_prelde" = 1 ]; then systemctl start preluded.service || true; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
if systemctl is-active --quiet preluded.service; then
    restore_prelde=1
    systemctl stop preluded.service
fi
mkdir -p state
"$binary" --state-dir "$PWD/state" "$@" &
child=$!
status=0
wait "$child" || status=$?
child=
exit "$status"
