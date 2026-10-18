#!/bin/sh
# Build a fixed-purpose runtime from authenticated Debian packages. No target
# maintainer scripts run on the host; users, links and units are set up below.
set -eu
umask 022
cd /work
root=/work/build/direct/rootfs
# This path is generated exclusively by this script and contains no user state.
rm -rf "$root"
mkdir -p "$root"
mkdir -p build/direct/manifests
: > build/direct/manifests/packages.tsv
: > build/direct/manifests/packages.sha256
for package in /work/build/direct/apt-arm64/archives/*.deb; do
    dpkg-deb -x "$package" "$root"
    dpkg-deb --show --showformat='${Package}\t${Version}\t${Architecture}\n' "$package" >> build/direct/manifests/packages.tsv
    sha256sum "$package" >> build/direct/manifests/packages.sha256
done
# The merged-/usr package layout supplies /bin, /sbin and /lib symlinks.
# Do not replace these with directories when adding configuration.
cp -a /work/firmware/direct/rootfs/. "$root/"
# Git does not record directory permissions. Normalize our overlay explicitly
# even when checkout/build umask is 0077; never copy that mode onto /usr or /etc.
python3 - "$root" <<'PYMODE'
import pathlib, sys
source = pathlib.Path('/work/firmware/direct/rootfs')
dest = pathlib.Path(sys.argv[1])
for path in [source, *source.rglob('*')]:
    target = dest / path.relative_to(source)
    if path.is_dir():
        target.chmod(0o755)
    elif path.is_file():
        target.chmod(0o755 if path.stat().st_mode & 0o111 else 0o644)
PYMODE
# BLAS/LAPACK share a Debian update-alternatives SONAME. This appliance
# extracts verified .deb payloads but deliberately does NOT run postinst
# scripts, so the standard linker paths are otherwise never created.
# Create only the two known reference-library links from the actual packages.
# The FFmpeg ARM64 runtime test below verifies the dynamic loader resolves them.
for math_lib in blas lapack; do
    provider="$root/usr/lib/aarch64-linux-gnu/$math_lib/lib$math_lib.so.3"
    standard="$root/usr/lib/aarch64-linux-gnu/lib$math_lib.so.3"
    [ -f "$provider" ] || { echo "Required lib$math_lib.so.3 package payload missing: $provider" >&2; exit 1; }
    if [ -L "$standard" ] || [ -e "$standard" ]; then
        [ -f "$standard" ] || { echo "Broken existing lib$math_lib.so.3 alternative" >&2; exit 1; }
    else
        ln -s "$math_lib/lib$math_lib.so.3" "$standard"
    fi
done
install -D -m0755 /work/dist/podthing-linux-arm64 "$root/usr/bin/podthing"
install -d "$root/usr/share/podthing/licenses" "$root/var/lib/podthing" "$root/mnt/uboot-env" "$root/run" "$root/tmp"
cp -a /work/dist/licenses/. "$root/usr/share/podthing/licenses/"
chmod 644 /work/build/direct/manifests/*
cp /work/build/direct/manifests/* "$root/usr/share/podthing/"
rm -f "$root/etc/gshadow" "$root/etc/shadow"
cp "$root/usr/share/base-passwd/passwd.master" "$root/etc/passwd"
cp "$root/usr/share/base-passwd/group.master" "$root/etc/group"
systemd-sysusers --root="$root"
# No password login or SSH service is exposed by this appliance.
awk -F: '{print $1 ":!:1:0:99999:7:::"}' "$root/etc/passwd" > "$root/etc/shadow"
chmod 640 "$root/etc/shadow"
chown 0:42 "$root/etc/shadow"
: > "$root/etc/machine-id"
ln -sfn /run/systemd/resolve/stub-resolv.conf "$root/etc/resolv.conf"
ln -sfn /usr/share/zoneinfo/UTC "$root/etc/localtime"
printf 'UTC\n' > "$root/etc/timezone"
chmod 1777 "$root/tmp"
# dpkg isn't used for live updates. Record source package controls as unpacked,
# accurately reflecting that their maintainer scripts have not been executed.
mkdir -p "$root/var/lib/dpkg/info" "$root/var/lib/dpkg/updates"
: > "$root/var/lib/dpkg/status"
for package in /work/build/direct/apt-arm64/archives/*.deb; do
    dpkg-deb -f "$package" >> "$root/var/lib/dpkg/status"
    printf 'Status: install ok unpacked\n\n' >> "$root/var/lib/dpkg/status"
done
systemctl --root="$root" enable podthing-recovery.service podthing-weston.service podthing.service podthing-usb.service podthing-boot-healthy.timer
ln -sfn /usr/lib/systemd/system/seatd.service "$root/etc/systemd/system/multi-user.target.wants/seatd.service"
systemctl --root="$root" set-default multi-user.target
# No terminal is visible on this display. Keep serial login disabled; recovery
# uses the local USB network console.
for unit in getty@tty1.service serial-getty@ttyAML0.service systemd-networkd.service systemd-resolved.service; do
    systemctl --root="$root" mask "$unit"
done
# Boot defaults otherwise normally created by package configuration scripts.
mkdir -p "$root/var/lib/dbus" "$root/var/log/journal" "$root/var/tmp"
ln -sfn /etc/machine-id "$root/var/lib/dbus/machine-id"
chmod 1777 "$root/var/tmp"
# Documentation copyright files stay with every package. Discard manuals and
# translated help text that this fixed-purpose UI never accesses.
rm -rf "$root/usr/share/man" "$root/usr/share/locale" "$root/usr/share/doc/apt/examples"
touch "$root/.podthing-assembled"
du -sh "$root"
