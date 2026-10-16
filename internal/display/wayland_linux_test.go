package display

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A protocol-level compositor checks requests against the core/xdg-shell wire
// contract and reads the actual SCM_RIGHTS-backed shared-memory pixel buffer.
// This is not a substitute for a real Weston/device test.
func TestWaylandHandshakeAndSharedMemoryFrame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wayland-test")
	listener, e := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	result := make(chan error, 1)
	pixel := make(chan []byte, 1)
	go func() {
		c, e := listener.AcceptUnix()
		if e != nil {
			result <- e
			return
		}
		defer c.Close()
		result <- fakeCompositor(c, pixel)
	}()
	w, e := Connect(path)
	if e != nil {
		t.Fatal(e)
	}
	defer w.Close()
	rgba := make([]byte, 800*480*4)
	rgba[0], rgba[1], rgba[2], rgba[3] = 0x11, 0x22, 0x33, 0xff
	if e = w.Present(rgba); e != nil {
		t.Fatal(e)
	}
	select {
	case p := <-pixel:
		if len(p) != 4 || p[0] != 0x33 || p[1] != 0x22 || p[2] != 0x11 || p[3] != 0xff {
			t.Fatalf("wrong XRGB pixel: %x", p)
		}
	case e := <-result:
		t.Fatalf("compositor: %v", e)
	case <-time.After(5 * time.Second):
		t.Fatal("no shared-memory frame committed")
	}
	if e = w.roundtrip(); e != nil {
		t.Fatal(e)
	}
	w.Close()
	select {
	case e := <-result:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("compositor did not close")
	}
}
func fakeCompositor(c *net.UnixConn, pixel chan []byte) error {
	objects := map[uint32]string{1: "wl_display"}
	pools := map[uint32]int{}
	buffers := map[uint32]int64{}
	bufferPools := map[uint32]uint32{}
	pendingFDs := []int{}
	allFDs := []int{}
	defer func() {
		for _, fd := range allFDs {
			_ = syscall.Close(fd)
		}
	}()
	var saved []byte
	var xdg, top, surface, attached uint32
	configured, acked := false, false
	send := func(id uint32, op uint16, b []byte) error {
		h := words(id, uint32(len(b)+8)<<16|uint32(op))
		_, e := c.Write(append(h, b...))
		return e
	}
	for {
		buf := make([]byte, 65536)
		oob := make([]byte, 256)
		n, on, _, _, e := c.ReadMsgUnix(buf, oob)
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		msgs, e := syscall.ParseSocketControlMessage(oob[:on])
		if e != nil {
			return e
		}
		for _, m := range msgs {
			fds, e := syscall.ParseUnixRights(&m)
			if e != nil {
				return e
			}
			pendingFDs = append(pendingFDs, fds...)
			allFDs = append(allFDs, fds...)
		}
		saved = append(saved, buf[:n]...)
		for len(saved) >= 8 {
			object := binary.LittleEndian.Uint32(saved)
			word := binary.LittleEndian.Uint32(saved[4:])
			size := int(word >> 16)
			op := uint16(word)
			if size < 8 {
				return fmt.Errorf("invalid client message size")
			}
			if len(saved) < size {
				break
			}
			b := saved[8:size]
			u := func(off int) uint32 {
				if off+4 > len(b) {
					return 0
				}
				return binary.LittleEndian.Uint32(b[off:])
			}
			switch objects[object] {
			case "wl_display":
				switch op {
				case 1:
					reg := u(0)
					objects[reg] = "wl_registry"
					for i, name := range []string{"wl_compositor", "wl_shm", "xdg_wm_base"} {
						v := uint32(1)
						if i == 0 {
							v = 4
						}
						data := append(words(uint32(i+1)), str(name)...)
						data = append(data, words(v)...)
						if e = send(reg, 0, data); e != nil {
							return e
						}
					}
				case 0:
					if e = send(u(0), 0, words(123)); e != nil {
						return e
					}
				}
			case "wl_registry":
				name, used, e := getstr(b[4:])
				if e != nil {
					return e
				}
				id := binary.LittleEndian.Uint32(b[4+used+4:])
				objects[id] = name
			case "wl_compositor":
				if op == 0 {
					surface = u(0)
					objects[surface] = "wl_surface"
				}
			case "xdg_wm_base":
				if op == 2 {
					xdg = u(0)
					if u(4) != surface {
						return fmt.Errorf("wrong xdg surface")
					}
					objects[xdg] = "xdg_surface"
				}
			case "xdg_surface":
				if op == 1 {
					top = u(0)
					objects[top] = "xdg_toplevel"
				}
				if op == 4 {
					if u(0) != 77 {
						return fmt.Errorf("wrong configure serial")
					}
					acked = true
				}
			case "xdg_toplevel":
				if op == 2 {
					s, _, e := getstr(b)
					if e != nil || s != "PodThing" {
						return fmt.Errorf("wrong window title")
					}
				}
			case "wl_shm":
				if op == 0 {
					if len(pendingFDs) == 0 {
						return fmt.Errorf("create_pool missing fd")
					}
					if u(4) != 800*480*4*2 {
						return fmt.Errorf("wrong pool size")
					}
					id := u(0)
					objects[id] = "wl_shm_pool"
					pools[id] = pendingFDs[0]
					pendingFDs = pendingFDs[1:]
				}
			case "wl_shm_pool":
				if op == 0 {
					id := u(0)
					if u(8) != 800 || u(12) != 480 || u(16) != 3200 || u(20) != 1 {
						return fmt.Errorf("invalid XRGB8888 buffer geometry")
					}
					objects[id] = "wl_buffer"
					buffers[id] = int64(u(4))
					bufferPools[id] = object
				}
			case "wl_surface":
				switch op {
				case 1:
					attached = u(0)
				case 6:
					if !configured {
						configured = true
						if e = send(top, 0, append(words(800, 480), words(0)...)); e != nil {
							return e
						}
						if e = send(xdg, 0, words(77)); e != nil {
							return e
						}
					} else if attached != 0 {
						if !acked {
							return fmt.Errorf("frame committed before ack_configure")
						}
						p := make([]byte, 4)
						if _, e = syscall.Pread(pools[bufferPools[attached]], p, buffers[attached]); e != nil {
							return e
						}
						pixel <- p
						if e = send(attached, 0, nil); e != nil {
							return e
						}
						attached = 0
					}
				}
			}
			saved = saved[size:]
		}
	}
}

func TestMissingCompositorFailsClearly(t *testing.T) {
	_, e := Connect(filepath.Join(t.TempDir(), "missing"))
	if e == nil {
		t.Fatal("connected to missing compositor")
	}
}
