//go:build linux

package hardware

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type Input struct {
	Action string
	Value  int
	X, Y   int
}

// ReadInputs discovers by kernel name rather than unstable event numbers.
// PodThing owns these inputs while Prelude's projection daemon is stopped.
func ReadInputs(ctx context.Context, emit func(Input), log func(string)) {
	paths, _ := filepath.Glob("/sys/class/input/event*")
	for _, p := range paths {
		name := read(p + "/device/name")
		if name != "gpio-keys-polled" && name != "rotary" && name != "tlsc6x" {
			continue
		}
		path := "/dev/input/" + filepath.Base(p)
		go readInput(ctx, path, name, emit, log)
	}
}
func readInput(ctx context.Context, path, name string, emit func(Input), log func(string)) {
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if e != nil {
		log("Input: " + e.Error())
		return
	}
	defer f.Close()
	grab := int32(1)
	request := uintptr(1<<30 | 4<<16 | 'E'<<8 | 0x90)
	if _, e = ioctl(f.Fd(), request, unsafe.Pointer(&grab)); e != nil {
		log("Cannot claim " + name + ": " + e.Error())
		return
	}
	defer func() { v := int32(0); _, _ = ioctl(f.Fd(), request, unsafe.Pointer(&v)) }()
	log("Input ready: " + name)
	size := int(unsafe.Sizeof(syscall.Timeval{})) + 8
	b := make([]byte, size)
	slot, x, y := 0, 0, 0
	down, started, ended, validX, validY := false, false, false, false, false
	startX, startY := 0, 0
	dropped := false
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, e := syscall.Read(int(f.Fd()), b)
		if e == syscall.EAGAIN {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Millisecond):
			}
			continue
		}
		if e != nil {
			log("Input disconnected: " + name + ": " + e.Error())
			return
		}
		if n != size {
			log(fmt.Sprintf("Short input event from %s: %d", name, n))
			return
		}
		off := size - 8
		kind, code, value := binary.LittleEndian.Uint16(b[off:]), binary.LittleEndian.Uint16(b[off+2:]), int(int32(binary.LittleEndian.Uint32(b[off+4:])))
		if kind == 0 && code == 3 {
			dropped = true
			down = false
			started = false
			ended = false
			continue
		}
		if dropped {
			if kind == 0 && code == 0 {
				dropped = false
			}
			continue
		}
		if name == "rotary" && kind == 2 && code == 6 {
			emit(Input{Action: "scroll", Value: value})
			continue
		}
		if name == "gpio-keys-polled" && kind == 1 && value == 1 {
			actions := map[uint16]string{2: "preset1", 3: "preset2", 4: "preset3", 5: "preset4", 50: "settings", 1: "back", 28: "select"}
			if a := actions[code]; a != "" {
				emit(Input{Action: a})
			}
			continue
		}
		if name != "tlsc6x" {
			continue
		}
		if kind == 3 && code == 0x2f {
			slot = value
			continue
		}
		if kind == 3 && slot != 0 {
			continue
		}
		if kind == 3 {
			switch code {
			case 0x39:
				if value >= 0 {
					down = true
					started = false
					ended = false
					validX = false
					validY = false
				} else {
					ended = true
				}
			case 0x35:
				x = value
				validX = true
			case 0x36:
				y = value
				validY = true
			}
		}
		if kind == 0 && code == 0 {
			if down && !started && validX && validY {
				startX, startY = y, 479-x
				started = true
			}
			if ended {
				if started {
					lx, ly := y, 479-x
					if x >= 0 && x < 480 && y >= 0 && y < 800 {
						dy := ly - startY
						if dy > 35 || dy < -35 {
							emit(Input{Action: "scroll", Value: -dy / 48})
						} else if abs(lx-startX) < 35 {
							emit(Input{Action: "tap", X: lx, Y: ly})
						}
					}
				}
				down = false
				ended = false
				started = false
			}
		}
	}
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func InputNames() []string {
	paths, _ := filepath.Glob("/sys/class/input/event*")
	var out []string
	for _, p := range paths {
		n := read(p + "/device/name")
		if n != "" {
			out = append(out, strings.TrimSpace(filepath.Base(p)+": "+n))
		}
	}
	return out
}
