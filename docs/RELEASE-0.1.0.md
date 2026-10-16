Independent replacement firmware for Spotify Car Thing, packaged for Terbium.
It replaces Prelude; SSH and ADB are not required for installation.

Download `podthing-0.1.0-flashthing.zip` and its `.sha256` file from the assets.
The source archive contains the application, firmware configuration, build
scripts and instructions.

This build passes ARM64 Cortex-A53 boot under emulation, systemd startup,
native Wayland/Weston rendering, browsing, recovery-log download, ALSA null-device
tests, and GPT/filesystem/ZIP checks.

**Physical Car Thing boot, inputs, USB roles, hub compatibility and stock iPod
classic control/audio remain unverified.** Apple accessory authentication may
block USB audio; no MFi certificate or authentication chip is supplied. This is
a hardware-test build, not a confirmed working iPod audio product.

Flashing overwrites the Car Thing bootloader and OS. Read `INSTALL.md` inside
the firmware ZIP and keep a known-working recovery installer. The source
archive includes `docs/FIRMWARE.md` and `docs/BUILD-STATUS.md`.

This installer is built and tested on GitHub Actions from the tagged source.
Use its companion checksum to verify this artifact; its hash differs from
separate local builds because build timestamps and filesystem IDs differ.
