# PodThing

A Car Thing interface for controlling an iPod classic: touchscreen browsing,
wheel navigation, four remappable presets, and USB audio diagnostics.

**PodThing is independent replacement firmware, with an experimental iPod
controller.** It uses the Car Thing's independent Linux board-support package;
it does not fork, install or require Prelude. Stock iPod USB accessory
authentication may block digital audio; the app reports that explicitly.

The iPod performs playback. PodThing never mounts or modifies its music library.
The intended connection is Car Thing → powered OTG hub → normal 30-pin USB data
cable → iPod, with the hub's USB DAC providing 3.5 mm AUX output.

## Build the replacement firmware

```sh
sh scripts/build.sh
sh scripts/build-firmware.sh
```

See [firmware build, flashing and on-screen testing](docs/FIRMWARE.md).
The image is intended to boot directly into PodThing and install through a
Terbium flash ZIP; no SSH/ADB setup is needed. A successfully built firmware
installer is named `dist/podthing-0.1.0-flashthing.zip`. App binaries alone are
not a flashable OS, and physical-device compatibility remains unverified.

The optional [existing-OS test guide](docs/DEVICE-TEST.md) describes running just
the app temporarily on a compatible Prelude installation. This is a developer
convenience, not a requirement or the primary installation path.

## Develop on Linux

Go 1.24+ is required for the app; the complete firmware builder needs Go 1.27.2+.
No Node toolchain is needed. Docker supplies the firmware cross-compiler.

```sh
go run ./cmd/podthing --demo --headless
# Open the device console at http://127.0.0.1:8787
go test -race ./...
sh scripts/build.sh
```

The bundle includes Linux ARM64 and ARMv7 executables built with CGo disabled.
ARM64 uses the OS's dynamic loader for native ALSA calls; ARMv7 is static.
Native display uses Wayland shared memory, and physical input uses evdev.
USB iAP uses Linux usbfs; ARM64/AMD64 audio calls the OS's ALSA library directly.
ARMv7 falls back to `alsaloop` or `gst-launch-1.0` when available.

Demo mode is explicitly labeled and never accesses an iPod or plays audio.
For read-only hardware discovery: `podthing --diagnose`.

See [references and dependency notices](THIRD_PARTY.md).
