//go:build linux

// Package hardware talks to Linux usbfs directly. The resulting executable
// needs no libusb installation or architecture-specific USB SDK.
package hardware

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/richbeatty/PodThing/internal/iap"
)

type Endpoint struct {
	Address    byte   `json:"address"`
	Attributes byte   `json:"attributes"`
	MaxPacket  uint16 `json:"max_packet"`
}
type Interface struct {
	Number       byte       `json:"number"`
	Alternate    byte       `json:"alternate"`
	Class        byte       `json:"class"`
	Subclass     byte       `json:"subclass"`
	Protocol     byte       `json:"protocol"`
	ReportLength uint16     `json:"report_length,omitempty"`
	Endpoints    []Endpoint `json:"endpoints"`
}
type Configuration struct {
	Value      byte        `json:"value"`
	Interfaces []Interface `json:"interfaces"`
}
type Device struct {
	Name           string          `json:"name"`
	Product        string          `json:"product"`
	Vendor         string          `json:"vendor"`
	ProductID      string          `json:"product_id"`
	Bus            int             `json:"bus"`
	Address        int             `json:"address"`
	Active         int             `json:"active_configuration"`
	IPod           bool            `json:"ipod"`
	Configurations []Configuration `json:"configurations"`
	Error          string          `json:"error,omitempty"`
	SysPath        string          `json:"-"`
}

func read(path string) string { b, _ := os.ReadFile(path); return strings.TrimSpace(string(b)) }
func number(path string) int  { v, _ := strconv.Atoi(read(path)); return v }

func ParseDescriptors(b []byte) ([]Configuration, error) {
	var configs []Configuration
	ci, ii := -1, -1
	for off := 0; off < len(b); {
		if len(b)-off < 2 {
			return nil, errors.New("truncated USB descriptor header")
		}
		n := int(b[off])
		if n < 2 || off+n > len(b) {
			return nil, errors.New("invalid USB descriptor length")
		}
		d := b[off : off+n]
		off += n
		switch d[1] {
		case 2:
			if n < 9 {
				return nil, errors.New("short configuration descriptor")
			}
			configs = append(configs, Configuration{Value: d[5]})
			ci = len(configs) - 1
			ii = -1
		case 4:
			if n < 9 || ci < 0 {
				return nil, errors.New("invalid interface descriptor")
			}
			configs[ci].Interfaces = append(configs[ci].Interfaces, Interface{Number: d[2], Alternate: d[3], Class: d[5], Subclass: d[6], Protocol: d[7]})
			ii = len(configs[ci].Interfaces) - 1
		case 5:
			if n < 7 || ci < 0 || ii < 0 {
				return nil, errors.New("invalid endpoint descriptor")
			}
			in := &configs[ci].Interfaces[ii]
			in.Endpoints = append(in.Endpoints, Endpoint{d[2], d[3], binary.LittleEndian.Uint16(d[4:6]) & 0x7ff})
		case 0x21:
			if ci >= 0 && ii >= 0 && n >= 9 {
				for j := 0; j < int(d[5]) && 6+j*3+3 <= n; j++ {
					p := 6 + j*3
					if d[p] == 0x22 {
						configs[ci].Interfaces[ii].ReportLength = binary.LittleEndian.Uint16(d[p+1:])
					}
				}
			}
		}
	}
	return configs, nil
}
func ScanUSB() ([]Device, error) {
	paths, e := filepath.Glob("/sys/bus/usb/devices/*")
	if e != nil {
		return nil, e
	}
	out := []Device{}
	for _, path := range paths {
		vendor := read(path + "/idVendor")
		if vendor == "" {
			continue
		}
		d := Device{Name: filepath.Base(path), Product: read(path + "/product"), Vendor: vendor, ProductID: read(path + "/idProduct"), Bus: number(path + "/busnum"), Address: number(path + "/devnum"), Active: number(path + "/bConfigurationValue"), SysPath: path}
		d.IPod = vendor == "05ac" && (strings.Contains(strings.ToLower(d.Product), "ipod") || d.ProductID == "1261" || d.ProductID == "1260" || d.ProductID == "1209")
		b, e := os.ReadFile(path + "/descriptors")
		if e == nil {
			d.Configurations, e = ParseDescriptors(b)
		}
		if e != nil {
			d.Error = e.Error()
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func MountedIPod(sysPath string) error {
	root, e := filepath.EvalSymlinks(sysPath)
	if e != nil {
		return e
	}
	blocks, e := filepath.Glob("/sys/class/block/*")
	if e != nil {
		return e
	}
	mounts, e := os.ReadFile("/proc/self/mountinfo")
	if e != nil {
		return e
	}
	swaps, e := os.ReadFile("/proc/swaps")
	if e != nil {
		return e
	}
	for _, b := range blocks {
		real, e := filepath.EvalSymlinks(b)
		if e != nil {
			return e
		}
		if !strings.HasPrefix(real, root+"/") {
			continue
		}
		dev := read(b + "/dev")
		for _, line := range strings.Split(string(mounts), "\n") {
			f := strings.Fields(line)
			if len(f) > 4 && f[2] == dev {
				return fmt.Errorf("iPod storage is mounted at %s; unmount it before connecting", f[4])
			}
		}
		for _, line := range strings.Split(string(swaps), "\n") {
			f := strings.Fields(line)
			if len(f) > 0 && f[0] == "/dev/"+filepath.Base(b) {
				return errors.New("iPod storage is in use as swap")
			}
		}
	}
	return nil
}

type controlTransfer struct {
	RequestType uint8
	Request     uint8
	Value       uint16
	Index       uint16
	Length      uint16
	Timeout     uint32
	Data        uintptr
}
type disconnectClaim struct {
	Interface uint32
	Flags     uint32
	Driver    [256]byte
}
type usbIoctl struct {
	Interface int32
	Code      int32
	Data      uintptr
}
type urb struct {
	Type         uint8
	Endpoint     uint8
	Status       int32
	Flags        uint32
	Buffer       uintptr
	BufferLength int32
	ActualLength int32
	StartFrame   int32
	Packets      int32
	ErrorCount   int32
	Signal       uint32
	UserContext  uintptr
}

func ioctlCode(dir, nr, size uintptr) uintptr { return dir<<30 | size<<16 | 'U'<<8 | nr }
func ioctl(fd uintptr, req uintptr, arg unsafe.Pointer) (uintptr, error) {
	r, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg))
	if e != 0 {
		return r, e
	}
	return r, nil
}

type USB struct {
	file       *os.File
	iface      Interface
	defs       []iap.Report
	endpoint   Endpoint
	reassembly iap.Reassembler
	original   int
	changed    bool
	restore    bool
	device     Device
	log        func(string)
}

// accessoryInterface selects an advertised interrupt-IN HID interface, giving
// preference to an accessory configuration with a USB Audio interface.
 // This never guesses configuration numbers or writes iPod storage.
func accessoryInterface(d Device) (*Configuration, Interface) {
	var best *Configuration
	var iface Interface
	score := -1
	for i := range d.Configurations {
		cfg := &d.Configurations[i]
		hasAudio := false
		for _, candidate := range cfg.Interfaces {
			if candidate.Class == 1 { hasAudio = true }
		}
		for _, candidate := range cfg.Interfaces {
			if candidate.Class != 3 || candidate.Alternate != 0 { continue }
			hasInterrupt := false
			for _, ep := range candidate.Endpoints {
				if ep.Address&0x80 != 0 && ep.Attributes&3 == 3 && ep.MaxPacket > 0 {
					hasInterrupt = true
				}
			}
			if !hasInterrupt { continue }
			rank := 0
			if int(cfg.Value) == d.Active { rank += 1 }
			if hasAudio { rank += 2 }
			if rank > score { best, iface, score = cfg, candidate, rank }
		}
	}
	return best, iface
}

func OpenIPod(d Device, log func(string)) (*USB, error) {
	if !d.IPod {
		return nil, errors.New("selected USB device is not an iPod")
	}
	if d.Error != "" {
		return nil, errors.New(d.Error)
	}
	if e := MountedIPod(d.SysPath); e != nil {
		return nil, e
	}
	if log == nil {
		log = func(string) {}
	}
	config, in := accessoryInterface(d)
	if config == nil {
		return nil, fmt.Errorf("iPod USB %s (%s:%s) exposes no HID accessory interface in %d configurations; remains in storage mode", d.Name, d.Vendor, d.ProductID, len(d.Configurations))
	}
	log(fmt.Sprintf("STAGE USB: iPod %s config %d -> accessory config %d HID interface %d", d.Name, d.Active, config.Value, in.Number))
	f, e := os.OpenFile(fmt.Sprintf("/dev/bus/usb/%03d/%03d", d.Bus, d.Address), os.O_RDWR, 0)
	if e != nil {
		return nil, fmt.Errorf("open iPod USB device (root/device permission needed): %w", e)
	}
	u := &USB{file: f, iface: in, original: d.Active, device: d, log: log}
	ok := false
	defer func() {
		if !ok {
			// If the configuration switch failed, rebind original drivers.
			// Once in accessory mode, keep that mode selected on failure,
			// instead of toggling the iPod back to mass storage.
			if !u.changed {
				u.RestoreOriginalOnClose()
			}
			_ = u.Close()
		}
	}()
	if int(config.Value) != d.Active {
		// Only disconnect the known iPod's interfaces, after checking mounts.
		for _, cfg := range d.Configurations {
			if int(cfg.Value) != d.Active {
				continue
			}
			for _, v := range cfg.Interfaces {
				if v.Alternate != 0 {
					continue
				}
				q := usbIoctl{Interface: int32(v.Number), Code: int32(ioctlCode(0, 22, 0))}
				_, err := ioctl(f.Fd(), ioctlCode(3, 18, unsafe.Sizeof(q)), unsafe.Pointer(&q))
				if err != nil && !errors.Is(err, syscall.ENODATA) && !errors.Is(err, syscall.EINVAL) {
					return nil, fmt.Errorf("detach iPod interface %d: %w", v.Number, err)
				}
			}
		}
		v := uint32(config.Value)
		if _, e = ioctl(f.Fd(), ioctlCode(2, 5, 4), unsafe.Pointer(&v)); e != nil {
			return nil, fmt.Errorf("select iPod accessory configuration: %w", e)
		}
		u.changed = true
		log(fmt.Sprintf("Selected USB configuration %d (was %d)", v, d.Active))
		// usbfs may briefly expose a stale configuration after switching.
		// Do not issue another configuration switch or USB reset in a loop.
		// Report the state so we can distinguish enumeration from iAP errors.
		if state := read(d.SysPath + "/bConfigurationValue"); state != "" {
			log("USB configuration reported by kernel: " + state)
		}
	}
	log(fmt.Sprintf("STAGE USB: claiming HID interface %d", in.Number))
	claim := disconnectClaim{Interface: uint32(in.Number)}
	if _, e = ioctl(f.Fd(), ioctlCode(2, 27, unsafe.Sizeof(claim)), unsafe.Pointer(&claim)); e != nil {
		return nil, fmt.Errorf("claim iPod HID interface: %w", e)
	}
	length := int(in.ReportLength)
	if length == 0 {
		length = 4096
	}
	descriptor := make([]byte, length)
	n, e := u.control(0x81, 6, 0x2200, uint16(in.Number), descriptor)
	if e != nil {
		return nil, fmt.Errorf("get iPod HID descriptor: %w", e)
	}
	log(fmt.Sprintf("STAGE USB: parsing %d HID descriptor bytes", n))
	u.defs, e = iap.ParseReports(descriptor[:n])
	if e != nil {
		return nil, e
	}
	hasOutput := false
	for _, d := range u.defs {
		if d.Output {
			hasOutput = true
		}
	}
	if !hasOutput {
		return nil, errors.New("HID descriptor has no output reports")
	}
	for _, ep := range in.Endpoints {
		if ep.Address&0x80 != 0 && ep.Attributes&3 == 3 {
			u.endpoint = ep
			break
		}
	}
	if u.endpoint.MaxPacket == 0 {
		return nil, errors.New("invalid HID interrupt endpoint")
	}
	log(fmt.Sprintf("STAGE USB: ready HID interrupt endpoint 0x%02x, max packet %d; input/output reports %d", u.endpoint.Address, u.endpoint.MaxPacket, len(u.defs)))
	ok = true
	return u, nil
}
func (u *USB) control(kind, request byte, value, index uint16, b []byte) (int, error) {
	var data uintptr
	if len(b) > 0 {
		data = uintptr(unsafe.Pointer(&b[0]))
	}
	q := controlTransfer{kind, request, value, index, uint16(len(b)), 1500, data}
	n, e := ioctl(u.file.Fd(), ioctlCode(3, 0, unsafe.Sizeof(q)), unsafe.Pointer(&q))
	runtime.KeepAlive(b)
	return int(n), e
}
func (u *USB) Send(frame []byte) error {
	reports, e := iap.Reports(frame, u.defs)
	if e != nil {
		return e
	}
	for _, r := range reports {
		n, e := u.control(0x21, 9, 0x0200|uint16(r[0]), uint16(u.iface.Number), r)
		if e != nil {
			return fmt.Errorf("HID SET_REPORT: %w", e)
		}
		if n != len(r) {
			return errors.New("short HID SET_REPORT transfer")
		}
	}
	return nil
}
func (u *USB) Receive(ctx context.Context) ([]byte, error) {
	for {
		report, e := u.interrupt(ctx)
		if e != nil {
			return nil, e
		}
		if len(report) == 0 {
			continue
		}
		frame, e := u.reassembly.Feed(report)
		if e != nil {
			return nil, e
		}
		if frame != nil {
			return frame, nil
		}
	}
}
func (u *USB) interrupt(ctx context.Context) ([]byte, error) {
	b := make([]byte, max(1024, int(u.endpoint.MaxPacket)))
	q := &urb{Type: 1, Endpoint: u.endpoint.Address, Buffer: uintptr(unsafe.Pointer(&b[0])), BufferLength: int32(len(b))}
	// usbfs retains both pointers until REAPURB. Pin across the asynchronous ioctl.
	var pin runtime.Pinner
	pin.Pin(q)
	pin.Pin(&b[0])
	defer pin.Unpin()
	if _, e := ioctl(u.file.Fd(), ioctlCode(2, 10, unsafe.Sizeof(*q)), unsafe.Pointer(q)); e != nil {
		return nil, fmt.Errorf("submit HID interrupt: %w", e)
	}
	for {
		var completed uintptr
		_, e := ioctl(u.file.Fd(), ioctlCode(1, 13, unsafe.Sizeof(completed)), unsafe.Pointer(&completed))
		if e == nil {
			if completed != uintptr(unsafe.Pointer(q)) {
				return nil, errors.New("unexpected USB completion")
			}
			if q.Status < 0 {
				return nil, syscall.Errno(-q.Status)
			}
			if q.ActualLength < 0 || int(q.ActualLength) > len(b) {
				return nil, errors.New("invalid USB completion length")
			}
			return b[:q.ActualLength], nil
		}
		if !errors.Is(e, syscall.EAGAIN) {
			// The kernel may still retain the submitted buffer on a fatal reap
			// error. Closing usbfs cancels outstanding URBs before unpinning.
			_ = u.file.Close()
			return nil, e
		}
		select {
		case <-ctx.Done():
			_, _ = ioctl(u.file.Fd(), ioctlCode(0, 11, 0), unsafe.Pointer(q))
			var completed uintptr
			// A discarded URB must be reaped before its memory can be unpinned.
			_, reapErr := ioctl(u.file.Fd(), ioctlCode(1, 12, unsafe.Sizeof(completed)), unsafe.Pointer(&completed))
			if reapErr != nil {
				_ = u.file.Close()
			}
			return nil, ctx.Err()
		case <-time.After(3 * time.Millisecond):
		}
	}
}
// RestoreOriginalOnClose is deliberately called only for a user-initiated
// disconnect. An aborted handshake keeps the accessory configuration active.
func (u *USB) RestoreOriginalOnClose() { u.restore = true }

func (u *USB) Close() error {
	if u.file == nil {
		return nil
	}
	n := uint32(u.iface.Number)
	_, _ = ioctl(u.file.Fd(), ioctlCode(2, 16, 4), unsafe.Pointer(&n))
	if u.restore && u.changed {
		v := uint32(u.original)
		if _, e := ioctl(u.file.Fd(), ioctlCode(2, 5, 4), unsafe.Pointer(&v)); e != nil {
			u.log("Could not restore original USB configuration: " + e.Error())
		}
	}
	if u.restore {
		// Reattach the original kernel drivers only on deliberate
		// disconnect, not during a failed accessory handshake.
		for _, cfg := range u.device.Configurations {
			if int(cfg.Value) != u.original {
				continue
			}
			for _, in := range cfg.Interfaces {
				if in.Alternate != 0 {
					continue
				}
				q := usbIoctl{Interface: int32(in.Number), Code: int32(ioctlCode(0, 23, 0))}
				_, _ = ioctl(u.file.Fd(), ioctlCode(3, 18, unsafe.Sizeof(q)), unsafe.Pointer(&q))
			}
		}
	}
	e := u.file.Close()
	u.file = nil
	return e
}
