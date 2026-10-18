# Serial iAP1 prototype (not yet hardware-verified)

This experiment is available on branch `serial-iap1-no-mount-prototype`. It allows PodThing to act as a **UART iAP1 accessory** with the iPod running its own playback. There is no USB mass-storage mount, iTunesDB access, FFmpeg track decoding, MFi certificate, or assertion of authenticated Display Remote/Digital Audio.

## Safety and required physical connection

**A standard 30-pin-to-USB cable is insufficient.** That cable carries USB power/data, not iPod UART. Use a proven 30-pin breakout built for the **stock** iPod Classic with the iPod's UART TX, UART RX and signal ground available, and a genuine **3.3 V TTL USB-UART adapter** connected via the Car Thing's powered USB host/OTG hub.

A TTL UART adapter is not an RS-232 ±12 V serial port and must not drive a 5 V logic level into the iPod pins. Verify the connector orientation, signal levels, ground continuity, accessory detection, and charging/power arrangement against a trusted schematic before connection; do not guess resistor values or put 5 V/12 V onto serial or accessory-detection pins. Do not expect PodThing to power the iPod through its UART adapter. The owner has not yet confirmed such a breakout is available.

Because only one Car Thing USB interface changes between host and recovery functions, leave the iPod disconnected from the Car Thing's normal USB storage path during serial experiments. If the UI asks for disk mode, that is the **separate, older USB storage path** and is not part of this serial experiment.

Reference research:
- https://www.scribd.com/document/694737660/MFiAccessoryHardwareSpecificationR9NoRestriction-875543417 : iAP UART accessory rate choices 19200 or 57600, subject to device/hardware specifications.
- https://github.com/RaduTek/pyPodControl : tested Python iAP UART implementation, uses 3.3 V TTL, default 19200 and an `FF 55` serial packet header.
- https://sven-schwermer.de/denon-ipod-dock : independent UART packet framing observations.
- https://github.com/VaAndCob/iPod-Smart-Dock : functional dock reference, public firmware sources not available.

## Use on PodThing

A prototype firmware would need to be built from this branch; **do not flash the branch source archive or a preexisting test.10 ZIP expecting it to contain this change**. First use CI and proper hardware, then explicitly select a candidate build. No physical compatibility has been claimed.

From **Settings > Connection**, the new controls appear in this order:
1. **Serial dock test**: finds exactly one `/dev/serial/by-id/*`, `/dev/ttyUSB*` or `/dev/ttyACM*` USB-UART port, opens 19200 8N1, and sends only General `Identify` for Simple Remote (lingo 2). It does not write to the disk or claim the iPod acknowledged identification.
2. **Serial Play / Pause**, **Serial Next**, **Serial Previous**: send a 150 ms lingo 2 button press followed by button release. The iPod is expected to handle playback. No acknowledgement is inherent in this legacy Simple Remote command, so **watch and listen to the actual iPod** before calling the test successful.
3. **Serial browse artists**: explicitly switches to an Extended Interface handshake and tries a small five-entry artist query. It may request MFi authorization, change the iPod remote-UI state, and fail. No credentials are supplied. Do not run this without accepting that on-screen iPod interaction might temporarily change.
4. **Close serial test**: closes serial without attempting any other iPod session commands or USB resets.

The app records serial port selection and commands in the existing saved diagnostic report. It does **not** set the main player to `connected=true` simply because a UART packet was written.

## Test sequence

1. First verify that the powered OTG USB hub enumerates **any** downstream device. The owner's earlier test.9 log listed only Linux root hubs. UART cannot solve an electrically unavailable Car Thing host port.
2. With the iPod connected to the safe breakout, open serial test and watch the iPod for an accessory indication. Record detected UART device name and errors.
3. With a known track playing on the iPod, try one Play/Pause and one Next. Check the physical iPod actually responds. An outgoing packet alone does not establish this.
4. When remote keys have been proven, deliberately test Extended Interface artist enumeration. Record ACK/status and exact failure stage if MFi is requested.
5. Close the serial test and verify local iPod controls return normally. Save the diagnostic report before switching USB recovery mode.
6. Only after this can the maintainer attempt real Display Remote, artwork, authenticated sessions or an integrated main-screen remote browsing mode.

## Known limitations

- No physical iPod Classic/Car Thing/USB-UART test yet.
- No on-screen UART port picker; the test requires exactly one detected adapter.
- No auto baud discovery. Default is 19200; `iap.OpenSerial(path,57600)` exists for later explicitly selected hardware tests.
- No carrier/line-power or accessory-ID pin driving. Hardware must do that safely.
- No MFi provider. Extended Interface may reject unauthenticated requests.
- No Display Remote `0x03` implementation or actual Digital Audio `0x0A` stream in this serial prototype.
- No guarantee old Car Thing BSP supports a given USB-UART dongle until built-in driver and physical host-mode tests complete.
- Existing storage player is untouched and does not become the fallback automatically.

## Implementation

- `internal/iap/serial_linux.go`: port discovery, termios 8N1, UART frame sync/length/checksum, bounded nonblocking reads/writes and context-aware reception.
- `internal/iap/serial_linux_test.go`: unit packet framing/corruption/bounds tests.
- `internal/player/serial.go`: explicit serial remote and Extended Interface probes, no mount.
- `internal/player/app.go`: isolated state/actions/cleanup.
- `internal/player/view.go`: Connection screen controls.
- `firmware/direct/kernel.cfg`: FTDI, CP210x, PL2303, CH341 and USB CDC-ACM driver support.

This code is an **initial transport prototype**, not an MFi unlock or certified accessory.
