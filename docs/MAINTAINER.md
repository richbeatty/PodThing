# PodThing Maintainer Guide

Last reviewed: 2026-10-18
Repository: https://github.com/richbeatty/PodThing
Working branch at review: `podthing-release-0.1.0-test`
Reviewed branch commit: `446b5d6b30876db4f9642a7a9abedd5a107d0c28`
Latest published prerelease at review: `podthing-v0.1.0-test.10` (2026-10-17)

## Updated product decision: accessory-first (2026-10-18)

**This decision supersedes the storage-first preference described elsewhere in the historical sections below.** The owner wants a real, authenticated iPod dock: iAP1 Display Remote (`0x03`), Extended Interface (`0x04`), and Digital Audio (`0x0A`) with playback remaining on the stock iPod. The user does not want iPod mass-storage mounting as PodThing's normal connection method. Read-only storage playback was an experiment and should be retained only as an optional fallback until accessory playback works on physical hardware, not expanded into the main product.

Full architecture, hardware/authentication gate, distinct acceptance milestones and reference sources: [Accessory-first roadmap](ACCESSORY-ROADMAP.md). No verified MFi chip/provider or end-to-end digital audio exists as of this decision; do not claim otherwise. The lack of enumeration in the owner's last test.9 report (#9) must be resolved before meaningful USB accessory handshakes can be tested.

## Why this file exists

This is the project handoff and continuity document. Read it before changing PodThing, especially when resuming work in a new chat or with a new maintainer. It records the actual implementation, the reasons for architectural changes, owner decisions and hardware observations, the build/release path, and work that has not yet been verified.

**Evidence labels matter:**
- **In source** means implementation is visible in the reviewed repository.
- **CI validated** means the GitHub build/release workflow reports success, not that physical hardware works.
- **Owner reported** means behavior was observed or described during hands-on tests.
- **Proposed** means a direction or requirement, not an implementation or a proven design.

Keep these categories separate. Never describe a passing emulator test as successful Car Thing, iPod, DAC, or vehicle integration.

## 1. Project mission and product decisions

PodThing aims to turn Spotify Car Thing hardware into a standalone, touch-first controller and music player for a stock-firmware iPod Classic. The long-term ambition is a manufacturable, inexpensive dedicated iPod dock with a refined interface, potentially running on custom hardware instead of Car Thing.

Primary current test hardware:
- Spotify Car Thing with its 800 x 480 display, touch, physical dial, and preset buttons.
- Owner's seventh-generation iPod Classic, flash-modified to roughly 512 GB, retaining stock firmware.
- Standard USB-to-30-pin iPod data cable and a powered USB OTG hub.
- A USB DAC / hub with 3.5 mm output is desired for real playback; owner reported not yet having that audio hub during test.8 development.
- A 2019 Kia Niro using USB-to-30-pin was a useful reference for iPod metadata, artwork, and touch controls. Its behavior is not proof that PodThing can copy the car's protocol or authentication.

Product constraints:
- Do not require Rockbox or firmware modification of the iPod.
- Do not sacrifice the user's existing music database to make a demo work.
- Avoid dependence on Car Thing's preset buttons in the primary UI; future hardware may have fewer buttons.
- Use genuine artwork or an honest placeholder, not invented covers.
- Prefer a minimal, modern, CarPlay-inspired music experience over a debug-panel UI.
- Preserve the possibility of moving the software and interaction model to a cost-effective custom product. No final production PCB, BOM, STL, or alternate SoC design has been validated or committed here.

## 2. Locate the real source before doing anything

**Do not start from `main`.** At this review, `main` is an initial, two-line README with no application source. The real Go application, firmware, tests and release workflows live on `podthing-release-0.1.0-test`.

Suggested checkout:

```sh
git clone https://github.com/richbeatty/PodThing.git
cd PodThing
git fetch --all --tags
git switch podthing-release-0.1.0-test
git status
```

Relevant historical branches include `podthing-readonly-storage-player`, `podthing-modern-ui-simulated-playback`, `podthing-test9-artwork-eject-minimal-ui`, and earlier USB/iAP experiment branches. Pull requests #1 through #8 were merged **into the release branch**, not `main`.

Current GitHub links:
- Active code: https://github.com/richbeatty/PodThing/tree/podthing-release-0.1.0-test
- Releases: https://github.com/richbeatty/PodThing/releases
- PR history: https://github.com/richbeatty/PodThing/pulls?q=is%3Apr
- CI: https://github.com/richbeatty/PodThing/actions

At the review snapshot, the release branch and test.10 tag resolve to the same commit, `446b5d6`. Recheck before every new release, and do not assume this document's snapshot SHA is still current.

## 3. Current architecture

PodThing is primarily Go, not a web app or a fork of Prelude.

```text
Car Thing, independent Linux firmware
  systemd + Weston + native Wayland framebuffer
  PodThing Go app
    touch + dial/buttons through evdev
    software-rendered 800x480 music UI
    local HTTP diagnostic/control console
    USB detection (sysfs / usbfs)
      Path A: legacy iAP1/HID experiments
      Path B: USB mass storage, read-only (preferred working direction)
        iTunesDB parser -> library/home/recents/playlists
        artwork: libgpod worker / ArtworkDB, fallback embedded extraction
        simulated player OR ffmpeg -> USB ALSA DAC
        guarded safe unmount
  USB recovery mode via local USB-NCM network
```

**Playback architecture matters:**
1. **Legacy accessory / iAP mode.** The initial approach attempted HID iAP1 session negotiation, remote controls, library browsing and digital audio. It met stock-device configuration changes, timeouts and MFi authentication requests. The path remains useful for controlled compatibility experiments but is not a proven playback solution.
2. **Read-only storage mode.** Newer releases intentionally use the iPod's ordinary USB disk-mode interface, parse its `iPod_Control/iTunes/iTunesDB` and read media files. This is not an MFi workaround: it is a different host-side playback design. This is the path that the owner reported could read the full music library.
3. **Simulated output.** Select a real track from the mounted library, then run its timer, queue, play/pause, skip, seek, repeat and shuffle without decoding or outputting any sound. This was explicitly chosen to allow UI testing before a USB DAC arrives.
4. **USB DAC output.** When selected and a valid USB ALSA output is present, FFmpeg decodes the actual file and sends PCM to the DAC. The DAC, external speakers and car AUX path are independent hardware requirements. Bluetooth A2DP transmit and USB audio gadget output to a vehicle are not implemented.

The Car Thing firmware is an independent Linux appliance, with a pinned Superbird BSP/kernel and boot chain, Debian ARM64 runtime, Wayland/Weston, ALSA, libgpod and FFmpeg. It does not install Prelude. The older temporary-on-Prelude test path remains for developer experiments only.

## 4. Where the code lives

| Path | Responsibility |
| --- | --- |
| `cmd/podthing/main.go` | Flags, process lifecycle, Wayland screen, input, HTTP console, ticks and scans. |
| `internal/player/app.go` | Central app state, USB connection modes, settings, actions, storage open/eject and report state. |
| `internal/player/storage.go` | Library browsing, filtered tracks, queue, seeking, actual vs simulated playback. |
| `internal/player/modern_data.go` | Recents, album cards, album identity, sort controls and home suggestions. |
| `internal/player/modern_ui.go` | Current minimal charcoal visual layout and touch targets. |
| `internal/player/view.go` | Legacy/configuration screen models, input targets, font/painter support. |
| `internal/player/modern_art.go` | Async covers, bounded cache, native Classic art and embedded fallback. |
| `internal/player/classic_artwork.go` | Isolated libgpod worker IPC, timeouts and decoded-image checks. |
| `firmware/direct/rootfs/usr/lib/podthing/artwork.py` | Python/ctypes libgpod read-only artwork decoding from the mounted iPod. |
| `internal/player/simulated.go` | No-sound timer/player state. |
| `internal/storage/mount_linux.go` | USB device partition matching, BLKROSET, read-only mount, recovery and unmount. |
| `internal/storage/library.go` | Bounded iTunesDB and playlist parser, paths and file resolution. |
| `internal/storage/playback.go` | FFmpeg-to-ALSA process lifecycle, stop, pause and position. |
| `internal/hardware/usb_linux.go` | USB enumeration and experimental usbfs/HID switching. |
| `internal/hardware/audio_linux.go`, `alsa_linux.go` | Audio device detection and native ALSA. |
| `internal/hardware/input_linux.go` | Physical controls. |
| `internal/display/wayland_linux.go` | Native Wayland presentation. |
| `internal/iap/` | Legacy accessory protocol, tests and transports. |
| `firmware/direct/` | Default standalone firmware assembly, image construction and system overlay. |
| `firmware/meta-podthing/` | Alternative Yocto image path; not the default deliverable. |
| `scripts/` | Build, package, validate, boot-smoke test and developer tools. |
| `.github/workflows/build.yml` | App tests and optional full firmware build. |
| `.github/workflows/release.yml` | Tagged/branch prerelease build, boot smoke, checksums and publication. |
| `docs/FIRMWARE.md` | Flashing, hardware testing, architecture evolution and USB recovery details. |
| `docs/BUILD-STATUS.md` | Cloud/emulator validation vs hardware-test caveats. |
| `docs/DEVICE-TEST.md` | Historical temporary app-on-Prelude guide, **not** current primary install. |
| `docs/RELEASE-0.1.0.md` | Cumulative release history, including superseded early limitations. |
| `THIRD_PARTY.md` | Dependencies, licenses and reference projects. |

## 5. Safety requirements that must not regress

Treat a full flash-modified iPod library as irreplaceable unless the owner confirms a separate verified backup.

**Read-only storage invariants:**
- Identify exactly one connected iPod and its USB-backed partitions. Do not scan-mount unrelated host disks.
- Apply Linux `BLKROSET` to the backing disk and partition before mounting.
- Mount only supported `vfat` / `hfsplus` partitions using `MS_RDONLY`, `MS_NODEV`, `MS_NOSUID`, `MS_NOEXEC` and the configured no-atime behavior. If read-only cannot be established, fail closed.
- Never call fsck, replay a journal, remount writable, repair iTunesDB, update play counts or ratings, or create artwork/media files on the iPod.
- Treat database lengths and file paths as untrusted. Keep parser caps, path confinement, symlink checks and bounded image decoding.
- Keep local settings, recents, caches and diagnostics on the **Car Thing**, never the iPod.
- Never allow a hardware probe to silently switch from read-only storage to iAP mode or vice versa.

**Safe eject invariants:**
- Stop the audio decoder, including paused children, and fail if it does not exit.
- Stop simulated playback and complete/block outstanding artwork access.
- Close the libgpod worker and release any database/file handles.
- Preserve the last diagnostic report when possible.
- Unmount only the verified PodThing mount (`/run/podthing-ipod`) backed by that actual connected iPod.
- Only show `Safe to disconnect` after a successful unmount. If unmount fails, leave an accurate busy/error state instead.
- An app restart can leave a mount alive. Recovery is permitted only if the kernel mount table proves it is read-only and belongs to the physically connected iPod. Test this before changing it.
- A successful host unmount is not proof that the iPod itself has changed its own display to `OK to disconnect`. Avoid overstating the guarantee.
- Never perform automatic destructive recovery when USB disappears or power is interrupted.

Owners have already seen `no mounted iPod to eject` despite prior library access. The recovery logic added in test.9 attempts to address that. **Physical regression testing is still required.**

**Source-review caveat (2026-10-18):** In `internal/player/app.go`, `ejectStorage` waits on `artUse` but calls `a.volume.Eject()` before `a.classicArt.Close()`. This is not the same as the intended sequence above. Confirm whether the libgpod worker can keep mounted artwork files open; if so, close it before the unmount and add a regression test. Do not claim this ordering has been fixed merely because test.9 recovery was merged.

## 6. Application behavior and design direction

User-facing behavior currently implemented in source:
- Home with recently played music and album suggestions.
- Library sections for Artists, Albums, Songs and Playlists.
- Album-cover grid/list presentation and several sorts.
- Now Playing controls, seeking, queues, repeat and shuffle.
- Four remappable presets behind a swipe-down Quick Actions tray, while physical buttons remain supported.
- Nested settings pages for connection, audio, controls and About.
- Adaptive artwork accent when a real cover is decoded, otherwise neutral fallback.
- Simulated playback by default unless USB audio is deliberately selected.

Owner's continuing design requirements:
- Visually simple, modern, fast and touch friendly. Draw inspiration from Apple CarPlay and Spotify browsing without duplicating them.
- Avoid clunky stacked panels, pixelated graphics, crowded controls, loud/ugly color palettes and a prominent PodThing brand on the top bar.
- Keep primary destinations high and accessible; avoid wasted header space and repeated transport controls.
- Ensure UI buttons are positioned so the Car Thing physical dial does not obscure them.
- Keep presets hidden by default and accessible with a swipe; do not make basic tasks depend on fixed hardware buttons.
- Keep album-art-driven accents restrained and legible. Album covers are essential, not a decorative afterthought.
- Continue improving sorting, recents, artist/album relationships, playlists and scrolling with large libraries.
- Ensure core UX can survive transition to future hardware with fewer physical controls.

**Known code/design mismatch:** `internal/player/modern_ui.go` still renders `PodThing` in the main header despite the owner's request to remove top branding. Do not consider the design finished merely because test.9 introduced a calmer charcoal palette.

## 7. Artwork notes

Stock Classic album art often lives in `iPod_Control/Artwork/ArtworkDB` and `.ithmb` files rather than embedded image tags.

- Test.8 supported bounded asynchronous embedded artwork extraction but not native ArtworkDB decoding.
- Test.9 added an isolated `libgpod` Python/ctypes worker that loads the authentic database read-only, looks up tracks by ID and produces temporary PNGs under `/tmp`.
- `internal/player/modern_art.go` requests covers, caches them and falls back to embedded media art.
- Missing art should show an honest neutral placeholder. Never fake album art.
- Owner reported missing artwork on physical Car Thing before the test.9 work. There is no owner-confirmed report in the supplied history that test.9 or test.10 fixed that on the owner's exact Classic.
- For debugging, capture `ArtworkDB unavailable`, worker errors, file existence and image decoding results, without copying personal music into logs.
- ArtworkDB thumbnails are an implementation, **not a guarantee of working covers** across all iPod models/database variants.

## 8. Build, test and release

Requirements from `docs/FIRMWARE.md`:
- Linux, Docker, Git and Python 3.
- Go 1.24+ for app work; the current full firmware workflow uses Go 1.27.2+.
- Roughly 16 GB RAM and 20 GB of free space for the full image build.
- Pinned firmware dependencies in `firmware/direct/sources.json`. Do not silently move upstream commits or weaken package signature verification.

Basic development:

```sh
go run ./cmd/podthing --demo --headless
# Local console: http://127.0.0.1:8787
go test -race ./...
go vet ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
sh scripts/build.sh
```

Full default firmware and boot checks:

```sh
sh scripts/build-firmware.sh
python3 scripts/test-firmware-boot.py
(cd dist && sha256sum -c podthing-0.1.0-flashthing.zip.sha256)
```

The unversioned local installer is `dist/podthing-0.1.0-flashthing.zip`. **Release assets** have revision-specific names such as `podthing-0.1.0-test.10-flashthing.zip`. Flash the `.zip`, **not** the `.sha256` text file or the `source.tar.gz` archive. The corresponding SHA-256 file is for verification.

The `build.yml` workflow runs tests/builds on branch pushes and PRs and offers a manual full firmware build. The `release.yml` workflow builds a full image, runs an ARM64 boot smoke test and publishes prereleases on pushes to the release branch or test tags. Branch pushes can create new test-numbered releases. Review resulting tag, commit, test status and version identity before telling anyone which ZIP is latest.

The release workflow's QEMU and Linux checks are useful, but real Car Thing boot, USB role transitions, touch/dial, exact iPod behavior and audible DAC output require hands-on verification.

## 9. Flash and recovery rules

Flashing installs a new bootloader and OS on **Car Thing**, overwriting its existing firmware. Keep a proven recovery installer and back up relevant device state before flashing. Disconnect the iPod while flashing.

Documented procedure: direct USB data connection to a computer; enter Car Thing mask-ROM mode by holding preset buttons **1 + 4** while plugging in; verify USB ID `1b8e:c003` where available; use Terbium's local/custom ZIP option. Do not unpack the ZIP.

The firmware normally boots with its USB port in host mode for a powered OTG hub and iPod. A single port cannot simultaneously serve host peripherals and USB device/recovery networking. Switch deliberately to computer recovery through Connection settings when done with the iPod session.

Local recovery endpoints documented by the firmware:
- Main UI/diagnostic console after USB-NCM recovery: `http://10.42.1.2:8787`
- Independent boot-log recovery page: `http://10.42.1.2:8788`
- Normal local headless app console: `http://127.0.0.1:8787`

The appliance's systemd service starts `/usr/bin/podthing --state-dir=/var/lib/podthing --listen=0.0.0.0:8787`; the USB gadget helper is `superbird-usb-role`. These local maintenance endpoints have no authentication. Keep USB recovery isolated, never forward these ports to the public Internet, and re-evaluate network exposure before selling a product.

Useful saved state: `/var/lib/podthing/settings.json`, `/var/lib/podthing/recents.json`, diagnostic files, and the last saved host-mode session. The read-only iPod mount itself is under `/run/podthing-ipod` and is not normal persistent storage.

## 10. History and why earlier ideas changed

| Change | Recorded significance |
| --- | --- |
| Initial firmware and early test builds | Standalone Car Thing appliance, UI and iAP-focused playback experiments. |
| PR #1 | Diagnose iPod accessory connect and USB storage-mode bounce. |
| PR #2 | Revise native interface, settings, revision display and host-mode boot. |
| PR #3 | Rework iAP1 USB HID/session negotiation. |
| PR #4 | MFi request diagnostics and unauthenticated Simple Remote attempt. |
| PR #5 / test.6 | Isolated compatibility probes and saved host-mode reports. |
| PR #6 / test.7 | Add read-only iPod USB-storage library, FFmpeg local playback and safe eject. |
| PR #7 / test.8 | Simulated playback, Home/recents, album grids, sorts, gesture tray, better UI. |
| PR #8 / test.9 | Minimal charcoal interface, Classic ArtworkDB worker and recovered eject. |
| test.10 prerelease | Latest published artifact at this review; see actual tag/commit before describing changes. |

The owner reported that older iAP experiments could cause the iPod to eject/disconnect, request authentication and fail to acknowledge remote commands. The pivotal discovery was that the storage approach could read the complete library. Do not restart iAP authentication experiments as though no one has tried them. The storage approach remains the preferred path unless new physical tests show a compelling reason to change it.

Past exploration included comparisons to a 2019 Kia Niro USB iPod connection and ideas from `oandrew/ipod`, `finsprings/arduinaap`, `mojyack/libiap`, PodEmu and related projects. See `THIRD_PARTY.md` for references. There is no working MFi authorization provider, and no claim that such authentication can be bypassed in software.

## 11. Current status as of this review

| Topic | Status / evidence |
| --- | --- |
| Repository and release code | Present on `podthing-release-0.1.0-test`. |
| test.10 prerelease | Published on GitHub; release branch build and release workflows report success. |
| Linux/QEMU/tests | Recorded as passing by CI; not equivalent to physical hardware testing. |
| iPod full-library read | **Owner reported successful** for an earlier storage-player test. |
| Simulated playback | In source and CI-tested; physical Car Thing UI was reported showing it. |
| Real DAC output | Implemented as a selected FFmpeg/ALSA path; awaiting verified compatible hub and audible test. |
| Safe eject | In source with test.9 recovery logic; owner had seen failure previously; exact physical fix unconfirmed. |
| Classic ArtworkDB decoding | In source; actual owner's artwork result unconfirmed after test.9. |
| Original iAP/MFi digital audio | Not proven working; authentication and handshake problems recorded. |
| Polished production UI | In progress; brand/layout/color requests still outstanding. |
| Custom replacement hardware | Exploration only; no validated manufacturable board or finalized schematic. |

## 12. Known issues and next physical test sequence

Run safety-first tests and record firmware tag, SHA, iPod model/firmware, hub model, power arrangement, USB role and repeatability. Save host-mode diagnostics *before* switching to recovery.

1. **Baseline boot and identity.** Confirm the correct test.10 (or newer) revision is actually running. Test panel, dial, touch, boot logo, shutdown and USB host default.
2. **Storage enumeration.** Use the powered OTG hub, stock Classic and normal data cable. Verify the iPod is found exactly once and choose `Open iPod music (read-only)`. Record file system, track and playlist counts. Never initiate an unapproved write or repair.
3. **Large-library navigation.** Exercise Home, Recents, Artists, Albums, Songs, playlists, art grid/list view and sorting. Watch memory, scrolling and touch placement.
4. **Simulated transport.** Without an audio hub, play real selected library tracks silently. Check pause, next, previous, seek, shuffle, repeat, end-of-queue, and recents persistence. Confirm that simulated playback does not claim audio output.
5. **Artwork.** Compare albums known to have covers on the iPod with the UI. Test native ArtworkDB lookup, the embedded fallback, caching, album switches and missing-cover placeholders. Save errors.
6. **Safe eject.** Eject while idle, during simulation, after an artwork request and after restarting the app with a verified remaining mount. Confirm **Safe to disconnect** only after successful unmount. Do not unplug mid-write because there should never be a writable mount; avoid destructive trials on the owner's only library.
7. **USB DAC hardware.** Once a known-working powered hub with DAC arrives, select USB audio, verify ALSA output detection, play a known supported track and confirm actual audible music and controls. Check power delivery and long-term stability.
8. **Failure/recovery.** Validate device disconnect, no-iPod states, USB recovery, report export and Car Thing recovery flashing without risking the iPod's stored files.

High-priority unresolved feedback:
- Missing album art on a prior physical build.
- `No mounted iPod to eject` on a prior physical build.
- Current UI still felt clunky/crowded or poorly colored in owner feedback.
- Real audio not yet verified through the desired 3.5 mm hub.
- Verify whether latest changes were actually flashed before diagnosing a regression.

## 13. Hardware portability and future dock

**Direction, not finished engineering:** The long-term device should be inexpensive to manufacture, have an excellent display and integrated physical dial, permit bootloader/firmware recovery through USB and work with the PodThing stack. Previous mockups placed the iPod alongside a display with their top/bottom aligned, and the dial angled in front of the iPod. The user prefers a real buildable design, not a concept render.

Candidates considered: Raspberry Pi Compute Module 5 (possibly excessive for cost), custom Linux-capable boards and newer ESP32-class devices including ESP32-P4 displays. No board has been selected or proven compatible.

Important feasibility constraint: the present firmware expects Linux, USB host block storage, filesystem mounting, FFmpeg, libgpod, ALSA, Weston/Wayland and a relatively large library parser. An ESP32-class MCU is **not a drop-in port**. A switch would require a new software architecture, realistic RAM/storage benchmarks, host USB support, codec/library/artwork support, boot/reflash recovery and a supported DAC/amplifier design. Do not label an MCU alternative compatible without a demonstrable prototype.

Manufacturing requirements to resolve separately: display and touch interface, processor/RAM, power, USB host and device/recovery mode, secure firmware updates, audio DAC versus powered speaker amp, enclosure, connectors, thermal behavior, regulatory testing, BOM and supply chain.

## 14. Documentation debt and maintainer handoff rules

Some older files are historically useful but **stale for the preferred storage-mode architecture**:
- The release-branch `README.md` still says PodThing never mounts or modifies the iPod library and that the iPod performs playback. That describes early iAP mode, not the newer storage player.
- `docs/DEVICE-TEST.md` describes the older temporary Prelude-based app and original top presets. Do not use it as current standalone-install instructions.
- Early portions of `docs/FIRMWARE.md` and cumulative `docs/RELEASE-0.1.0.md` describe iAP-first or test.8 limitations that later test.9 code supersedes. Read the later sections and source before drawing conclusions.
- The main branch has no active application. Eventually choose a deliberate branch strategy rather than relying forever on an undocumented release branch.
- `internal/player/modern_ui.go` still shows a PodThing wordmark in the header, contrary to owner direction.

**Before every change:** inspect the current release-branch HEAD and recent PRs; identify the affected source and tests; protect iPod read-only and eject behavior; make the smallest coherent code change; run tests; describe what CI does and does not prove; ask for real-device evidence only when required.

**After every change:** record which branch/commit changed, which artifact corresponds to it, tests and hardware tests actually run, observed bugs, remaining work, and user-facing decisions. Update this guide when an architectural choice or unresolved hardware result changes. Keep recovery and a backup plan available before suggesting another flash.

**Never:** invent successful tests, invent a release URL, mislabel `.sha256` as flash firmware, overwrite iPod data, claim simulated playback is audible, promise MFi unlocking, or assume the latest GitHub ZIP is the one installed on the device.

## 15. Immediate development priorities

1. Verify test.10 on the owner's physical Car Thing and capture a consistent device test report.
2. Confirm test.9/test.10 restored safe eject and native Classic artwork on the actual iPod.
3. Fix remaining visual issues, including top branding, navigation density, type/icon quality, and artwork-appropriate colors.
4. Validate actual audio when a powered USB DAC hub is available, without regressing simulation.
5. Consolidate stale documentation and settle the default branch strategy.
6. Only then design the custom, manufacturable dock around a proven hardware/software requirement set.

## 16. Minimal resume prompt for the next maintainer

> Work on `richbeatty/PodThing`, branch `podthing-release-0.1.0-test`, not the nearly empty `main` branch. Read `docs/MAINTAINER.md`, `docs/FIRMWARE.md`, recent PRs and current source before changing anything. This is standalone Car Thing firmware for a stock 7G iPod Classic. Preferred mode is kernel-enforced read-only USB storage with iTunesDB parsing, simulated playback or FFmpeg to a real USB DAC, ArtworkDB art via an isolated libgpod worker, and verified safe eject. Do not write to the iPod. Respect the owner's minimal touch UI direction and distinguish CI success from actual hardware tests. Verify current HEAD, release and device behavior before asserting a feature works.
