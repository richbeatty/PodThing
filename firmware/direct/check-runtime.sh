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
qemu-aarch64 -L "$root" /work/build/direct/arm64-tests/hardware.test -test.v > /work/build/direct/arm64-smoke.log
cat /work/build/direct/arm64-smoke.log
! grep -q 'SKIP\|FAIL' /work/build/direct/arm64-smoke.log
# Generate the input-device hardware database using its real ARM64 utility.
qemu-aarch64 -L "$root" "$root/usr/bin/systemd-hwdb" --root="$root" update
# Generate a cache for the target architecture; no host ELF cache is copied.
qemu-aarch64 -L "$root" "$root/usr/sbin/ldconfig" -r "$root"
