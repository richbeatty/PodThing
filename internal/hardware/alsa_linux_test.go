//go:build linux && (amd64 || arm64)

package hardware

import (
	"testing"
	"time"
)

func TestNativeALSAOutputAndCancellation(t *testing.T) {
	// ALSA's null plugin exercises the real dynamic ABI, PCM setup, stereo
	// writes and cleanup, without sending a tone to any physical audio device.
	if _, e := loadALSA(); e != nil {
		t.Skipf("ALSA runtime unavailable: %v", e)
	}
	done := make(chan error, 1)
	stop, e := startALSA("", "null", func(e error) { done <- e })
	if e != nil {
		t.Fatal(e)
	}
	time.Sleep(35 * time.Millisecond)
	stop()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("native audio did not stop")
	}
}
func TestNativeALSACaptureRoute(t *testing.T) {
	if _, e := loadALSA(); e != nil {
		t.Skipf("ALSA runtime unavailable: %v", e)
	}
	done := make(chan error, 1)
	stop, e := startALSA("null", "null", func(e error) { done <- e })
	if e != nil {
		t.Fatal(e)
	}
	time.Sleep(20 * time.Millisecond)
	stop()
	if e = <-done; e != nil {
		t.Fatal(e)
	}
}
