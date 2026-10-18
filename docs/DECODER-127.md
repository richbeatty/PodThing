# Diagnosing audio decoder exit status 127

**Owner symptom (2026-10-18):** attempting to play music from a read-only-mounted iPod Classic in PodThing displays `decoder exit status 127`. This error refers to the **Car Thing's local FFmpeg process**, not an error decoding the iPod's iTunesDB, and not evidence of a write to the iPod.

## Cause and confidence
- **Direct code evidence:** `internal/storage/playback.go` starts `/usr/bin/ffmpeg` to decode the selected iPod music file and outputs PCM to ALSA. `127` is the actual FFmpeg process exit status. The prior firmware smoke test booted the Go application but **never executed FFmpeg**, so a missing dynamically linked runtime dependency could escape validation.
- **Likely but not proven on this specific Car Thing:** the ARM64 FFmpeg loader or one of its dynamically linked shared libraries is unavailable, causing a loader exit 127. Exact dependency requires the original FFmpeg stderr, obtainable in the new decoder error and JSON report. An ALSA output/device error is another possibility but normally returns a different FFmpeg exit code. A hardware screenshot or bare `127` cannot identify the library.
- **New checks:** full firmware build executes ARM64 `ffmpeg -version` under QEMU with the assembled runtime loader/library path, processes a short generated audio sample through the FFmpeg audio pipeline, and verifies ALSA output support. If the decoder is broken in the image, the release **fails rather than shipping**.
- **At runtime:** first real playback performs a bounded FFmpeg startup probe. When it fails, the error preserves the dynamic linker's diagnostic line instead of falsely claiming the song is playing. Actual decoder failures are also added once to `Log` and saved in `/api/report`.

## How to test without risking iPod data
1. Check **Settings → About → Version**. An old build may not have the current UI/decoder improvements even if GitHub published newer firmware.
2. Confirm the iPod music library actually mounted read-only. This issue is independent of whether artwork is present.
3. Choose **Settings → Audio output** and a physically attached USB DAC. Simulated mode only tests the UI; it never emits music.
4. Play one track. If error 127 persists, **Save diagnostics while still in host mode** and fetch `/api/report` or later `/api/report/saved` after entering recovery. Look for the `ffmpeg` runtime field and `AUDIO decoder:` log entry. The loader will often name the exact missing `lib*.so`. No need to deliberately unplug the mounted iPod.
5. Eject normally and wait for an unmount success report before disconnecting.

## Prevent regression
A GitHub "Build PodThing" job establishes Go tests, vet, Python tests and application cross-compilation but does not run the full firmware root filesystem. Only the **Publish PodThing hardware test** workflow exercises the new assembled ARM64 FFmpeg preflight. The full QEMU boot and firmware package still need successful results before a new flashable ZIP is safe to recommend.

No modifications to the iPod filesystem, iTunesDB, firmware or USB read-only disk mount are needed for this fix.
