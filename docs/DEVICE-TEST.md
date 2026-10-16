# PodThing 0.1.0 test build

This is a standalone Car Thing application for **current mainline/Yocto Prelude
with systemd and Weston**. It temporarily replaces the running Prelude app and
uses its Linux hardware support. It is **not** a Terbium installer or a complete
firmware image. Earlier Buildroot Prelude releases are not validated; the
launcher checks for Weston and refuses to stop Prelude if it is absent.

The USB implementation is experimental and has not been tested against a
physical iPod. A seventh-generation classic running stock firmware is the
initial target. The 512 GB flash modification is not accessed or modified by
PodThing. This app controls the iPod; it does not mount storage or play its files.

## Install from your computer

1. Keep your existing working Prelude setup. Establish SSH access before
   connecting the iPod/hub. A computer attached to the same USB host port cannot
   simultaneously provide a USB-device recovery connection. Use a working
   network connection for SSH. If Prelude uses a USB Wi-Fi adapter, this needs
   another downstream hub port. The three-in-one adapter alone does not provide
   networking. Stopping Prelude can also stop its managed Wi-Fi access point;
   see the offline test procedure below.
2. Download and extract `podthing-0.1.0-test.tar.gz` on your computer. For a
   source checkout, build it with Go 1.24+ using `sh scripts/build.sh`.
3. Copy the bundle to the Car Thing using a writable directory. From a source
   checkout, `sh scripts/deploy.sh root@CAR_THING_IP` does this, checks the
   checksums and prints the exact launch path. Otherwise:

   ```sh
   ssh root@CAR_THING_IP 'mkdir -p /var/lib/prelude/podthing-test'
   scp podthing-0.1.0-test.tar.gz root@CAR_THING_IP:/var/lib/prelude/podthing-test/
   ssh root@CAR_THING_IP
   cd /var/lib/prelude/podthing-test
   tar -xzf podthing-0.1.0-test.tar.gz
   sha256sum -c SHA256SUMS
   chmod +x podthing-linux-* run-on-prelude.sh
   ```

4. Start with the **demo** to verify the screen, wheel, and buttons:

   ```sh
   systemd-run --unit=podthing-test --collect /var/lib/prelude/podthing-test/run-on-prelude.sh --demo
   ```

   Use the versioned directory printed by `deploy.sh` instead if applicable.
   The temporary service lets the app survive an SSH disconnect. The launcher
   stops `preluded.service`, runs PodThing, and restores Prelude on exit.
   It does not disable Prelude, change USB host policy, or rewrite firmware.

5. To stop the demo and restart in hardware mode:

   ```sh
   systemctl stop podthing-test
   systemd-run --unit=podthing-test --collect /var/lib/prelude/podthing-test/run-on-prelude.sh
   ```

   Rebooting also returns to the original Prelude boot configuration. If the
   launcher is forcibly killed, restore its app with `systemctl start preluded`.

## Offline test when SSH disappears

Prelude's daemon may own the wireless access point. PodThing does not take over
that network service. Transfer the bundle first, then schedule a bounded test:

```sh
systemd-run --unit=podthing-test --collect --property=RuntimeMaxSec=300 /var/lib/prelude/podthing-test/run-on-prelude.sh
```

This stops the test after five minutes and runs the launcher's cleanup to restart
Prelude. You can also reboot. Reports saved from the touchscreen persist in the
bundle's `state/` directory and can be copied after reconnecting. Do not rely on
the browser console or a live SSH session during an offline test.

## Controls

- Four top presets: Artists, Albums, Songs, Playlists.
- Wheel: move selection; wheel press: select. Touch rows to open them.
- Back button: parent category, or return from settings/Now Playing.
- Far-right top button: settings. Select a preset row to cycle its assignment.
- Swipe lists vertically to scroll. Playback controls remain in the footer.
- Settings persist next to the executable under `state/settings.json`.

## Hardware test order

1. Connect the powered OTG hub and the iPod with its normal USB-to-30-pin data
   cable. USB host mode must already work in your Prelude installation.
   PodThing does not change the MAX20332, charging voltages or USB role switch.
2. Open **Settings → USB & audio diagnostics → Rescan devices**. Find the iPod
   and hub DAC in the USB/audio list. A connected storage device alone does not
   establish accessory control or digital audio.
3. Choose **Connect iPod**. The app checks that the iPod is not mounted, looks
   for an advertised HID accessory configuration, temporarily selects it if
   necessary, reads its HID report descriptor, and attempts legacy iAP Extended
   Interface control. It does not guess undocumented USB configurations or
   force the device into a different mode. An iPod that does not advertise the
   required configuration will produce a diagnostic error, not a fake library.
4. Browse Artists → Albums → Songs, play a track, and test previous/next and
   play/pause. The classic may lock its local UI while controlled by an
   accessory. Disconnect/normal exit attempts to release this mode and restore
   the original USB configuration.
5. **Audio is a separate test.** Choose **Request iPod digital audio**. This
   advertises the legacy digital-audio lingo and answers sample-rate requests
   at 44.1 kHz. If the iPod requests accessory authentication, PodThing reports
   it explicitly. No authentication chip/provider is included, and no audio
   unlock is promised. Control can succeed while audio remains unavailable.
6. Rescan. Cycle **Hub / DAC output** until your hub's playback device is
   selected. Turn car AUX volume down, then choose **Test DAC tone**. The tone
   tests only the hub output, not the iPod connection.
7. If an **iPod audio capture** PCM exists, select it and choose **Start iPod
   audio route**. ARM64 uses the installed ALSA library directly. The ARMv7
   fallback uses `alsaloop` or `gst-launch-1.0` from the installed OS, and reports
   a missing runtime prerequisite if neither is present.
   A running route is not proof of sound: confirm music through AUX, then
   confirm that pause and skip change what you hear.
8. If any step fails, choose **Save diagnostic report**. Copy the latest
   `state/report-*.json` and the journal:

   ```sh
   journalctl -u podthing-test --no-pager -n 200
   ```

Reports contain device names/descriptors, track metadata currently shown on
screen, software/tool status, and command IDs/counts. They do not record iPod
serial numbers, raw authentication payloads, credentials or music files.

## Console from your computer

If network access remains available while PodThing runs:

```sh
ssh -L 8787:127.0.0.1:8787 root@CAR_THING_IP
```

Open `http://127.0.0.1:8787` on your computer. It shows the same rendered screen,
provides simulated hardware buttons, and downloads the JSON diagnostic report.
The app binds the console to loopback by default; no Internet service is used.

For development on Linux without hardware:

```sh
go run ./cmd/podthing --demo --headless
```

The orange **DEMO · NO AUDIO** label identifies simulated data. Demo mode never
opens an iPod or routes audio. `--headless` hardware mode provides USB testing on
a Linux computer before a Car Thing test, without reading its keyboard inputs.

## About the three-in-one USB-C hub

Its connectors match the desired arrangement, but power/host compatibility is
unverified. Current Prelude documentation specifically recommends a powered
legacy USB 2.0 OTG adapter rather than a laptop-style USB-C/PD dock:
https://github.com/usenocturne/prelude/blob/main/image/README.md#flash-and-usb-host-bring-up

If the iPod/DAC do not enumerate, test a known-working Prelude OTG arrangement
before changing software. A 60 W label does not establish charge-through OTG
compatibility with the Car Thing.

## Replacing Prelude permanently

This package replaces the running app during a test, and retains Prelude's
kernel, bootloader, Weston, USB host support and audio drivers. A full custom
image/boot-time takeover is deliberately a later packaging step once actual
iPod USB behavior is known. No flashable image has been built or hardware-tested
as part of this release. Do not feed this application archive into Terbium.
