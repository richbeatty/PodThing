# PodThing replacement firmware

`podthing-0.1.0-flashthing.zip` installs an independent Linux appliance that
boots into PodThing. It replaces Prelude and does not require an existing OS,
SSH, ADB, Android or a phone-projection dongle. This is an experimental hardware
test image: successful software checks do not establish that your iPod's
accessory authentication, digital audio or particular hub will work.

## Install with Terbium

Flashing replaces the Car Thing's bootloader and OS partitions. Save anything
you need from the current installation and keep a known-working recovery ZIP.
Disconnect the iPod and hub while flashing. The installer targets the Car Thing,
not the iPod.

1. Verify the ZIP against `podthing-0.1.0-flashthing.zip.sha256`.
2. Connect Car Thing directly to your computer with a USB data cable. Enter
   mask-ROM mode by holding preset buttons **1 + 4** while connecting USB, using
   the Car Thing recovery/reset procedure if necessary. The expected mask-ROM
   USB identifier is `1b8e:c003`.
3. Open a current [Terbium](https://terbium.app) in a browser with WebUSB support.
   Choose its local/custom flash ZIP option and select the ZIP without unpacking
   it. This package uses the BSP's metadataVersion 2 format: signed bootloader
   writes to both eMMC boot partitions, then a GPT disk image to the user area.
4. Let flashing finish before disconnecting. Reconnect power and the powered
   OTG hub. The firmware starts PodThing automatically in hardware mode.

If startup fails, re-enter mask-ROM and restore the known-working installer.
This recovery procedure has not been exercised on a physical device here.

## First iPod test

Use Car Thing → powered USB-C OTG hub → ordinary USB-to-30-pin data cable →
iPod classic, and connect the hub's 3.5 mm output to AUX.

The firmware now boots with its USB port in **host mode**. Attach a powered
OTG hub and the iPod, then choose **Connect iPod**. Use Settings > Connection
> Computer recovery to switch temporarily to USB device/network mode.
It will not mount or rewrite the iPod's music filesystem.

- The four presets open Artists, Albums, Songs and Playlists. Turn the wheel to
  scroll and press it to select, or navigate by touch. The far-right top button
  opens Settings; presets can be remapped there.
- In **Settings → USB & audio diagnostics**, request digital audio. If the iPod
  requests accessory authentication, PodThing reports it. No MFi certificate,
  authentication chip emulation or claim that software can bypass it is included.
- Select the hub DAC, lower AUX volume and try the three-second test tone.
- If an iPod ALSA capture PCM appears, select it and start the audio route.
  Verify real music, pause and skipping. Browsing alone does not prove audio.

If a step fails, save a report before disconnecting. The supplied adapter's
connectors match the desired arrangement, but its advertised PD wattage does
not establish Car Thing charge-through OTG or USB DAC compatibility.

## Get a diagnostic report without SSH

1. Choose **Save diagnostic report**, then **USB recovery console**. This stops
   the accessory/audio session and switches the port back to device mode.
2. Disconnect the hub and connect Car Thing directly to your computer.
3. With USB-NCM support, your computer receives a local USB-network address.
   Open `http://10.42.1.2:8787` and choose **Download report**.

If the screen stays blank or the main console cannot start, reconnect directly
by USB and open `http://10.42.1.2:8788`. The independent recovery page can download
the boot journal even when the display app has failed.

This is a local maintenance console without a login. No remote account is
required. Reports include USB descriptors, ALSA PCMs, command IDs/counts,
metadata and errors; they omit iPod serial numbers, raw authentication data,
credentials and music files.

## Build from source

Use Linux, Docker, Git, Python 3, Go **1.27.2 or newer**, 16 GB RAM and at least
20 GB free space. Four compile jobs are used by default. Use the existing
checkout; an extra Git worktree is unnecessary.

```sh
sh scripts/build.sh
sh scripts/build-firmware.sh
python3 scripts/test-firmware-boot.py
```

The default builder compiles the pinned Car Thing kernel and mainline U-Boot,
then assembles a fixed-purpose ARM64 runtime from Debian 13 packages. APT checks
Debian's signed InRelease and every package hash. Runtime users, links and
systemd units are configured offline; target package maintainer scripts are not
executed on the host. The image is an appliance, not a supported live-upgrade
Debian installation. Rebuild and reflash for updates.

The image contains the native PodThing app, Weston with software rendering,
ALSA, USB HID/audio drivers, recovery networking and a boot-health service.
Both A/B slots contain real boot and root filesystems. The boot-health service
resets the active slot's retry counter only after the compositor and app are
running and the app answers a local request.

Source commits are pinned in `firmware/direct/sources.json`. The kernel uses
all 25 patches and the device tree from the independent
[yocto-superbird BSP](https://github.com/JoeyEamigh/yocto-superbird), with PodThing
USB/runtime config fragments. The signed boot chain uses the BSP's pinned
[superbird-uboot](https://github.com/ThingLabsOSS/superbird-uboot) and
[superbird-fip-tools](https://github.com/ThingLabsOSS/superbird-fip-tools).
No Prelude application code is included.

The ZIP includes `BUILD-INFO.json`, exact Debian package versions/checksums,
installation instructions and dependency notices. Package copyright notices,
Go dependency licenses and the kernel config also reside in the rootfs.
Existing HTTPS proxy settings and a trusted host CA bundle are supported;
`PODTHING_CA_BUNDLE` can select another trusted bundle. TLS, package signatures
and source verification remain enabled.

`scripts/test-firmware-boot.py` boots the actual kernel and a copy of the rootfs
on an emulated Cortex-A53 with 512 MiB RAM. Only that copy enables demo mode, uses Weston's headless 800×480 display
backend and masks the physical USB/seat services. The app still presents through
native Wayland. It checks ext4 boot, systemd startup, ARM64 Weston/app rendering,
a browse action and the independent boot-log download. The
production image starts the native hardware interface. QEMU cannot validate
Car Thing panel initialization, physical inputs, USB roles or iPod audio.

An alternative Yocto image definition remains under `firmware/meta-podthing`;
`sh scripts/build-firmware-yocto.sh` uses it. Its GNU/Yocto upstream downloads
are blocked by this cloud's egress proxy. The default Debian-based build uses
accessible authenticated sources and does not depend on those downloads.

## iPod connection hardware-test fixes

The earlier test image waited for an ACK to legacy Identify. This command
has no acknowledgment, so an otherwise valid iPod could time out before
EnterRemoteUIMode. Identify is now send-only, and connect retries no longer
close an existing session.

After a failed USB accessory handshake, PodThing leaves a successfully selected
HID accessory configuration active rather than immediately restoring mass
storage. Deliberate Disconnect restores the original USB configuration.
If selecting the accessory configuration fails, it reconnects the original
drivers. The USB & Audio diagnostics view includes the last error even if
USB recovery networking is unavailable.

This fixes a known handshake bug but does not guarantee that a particular
iPod Classic exposes the required USB HID accessory interface or that
USB digital audio works. Authentication remains unsupported.

## Revision, boot logo and automatic USB host

Each revision has a uniquely named Terbium ZIP and shows its version inside
Settings. The boot slots contain a full-screen black `logo.bmp` to override
the upstream ThingLabs splash without changing the signed U-Boot binary.
Credits remain visible under Settings > About.

USB boots in host mode for the iPod and powered hub. Switch to USB recovery
mode from Settings > Connection to connect a computer for diagnostic access.
Hardware recovery using preset buttons 1 + 4 remains available.

## USB/iAP1 session revision

USB connection stages are shown directly on the Car Thing's bottom status
line and in the locally stored report. An iPod can enumerate as ordinary USB
storage before PodThing attempts the separate HID accessory configuration.
The updated code selects an advertised accessory configuration with interrupt
HID and preferentially USB audio support; it does not invent configuration
values, repeatedly reset the bus, or mount music storage.

Once HID is open, the accessory sends legacy Identify and checks whether the
iPod has *already* entered Extended Interface mode. Only if needed does it
send EnterRemoteUIMode. The normal pending-ACK and final ACK sequence is
handled without treating the pending response as success. A missing iPod-name
response does not reset an otherwise valid remote session. Authentication is
still not emulated. A successful USB connection alone does not guarantee
digital audio support.

If the status stops at USB enumeration, HID claim, remote UI mode, or database
browsing, report that exact stage/error. The firmware revision is visible in
Settings > About, which helps exclude a cached/older Terbium ZIP.

## 0.1.0-test.5 authentication compatibility probe

When the stock iPod requests an accessory certificate (iAP general
`00/0014`), a signature (general `00/0017`), or rejects a command
with an unauthenticated ACK (status `07`), PodThing records the exact
request and which connection stage was active. The report and Connection
settings expose these facts without saving the cryptographic challenge.

Instead of immediately closing the USB HID session, an **explicit**
Connection > Try Simple Remote action sends legacy Identify for lingo
`02`, leaving Extended Interface negotiation. The user can then test
Play/Pause, Next and Previous using Simple Remote lingo `02`,
command `00` (button press followed by release). These commands have
**no acknowledgments**; a successful send does **not** prove the iPod
accepted the buttons. Watch the iPod's own screen and playback to
determine whether the test worked.

The probe cannot browse the database, retrieve artwork or enable USB
digital audio. It does not forge a certificate, invent a private key,
emulate an MFi chip or claim successful authentication. Disconnect
before attempting a fresh Extended Interface session. If only
authenticated mode works on stock firmware, a compatible authorized
MFi authentication provider or a different audio/control architecture
will be necessary.

## 0.1.0-test.6: saved diagnostics and separate USB compatibility probes

Use **Connection → Simple Remote first** before attempting legacy
Extended Interface. It sends legacy Identify with lingo 0x02 as the
first iAP command, then holds each Play/Next/Previous key for 150 ms
before releasing it. These commands have no acknowledgment: observe
playback on the iPod itself.

**Connection → Modern accessory probe** separately sends
IdentifyDeviceLingoes (General, Simple, Display, Extended), with
zero authentication options and device ID, and no Digital Audio
lingo. It checks the identification ACK, iPod name, and remote UI
state without entering remote mode. This borrows the approach used
by the accessory-side Linux host in Senyeah/pi-iap-bt-bridge, but
does not present a certificate or try to bypass MFi authentication.
It may be rejected by stock firmware.

The older **Connect iPod (legacy)** flow remains available for
comparison. **Disconnect and physically replug** the iPod between
test modes. Never interpret successful USB writes as verified control.

After each host-mode test PodThing automatically saves
`state/last-session.json`. Entering **Computer recovery** saves the
host-mode report **before** resetting the USB role and the iAP
session. In the recovery browser console, choose **Download saved
session** (not **Live snapshot**). The saved report survives an app
restart. If no session was saved, the endpoint reports a clear 404
instead of silently substituting a recovery-mode report.

## Experimental read-only USB storage player (after test.6)

This is a *different* playback architecture, not MFi authentication.
With a stock iPod Classic in USB disk mode, **Connection → Open iPod
music (read-only)** finds a USB storage partition associated with that
specific iPod, applies Linux BLKROSET to both disk and partition, and
mounts only `vfat` or `hfsplus` with `MS_RDONLY`, `MS_NOEXEC`,
`MS_NOSUID` and `MS_NODEV`. It never writes the iTunesDB, music files,
filesystem journal, play counts or rating data. It never repairs a
filesystem or retries a read-write mount.

PodThing reads `iPod_Control/iTunes/iTunesDB` into a bounded parser,
then offers Artists, Albums, Songs and Playlists. Music filenames are
resolved only beneath the verified mount root. Playback uses the
ffmpeg decoder to play via a **real ALSA USB DAC output** selected in
Audio settings. No audio destination is invented. If the hub exposes no
USB playback PCM, browsing is still possible but listening is not;
Car Thing originally relied on a phone for its car audio, so this
hardware check matters. This build doesn't implement Bluetooth A2DP
transmission or USB audio gadget output to the car.

**Eject iPod safely** stops any decoder process (including paused
processes), waits for it to release files, saves the diagnostic report,
and unmounts normally. Only an actually successful unmount displays
**Safe to disconnect**. If the mount is busy or any step fails,
PodThing keeps it mounted and shows the error. The iPod may still
display its own USB/disk-mode screen until the cable is disconnected.
The kernel read-only block flag also prevents host writes if power is
lost or the iPod is unplugged during a read.

Limitations: iTunesDB variants and case-sensitive path differences
need physical Classic testing. Artwork is extracted from audio-file
embedded pictures when present; iPod-specific ArtworkDB / .ithmb
images may not appear. Protected DRM tracks cannot be played via
ffmpeg. The 160 MiB database memory cap prevents an unbounded
read from overwhelming the device; exceptionally large databases
need a streaming reader. A safe copy of the music library is
recommended before trying unfamiliar firmware or hub combinations.

## 0.1.0-test.8: modern library and silent simulation

**Playback defaults to Simulated** on existing and new settings until you
select the physical **USB DAC** mode under Settings → Audio. Simulated
playback updates progress, pause/resume, seek, next/previous, shuffle, repeat,
queue and Recently Played. It does **not** read/decode audio tracks or send
sound to the car. The USB DAC path remains available, and errors honestly
when no usable audio output is attached.

With an iPod mounted read-only, PodThing opens **Home**:
- Recently Played track cards, saved locally in
  `/var/lib/podthing/recents.json` rather than on the iPod; an initial
  empty history shows album suggestions.
- Touch-friendly Home/Library/Playing destinations.
- Artists, Albums, Songs and Playlists tabs with sorting
  (A–Z, Z–A, Artist, Album); album art card grid or list.
- Tap/scroll albums to browse songs. Album selection preserves both
  artist and album identity.
- A swipe-down Quick Actions tray hides four configurable presets until
  needed. Swipe up (or tap outside) to dismiss. Existing physical
  preset buttons continue working. A small chevron opens it without
  swiping; the browser console supports the same gesture.
- Modern Now Playing with position seeking and adaptive accent when real
  album artwork is available.

Artwork: PodThing asynchronously extracts embedded image data from
tracks, with short FFmpeg timeouts and a bounded thumbnail cache.
A **generic music placeholder**, not invented album art, appears when
the iPod stores its covers only in proprietary ArtworkDB/.ithmb files.
Those formats remain to be implemented.

Safety: disk and partition remain kernel-locked read-only and the
filesystem mounted read-only. Artwork workers finish before an eject;
the player displays safe-to-unplug only after a successful unmount.
No tracks, artwork, play counts, music files or metadata are modified
on the iPod.

This release is experimental until tested on physical Car Thing and
iPod Classic hardware. QEMU/UI tests cannot establish real USB or DAC
compatibility.


## 0.1.0-test.9: calmer player, native Classic artwork and safer eject

After feedback from a physical Car Thing showing simulated playback but no cover
art, the interface has been redesigned with a **neutral charcoal background**,
soft off-white text, restrained accent, a single slim header and minimal
transport controls. Now Playing no longer repeats the controls in both a large
panel and a footer. Transport controls avoid the portion of the display covered
by the Car Thing's physical dial. The four presets remain behind the swipe-down
tray, and library, recents, sorting, playlists and simulation are retained.

**Real iPod artwork:** iPod Classic commonly stores album covers in
`iPod_Control/Artwork/ArtworkDB` with images in `.ithmb` files, not as
embedded JPEGs. PodThing now includes libgpod and an isolated Python/ctypes
worker that reads the authentic ArtworkDB **read-only**, extracts PNGs into
the Car Thing's `/tmp`, and passes them to the bounded cover cache.
Embedded FFmpeg extraction remains a fallback. When an album has no cover,
the dark generic music placeholder remains. Not every ArtworkDB format
can be guaranteed without a physical iPod test, and problems are logged.

**Safe ejection:** PodThing can reattach a stale application mount record after
a process restart only if Linux's mount table proves the exact
`/run/podthing-ipod` filesystem is read-only and its backing block
partition belongs to the connected iPod. Eject stops audio, waits for
the artwork worker, closes its database, saves diagnostics and unmounts.
A successful operation displays a dedicated **Safe to disconnect** screen.
The Eject action isn't offered in the Quick Actions tray once the music
volume is no longer open. A duplicate Eject after an already verified
successful unmount is harmless.

**What is not claimed:** These tests do not prove the artwork codec can decode
the user's exact 7G artwork formats, or that the Car Thing hardware is
physically tested. They *do* test the app/UI, safe eject state handling,
and untrusted mount table validation. No iPod writes or repairs are added.
