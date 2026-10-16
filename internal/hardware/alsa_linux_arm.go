//go:build linux && arm

package hardware

import "errors"

var errNativeUnavailable = errors.New("native ALSA is unavailable on this architecture")

func startALSA(capture, playback string, finished func(error)) (func(), error) {
	return nil, errNativeUnavailable
}
