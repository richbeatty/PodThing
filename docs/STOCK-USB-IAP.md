# Stock-hardware USB accessory implementation path

**Owner requirement (2026-10-18):** The user will not add a 30-pin serial breakout, USB-UART adapter, MFi authorization chip, donor accessory, or iPod firmware modification. Work with the existing Spotify Car Thing, stock-firmware 7th-generation iPod Classic, its ordinary USB-to-30-pin cable, and the existing host/OTG cable or powered hub already in use. **Do not propose the serial dock as the plan.** Draft PR #13 was explicitly rejected and closed unmerged.

## What this branch changes

- In the **disconnected** UI, the primary **Connect iPod** action now calls the existing `App.connect()` iAP1/USB HID path, not `storage.OpenUSB`. This is a meaningful software-path change but **not** a working authenticated iPod session by itself.
- In **Settings > Connection**, the first action is `Connect iPod (USB accessory)`, followed by rescan, explicit USB Simple Remote/identification probes, and diagnostics.
- `Open music as disk (experimental)` is clearly labeled and located after accessory controls. The older read-only storage fallback is preserved but is not the normal path.
- USB discovery distinguishes *wrong USB role*, *Linux root hubs only*, *downstream hub but no Apple iPod*, *unrecognized Apple USB device*, *one iPod*, and *multiple iPods*, before making iAP commands or suggesting disk mode.
- Sanitized USB device transitions can be saved in existing diagnostic reports (inherited from the original draft USB-diagnostics change).

## Exact feasibility limits

Apple's historical hardware specification documents the iPod USB Interface, iUI: a **USB HID control transport** for iAP, plus a **USB Audio Class** streaming function on models that support digital audio. It is not USB mass storage. This is the correct **software transport** for a stock 30-pin USB cable.

Apple documents authentication using a genuine Apple-provided certificate and a signed challenge response from an authentication IC; untrusted software cannot merely claim that a capability mask or successful HID write authenticates the accessory. The current code identifies and reports requests for MFi certificate `00/0014` and signature `00/0017`, but **cannot complete them using the existing stock hardware**. No one has demonstrated a software-only path to full MFi behavior in this project. Do not assert that Display Remote, Extended Interface, album artwork or digital audio are working until the stock iPod actually accepts commands and captures/output music. If the cryptographic requirement cannot be satisfied under the owner's constraints, report this as a hard limit rather than recommending equipment the owner rejected.

## First hardware test with *existing equipment*

1. Power the Car Thing in normal host mode with the same cable/USB host hub previously used and the stock Classic plugged in.
2. In **Settings > Connection**, choose **Rescan USB devices**. Check the report for an external USB hub and an `05ac` Apple device. **Only two `1d6b` xHCI root controllers**, as in test.9, means no downstream data device enumerated; no iAP/MFi packet can help at this stage.
3. If the iPod enumerates, choose **Connect iPod (USB accessory)** once. It uses the iPod's *actual advertised USB configuration* to choose an HID iUI interface, avoiding guesses and repeated SET_CONFIGURATION. Capture the stage reached: configuration selected, HID claimed, HID report descriptor read, identification, authentication requested, or database response.
4. If the iPod bounces back to storage/configuration and USB disconnects, save the host log before changing roles. Do not repeat configuration changes automatically or write to an iPod filesystem.
5. Test **USB Simple Remote** with the iPod showing a currently playing track and physically verify Play/Pause. Sending the request is not equivalent to observing acceptance.
6. If authentication is requested, display AUTH REQUIRED, preserve the diagnostic detail without storing certificate/key material, and stop gated commands. No fake MFi response and no infinite handshake loops.

## What is not implemented here

- Full MFi cryptographic authentication with unchanged Car Thing hardware.
- Successful external Display Remote, album art or Extended Interface browsing on the owner's Classic.
- UAC source PCM capture or end-to-end real digital audio.
- A verified fix for the upstream USB hub not enumerating.
- Any serial adapter or iPod physical modification.

## References

- Apple hardware specification, iPod USB Interface: https://www.scribd.com/document/694737660/MFiAccessoryHardwareSpecificationR9NoRestriction-875543417
- Apple, accessory verification: https://support.apple.com/guide/security/verifying-accessories-sec70a4f377d/web
- Repo: `internal/hardware/usb_linux.go`, `internal/iap/client.go`, `internal/player/app.go`, `internal/player/view.go`.
- Hardware/USB bug: #9; product constraint: #11; serial branch rejected: PR #13.

**Testing policy:** Unit and ARM cross-compilation success establish implementation consistency, not real Classic firmware/Car Thing USB host/MFi functionality. Keep draft until the physical report verifies progress.
