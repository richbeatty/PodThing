# Read-only iPod player cleanup

**Owner decision — 2026-10-18:** the shipping/test Car Thing firmware returns to a **read-only iPod music player**, with files decoded on the Car Thing and output to a real selected USB DAC / ALSA playback device. Do **not** present iAP accessory mode, serial breakout hardware, MFi/Display Audio probes or alternative UIs as primary options. No Rockbox or changed iPod firmware. The old MFi and serial research remains archived in historical PRs, not part of this cleanup.

## What changed

### One consistent interface
- `BuildView` and `Painter.Render` select the **same charcoal touch-first UI** for disconnected, browsing, playing, settings and safely ejected states.
- No separate blue legacy screen or switching to a second design at connection time. Former legacy code now provides settings row data, not a second renderer.
- Top navigation: Home, Music, Now playing. **Eject** appears once in the header whenever a read-only volume is mounted, including on the Now playing and Settings screens. It is kept left of the Car Thing hardware dial.
- The bottom-left settings icon leads to a short set of submenus: iPod & USB, Audio output, Controls and About.
- The swipe-down preset panel holds only four configured presets, not another duplicate Eject/Settings/Shuffle/Repeat menu.
- Disconnect screen shows a safe-to-unplug message and the choice to reconnect. Errors are visible without restoring the legacy UI.

### Safe eject
- The iPod's block disk and partition are still kernel-set read-only before mounting; the filesystem is mounted with `MS_RDONLY` and other protective flags.
- Player decoder terminates before eject, and any artwork readers finish; **libgpod's helper is closed and waited for before unmount** (previously done after unmount).
- The pre-eject diagnostics file is best-effort: disk-full or report failure can no longer prevent otherwise safe unmount, but is reported.
- If the app restarts after a mount, startup recovers only a mount at `/run/podthing-ipod` belonging to the connected physical iPod and verified read-only. If its library is corrupt, Eject remains available without reparsing the music database.
- Eject claims success **only after actual Linux unmount succeeds**. It cannot compel the iPod itself to leave the USB connection screen. If no iPod is detected by Linux, eject cannot be performed and the UI reports that honestly.

### Real playback
- A fresh hardware install defaults to real USB/ALSA output rather than simulated silence. If **exactly one** external playback DAC is detected, PodThing selects it automatically; saved user settings are preserved.
- If no real USB playback output is present, PodThing must not claim music is audible. The user may select a physically present DAC in Audio output. Silent simulated mode remains available explicitly for testing without a DAC.
- Car Thing's own internal ALSA capture devices are not external speakers or outputs. No DAC or 3.5mm jack has been fabricated by software.

### Album artwork
- The stock iPod's `iPod_Control/Artwork/ArtworkDB` and `.ithmb` thumbnails are queried through the read-only libgpod helper. Previously, a failure resolving the representative track's music file immediately abandoned this route. ArtworkDB is now queried first, independently.
- Embedded AAC/MP3 image tags are still the fallback and are attempted for additional album candidates.
- Logs now record the first meaningful artwork failure in the diagnostic report rather than silently showing blank covers.
- **Not verified on the owner's physical device yet.** Missing ArtworkDB, unsupported libgpod format, a missing album thumbnail, invalid music paths and an iPod that does not enumerate are distinguishable failure stages, not reasons to invent substitute covers.

## Physical test procedure

1. Use the owner's existing stock iPod Classic, USB-to-30-pin cable and powered host adapter/hub. Confirm USB diagnostics show an Apple VID/PID and not just Linux root hubs. The earlier test.9 diagnostic showed **zero external USB devices**, preventing *any* library or art reading at that point.
2. Press the single **Open iPod music (read-only)** action on the disconnected screen. Confirm a nonzero artist/album/song library appears and menus retain the same style before/after connection. The iPod may show its normal USB/disk connection screen; PodThing itself handles playback.
3. In **Settings > Audio output**, choose the attached USB DAC/output, if any. Select USB audio mode rather than simulated silence; play a real track, pause, seek and skip.
4. Open albums and Now playing. Check that real covers load after the asynchronous artwork worker runs. If blank, save the diagnostic report after allowing time for a thumbnail; use the first `ARTWORK:` warning and worker stderr/boot log to distinguish the reason.
5. Tap the **top-bar Eject** once. Confirm playback stops, the album-art helper releases iPod files, the read-only mount disappears, and the UI says **Safe to disconnect**. Unplug the cable only after that indication. Repeat with a song playing and album art actively being requested.
6. If rebooting while a verified PodThing read-only mount exists, ensure the app reattaches safely and Eject remains available. This is a recovery scenario, not an invitation to unplug during file access.
7. If safe eject fails, do not power-cycle or pull the iPod while PodThing claims it is mounted. Save the error/report and investigate open handles. **No forced or lazy unmount** is allowed.

## Release evidence

App tests and ARM builds are useful but do not prove physical Car Thing, iPod or DAC behavior. Firmware/QEMU validation is a different gate; actual hardware owner reports are the last gate. Do not claim artwork/eject are repaired solely because the Go/CI jobs pass.

Relevant source: `internal/player/view.go`, `modern_ui.go`, `modern_art.go`, `classic_artwork.go`, `app.go`, `storage/mount_linux.go`, and `storage/playback.go`.
