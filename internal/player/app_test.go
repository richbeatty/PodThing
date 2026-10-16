package player

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func demoApp(t *testing.T) *App {
	t.Helper()
	a, e := New(true, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(a.Close)
	return a
}
func action(t *testing.T, a *App, name string, value int, text string) {
	t.Helper()
	if e := a.Action(Action{Name: name, Value: value, Text: text}); e != nil {
		t.Fatal(e)
	}
}
func TestDemoNavigationPlaybackAndBack(t *testing.T) {
	a := demoApp(t)
	if len(a.Snapshot().Entries) != 4 {
		t.Fatal("missing artists")
	}
	action(t, a, "open", 2, "")
	if a.Snapshot().Category != "Albums" {
		t.Fatal("artist drilldown")
	}
	action(t, a, "open", 0, "")
	if a.Snapshot().Category != "Songs" || len(a.Snapshot().Entries) != 4 {
		t.Fatal("album drilldown")
	}
	action(t, a, "open", 1, "")
	s := a.Snapshot()
	if s.Screen != "playing" || !s.Now.Playing || s.Now.Title != "One More Time" {
		t.Fatalf("playback: %+v", s.Now)
	}
	action(t, a, "back", 0, "")
	action(t, a, "back", 0, "")
	if a.Snapshot().Category != "Albums" {
		t.Fatal("back did not restore parent")
	}
	action(t, a, "preset4", 0, "")
	if a.Snapshot().Category != "Playlists" {
		t.Fatal("preset mapping")
	}
}
func TestSettingsPersistenceAndValidation(t *testing.T) {
	dir := t.TempDir()
	a, e := New(true, dir)
	if e != nil {
		t.Fatal(e)
	}
	action(t, a, "remap", 0, "")
	action(t, a, "invert", 0, "")
	a.Close()
	b, e := New(true, dir)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	if b.Snapshot().Settings.Buttons[0] != "Albums" || !b.Snapshot().Settings.InvertWheel {
		t.Fatal("settings not persisted")
	}
	if e = b.Action(Action{Name: "remap", Value: 5}); e == nil {
		t.Fatal("accepted invalid preset")
	}
	if e = os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = New(true, dir); e == nil {
		t.Fatal("silently overwrote invalid user configuration")
	}
}
func TestDemoCannotAccessHardwareAudio(t *testing.T) {
	a := demoApp(t)
	for _, name := range []string{"digital", "audio-start", "tone"} {
		if e := a.Action(Action{Name: name}); e == nil {
			t.Fatalf("demo allowed %s", name)
		}
	}
}
func TestHTTPActionsAndCrossOrigin(t *testing.T) {
	a := demoApp(t)
	frames := &Frames{}
	frames.PNG.Store([]byte{1, 2, 3})
	h := a.Handler(frames)
	for _, tt := range []struct {
		body, origin string
		status       int
	}{{`{"name":"navigate","text":"Songs"}`, "", 200}, {`{"name":"next"}`, "https://evil.invalid", 403}, {`{"name":"next","unknown":true}`, "", 400}, {`{"name":"next"} {}`, "", 400}, {`{"name":"nonsense"}`, "", 409}} {
		r := httptest.NewRequest("POST", "http://localhost/api/action", bytes.NewBufferString(tt.body))
		r.Header.Set("Origin", tt.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tt.status {
			t.Fatalf("%s => %d %s", tt.body, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "http://localhost/api/state", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var s Snapshot
	if e := json.Unmarshal(w.Body.Bytes(), &s); e != nil || !s.Demo || s.Category != "Songs" {
		t.Fatal("invalid API state")
	}
	r = httptest.NewRequest("GET", "http://localhost/api/report", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !json.Valid(w.Body.Bytes()) {
		t.Fatal("report invalid")
	}
}
func TestNativeAndConsoleUseSameTouchTargets(t *testing.T) {
	a := demoApp(t)
	action(t, a, "tap", 0, "")
	if e := a.Action(Action{Name: "tap", X: 220, Y: 104}); e != nil {
		t.Fatal(e)
	}
	if a.Snapshot().Category != "Albums" {
		t.Fatal("touch tab did not navigate")
	}
	p, e := NewPainter()
	if e != nil {
		t.Fatal(e)
	}
	img := p.Render(a.View())
	if img.Bounds().Dx() != 800 || img.Bounds().Dy() != 480 {
		t.Fatal("incorrect Car Thing geometry")
	}
}
