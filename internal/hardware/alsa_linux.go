//go:build linux && (amd64 || arm64)

package hardware

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

type alsaAPI struct {
	open    func(*uintptr, string, int32, int32) int32
	close   func(uintptr) int32
	params  func(uintptr, int32, int32, uint32, uint32, int32, uint32) int32
	read    func(uintptr, unsafe.Pointer, uintptr) int64
	write   func(uintptr, unsafe.Pointer, uintptr) int64
	recover func(uintptr, int32, int32) int32
}

var alsaOnce sync.Once
var alsa alsaAPI
var alsaErr error
var errNativeUnavailable = errors.New("native ALSA is unavailable")

func loadALSA() (*alsaAPI, error) {
	alsaOnce.Do(func() {
		defer func() {
			if v := recover(); v != nil {
				alsaErr = fmt.Errorf("%w: %v", errNativeUnavailable, v)
			}
		}()
		h, e := purego.Dlopen("libasound.so.2", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if e != nil {
			alsaErr = fmt.Errorf("%w: %v", errNativeUnavailable, e)
			return
		}
		// The library remains loaded for the lifetime of the process. No C-owned
		// pointers to Go memory are retained across calls.
		purego.RegisterLibFunc(&alsa.open, h, "snd_pcm_open")
		purego.RegisterLibFunc(&alsa.close, h, "snd_pcm_close")
		purego.RegisterLibFunc(&alsa.params, h, "snd_pcm_set_params")
		purego.RegisterLibFunc(&alsa.read, h, "snd_pcm_readi")
		purego.RegisterLibFunc(&alsa.write, h, "snd_pcm_writei")
		purego.RegisterLibFunc(&alsa.recover, h, "snd_pcm_recover")
	})
	if alsaErr != nil {
		return nil, alsaErr
	}
	return &alsa, nil
}

// startALSA routes PCM decoded by the iPod, or produces a low-level DAC test
// tone when capture is empty. Hardware endpoints are opened NONBLOCK so stopping
// the app cannot hang forever waiting for a disconnected USB audio device.
func startALSA(capture, playback string, finished func(error)) (func(), error) {
	api, e := loadALSA()
	if e != nil {
		return nil, e
	}
	var input, output uintptr
	if v := api.open(&output, playback, 0, 1); v < 0 {
		return nil, fmt.Errorf("open ALSA output %s: error %d", playback, v)
	}
	if v := api.params(output, 2, 3, 2, 44100, 1, 100000); v < 0 {
		api.close(output)
		return nil, fmt.Errorf("configure stereo S16_LE 44.1kHz output: ALSA error %d", v)
	}
	if capture != "" {
		if v := api.open(&input, capture, 1, 1); v < 0 {
			api.close(output)
			return nil, fmt.Errorf("open iPod capture %s: ALSA error %d", capture, v)
		}
		if v := api.params(input, 2, 3, 2, 44100, 1, 100000); v < 0 {
			api.close(input)
			api.close(output)
			return nil, fmt.Errorf("configure stereo S16_LE 44.1kHz capture: ALSA error %d", v)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e := pumpPCM(ctx, api, input, output)
		if input != 0 {
			api.close(input)
		}
		api.close(output)
		close(done)
		finished(e)
	}()
	return func() { cancel(); <-done }, nil
}
func pumpPCM(ctx context.Context, api *alsaAPI, input, output uintptr) error {
	const frames = 441 // 10ms stereo blocks
	b := make([]int16, frames*2)
	sample := 0
	toneEnd := time.Now().Add(3 * time.Second)
	wait := func() bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Millisecond):
			return true
		}
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		n := int64(frames)
		if input == 0 {
			if time.Now().After(toneEnd) {
				return nil
			}
			for i := 0; i < frames; i++ {
				v := int16(1300 * math.Sin(float64(sample)*2*math.Pi*440/44100))
				sample++
				b[i*2] = v
				b[i*2+1] = v
			}
		} else {
			n = api.read(input, unsafe.Pointer(&b[0]), frames)
			runtime.KeepAlive(b)
			if n == -11 {
				if !wait() {
					return nil
				}
				continue
			}
			if n < 0 {
				if r := api.recover(input, int32(n), 1); r < 0 {
					return fmt.Errorf("iPod capture stopped: ALSA error %d", r)
				}
				continue
			}
			if n > frames {
				return errors.New("ALSA returned too many input frames")
			}
		}
		for off := int64(0); off < n; {
			if ctx.Err() != nil {
				return nil
			}
			written := api.write(output, unsafe.Pointer(&b[off*2]), uintptr(n-off))
			runtime.KeepAlive(b)
			if written == -11 || written == 0 {
				if !wait() {
					return nil
				}
				continue
			}
			if written < 0 {
				if r := api.recover(output, int32(written), 1); r < 0 {
					return fmt.Errorf("DAC output stopped: ALSA error %d", r)
				}
				continue
			}
			if written > n-off {
				return errors.New("ALSA returned too many output frames")
			}
			off += written
		}
		if input == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
}
