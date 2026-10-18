# Accessory-first iAP1, Display Remote and Digital Audio roadmap

Status: architectural decision and research plan, NOT a functioning MFi implementation.
Recorded: 2026-10-18
Base: `podthing-release-0.1.0-test`

## Owner decision

**Primary product behavior: PodThing acts like a real iPod dock, not a host computer mounting the iPod's filesystem.** The stock 7th-generation iPod Classic should remain in control of its playback and music library. PodThing is the accessory/head unit, displaying browsing/metadata/artwork and issuing touch/dial remote controls. Digital music should originate from the iPod's USB Audio Class source interface or, as an explicitly labeled alternative, the 30-pin analog line-out. The owner explicitly does not want the read-only mass-storage/FFmpeg implementation to become the production architecture.

**Do not delete the existing storage player yet.** Retain it isolated as an experimental, read-only compatibility fallback and regression reference until an accessory-mode prototype has demonstrated useful behavior on actual hardware. It should not be the default end-user connection method after accessory mode is working.

## Protocol vocabulary: do not conflate features

- General lingo (`0x00`): identification, mode/session and authentication events.
- Simple Remote (`0x02`): basic play/pause/skip. Limited functionality without MFi may be possible; the user's earlier physical tests did NOT confirm commands were acted upon.
- Display Remote (`0x03`): remote playback state/metadata/battery/controls and possibly track artwork; available commands and models require testing; do not equate this with audio output.
- Extended Interface (`0x04`): browse artists/albums/playlists and choose media on the iPod.
- Digital Audio (`0x0A`): negotiate iPod-originated digital audio; the physical audio stream typically travels over a USB Audio Class 1.0 source endpoint, separate from iAP control traffic.

The phrase "Display Audio" in owner discussions is shorthand for the **combination** of Display Remote UI and Digital Audio. Treat them as separate milestones and expose their individual status in diagnostics.

## Authentication is a real hardware dependency

The source currently recognizes but cannot answer iAP1 authentication events `00/0014` (certificate request) and `00/0017` (signature request). `internal/iap/client.go` reports `ErrAuthentication`. This is deliberate; no certificate or private key exists in the firmware.

A serious implementation needs an authenticated, authorized hardware/provider path that can return the needed certificate material and perform the device's challenge-response signing. Never fabricate cryptographic responses, silently claim authenticated success, or treat a sent USB control request as proof that the iPod accepted it. A USB DAC alone does **not** provide MFi authentication.

For a product to manufacture and sell, investigate Apple's [MFi Program](https://mfi.apple.com/en/how-it-works.html) or a legitimately authorized accessory manufacturer. Today's published MFi materials emphasize iAP2 and newer accessories; availability and certification for legacy stock Classic/iAP1/30-pin products is **unconfirmed**, and must be resolved as a commercial feasibility gate. Licensed technology is not automatically obtained by implementing protocol packet framing.

A genuine, old MFi accessory may be useful as a **behavioral reference** or a consented, non-destructive hardware experiment. Do not assume its authentication hardware/credentials can be repurposed or is a reliable, licensable production pathway. Do not design an unauthorized certificate-clone product.

## Two candidate transports

### A. USB iAP1 HID and iPod USB Audio Class (preferred digital path)

```
stock iPod Classic (30-pin USB)
   -> powered, verified USB host + data/power negotiation
   -> USB descriptor/interface probe (non-destructive)
   -> iAP1/HID session
   -> device identification + valid MFi challenge/response
   -> request Display Remote / Extended Interface features
   -> request Digital Audio lingo
   -> enumerate/select UAC source streaming interface and capture PCM
   -> ALSA playback via a separate local DAC, or another confirmed audio output
```

The iPod is the player. PodThing must not parse iTunesDB, mount the disk, or decode individual music files in this path.

Critical risk: The last `test.9` owner's report showed only Linux xHCI root hubs (`1d6b:0002` and `1d6b:0003`) and no iPod or downstream hub, despite `host_role: host`. Fix electrical/role/hub/USB enumeration **before** reasoning about accessory sessions. Don't force SET_CONFIGURATION repeatedly when a device re-enumerates or bounces.

### B. 30-pin serial iAP1 + analog line-out (hardware fallback, not USB digital)

A separately designed 30-pin accessory with correct serial levels, accessory detection, power and analog line-out can allow the iPod to provide its own decoded analog audio without filesystem access or a USB audio streaming negotiation. Basic simple-remote controls may not require authentication. Display Remote/Extended Interface features still need proper protocol and, for gated functions, MFi authentication. The physical setup is **not** equivalent to plugging the existing USB cable into the Car Thing, and needs a validated interface board, not guessed wiring.

Do not present analog audio as "Digital Audio working." This path may help validate the owner experience before digital audio is available, but remains a hardware development project.

## What current code already supplies

- `internal/iap/protocol.go`: iAP1 framing, checksum and HID fragmentation/reassembly.
- `internal/hardware/usb_linux.go`: USB descriptors, selection of an advertised HID accessory interface, usbfs transfers and controlled re-enumeration handling.
- `internal/iap/client.go`: Extended Interface browsing, playback commands, Simple Remote probe, modern identification probe, Digital Audio request attempt and explicit detection of authentication requests.
- `internal/hardware/audio_linux.go` and `alsa_linux.go`: ALSA device handling.
- `internal/player/app.go`: connection state, diagnostics and existing UI action routing.
- `internal/player/modern_ui.go`: touch UI that can be fed by an authenticated accessory session instead of storage-model metadata.

Not yet supplied/proven:
- Any **working** MFi provider or auth coprocessor integration.
- A tested, successful iAP1/IDPS certificate and signature exchange with stock Classic.
- A stable connected USB HID accessory mode on the owner's physical setup.
- Verified `0x03` Display Remote metadata/artwork for the user's Classic.
- Verified accessory-mode Extended Interface navigation and track selection.
- Verified USB Audio Class source enumeration, isochronous capture or audible digital output.
- A legitimate production hardware and MFi certification/sourcing plan.
- An end-to-end test on owner's Kia-like behavior without storage mounting.

## Implementation milestones and acceptance criteria

**M0: stable electrical/data connection.** With a known-powered hub/cable, report the downstream hub, an Apple VID/PID, its configurations, USB role, power and descriptor parse results. Survive connect/disconnect without unintended filesystem writes or session resets. Capture the before/after USB topology and host kernel log. Current issue: #9.

**M1: reliable iAP1 transport.** Parse actual stock iPod HID reports with correct report IDs/fragments/transaction handling, bounded timeouts and distinct USB failure stages. Non-destructive test script and emulator-based packet tests. Do not probe MFi-gated functionality repeatedly without a provider.

**M2: authenticated session.** Define an injectable authentication provider (real externally connected authorized hardware, no embedded secrets). Implement `RequestAuthenticationCertificate` and `RequestAuthenticationSignature` interaction, track definitive device authentication status, verify failure/timeout behavior and do not claim success until the actual iPod confirms it. Simulator-based tests are separate from a genuine hardware success.

**M3: Display Remote and Extended Interface.** Acquire now-playing metadata, progress, transport, browse lists and actual album artwork from iAP messages. Preserve the touch-first UI; use iPod's media indices/identities rather than local filesystem paths. Record missing firmware/model-specific commands.

**M4: digital sound end to end.** Advertise and negotiate Digital Audio only after required device/auth gates, identify real USB Audio Class streaming endpoints/alternate settings and supported PCM rate/channel formats, create a reliable capture path, route to selected local playback output, and validate audibly on the owner's actual iPod. A three-second DAC tone by itself is **not** this test.

**M5: make accessory mode the default.** UI automatically enters authenticated accessory mode after successful connection, with real iPod controls, battery/track/artwork and no disk-mode workflow. Keep the storage prototype accessible only as an explicitly opt-in legacy diagnostic mode while transitioning.

**M6: production feasibility.** Clarify authorized 30-pin/iAP1 authentication hardware availability, power/hub/USB controller choices, licensing, safety and supply chain, before choosing CM5 vs ESP32-P4 vs custom Linux SoC.

## Test evidence and prior work to preserve

- A 2019 Kia Niro has been owner-verified to read metadata and artwork and control the stock iPod over a USB-to-30-pin cable, showing the desired user experience can exist on actual Apple stock firmware. This observation does **not** reveal the Kia's exact chipset, lingo mix or credential system.
- User's old PodThing tests reported iPod USB configuration bouncing, incomplete iAP1 handshake, MFi auth requests and Simple Remote presses not acknowledged.
- The storage player could read the full music library in an older test but that does not fulfill the accessory-mode product goal.
- Owner's last uploaded test.9 report contains no external USB devices. Resolve that independently first.
- Existing docs and some legacy README content focus on storage playback; those are interim history, not a final product requirement.

## Research reference starting points

- Apple MFi: https://mfi.apple.com/en/how-it-works.html
- Apple accessory authentication: https://support.apple.com/guide/security/verifying-accessories-sec70a4f377d/web
- Apple's public Display Remote patent: https://patents.google.com/patent/US8171195B2
- `oandrew/ipod` (Go implementation of iPod-side iAP; **opposite end** from PodThing accessory): https://github.com/oandrew/ipod
- `oandrew/ipod-gadget` (USB gadget emulating iPod; **opposite role**): https://github.com/oandrew/ipod-gadget
- `finsprings/arduinaap` (iAP legacy serial reference): https://github.com/finsprings/arduinaap
- `EgorShtab/rocketpod` (modified Rockbox; **iPod firmware replacement**, not directly usable on owner's stock firmware): https://github.com/EgorShtab/rocketpod
- 2019 Kia Niro manual confirms head-unit iPod mode/artwork/list navigation, but not its private iAP implementation.

## Maintenance rule

Do not mark MFi/Display Remote/Digital Audio as complete on emulators or by claiming Apple security can simply be spoofed with software. Show owner whether each function is (1) implemented and unit-tested, (2) recognized by the stock iPod, (3) authenticated, and (4) actually working on the physical device. The user's goal is native iPod playback with a modern external interface and safe disconnect, **not** a small computer mounting an iPod.
