#!/bin/sh
# Build PodThing's complete independent ARM64 appliance and Terbium installer.
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GO=${GO:-go}
# Use an absolute Go path because the FIP subproject builds in another directory.
GO=$(command -v "$GO")
command -v docker >/dev/null
command -v python3 >/dev/null
[ -f dist/podthing-linux-arm64 ] || { echo 'Run sh scripts/build.sh first.' >&2; exit 1; }
mkdir -p build/direct/builder sources
for generated in build sources; do
    [ -e "$generated/go.mod" ] || printf 'module podthing-generated-%s\n\ngo 1.24.0\n' "$generated" > "$generated/go.mod"
done
python3 scripts/fetch-firmware-sources.py
cp firmware/direct/Dockerfile build/direct/builder/Dockerfile
install -m0644 "${PODTHING_CA_BUNDLE:-/etc/ssl/certs/ca-certificates.crt}" build/direct/builder/ca-bundle.crt
# BuildKit doesn't inherit container-specific /etc/hosts. If the configured
# proxy has a local hostname, resolve and pass that same proxy to the builder.
# This does not select a different proxy or bypass the managed network route.
set --
proxy_ip=
proxy_host=$(python3 - <<'PY'
import os, urllib.parse
print(urllib.parse.urlparse(os.environ.get('HTTPS_PROXY', '')).hostname or '')
PY
)
if [ -n "$proxy_host" ]; then
    proxy_ip=$(getent ahostsv4 "$proxy_host" | head -n1 | awk '{print $1}')
    [ -n "$proxy_ip" ] || { echo "Cannot resolve configured proxy hostname" >&2; exit 1; }
    set -- --add-host "$proxy_host:$proxy_ip"
fi
docker build --network=host "$@" --build-arg HTTP_PROXY --build-arg HTTPS_PROXY --build-arg NO_PROXY \
    -t podthing-direct-builder build/direct/builder
run_user() {
    docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/work" -w /work \
        --entrypoint /bin/sh podthing-direct-builder "$@"
}
run_root() {
    docker run --rm --user 0 --network=host ${proxy_host:+--add-host} ${proxy_host:+"$proxy_host:$proxy_ip"} -e HTTP_PROXY -e HTTPS_PROXY -e NO_PROXY -e http_proxy -e https_proxy -e no_proxy \
        -v "$PWD:/work" -w /work --entrypoint /bin/sh podthing-direct-builder "$@"
}
run_user firmware/direct/build-uboot.sh
run_user firmware/direct/build-kernel.sh
(cd build/direct/fip-tools/fip-tool && CGO_ENABLED=0 "$GO" build -trimpath -o ../../fip-tool .)
build/direct/fip-tool sign -k build/direct/fip-tools/keys/aml-user-key.sig \
    -o build/direct/fip-out build/direct/uboot-out/u-boot.bin
build/direct/fip-tool flash ours --stock-bootloader build/direct/fip-tools/stock.bootloader.bin \
    --signed-fip build/direct/fip-out/u-boot.bin.spotify.encrypt -o build/direct/superbird-boot.bin --dry-run
run_root firmware/direct/fetch-rootfs.sh
run_root firmware/direct/assemble-rootfs.sh
mkdir -p build/direct/arm64-tests
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 "$GO" test -c -o build/direct/arm64-tests/hardware.test ./internal/hardware
run_root firmware/direct/check-runtime.sh
run_root firmware/direct/make-images.sh
python3 scripts/package-firmware.py
python3 scripts/validate-firmware.py dist/podthing-0.1.0-flashthing.zip
(cd dist && sha256sum podthing-0.1.0-flashthing.zip > podthing-0.1.0-flashthing.zip.sha256)
printf '%s\n' 'Built dist/podthing-0.1.0-flashthing.zip (physical hardware test still required)'
