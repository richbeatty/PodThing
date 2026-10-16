#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
if [ "$#" != 1 ]; then printf '%s\n' 'Usage: sh scripts/deploy.sh root@CAR_THING_IP' >&2; exit 2; fi
case "$1" in -*|*[!a-zA-Z0-9_.@:-]*) printf '%s\n' 'Invalid SSH destination.' >&2; exit 2 ;; esac
archive=dist/podthing-0.1.0-test.tar.gz
[ -f "$archive" ] || { printf '%s\n' 'Build the test archive first: sh scripts/build.sh' >&2; exit 1; }
# Versioned installation avoids overwriting an earlier build or its state.
stamp=$(date -u +%Y%m%dT%H%M%SZ)
remote_dir="/var/lib/prelude/podthing-tests/$stamp"
ssh "$1" "mkdir -p '$remote_dir'"
scp "$archive" "$1:$remote_dir/podthing.tar.gz"
ssh "$1" "cd '$remote_dir' && tar -xzf podthing.tar.gz && sha256sum -c SHA256SUMS && chmod +x podthing-linux-arm64 podthing-linux-arm run-on-prelude.sh"
printf '\n%s\n' "Installed in $remote_dir" "Start: ssh $1 'systemd-run --unit=podthing-test --collect $remote_dir/run-on-prelude.sh'" "Stop: ssh $1 'systemctl stop podthing-test'"
