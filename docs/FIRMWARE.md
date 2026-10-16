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

The firmware boots with its USB port in **device/recovery mode**. With the hub
powered, press **Enable USB host**, attach the iPod and choose **Connect iPod**.
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
