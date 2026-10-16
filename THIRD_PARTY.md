# References and dependencies

PodThing implements its own accessory-side framing, transport and application.
It does not incorporate Prelude, Rockbox, arduinaap or ipod-gadget source code.
The replacement image includes its own Linux runtime and boots independently.

Protocol and hardware references consulted:

- [finsprings/arduinaap](https://github.com/finsprings/arduinaap): legacy serial
  Advanced Remote commands, browsing behavior and hardware control semantics.
- [oandrew/ipod](https://github.com/oandrew/ipod): iAP HID framing, link-control
  fragmentation and digital-audio command definitions. This project emulates
  the iPod side; PodThing implements the accessory side.
- [mojyack/libiap](https://github.com/mojyack/libiap): command layouts and the
  distinction between commands requiring authentication and original commands.
- [usenocturne/prelude](https://github.com/usenocturne/prelude): Linux input
  mappings, 800×480 screen, rotated touch coordinates and current runtime.
- [JoeyEamigh/yocto-superbird](https://github.com/JoeyEamigh/yocto-superbird):
  current systemd/Weston runtime paths and host-mode limitations.
- Wayland core and xdg-shell protocol specifications; Linux usbdevice_fs.h UAPI.

Go module dependencies are pinned in go.mod/go.sum:

- `golang.org/x/image`: font rasterization and Go Regular/Go Bold fonts.
- `golang.org/x/text`: indirect dependency of the font implementation.
- `github.com/ebitengine/purego`: dynamically calls the OS's ALSA library on
  Linux ARM64/AMD64 without a C cross-compiler; MIT license.

The Go modules use BSD-style licensing except purego (MIT); the Go fonts have
their own notice. Complete module license notices are included under `licenses/` in
the distributed archive. All software is independent of Apple and Spotify;
product names identify supported/test targets, not endorsement.

Firmware components:

- Linux 7.0.2 (GPL-2.0-only) and 25 BSP patches/device tree: exact source
  commits are recorded in `firmware/direct/sources.json`. The unchanged patch
  series is available from the pinned yocto-superbird repository.
- Mainline U-Boot for Car Thing (GPL-2.0+), superbird-fip-tools and the
  boot-chain artifacts shipped by that pinned upstream. Their source URLs and
  exact commits are in the same manifest; upstream license/notices apply.
- Debian 13 ARM64 runtime packages: exact names, versions, architectures and
  artifact SHA-256 values accompany the ZIP. Each package's copyright/license
  notice is retained in `/usr/share/doc/<package>/copyright` in the image.
  Corresponding Debian package sources are available from
  https://deb.debian.org/debian/ and https://sources.debian.org/.
- The rootfs includes the Go/runtime/module/font notices under
  `/usr/share/podthing/licenses`. No Apple authentication certificate is shipped.

All PodThing code, configuration and build scripts are in this checkout. Use
`scripts/build-firmware.sh` with the pinned source manifest to rebuild the
firmware; the kernel config is also retained inside the image.
