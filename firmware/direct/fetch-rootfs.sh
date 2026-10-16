#!/bin/sh
# Resolve the complete ARM64 runtime with an empty dpkg status database.
# APT verifies Debian's InRelease signature and package hashes.
set -eu
cd /work
cache=/work/build/direct/apt-arm64
mkdir -p "$cache/lists/partial" "$cache/archives/partial"
[ -e "$cache/status" ] || : > "$cache/status"
printf '%s\n' 'deb [arch=arm64 signed-by=/usr/share/keyrings/debian-archive-keyring.gpg] https://deb.debian.org/debian trixie main' > "$cache/sources.list"
cat > "$cache/apt.conf" <<CONF
APT::Architecture "arm64";
APT::Architectures { "arm64"; };
Dir::Etc::parts "-";
Dir::Etc::main "-";
Dir::Etc::sourcelist "$cache/sources.list";
Dir::Etc::sourceparts "-";
Dir::State::status "$cache/status";
Dir::State::lists "$cache/lists";
Dir::Cache::archives "$cache/archives";
Acquire::Languages "none";
Acquire::https::CaInfo "/etc/ssl/certs/ca-certificates.crt";
APT::Update::Error-Mode "any";
CONF
apt-get -c "$cache/apt.conf" update
apt-get -c "$cache/apt.conf" -y --download-only --no-install-recommends install \
 base-files base-passwd bash coreutils dash debianutils diffutils findutils grep gzip \
 hostname init-system-helpers libc-bin login mawk ncurses-base ncurses-bin perl-base \
 sed sysvinit-utils tar util-linux mount dpkg procps systemd-sysv systemd udev kmod \
 seatd weston libasound2t64 alsa-utils dbus iproute2 busybox-static e2fsprogs \
 dosfstools libubootenv-tool dnsmasq-base
