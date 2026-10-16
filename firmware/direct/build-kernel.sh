#!/bin/sh
set -eu
cd /work
bsp=/work/sources/yocto-superbird/meta-superbird/recipes-kernel/linux/linux-superbird
kernel=/work/build/direct/kernel
out=/work/build/direct/kernel-out
mkdir -p "$out"
make -C "$kernel" O="$out" ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- defconfig
"$kernel/scripts/kconfig/merge_config.sh" -m -O "$out" "$out/.config" \
 "$bsp/superbird.cfg" "$bsp/superbird-disable.cfg" "$bsp/superbird-usb-host.cfg" \
 /work/firmware/meta-podthing/recipes-kernel/linux/files/podthing-usb.cfg \
 /work/firmware/direct/kernel.cfg
make -C "$kernel" O="$out" ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- olddefconfig
make -C "$kernel" O="$out" ARCH=arm64 CROSS_COMPILE=aarch64-linux-gnu- -j"${PODTHING_JOBS:-4}" Image amlogic/meson-g12a-superbird.dtb modules
