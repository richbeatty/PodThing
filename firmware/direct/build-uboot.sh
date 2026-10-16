#!/bin/sh
set -eu
cd /work
out=/work/build/direct/uboot-out
make -C /work/build/direct/uboot O="$out" CROSS_COMPILE=aarch64-linux-gnu- spotify_carthing_defconfig
make -C /work/build/direct/uboot O="$out" CROSS_COMPILE=aarch64-linux-gnu- -j"${PODTHING_JOBS:-4}"
