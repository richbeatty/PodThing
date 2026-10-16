//go:build linux

package hardware

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type PCM struct {
	Card    int    `json:"card"`
	Device  int    `json:"device"`
	Name    string `json:"name"`
	ID      string `json:"id"`
	Capture bool   `json:"capture"`
	IPod    bool   `json:"ipod"`
	USB     bool   `json:"usb"`
	Formats string `json:"formats,omitempty"`
}

var pcmName = regexp.MustCompile(`^pcmC([0-9]+)D([0-9]+)([cp])$`)

func ScanAudio() []PCM {
	paths, _ := filepath.Glob("/sys/class/sound/pcmC*D*")
	out := []PCM{}
	for _, p := range paths {
		m := pcmName.FindStringSubmatch(filepath.Base(p))
		if m == nil {
			continue
		}
		card, _ := strconv.Atoi(m[1])
		dev, _ := strconv.Atoi(m[2])
		base := fmt.Sprintf("/proc/asound/card%d", card)
		id := read(base + "/id")
		real, _ := filepath.EvalSymlinks(p)
		stream := read(base + "/stream0")
		name := id
		if line, _, ok := strings.Cut(stream, "\n"); ok {
			name = line
		}
		vendor := read(fmt.Sprintf("/sys/class/sound/card%d/device/../idVendor", card))
		prod := read(fmt.Sprintf("/sys/class/sound/card%d/device/../product", card))
		out = append(out, PCM{card, dev, name, fmt.Sprintf("hw:%d,%d", card, dev), m[3] == "c", vendor == "05ac" && strings.Contains(strings.ToLower(prod+" "+id+" "+name), "ipod"), strings.Contains(real, "/usb") || stream != "", stream})
	}
	return out
}

type Audio struct {
	mu         sync.Mutex
	cmd        *exec.Cmd
	nativeStop func()
	status     string
	stderr     *boundedLog
	generation uint64
}
type boundedLog struct {
	mu sync.Mutex
	b  []byte
}

func (b *boundedLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.b = append(b.b, p...)
	if len(b.b) > 4096 {
		b.b = b.b[len(b.b)-4096:]
	}
	return len(p), nil
}
func (b *boundedLog) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.b) }
func (a *Audio) Status() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status == "" {
		return "Stopped"
	}
	return a.status
}
func (a *Audio) stopLocked() {
	a.generation++
	if a.nativeStop != nil {
		a.nativeStop()
		a.nativeStop = nil
	}
	if a.cmd != nil && a.cmd.Process != nil {
		_ = syscall.Kill(-a.cmd.Process.Pid, syscall.SIGTERM)
	}
	a.cmd = nil
	a.status = "Stopped"
}
func (a *Audio) Stop() { a.mu.Lock(); defer a.mu.Unlock(); a.stopLocked() }
func (a *Audio) Start(capture, playback string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopLocked()
	if capture == playback {
		return errors.New("capture and output must be different devices")
	}
	var source, sink *PCM
	for _, p := range ScanAudio() {
		p := p
		if p.ID == capture && p.Capture && p.IPod {
			source = &p
		}
		if p.ID == playback && !p.Capture && p.USB && !p.IPod {
			sink = &p
		}
	}
	if source == nil {
		return errors.New("no selected iPod USB capture device; digital audio may not be unlocked")
	}
	if sink == nil {
		return errors.New("select a USB DAC playback device")
	}
	gen := a.generation
	stop, nativeErr := startALSA(capture, playback, func(e error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if gen != a.generation {
			return
		}
		if e != nil {
			a.status = "Audio route failed: " + e.Error()
		} else {
			a.status = "Audio route ended"
		}
	})
	if nativeErr == nil {
		a.nativeStop = stop
		a.status = "Native USB audio route running (confirm sound at AUX)"
		return nil
	}
	if !errors.Is(nativeErr, errNativeUnavailable) {
		return nativeErr
	}
	// alsaloop is in the ALSA utilities family. GStreamer is the alternative
	// on Prelude images which omit the alsaloop subpackage or gst-launch CLI.
	var cmd *exec.Cmd
	if path, e := exec.LookPath("alsaloop"); e == nil {
		cmd = exec.Command(path, "-C", capture, "-P", playback, "-r", "44100", "-c", "2", "-f", "S16_LE", "-t", "50000", "-S", "1")
	} else if path, e := exec.LookPath("gst-launch-1.0"); e == nil {
		cmd = exec.Command(path, "-q", "alsasrc", "device="+capture, "!", "audioconvert", "!", "audioresample", "!", "alsasink", "device="+playback, "sync=false")
	} else {
		return errors.New("audio routing needs alsaloop or gst-launch-1.0; neither is installed")
	}
	a.stderr = &boundedLog{}
	cmd.Stderr = a.stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if e := cmd.Start(); e != nil {
		return e
	}
	a.cmd = cmd
	a.status = "Starting USB audio route"
	gen = a.generation
	log := a.stderr
	go func() {
		e := cmd.Wait()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.generation != gen {
			return
		}
		a.cmd = nil
		if e != nil {
			a.status = "Audio route failed: " + e.Error() + " " + strings.TrimSpace(log.String())
		} else {
			a.status = "Audio route ended"
		}
	}()
	go func() {
		time.Sleep(700 * time.Millisecond)
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.generation == gen && a.cmd == cmd {
			a.status = "USB audio route running (confirm sound at AUX)"
		}
	}()
	return nil
}

// Tone exercises only the selected DAC. It makes no claim about iPod audio.
func (a *Audio) Tone(playback string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopLocked()
	valid := false
	for _, p := range ScanAudio() {
		if p.ID == playback && !p.Capture && p.USB && !p.IPod {
			valid = true
		}
	}
	if !valid {
		return errors.New("select a USB DAC output first")
	}
	gen := a.generation
	stop, nativeErr := startALSA("", playback, func(e error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if gen != a.generation {
			return
		}
		if e != nil {
			a.status = "DAC test failed: " + e.Error()
		} else {
			a.status = "DAC tone finished; confirm whether you heard it"
		}
	})
	if nativeErr == nil {
		a.nativeStop = stop
		a.status = "DAC test tone playing (3 seconds)"
		return nil
	}
	if !errors.Is(nativeErr, errNativeUnavailable) {
		return nativeErr
	}
	path, e := exec.LookPath("speaker-test")
	if e != nil {
		return errors.New("speaker-test is not installed")
	}
	cmd := exec.Command(path, "-D", playback, "-t", "sine", "-f", "440", "-c", "2", "-l", "1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	a.stderr = &boundedLog{}
	cmd.Stderr = a.stderr
	if e = cmd.Start(); e != nil {
		return e
	}
	a.cmd = cmd
	a.status = "DAC test tone playing"
	gen = a.generation
	log := a.stderr
	go func() {
		timer := time.AfterFunc(3*time.Second, func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) })
		e := cmd.Wait()
		timer.Stop()
		a.mu.Lock()
		defer a.mu.Unlock()
		if gen != a.generation {
			return
		}
		a.cmd = nil
		if e != nil && len(bytes.TrimSpace([]byte(log.String()))) > 0 {
			a.status = "DAC test: " + log.String()
		} else {
			a.status = "DAC tone finished; confirm whether you heard it"
		}
	}()
	return nil
}

func CommandVersion(name string) string {
	path, e := exec.LookPath(name)
	if e != nil {
		return "not installed"
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b, _ := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	scan := bufio.NewScanner(bytes.NewReader(b))
	if scan.Scan() {
		return scan.Text()
	}
	return path
}
func HostRole() string {
	p, _ := filepath.Glob("/sys/class/usb_role/*/role")
	for _, v := range p {
		if b, e := os.ReadFile(v); e == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return "not reported"
}

// SetHostRole uses the BSP's supported role-switch helper. It does not poke
// charger/mux registers or change persistent boot policy.
func SetHostRole(host bool) error {
	path, e := exec.LookPath("superbird-usb-role")
	if e != nil {
		return errors.New("this OS does not provide the supported USB role-switch helper")
	}
	args := []string{"to-device"}
	if host {
		args = []string{"to-host", "--no-revert"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if e != nil {
		return fmt.Errorf("USB role change: %w: %s", e, strings.TrimSpace(string(b)))
	}
	return nil
}
