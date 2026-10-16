#!/bin/sh
set -eu
umask 022
cd /work
out=/work/build/direct
root="$out/rootfs"
[ -f "$root/.podthing-assembled" ]
[ -f "$out/kernel-out/arch/arm64/boot/Image" ]
make -C "$out/kernel" O="$out/kernel-out" ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- \
 INSTALL_MOD_PATH="$root" INSTALL_MOD_STRIP=1 modules_install
# Use the release embedded in the compiled modules; make kernelrelease can
# omit the SCM suffix when invoked as a standalone query.
release=$(cat "$out/kernel-out/include/config/kernel.release")
depmod -b "$root" "$release"
python3 /work/firmware/direct/trim-modules.py "$root" "$release"
depmod -b "$root" "$release"
cp "$out/kernel-out/.config" "$root/usr/share/podthing/kernel.config"
install -m0644 /work/THIRD_PARTY.md "$root/usr/share/podthing/THIRD_PARTY.md"
mkdir -p "$out/boot-tree/extlinux"
cp "$out/kernel-out/arch/arm64/boot/Image" "$out/boot-tree/extlinux/Image"
cp "$out/kernel-out/arch/arm64/boot/dts/amlogic/meson-g12a-superbird.dtb" "$out/boot-tree/extlinux/"
cat > "$out/boot-tree/extlinux/extlinux.conf" <<'CONF'
LABEL primary
    LINUX /extlinux/Image
    FDT /extlinux/meson-g12a-superbird.dtb
    APPEND root=PARTLABEL=root_${slot} rw rootwait rootfstype=ext4 init=/sbin/init console=ttyAML0,115200n8 earlycon=meson,0xff803000 no_console_suspend ramoops.pstore_disable=0 max_cpufreq=${charger_cap_khz} superbird.slot=${slot}
CONF
truncate -s 64M "$out/boot.img"
mkfs.vfat -n PODBOOT "$out/boot.img"
mcopy -i "$out/boot.img" -s "$out/boot-tree/extlinux" ::/
cp /work/sources/yocto-superbird/meta-superbird/recipes-bsp/superbird-uenv/files/uboot-env.txt "$out/uboot-env.txt"
mkenvimage -s 0x10000 -o "$out/uboot.env" "$out/uboot-env.txt"
truncate -s 8M "$out/env.img"
mkfs.vfat -n ENV "$out/env.img"
mcopy -i "$out/env.img" "$out/uboot.env" ::/
sh /work/firmware/direct/check-env.sh
# Fail instead of silently overflowing a partition or emitting a partial ZIP.
size_kib=$(du -sk "$root" | cut -f1)
[ "$size_kib" -lt 720896 ] || { echo "Rootfs too large for 768 MiB slots: $size_kib KiB" >&2; exit 1; }
truncate -s 768M "$out/rootfs.img"
mkfs.ext4 -F -L podthing-root -m 0 -E lazy_itable_init=0,lazy_journal_init=0 -d "$root" "$out/rootfs.img"
e2fsck -fn "$out/rootfs.img"
truncate -s 192M "$out/bandaid.img"
mkfs.ext4 -F -L bandaid -m 0 "$out/bandaid.img"
# Both boot slots contain the complete app/runtime, rather than placeholder
# partitions. No existing OS is required on the device.
python3 /work/firmware/direct/make-disk.py
