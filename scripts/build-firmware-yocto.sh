#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
image=ghcr.io/siemens/kas/kas@sha256:3f3d3257e8d4ca3adfcb2e0025e6da24339fb8d68a6999ca36c37381351582d5
command -v docker >/dev/null || { printf '%s\n' 'Docker is required for the pinned Yocto builder.' >&2; exit 1; }
[ -f dist/podthing-linux-arm64 ] || { printf '%s\n' 'Run sh scripts/build.sh first.' >&2; exit 1; }
# An isolated build tree in the current checkout; no additional Git worktree.
mkdir -p build/firmware
mkdir -p sources
# Keep third-party source trees/build fixtures out of the application's ./...
# Go package discovery without modifying any checked-out upstream repository.
for generated in build sources; do
    if [ ! -e "$generated/go.mod" ]; then
        printf 'module podthing-generated-%s\n\ngo 1.24.0\n' "$generated" > "$generated/go.mod"
    fi
done
set --
# Reuse the host's verified CA bundle when building behind an enterprise/cloud
# HTTPS proxy. TLS verification stays enabled for every download.
ca_bundle=${PODTHING_CA_BUNDLE:-/etc/ssl/certs/ca-certificates.crt}
if [ -f "$ca_bundle" ]; then
    set -- "$@" -v "$ca_bundle:/etc/ssl/certs/ca-certificates.crt:ro"
fi
docker run --rm --name podthing-firmware-build \
    --user "$(id -u):$(id -g)" \
    -e HTTP_PROXY -e HTTPS_PROXY -e ALL_PROXY -e NO_PROXY \
    -e KAS_WORK_DIR=/work -e KAS_BUILD_DIR=/work/build/firmware \
    -v "$PWD:/work" -w /work \
    "$@" \
    "$image" build firmware/podthing.yml
deploy=build/firmware/tmp/deploy/images/superbird
# Check the built package manifest so a BSP-only or unrelated image cannot be
# presented as PodThing just because it has valid partitions.
python3 - "$deploy" <<'PY'
import pathlib, sys
directory = pathlib.Path(sys.argv[1])
manifests = list(directory.glob('podthing-image-superbird*.manifest'))
if not manifests:
    raise SystemExit('No PodThing rootfs package manifest was produced')
manifest = max(manifests, key=lambda p: p.stat().st_mtime)
packages = {line.split()[0] for line in manifest.read_text().splitlines() if line.strip()}
if not {'podthing-app', 'weston'} <= packages:
    raise SystemExit('Rootfs does not contain the PodThing app and Weston')
if any(p.startswith(('prelude', 'chromium', 'bridgething')) for p in packages):
    raise SystemExit('Unexpected phone-projection/browser application in PodThing rootfs')
PY
python3 scripts/validate-firmware.py "$deploy/podthing-image-superbird-flashthing.zip"
cp -L "$deploy/podthing-image-superbird-flashthing.zip" dist/podthing-0.1.0-flashthing.zip
sha256sum dist/podthing-0.1.0-flashthing.zip > dist/podthing-0.1.0-flashthing.zip.sha256
printf '%s\n' 'Built dist/podthing-0.1.0-flashthing.zip'
