#!/bin/sh
set -eu
out=/work/build/direct
root="$out/rootfs"
mkdir -p "$out/env-test"
mcopy -o -i "$out/env.img" ::/uboot.env "$out/env-test/uboot.env"
printf '%s 0 0x10000\n' "$out/env-test/uboot.env" > "$out/env-test/fw_env.config"
read_tries() {
    qemu-aarch64 -L "$root" "$root/usr/bin/fw_printenv" -c "$out/env-test/fw_env.config" -n slot_a_tries
}
[ "$(read_tries)" = 3 ]
qemu-aarch64 -L "$root" "$root/usr/bin/fw_setenv" -c "$out/env-test/fw_env.config" slot_a_tries 2
[ "$(read_tries)" = 2 ]
echo 'PASS: real FAT boot environment CRC, ARM64 read/write boot retry variables (test copy only)'
