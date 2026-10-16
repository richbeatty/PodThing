# Build and validation status

Validated in the cloud Linux workspace on 2026-10-16.

The complete installer is `dist/podthing-0.1.0-flashthing.zip`, with a companion
SHA-256 file. It contains a signed bootloader, GPT disk, real FAT boot/environment
filesystems and populated ext4 roots in both A/B slots. It installs independent
PodThing firmware; Prelude is not required or included.

## Checks passed

- Go tests with the race detector, `go vet`, and three firmware-validator tests.
- iAP framing/checksum/fragmentation, HID descriptors, paged accessory browsing,
  command rejection, digital-audio capabilities and explicit authentication
  failure behavior against scripted transports.
- ARM64 and ARMv7 cross-compilation. ARM64 uses the image's dynamic loader/libc
  for native ALSA; ARMv7 is static.
- Actual ARM64 executable and four hardware-layer tests under QEMU user-mode,
  including native ALSA null capture/output/cancellation and USB ioctl layout.
- Native Wayland protocol/shared-memory tests and an AMD64 run against real
  Weston 14 with an 800×480 headless output.
- Browser touch/actions, wheel/keyboard navigation, simulated playback, preset
  remapping, diagnostic export and demo isolation.
- Pinned kernel 7.0.2, all 25 Car Thing BSP patches and the board device tree;
  mainline Car Thing U-Boot compiled and the FIP signer verified its output.
- Debian archive signatures and package hashes; exact versions and artifact
  checksums retained with the image. No target maintainer scripts ran on host.
- Offline systemd unit verification; ARM64 hardware database and linker-cache
  generation. Overlay permissions are normalized independently of checkout umask.
- ARM64 U-Boot environment tools read/updated retry variables in a copy extracted
  from the real FAT environment image, including its CRC.
- Filesystem checks, GPT verification and ZIP/flash metadata validation.
- Full-system Cortex-A53 boot with 512 MiB RAM: actual kernel and a copy of the
  production rootfs, systemd startup, ARM64 Weston, native Wayland app, browsing,
  updated PNG rendering and independent recovery boot-log download. Only the
  copy enables demo mode, substitutes a headless display backend and masks USB,
  seat and boot-health services that need physical hardware.

The boot test exposed restrictive directory permissions in the first image.
That image was withdrawn; the corrected rootfs booted successfully. The test
also proved recovery logs remain downloadable when the display app fails.

## Physical hardware still required

- Car Thing mask-ROM flashing, signed boot-chain execution, real panel startup,
  touch/wheel/buttons, power delivery and USB host/device role switching.
- Stock 7th-gen iPod classic USB iAP connection, library browsing and playback.
- iPod digital-audio authorization/capture and audible output through the hub DAC.
  MFi authentication can block this; no certificate or authentication chip is
  supplied. Successful emulator or null-device tests cannot establish audio.

The default Debian-based builder completed locally. The optional Yocto builder
parsed and started its task graph, but its GNU/Yocto source downloads remain
blocked by the managed cloud proxy. It is not required for the delivered image.
TLS, signatures and expected checksums remained enabled throughout.

See [installation and hardware test instructions](FIRMWARE.md). The separate
`podthing-0.1.0-test.tar.gz` is an optional app-only developer bundle, not an OS
installer. No hardware test, GitHub Actions run or publication is implied by
its presence or by the firmware ZIP's format checks.
