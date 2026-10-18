#!/bin/sh
set -eu
cd /work
root=/work/build/direct/rootfs
[ -f "$root/.podthing-assembled" ]
# Verify units and actual target executables without running ARM maintainer
# scripts or assuming a successful cross-compile establishes runtime linkage.
systemd-analyze --root="$root" --man=no verify \
 /etc/systemd/system/podthing.service /etc/systemd/system/podthing-weston.service \
 /etc/systemd/system/podthing-usb.service /etc/systemd/system/podthing-boot-healthy.service \
 /etc/systemd/system/podthing-boot-healthy.timer /etc/systemd/system/podthing-recovery.service
qemu-aarch64 -L "$root" "$root/usr/bin/podthing" --version
# Validate the real decoder in the assembled ARM64 runtime, not just the
# Go GUI executable. A missing ELF loader/shared library can make ffmpeg exit
# 127 on device while every previous firmware and UI smoke test passes.
# Keep its exact dynamic-linker stderr in the CI log on failure.
[ -x "$root/usr/bin/ffmpeg" ] || { echo "ARM64 FFmpeg executable missing from rootfs" >&2; exit 1; }
echo "Validating ARM64 FFmpeg runtime and ALSA output support"
qemu-aarch64 -L "$root" "$root/usr/bin/ffmpeg" -hide_banner -version > /work/build/direct/ffmpeg-runtime.txt
qemu-aarch64 -L "$root" "$root/usr/bin/ffmpeg" -hide_banner -loglevel error \
  -f lavfi -i anullsrc=channel_layout=stereo:sample_rate=44100 \
  -t 0.05 -ar 44100 -ac 2 -f null - 
qemu-aarch64 -L "$root" "$root/usr/bin/ffmpeg" -hide_banner -devices > /work/build/direct/ffmpeg-devices.txt
grep -E '^[[:space:]]*[A-Z.][E][[:space:]]+alsa[[:space:]]' /work/build/direct/ffmpeg-devices.txt \
  || { echo "ARM64 FFmpeg lacks ALSA playback device support" >&2; exit 1; }
echo "PASS: ARM64 FFmpeg loads, decodes, and advertises ALSA output"

qemu-aarch64 -L "$root" /work/build/direct/arm64-tests/hardware.test -test.v > /work/build/direct/arm64-smoke.log
cat /work/build/direct/arm64-smoke.log
! grep -q 'SKIP\|FAIL' /work/build/direct/arm64-smoke.log
# Generate the input-device hardware database using its real ARM64 utility.
qemu-aarch64 -L "$root" "$root/usr/bin/systemd-hwdb" --root="$root" update
# Generate a cache for the target architecture; no host ELF cache is copied.
qemu-aarch64 -L "$root" "$root/usr/sbin/ldconfig" -r "$root"
