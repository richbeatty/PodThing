#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
GO=${GO:-go}
PODTHING_VERSION=${PODTHING_VERSION:-0.1.0-test}
LDFLAGS="-s -w -buildid= -X github.com/richbeatty/PodThing/internal/player.Version=$PODTHING_VERSION"
mkdir -p dist
"$GO" mod verify
"$GO" test ./...
for target in arm64 arm; do
    case "$target" in arm) export GOARM=7 ;; esac
    CGO_ENABLED=0 GOOS=linux GOARCH="$target" "$GO" build -trimpath -ldflags="$LDFLAGS" -o "dist/podthing-linux-$target" ./cmd/podthing
done
CGO_ENABLED=0 "$GO" build -trimpath -ldflags="$LDFLAGS" -o dist/podthing-host ./cmd/podthing
cp scripts/run-on-prelude.sh dist/run-on-prelude.sh
cp docs/DEVICE-TEST.md dist/DEVICE-TEST.md
cp THIRD_PARTY.md dist/THIRD_PARTY.md
mkdir -p dist/licenses
image_module=$("$GO" list -m -f '{{.Dir}}' golang.org/x/image)
text_module=$("$GO" list -m -f '{{.Dir}}' golang.org/x/text)
purego_module=$("$GO" list -m -f '{{.Dir}}' github.com/ebitengine/purego)
install -m 0644 "$image_module/LICENSE" dist/licenses/golang-x-image.txt
install -m 0644 "$text_module/LICENSE" dist/licenses/golang-x-text.txt
install -m 0644 "$image_module/font/gofont/ttfs/README" dist/licenses/go-fonts.txt
install -m 0644 "$("$GO" env GOROOT)/LICENSE" dist/licenses/go-runtime.txt
install -m 0644 "$purego_module/LICENSE" dist/licenses/purego.txt
(cd dist && sha256sum podthing-linux-arm64 podthing-linux-arm run-on-prelude.sh DEVICE-TEST.md THIRD_PARTY.md licenses/*.txt > SHA256SUMS)
tar -czf dist/podthing-0.1.0-test.tar.gz -C dist podthing-linux-arm64 podthing-linux-arm run-on-prelude.sh DEVICE-TEST.md THIRD_PARTY.md SHA256SUMS licenses
printf '%s\n' 'Built dist/podthing-0.1.0-test.tar.gz'
