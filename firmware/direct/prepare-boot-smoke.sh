#!/bin/sh
set -eu
cd /work
out=/work/build/direct
cp --reflink=auto "$out/rootfs.img" "$out/rootfs-smoke.img"
cat > "$out/qemu-smoke.conf" <<'CONF'
[Service]
ExecStart=
ExecStart=/usr/bin/podthing --demo --state-dir=/var/lib/podthing --listen=0.0.0.0:8787
CONF
debugfs -w -R 'mkdir /etc/systemd/system/podthing.service.d' "$out/rootfs-smoke.img"
debugfs -w -R "write $out/qemu-smoke.conf /etc/systemd/system/podthing.service.d/qemu-smoke.conf" "$out/rootfs-smoke.img"
printf '[core]\nrequire-input=false\n' > "$out/qemu-weston.ini"
debugfs -w -R "write $out/qemu-weston.ini /etc/podthing/weston-smoke.ini" "$out/rootfs-smoke.img"
cat > "$out/qemu-weston.conf" <<'CONF'
[Service]
ExecStart=
ExecStart=/usr/bin/weston --backend=headless --width=800 --height=480 --renderer=pixman --shell=kiosk-shell.so --config=/etc/podthing/weston-smoke.ini --socket=wayland-1
CONF
debugfs -w -R 'mkdir /etc/systemd/system/podthing-weston.service.d' "$out/rootfs-smoke.img"
debugfs -w -R "write $out/qemu-weston.conf /etc/systemd/system/podthing-weston.service.d/qemu-smoke.conf" "$out/rootfs-smoke.img"
e2fsck -fn "$out/rootfs-smoke.img"
