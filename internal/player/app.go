package player

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/richbeatty/PodThing/internal/hardware"
	"github.com/richbeatty/PodThing/internal/iap"
)

var Version = "0.1.0-test" // Linked to the firmware revision.
const PageSize = 25

var Categories = []string{"Artists", "Albums", "Songs", "Playlists"}
var codes = map[string]byte{"Artists": 2, "Albums": 3, "Songs": 5, "Playlists": 1}
var bindings = []string{"Artists", "Albums", "Songs", "Playlists", "Now Playing", "Play / Pause", "Previous", "Next", "Shuffle", "Repeat", "Diagnostics"}

type Settings struct {
	Buttons     [4]string `json:"buttons"`
	InvertWheel bool      `json:"invert_wheel"`
	Capture     string    `json:"capture"`
	Output      string    `json:"output"`
}
type Snapshot struct {
	Version     string            `json:"version"`
	Demo        bool              `json:"demo"`
	Connected   bool              `json:"connected"`
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Error       string            `json:"error,omitempty"`
	Busy        bool              `json:"busy"`
	Screen      string            `json:"screen"`
	Category    string            `json:"category"`
	Path        []iap.Selection   `json:"path"`
	Entries     []iap.Entry       `json:"entries"`
	Offset      uint32            `json:"offset"`
	Total       uint32            `json:"total"`
	Focus       int               `json:"focus"`
	Now         iap.NowPlaying    `json:"now"`
	Accent      [3]uint8         `json:"accent"`
	HasArtwork  bool             `json:"has_artwork"`
	Settings    Settings          `json:"settings"`
	USB         []hardware.Device `json:"usb"`
	Audio       []hardware.PCM    `json:"audio"`
	AudioStatus string            `json:"audio_status"`
	Inputs      []string          `json:"inputs"`
	HostRole    string            `json:"host_role"`
	Log         []string          `json:"log"`
	Updated     string            `json:"updated"`
}
type Action struct {
	Name  string `json:"name"`
	Value int    `json:"value,omitempty"`
	Text  string `json:"text,omitempty"`
	X     int    `json:"x,omitempty"`
	Y     int    `json:"y,omitempty"`
}
type App struct {
	mu        sync.Mutex
	op        sync.Mutex
	s         Snapshot
	client    *iap.Client
	audio     hardware.Audio
	stateDir  string
	demoTrack int
	closed    bool
}

func New(demo bool, stateDir string) (*App, error) {
	if e := os.MkdirAll(stateDir, 0700); e != nil {
		return nil, e
	}
	a := &App{stateDir: stateDir, s: Snapshot{Version: Version, Demo: demo, Status: "Connect your iPod, then choose Connect", Screen: "library", Category: "Artists", Focus: 0, Accent: [3]uint8{96, 154, 255}, Settings: Settings{Buttons: [4]string{"Artists", "Albums", "Songs", "Playlists"}}}}
	b, e := os.ReadFile(filepath.Join(stateDir, "settings.json"))
	if e == nil {
		if e = json.Unmarshal(b, &a.s.Settings); e != nil {
			return nil, fmt.Errorf("read saved settings: %w", e)
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	for i, v := range a.s.Settings.Buttons {
		valid := false
		for _, allowed := range bindings {
			if v == allowed {
				valid = true
			}
		}
		if !valid {
			a.s.Settings.Buttons[i] = Categories[i]
		}
	}
	if demo {
		a.s.Connected = true
		a.s.Name = "Demo iPod"
		a.s.Status = "Demo mode — no physical iPod or audio"
		a.demoBrowse()
		a.s.Now = demoNow(0, false)
	}
	a.Scan()
	return a, nil
}
func (a *App) Log(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.s.Log = append(a.s.Log, time.Now().Format("15:04:05")+" "+s)
	if len(a.s.Log) > 120 {
		a.s.Log = a.s.Log[len(a.s.Log)-120:]
	}
}
func (a *App) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.s
	s.Path = append([]iap.Selection{}, s.Path...)
	s.Entries = append([]iap.Entry{}, s.Entries...)
	s.USB = append([]hardware.Device{}, s.USB...)
	s.Audio = append([]hardware.PCM{}, s.Audio...)
	s.Log = append([]string{}, s.Log...)
	s.AudioStatus = a.audio.Status()
	s.Updated = time.Now().UTC().Format(time.RFC3339)
	return s
}
func (a *App) Scan() {
	u, e := hardware.ScanUSB()
	p := hardware.ScanAudio()
	a.mu.Lock()
	a.s.USB = u
	a.s.Audio = p
	a.s.Inputs = hardware.InputNames()
	a.s.HostRole = hardware.HostRole()
	a.mu.Unlock()
	if e != nil {
		a.Log("USB scan: " + e.Error())
	}
}
func (a *App) save() error {
	a.mu.Lock()
	b, e := json.MarshalIndent(a.s.Settings, "", "  ")
	a.mu.Unlock()
	if e != nil {
		return e
	}
	tmp := filepath.Join(a.stateDir, "settings.json.tmp")
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, filepath.Join(a.stateDir, "settings.json"))
}
func (a *App) run(fn func() error) error {
	if !a.op.TryLock() {
		return errors.New("iPod operation in progress; please wait")
	}
	defer a.op.Unlock()
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return errors.New("PodThing is shutting down")
	}
	a.s.Busy = true
	a.s.Error = ""
	a.mu.Unlock()
	e := fn()
	a.mu.Lock()
	a.s.Busy = false
	if e != nil {
		a.s.Error = e.Error()
	}
	a.mu.Unlock()
	if e != nil {
		a.Log(e.Error())
	}
	return e
}
func (a *App) Close() {
	a.op.Lock()
	defer a.op.Unlock()
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
	a.audio.Stop()
	if a.client != nil {
		_ = a.client.Close()
		a.client = nil
	}
}
func (a *App) connect() error {
	if a.s.Demo {
		return nil
	}
	// Pressing Connect twice must not disconnect an already active iPod.
	if a.client != nil {
		return nil
	}
	a.Scan()
	s := a.Snapshot()
	a.mu.Lock()
	a.s.Connected = false
	a.s.Name = ""
	a.s.Entries = nil
	a.s.Total = 0
	a.s.Status = "Looking for iPod USB accessory mode"
	a.mu.Unlock()
	var found []hardware.Device
	for _, d := range s.USB {
		if d.IPod {
			found = append(found, d)
		}
	}
	if len(found) != 1 {
		return fmt.Errorf("found %d iPods; connect exactly one iPod using a data cable and a powered OTG hub", len(found))
	}
	w, e := hardware.OpenIPod(found[0], a.Log)
	if e != nil {
		return e
	}
	c := iap.New(w, a.Log)
	name, e := c.Handshake()
	if e != nil {
		_ = c.Abort()
		return e
	}
	a.client = c
	a.mu.Lock()
	a.s.Connected = true
	a.s.Name = name
	a.s.Status = "iPod control connected; audio must be tested separately"
	a.s.Path = nil
	a.s.Offset = 0
	a.s.Focus = 0
	a.s.Category = "Artists"
	a.s.Screen = "library"
	a.mu.Unlock()
	return a.load()
}
func (a *App) load() error {
	s := a.Snapshot()
	if s.Demo {
		a.mu.Lock()
		a.demoBrowse()
		a.mu.Unlock()
		return nil
	}
	if a.client == nil {
		return errors.New("connect the iPod first")
	}
	entries, total, e := a.client.Browse(s.Path, codes[s.Category], s.Offset, PageSize)
	if e != nil {
		return e
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Index < entries[j].Index })
	a.mu.Lock()
	a.s.Entries = entries
	a.s.Total = total
	if a.s.Focus >= len(entries) {
		a.s.Focus = max(0, len(entries)-1)
	}
	a.mu.Unlock()
	return nil
}
func (a *App) navigate(name string) error {
	if _, ok := codes[name]; !ok {
		return errors.New("unknown category")
	}
	a.mu.Lock()
	a.s.Screen = "library"
	a.s.Category = name
	a.s.Path = nil
	a.s.Offset = 0
	a.s.Focus = 0
	a.s.Entries = nil
	a.s.Total = 0
	a.mu.Unlock()
	if !a.Snapshot().Connected {
		return nil
	}
	return a.load()
}
func (a *App) activate(index int) error {
	s := a.Snapshot()
	if index < 0 || index >= len(s.Entries) {
		return errors.New("no item selected")
	}
	entry := s.Entries[index]
	if s.Category == "Songs" {
		if s.Demo {
			a.mu.Lock()
			a.demoTrack = int(entry.Index)
			if len(s.Path) > 0 {
				a.demoTrack += int(s.Path[0].Index%4) * 4
			}
			a.s.Now = demoNow(a.demoTrack, true)
			a.s.Screen = "playing"
			a.s.Focus = 0
			a.mu.Unlock()
			return nil
		}
		if a.client == nil {
			return errors.New("iPod disconnected")
		}
		if e := a.client.Play(entry.Index); e != nil {
			return e
		}
		a.mu.Lock()
		a.s.Screen = "playing"
		a.s.Focus = 0
		a.mu.Unlock()
		return a.refresh()
	}
	next := "Songs"
	if s.Category == "Artists" {
		next = "Albums"
	}
	a.mu.Lock()
	a.s.Path = append(a.s.Path, iap.Selection{Category: codes[s.Category], Index: entry.Index, Name: entry.Name})
	a.s.Category = next
	a.s.Offset = 0
	a.s.Focus = 0
	a.s.Entries = nil
	a.s.Total = 0
	a.mu.Unlock()
	return a.load()
}
func (a *App) back() error {
	s := a.Snapshot()
	if strings.HasPrefix(s.Screen, "settings-") {
		a.mu.Lock()
		a.s.Screen = "settings"
		a.s.Focus = 0
		a.mu.Unlock()
		return nil
	}
	if s.Screen != "library" {
		a.mu.Lock()
		a.s.Screen = "library"
		a.s.Focus = 0
		a.mu.Unlock()
		return nil
	}
	if len(s.Path) == 0 {
		return nil
	}
	last := s.Path[len(s.Path)-1]
	a.mu.Lock()
	a.s.Path = a.s.Path[:len(a.s.Path)-1]
	for k, v := range codes {
		if v == last.Category {
			a.s.Category = k
		}
	}
	a.s.Offset = last.Index / PageSize * PageSize
	a.s.Focus = int(last.Index % PageSize)
	a.mu.Unlock()
	return a.load()
}
func (a *App) refresh() error {
	if a.client == nil {
		return nil
	}
	n, e := a.client.Now()
	if e != nil {
		return e
	}
	a.mu.Lock()
	a.s.Now = n
	a.mu.Unlock()
	return nil
}
func (a *App) Tick() {
	if !a.op.TryLock() {
		return
	}
	defer a.op.Unlock()
	s := a.Snapshot()
	if s.Demo {
		a.mu.Lock()
		if a.s.Now.Playing {
			a.s.Now.Position += 1000
			if a.s.Now.Position >= a.s.Now.Duration {
				a.demoTrack++
				a.s.Now = demoNow(a.demoTrack, true)
			}
		}
		a.mu.Unlock()
		return
	}
	if a.client != nil {
		if e := a.refresh(); e != nil {
			a.Log("Playback refresh: " + e.Error())
			a.mu.Lock()
			a.s.Error = e.Error()
			a.s.Connected = false
			a.s.Status = "iPod connection lost; reconnect to retry"
			a.mu.Unlock()
			_ = a.client.Abort()
			a.client = nil
			a.audio.Stop()
		}
	}
}

func (a *App) Action(v Action) error {
	if v.Name == "tap" {
		for _, h := range a.View().Hits {
			if v.X >= h.X && v.X < h.X+h.W && v.Y >= h.Y && v.Y < h.Y+h.H {
				return a.Action(h.Action)
			}
		}
		return nil
	}
	if strings.HasPrefix(v.Name, "preset") {
		i := int(v.Name[len(v.Name)-1] - '1')
		if i < 0 || i > 3 {
			return errors.New("invalid preset")
		}
		s := a.Snapshot()
		return a.binding(s.Settings.Buttons[i])
	}
	if v.Name == "scroll" {
		return a.scroll(v.Value)
	}
	if v.Name == "settings" || v.Name == "diagnostics" || v.Name == "playing" || v.Name == "settings-page" {
		screen := v.Name
		if screen == "diagnostics" {
			screen = "settings-usb"
		}
		if screen == "settings-page" {
			if v.Text != "settings-usb" && v.Text != "settings-audio" && v.Text != "settings-controls" && v.Text != "settings-about" {
				return errors.New("unknown settings page")
			}
			screen = v.Text
		}
		a.mu.Lock()
		a.s.Screen = screen
		a.s.Focus = 0
		a.mu.Unlock()
		return nil
	}
	if v.Name == "select" {
		s := a.Snapshot()
		if s.Screen == "library" {
			return a.Action(Action{Name: "open", Value: s.Focus})
		}
		view := a.View()
		if s.Focus >= 0 && s.Focus < len(view.Rows) {
			return a.Action(view.Rows[s.Focus].Action)
		}
		return nil
	}
	return a.run(func() error {
		s := a.Snapshot()
		switch v.Name {
		case "connect":
			return a.connect()
		case "disconnect":
			a.audio.Stop()
			if a.client != nil {
				e := a.client.Close()
				a.client = nil
				a.mu.Lock()
				a.s.Connected = false
				a.s.Status = "iPod disconnected"
				a.s.Entries = nil
				a.s.Total = 0
				a.mu.Unlock()
				return e
			}
			return nil
		case "navigate":
			return a.navigate(v.Text)
		case "back":
			return a.back()
		case "open":
			return a.activate(v.Value)
		case "play", "previous", "next":
			if s.Demo {
				a.mu.Lock()
				playing := a.s.Now.Playing
				if v.Name == "play" {
					playing = !playing
				} else if v.Name == "next" {
					a.demoTrack++
				} else {
					a.demoTrack = max(0, a.demoTrack-1)
				}
				a.s.Now = demoNow(a.demoTrack, playing)
				a.mu.Unlock()
				return nil
			}
			if a.client == nil {
				return errors.New("connect the iPod first")
			}
			code := byte(1)
			if v.Name == "next" {
				code = 3
			}
			if v.Name == "previous" {
				code = 4
			}
			if e := a.client.Control(code); e != nil {
				return e
			}
			return a.refresh()
		case "shuffle", "repeat":
			value := (s.Now.Shuffle + 1) % 3
			if v.Name == "repeat" {
				value = (s.Now.Repeat + 1) % 3
			}
			if !s.Demo {
				if a.client == nil {
					return errors.New("connect the iPod first")
				}
				var e error
				if v.Name == "shuffle" {
					e = a.client.SetShuffle(value)
				} else {
					e = a.client.SetRepeat(value)
				}
				if e != nil {
					return e
				}
			}
			a.mu.Lock()
			if v.Name == "shuffle" {
				a.s.Now.Shuffle = value
			} else {
				a.s.Now.Repeat = value
			}
			a.mu.Unlock()
			return nil
		case "remap":
			if v.Value < 0 || v.Value > 3 {
				return errors.New("invalid button")
			}
			a.mu.Lock()
			current := a.s.Settings.Buttons[v.Value]
			for i, b := range bindings {
				if b == current {
					a.s.Settings.Buttons[v.Value] = bindings[(i+1)%len(bindings)]
					break
				}
			}
			a.mu.Unlock()
			return a.save()
		case "invert":
			a.mu.Lock()
			a.s.Settings.InvertWheel = !a.s.Settings.InvertWheel
			a.mu.Unlock()
			return a.save()
		case "rescan":
			a.Scan()
			return nil
		case "usb-host", "usb-recovery":
			if s.Demo {
				return errors.New("USB role changes require the real Car Thing")
			}
			a.audio.Stop()
			if a.client != nil {
				_ = a.client.Close()
				a.client = nil
			}
			a.mu.Lock()
			a.s.Connected = false
			a.s.Entries = nil
			a.s.Total = 0
			a.mu.Unlock()
			if e := hardware.SetHostRole(v.Name == "usb-host"); e != nil {
				return e
			}
			a.Scan()
			a.mu.Lock()
			if v.Name == "usb-host" {
				a.s.Status = "USB host enabled; connect iPod and rescan"
			} else {
				a.s.Status = "Recovery: connect a computer, then open http://10.42.1.2:8787"
			}
			a.mu.Unlock()
			return nil
		case "digital":
			if s.Demo {
				return errors.New("digital audio requires a real iPod")
			}
			if a.client == nil {
				return errors.New("connect the iPod first")
			}
			if e := a.client.RequestDigitalAudio(); e != nil {
				return e
			}
			a.Scan()
			a.mu.Lock()
			a.s.Status = "Digital audio requested; check for iPod capture and test audio"
			a.mu.Unlock()
			return nil
		case "capture", "output":
			ids := []string{""}
			for _, p := range s.Audio {
				if v.Name == "capture" && p.Capture && p.IPod || v.Name == "output" && !p.Capture && p.USB && !p.IPod {
					ids = append(ids, p.ID)
				}
			}
			current := s.Settings.Output
			if v.Name == "capture" {
				current = s.Settings.Capture
			}
			idx := 0
			for i, id := range ids {
				if id == current {
					idx = (i + 1) % len(ids)
					break
				}
			}
			a.audio.Stop()
			a.mu.Lock()
			if v.Name == "capture" {
				a.s.Settings.Capture = ids[idx]
			} else {
				a.s.Settings.Output = ids[idx]
			}
			a.mu.Unlock()
			return a.save()
		case "audio-start":
			if s.Demo {
				return errors.New("demo mode does not route audio")
			}
			return a.audio.Start(s.Settings.Capture, s.Settings.Output)
		case "audio-stop":
			a.audio.Stop()
			return nil
		case "tone":
			if s.Demo {
				return errors.New("demo mode does not play audio")
			}
			return a.audio.Tone(s.Settings.Output)
		case "export":
			b, e := a.Report()
			if e != nil {
				return e
			}
			path := filepath.Join(a.stateDir, "report-"+time.Now().Format("20060102-150405")+".json")
			if e = os.WriteFile(path, b, 0600); e != nil {
				return e
			}
			a.Log("Saved report: " + path)
			return nil
		default:
			return errors.New("unknown action")
		}
	})
}
func (a *App) binding(name string) error {
	switch name {
	case "Now Playing":
		return a.Action(Action{Name: "playing"})
	case "Play / Pause":
		return a.Action(Action{Name: "play"})
	case "Previous":
		return a.Action(Action{Name: "previous"})
	case "Next":
		return a.Action(Action{Name: "next"})
	case "Shuffle":
		return a.Action(Action{Name: "shuffle"})
	case "Repeat":
		return a.Action(Action{Name: "repeat"})
	case "Diagnostics":
		return a.Action(Action{Name: "diagnostics"})
	default:
		return a.Action(Action{Name: "navigate", Text: name})
	}
}
func (a *App) scroll(delta int) error {
	if delta == 0 {
		return nil
	}
	s := a.Snapshot()
	if s.Settings.InvertWheel {
		delta = -delta
	}
	delta = max(-100, min(delta, 100))
	count := len(s.Entries)
	if s.Screen != "library" {
		count = len(a.View().Rows)
	}
	if count == 0 {
		return nil
	}
	next := s.Focus + delta
	if s.Screen == "library" && (next < 0 || next >= count) && s.Connected {
		absolute := int64(s.Offset) + int64(next)
		if absolute >= 0 && absolute < int64(s.Total) {
			return a.run(func() error {
				a.mu.Lock()
				a.s.Offset = uint32(absolute) / PageSize * PageSize
				a.s.Focus = int(uint32(absolute) % PageSize)
				a.mu.Unlock()
				return a.load()
			})
		}
	}
	a.mu.Lock()
	a.s.Focus = max(0, min(next, count-1))
	a.mu.Unlock()
	return nil
}
func (a *App) Report() ([]byte, error) {
	return json.MarshalIndent(struct {
		Snapshot Snapshot          `json:"state"`
		System   map[string]string `json:"system"`
	}{a.Snapshot(), map[string]string{"alsaloop": hardware.CommandVersion("alsaloop"), "gst-launch-1.0": hardware.CommandVersion("gst-launch-1.0"), "speaker-test": hardware.CommandVersion("speaker-test")}}, "", "  ")
}

var demoSongs = []string{"Dreams", "Everywhere", "Go Your Own Way", "The Chain", "Something", "Here Comes the Sun", "Come Together", "Golden Slumbers", "Digital Love", "One More Time", "Something About Us", "Voyager", "Everything In Its Right Place", "Kid A", "Idioteque", "Motion Picture Soundtrack"}
var demoArtists = []string{"Fleetwood Mac", "The Beatles", "Daft Punk", "Radiohead"}
var demoAlbums = []string{"Rumours & Tango", "Abbey Road", "Discovery", "Kid A"}

func demoNow(index int, playing bool) iap.NowPlaying {
	index = index % len(demoSongs)
	return iap.NowPlaying{Title: demoSongs[index], Artist: demoArtists[index/4], Album: demoAlbums[index/4], Duration: 245000, Playing: playing}
}
func (a *App) demoBrowse() {
	s := &a.s
	var names []string
	switch s.Category {
	case "Artists":
		names = demoArtists
	case "Albums":
		names = demoAlbums
		if len(s.Path) > 0 {
			idx := int(s.Path[0].Index) % 4
			names = []string{demoAlbums[idx]}
		}
	case "Playlists":
		names = []string{"On the Road", "All-Time Favorites", "Late Night", "Recently Added"}
	case "Songs":
		names = demoSongs
		if len(s.Path) > 0 {
			idx := int(s.Path[0].Index) % 4
			names = demoSongs[idx*4 : idx*4+4]
		}
	}
	s.Total = uint32(len(names))
	s.Entries = nil
	for i := int(s.Offset); i < min(int(s.Offset)+PageSize, len(names)); i++ {
		s.Entries = append(s.Entries, iap.Entry{Index: uint32(i), Name: names[i]})
	}
}
