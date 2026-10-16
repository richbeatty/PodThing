## Experimental read-only iPod music player

This branch adds a stock-firmware USB storage pathway, **not** an MFi
authentication bypass. It can mount only the detected iPod's disk
and partition **read-only** (enforced with BLKROSET), parse legacy
iTunesDB track and playlist information, browse through the existing
Car Thing UI, and decode music files via ffmpeg to a selectable USB
ALSA playback device. It has play/pause, previous/next, seek,
shuffle and repeat controls, and a guarded **Eject iPod safely** action.
The device's music files, play counts and iTunesDB are never modified.

No extra accessories are shipped. The previous hardware report
listed **only internal Superbird capture devices and no USB DAC
playback output**. Without an existing usable output (or future
Bluetooth A2DP support), local music playback cannot reach the
car speakers. Tests validate code, not the actual iPod's partition,
filesystem and sound output. ArtworkDB is not yet decoded.

Use *Connection → Open iPod music (read-only)* to test. Eject before
unplugging whenever possible. If no real USB playback PCM is
detected, the player explains why it can't start audio. The original
iAP connection mode remains accessible for comparison.

## 0.1.0-test.6 hardware compatibility probes

- New fresh Simple Remote test without prior Extended Interface handshake;
  each Play/Pause, Next and Previous press lasts 150 ms and remains
  unacknowledged by iAP.
- Separate modern IdentifyDeviceLingoes test with no MFi
  credential claims or digital audio support; records ACK, device
  queries, and explicit authentication refusals.
- Persist last host-mode diagnostics to `state/last-session.json`
  before switching USB roles. The browser now distinguishes
  **Download saved session** from **Live snapshot**.
- Adds iAP packet-sequence, timing and persisted-HTTP regression tests.

**Experimental:** Physical control and USB audio on Apple's stock
iPod Classic are not yet verified. This release does not unlock MFi.

Independent replacement firmware for Spotify Car Thing, packaged for Terbium.
It replaces Prelude; SSH and ADB are not required for installation.

Download the revision-specific installer ending in `-flashthing.zip` and its matching `.sha256` file from the assets. Confirm the version on the Settings screen after flashing.
The source archive contains the application, firmware configuration, build
scripts and instructions.

This hardware-test revision adds a modern native interface, nested settings,
automatic host mode and black boot splash. Art-based color extraction is built,
but retrieving artwork from a stock iPod remains unimplemented until the iPod
control protocol is confirmed working on physical hardware.

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
