//go:build linux

// Package display provides a small wl_shm/xdg-shell client. It uses the Weston
// already installed by Prelude, so PodThing does not need Chromium or a GPU SDK.
package display

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type global struct{ name, version uint32 }
type Window struct {
	conn     *net.UnixConn
	sendMu   sync.Mutex
	mu       sync.Mutex
	next     uint32
	handlers map[uint32]func(uint16, []byte)
	globals  map[string]global
	ready    chan struct{}
	done     chan struct{}
	err      error
	surface  uint32
	buffers  [2]uint32
	busy     [2]bool
	memory   []byte
	file     *os.File
	closed   bool
}

func words(v ...uint32) []byte {
	b := make([]byte, len(v)*4)
	for i, n := range v {
		binary.LittleEndian.PutUint32(b[i*4:], n)
	}
	return b
}
func str(s string) []byte {
	b := make([]byte, 4+(len(s)+1+3)&^3)
	binary.LittleEndian.PutUint32(b, uint32(len(s)+1))
	copy(b[4:], s)
	return b
}
func getstr(b []byte) (string, int, error) {
	if len(b) < 4 {
		return "", 0, io.ErrUnexpectedEOF
	}
	n := int(binary.LittleEndian.Uint32(b))
	if n < 1 || n > len(b)-4 {
		return "", 0, io.ErrUnexpectedEOF
	}
	return string(b[4 : 4+n-1]), 4 + (n+3)&^3, nil
}
func (w *Window) object(fn func(uint16, []byte)) uint32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.next++
	w.handlers[w.next] = fn
	return w.next
}
func (w *Window) send(id uint32, op uint16, b []byte, fd int) error {
	w.sendMu.Lock()
	defer w.sendMu.Unlock()
	h := words(id, uint32(len(b)+8)<<16|uint32(op))
	h = append(h, b...)
	var err error
	if fd >= 0 {
		var n int
		n, _, err = w.conn.WriteMsgUnix(h, syscall.UnixRights(fd), nil)
		if err == nil && n != len(h) {
			err = io.ErrShortWrite
		}
	} else {
		_, err = w.conn.Write(h)
	}
	return err
}
func (w *Window) fail(e error) {
	w.mu.Lock()
	if w.err == nil {
		w.err = e
	}
	w.mu.Unlock()
}
func (w *Window) readLoop() {
	defer close(w.done)
	for {
		h := make([]byte, 8)
		if _, e := io.ReadFull(w.conn, h); e != nil {
			w.fail(e)
			return
		}
		id := binary.LittleEndian.Uint32(h)
		v := binary.LittleEndian.Uint32(h[4:])
		n := int(v >> 16)
		if n < 8 || n > 65535 {
			w.fail(errors.New("invalid Wayland message size"))
			return
		}
		b := make([]byte, n-8)
		if _, e := io.ReadFull(w.conn, b); e != nil {
			w.fail(e)
			return
		}
		w.mu.Lock()
		fn := w.handlers[id]
		w.mu.Unlock()
		if fn != nil {
			fn(uint16(v), b)
		}
	}
}
func (w *Window) roundtrip() error {
	ch := make(chan struct{})
	var once sync.Once
	id := w.object(func(op uint16, b []byte) {
		if op == 0 {
			once.Do(func() { close(ch) })
		}
	})
	if e := w.send(1, 0, words(id), -1); e != nil {
		return e
	}
	select {
	case <-ch:
		return nil
	case <-w.done:
		return w.Err()
	case <-time.After(5 * time.Second):
		return errors.New("Wayland compositor did not answer")
	}
}
func (w *Window) bind(reg uint32, name string, maxVersion uint32, fn func(uint32, uint16, []byte)) (uint32, error) {
	w.mu.Lock()
	g, ok := w.globals[name]
	w.mu.Unlock()
	if !ok {
		return 0, fmt.Errorf("Wayland global %s unavailable", name)
	}
	id := w.object(nil)
	if fn != nil {
		w.mu.Lock()
		w.handlers[id] = func(op uint16, b []byte) { fn(id, op, b) }
		w.mu.Unlock()
	}
	b := append(words(g.name), str(name)...)
	b = append(b, words(min(g.version, maxVersion), id)...)
	return id, w.send(reg, 0, b, -1)
}
func Connect(socket string) (*Window, error) {
	if socket == "" {
		name := os.Getenv("WAYLAND_DISPLAY")
		if name == "" {
			name = "wayland-1"
		}
		root := os.Getenv("XDG_RUNTIME_DIR")
		if root == "" {
			root = "/run/weston"
		}
		socket = filepath.Join(root, name)
		if filepath.IsAbs(name) {
			socket = name
		}
	}
	c, e := net.DialUnix("unix", nil, &net.UnixAddr{Name: socket, Net: "unix"})
	if e != nil {
		return nil, fmt.Errorf("connect Weston: %w", e)
	}
	w := &Window{conn: c, next: 1, handlers: map[uint32]func(uint16, []byte){}, globals: map[string]global{}, ready: make(chan struct{}), done: make(chan struct{})}
	ok := false
	defer func() {
		if !ok {
			w.Close()
		}
	}()
	w.handlers[1] = func(op uint16, b []byte) {
		if op == 0 {
			w.fail(fmt.Errorf("Wayland protocol error: %x", b))
			_ = c.Close()
		}
	}
	reg := w.object(func(op uint16, b []byte) {
		if op != 0 || len(b) < 8 {
			return
		}
		name := binary.LittleEndian.Uint32(b)
		s, n, e := getstr(b[4:])
		if e == nil && len(b) >= 4+n+4 {
			w.mu.Lock()
			w.globals[s] = global{name, binary.LittleEndian.Uint32(b[4+n:])}
			w.mu.Unlock()
		}
	})
	go w.readLoop()
	if e = w.send(1, 1, words(reg), -1); e != nil {
		return nil, e
	}
	if e = w.roundtrip(); e != nil {
		return nil, e
	}
	compositor, e := w.bind(reg, "wl_compositor", 4, nil)
	if e != nil {
		return nil, e
	}
	shm, e := w.bind(reg, "wl_shm", 1, nil)
	if e != nil {
		return nil, e
	}
	shell, e := w.bind(reg, "xdg_wm_base", 1, func(id uint32, op uint16, b []byte) {
		if op == 0 && len(b) == 4 {
			if e := w.send(id, 3, b, -1); e != nil {
				w.fail(e)
			}
		}
	})
	if e != nil {
		return nil, e
	}
	w.surface = w.object(nil)
	if e = w.send(compositor, 0, words(w.surface), -1); e != nil {
		return nil, e
	}
	var configured sync.Once
	var xdg uint32
	xdg = w.object(func(op uint16, b []byte) {
		if op == 0 && len(b) == 4 {
			if e := w.send(xdg, 4, b, -1); e != nil {
				w.fail(e)
			}
			configured.Do(func() { close(w.ready) })
		}
	})
	if e = w.send(shell, 2, words(xdg, w.surface), -1); e != nil {
		return nil, e
	}
	top := w.object(func(op uint16, b []byte) {
		if op == 1 {
			w.fail(errors.New("window closed"))
			_ = c.Close()
		}
		if op == 0 && len(b) >= 8 {
			x, y := binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint32(b[4:])
			if x != 0 && y != 0 && (x != 800 || y != 480) {
				w.fail(fmt.Errorf("display configured %dx%d; PodThing needs 800x480", x, y))
			}
		}
	})
	for _, q := range []struct {
		id uint32
		op uint16
		b  []byte
	}{{xdg, 1, words(top)}, {top, 2, str("PodThing")}, {top, 3, str("org.podthing.player")}, {top, 7, words(800, 480)}, {top, 8, words(800, 480)}, {top, 11, words(0)}, {w.surface, 6, nil}} {
		if e = w.send(q.id, q.op, q.b, -1); e != nil {
			return nil, e
		}
	}
	select {
	case <-w.ready:
	case <-w.done:
		return nil, w.Err()
	case <-time.After(5 * time.Second):
		return nil, errors.New("Wayland window configure timeout")
	}
	if e = w.Err(); e != nil {
		return nil, e
	}
	w.file, e = os.CreateTemp("", "podthing-shm-*")
	if e != nil {
		return nil, e
	}
	_ = os.Remove(w.file.Name())
	size := 800 * 480 * 4 * 2
	if e = w.file.Truncate(int64(size)); e != nil {
		return nil, e
	}
	w.memory, e = syscall.Mmap(int(w.file.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if e != nil {
		return nil, e
	}
	pool := w.object(nil)
	if e = w.send(shm, 0, words(pool, uint32(size)), int(w.file.Fd())); e != nil {
		return nil, e
	}
	for i := 0; i < 2; i++ {
		i := i
		id := w.object(func(op uint16, b []byte) {
			if op == 0 {
				w.mu.Lock()
				w.busy[i] = false
				w.mu.Unlock()
			}
		})
		w.buffers[i] = id
		if e = w.send(pool, 0, words(id, uint32(i*800*480*4), 800, 480, 800*4, 1), -1); e != nil {
			return nil, e
		}
	}
	if e = w.send(pool, 1, nil, -1); e != nil {
		return nil, e
	}
	ok = true
	return w, nil
}
func (w *Window) Err() error { w.mu.Lock(); defer w.mu.Unlock(); return w.err }

// Present receives Go RGBA pixels and converts to wl_shm XRGB8888 (B,G,R,X).
func (w *Window) Present(rgba []byte) error {
	if len(rgba) != 800*480*4 {
		return errors.New("invalid frame size")
	}
	w.mu.Lock()
	if w.err != nil {
		e := w.err
		w.mu.Unlock()
		return e
	}
	slot := -1
	for i := range w.busy {
		if !w.busy[i] {
			slot = i
			w.busy[i] = true
			break
		}
	}
	w.mu.Unlock()
	if slot < 0 {
		return nil
	}
	b := w.memory[slot*len(rgba) : (slot+1)*len(rgba)]
	for i := 0; i < len(b); i += 4 {
		b[i], b[i+1], b[i+2], b[i+3] = rgba[i+2], rgba[i+1], rgba[i], 255
	}
	for _, q := range []struct {
		op uint16
		b  []byte
	}{{1, words(w.buffers[slot], 0, 0)}, {2, words(0, 0, 800, 480)}, {6, nil}} {
		if e := w.send(w.surface, q.op, q.b, -1); e != nil {
			return e
		}
	}
	return nil
}
func (w *Window) Close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.mu.Unlock()
	_ = w.conn.Close()
	<-w.done
	if w.memory != nil {
		_ = syscall.Munmap(w.memory)
	}
	if w.file != nil {
		_ = w.file.Close()
	}
}
