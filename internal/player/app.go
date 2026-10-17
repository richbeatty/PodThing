package player

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/richbeatty/PodThing/internal/hardware"
	"github.com/richbeatty/PodThing/internal/iap"
	"github.com/richbeatty/PodThing/internal/storage"
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
	AudioMode   string    `json:"audio_mode"`
}
type Snapshot struct {
	Version     string            `json:"version"`
	Demo        bool              `json:"demo"`
	Connected   bool              `json:"connected"`
	AuthRequired bool             `json:"auth_required"`
	AuthDetail   string           `json:"auth_detail,omitempty"`
	AuthStage    string           `json:"auth_stage,omitempty"`
	BasicRemote  bool             `json:"basic_remote"`
	StorageMode  bool             `json:"storage_mode"`
	TrayOpen     bool             `json:"tray_open"`
	SortMode     string           `json:"sort_mode"`
	GridView     bool             `json:"grid_view"`
	Recent       []RecentItem     `json:"recent"`
	HomeAlbums   []MusicCard      `json:"home_albums"`
	Cards        []MusicCard      `json:"cards"`
	Covers       map[string]image.Image `json:"-"`
	TrackSummaries map[uint32]string `json:"-"`
	SafeToDisconnect bool         `json:"safe_to_disconnect"`
	BasicLast    string           `json:"basic_last,omitempty"`
	AccessoryProbe bool          `json:"accessory_probe"`
	ProbeResult string           `json:"probe_result,omitempty"`
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
	Artwork     image.Image      `json:"-"`
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
	volume    *storage.Volume
	library   *storage.Library
	player    storage.Playback
	activeTrack int
	queue []int
	sim SimulatedPlayback
	// Prevent an artwork reader from keeping a file open during eject.
	artUse sync.RWMutex
	artFetch sync.Mutex
	coverTried map[string]bool
	coverOrder []string
	classicArt classicArtwork
	audio     hardware.Audio
	stateDir  string
	demoTrack int
	closed    bool
}

func New(demo bool, stateDir string) (*App, error) {
	if e := os.MkdirAll(stateDir, 0700); e != nil {
		return nil, e
	}
	a := &App{stateDir: stateDir, s: Snapshot{Version: Version, Demo: demo, Status: "Connect your iPod, then choose Connect", Screen: "library", Category: "Artists", Focus: 0, Accent: [3]uint8{96, 154, 255}, Settings: Settings{Buttons: [4]string{"Artists", "Albums", "Songs", "Playlists"}, AudioMode: "simulated"}}}
	b, e := os.ReadFile(filepath.Join(stateDir, "settings.json"))
	if e == nil {
		if e = json.Unmarshal(b, &a.s.Settings); e != nil {
			return nil, fmt.Errorf("read saved settings: %w", e)
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if a.s.Settings.AudioMode!="usb" {a.s.Settings.AudioMode="simulated"}
	a.s.SortMode="A-Z"
	a.s.GridView=true
	a.loadRecent()
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
	if strings.HasPrefix(s, "STAGE ") {
		a.s.Status = strings.TrimPrefix(s, "STAGE ")
	}
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
	s.Recent = append([]RecentItem{}, s.Recent...)
	s.Cards = append([]MusicCard{}, s.Cards...)
	s.HomeAlbums = append([]MusicCard{}, s.HomeAlbums...)
	s.Covers=make(map[string]image.Image,len(a.s.Covers))
	for k,img:=range a.s.Covers {s.Covers[k]=img}
	s.TrackSummaries=make(map[uint32]string,len(a.s.TrackSummaries))
	for k,v:=range a.s.TrackSummaries {s.TrackSummaries[k]=v}
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
	// An authentication request from any Extended Interface operation
	// (navigation, playback, metadata, or audio) is diagnostic, not a
	// reason to discard the physical USB session.
	if errors.Is(e, iap.ErrAuthentication) {
		a.recordAuthentication(e, true)
		e = nil
	}
	a.mu.Lock()
	a.s.Busy = false
	if e != nil {
		a.s.Error = e.Error()
	}
	a.mu.Unlock()
	if e != nil {
		a.Log(e.Error())
	}
	// Preserve the host-mode session independently of the recovery view.
	// Never overwrite a saved host report with a device-mode snapshot.
	latest := a.Snapshot()
	// A switch *to* host mode has no iPod session yet. Don't overwrite
	// the last useful report with an empty post-recovery host snapshot.
	if !latest.Demo && latest.HostRole != "device" && (a.client != nil || a.volume != nil || e != nil) {
		if se := a.saveSessionReport(); se != nil {
			a.Log("Could not preserve USB session: " + se.Error())
		}
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
	a.sim.Reset()
	a.artUse.Lock()
	defer a.artUse.Unlock()
	a.classicArt.Close()
	if a.volume!=nil {
		if e:=a.player.Stop();e==nil {
			if e=a.volume.Eject();e!=nil {
				a.Log("iPod needs manual safe eject: "+e.Error())
			}else{
				a.volume=nil
				a.library=nil
			}
		}
	}
	if a.client != nil {
		_ = a.client.Close()
		a.client = nil
	}
}
// recordAuthentication keeps the transport open for a user-invoked
// compatibility probe. No MFi success is implied by this diagnostic state.
func (a *App) recordAuthentication(err error, blocked bool) {
	a.mu.Lock()
	stage := a.s.Status
	a.s.AuthRequired = true
	a.s.AuthDetail = err.Error()
	a.s.AuthStage = stage
	a.s.BasicRemote = false
	a.s.BasicLast = ""
	a.s.AccessoryProbe = false
	a.s.ProbeResult = ""
	if blocked {
		a.s.Connected = false
		a.s.Entries = nil
		a.s.Total = 0
	}
	a.s.Status = "MFi authentication requested; try basic controls in Connection settings"
	a.s.Screen = "settings-usb"
	a.s.Focus = 0
	a.mu.Unlock()
	a.Log("AUTH captured at " + stage + ": " + err.Error())
}
// startProbe opens HID afresh and performs only the user-selected test.
// Switching between protocols requires disconnect/replug, not an implicit
// transition from an already failed Extended Interface session.
func (a *App) startProbe(simple bool) error {
	if a.s.Demo { return errors.New("hardware probe needs a real iPod") }
	if a.client != nil { return errors.New("disconnect, then replug the iPod before switching test modes") }
	a.Scan()
	s := a.Snapshot()
	var found []hardware.Device
	for _, d := range s.USB { if d.IPod { found = append(found,d) } }
	if len(found) != 1 { return fmt.Errorf("found %d iPods; connect one iPod via powered host adapter",len(found)) }
	a.mu.Lock()
	a.s.Connected=false
	a.s.AuthRequired=false
	a.s.AuthDetail=""
	a.s.AuthStage=""
	a.s.BasicRemote=false
	a.s.BasicLast=""
	a.s.AccessoryProbe=false
	a.s.ProbeResult=""
	a.s.Entries=nil
	a.s.Total=0
	a.s.Status="USB: opening fresh probe"
	a.mu.Unlock()
	w,err:=hardware.OpenIPod(found[0],a.Log)
	if err!=nil { return fmt.Errorf("USB HID setup: %w",err) }
	c:=iap.New(w,a.Log)
	a.client=c
	if simple {
		if err=c.BeginSimpleRemote();err!=nil {
			_ = c.Abort()
			a.client=nil
			return err
		}
		a.mu.Lock()
		a.s.BasicRemote=true
		a.s.Status="Simple Remote first: use buttons and watch the iPod"
		a.s.Screen="settings-usb"
		a.s.Focus=0
		a.mu.Unlock()
		a.Log("PROBE sent Simple Remote Identify without Extended Interface handshake")
		return nil
	}
	a.mu.Lock()
	a.s.AccessoryProbe=true
	a.s.Screen="settings-usb"
	a.s.Focus=0
	a.mu.Unlock()
	name,err:=c.ProbeAccessory()
	if err!=nil {
		if errors.Is(err,iap.ErrAuthentication) { a.recordAuthentication(err,true);return nil }
		a.mu.Lock()
		a.s.ProbeResult=err.Error()
		a.s.Status="Modern identification incomplete; review saved report"
		a.mu.Unlock()
		a.Log("PROBE modern identification: "+err.Error())
		return nil
	}
	a.mu.Lock()
	a.s.Name=name
	a.s.ProbeResult="Device queries succeeded; browsing and digital audio not tested"
	a.s.Status="Modern identification answered; save/download report"
	a.mu.Unlock()
	return nil
}
// openStorage uses only the iPod's USB mass-storage interface. It never
// sends iAP accessory commands or writes to any iPod filesystem.
func (a *App) openStorage() error {
	if a.s.Demo {return errors.New("storage mode requires a real iPod")}
	if a.volume!=nil{return errors.New("iPod storage is already mounted")}
	if a.client!=nil {return errors.New("disconnect the iAP session and reconnect the iPod in disk mode")}
	a.Scan()
	s:=a.Snapshot()
	var ipods []hardware.Device
	for _,d:=range s.USB {if d.IPod {ipods=append(ipods,d)}}
	if len(ipods)!=1 {return fmt.Errorf("found %d iPods; connect exactly one stock iPod through the USB host hub",len(ipods))}
	a.Log("STAGE Storage: locking iPod disk read-only before mount")
	vol,lib,err:=storage.OpenUSB(ipods[0].SysPath)
	if err!=nil{return err}
	a.volume=vol
	a.library=lib
	a.activeTrack=0
	a.queue=nil
	a.mu.Lock()
	a.s.StorageMode=true
	a.s.SafeToDisconnect=false
	a.s.Connected=true
	a.s.AuthRequired=false
	a.s.BasicRemote=false
	a.s.Name="iPod Music (read-only)"
	a.s.Status=fmt.Sprintf("Read-only music library: %d tracks, %d playlists",len(lib.Tracks),len(lib.Playlists))
	a.s.Screen="home"
	a.s.Category="Artists"
	a.s.Path=nil
	a.s.Offset=0
	a.s.Focus=0
	a.s.TrayOpen=false
	a.s.Covers=make(map[string]image.Image)
	a.coverTried=make(map[string]bool)
	a.coverOrder=nil
	a.classicArt.Close()
	a.mu.Unlock()
	a.rebuildHomeAlbums()
	cards:=a.Snapshot().HomeAlbums
	if len(cards)>4 {cards=cards[:4]}
	a.requestCovers(cards)
	a.Log(fmt.Sprintf("STORAGE mounted %s %s read-only; %d tracks, %d playlists",vol.Device,vol.FSType,len(lib.Tracks),len(lib.Playlists)))
	return a.load()
}
func (a *App) ejectStorage() error {
	a.artUse.Lock()
	defer a.artUse.Unlock()
	if a.volume==nil {return errors.New("no mounted iPod to eject")}
	if err:=a.player.Stop();err!=nil{return err}
	a.sim.Reset()
	// Preserve the last storage session before the state is replaced by
	// Safe to Disconnect, for recovery-mode diagnostics.
	if err:=a.saveSessionReport();err!=nil {return fmt.Errorf("preserve storage session before eject: %w",err)}
	if err:=a.volume.Eject();err!=nil{return err}
	a.volume=nil
	a.library=nil
	a.queue=nil
	a.mu.Lock()
	a.s.StorageMode=false
	a.s.Connected=false
	a.s.TrayOpen=false
	a.s.Cards=nil
	a.s.HomeAlbums=nil
	a.s.Covers=nil
	a.coverTried=nil
	a.coverOrder=nil
	a.classicArt.Close()
	a.s.SafeToDisconnect=true
	a.s.Name=""
	a.s.Status="Safe to disconnect iPod. Reconnect to browse again."
	a.s.Now=iap.NowPlaying{}
	a.s.Entries=nil
	a.s.Total=0
	a.s.Path=nil
	a.s.Screen="settings-usb"
	a.mu.Unlock()
	a.Log("STORAGE unmounted cleanly; safe for host to unplug")
	return nil
}
func (a *App) connect() error {
	if a.s.Demo {
		return nil
	}
	// If remote mode succeeded but the first database query failed, Connect
	// retries browsing in the existing session instead of reconfiguring USB.
	if a.client != nil && a.Snapshot().AuthRequired {
		// Explicit Simple Remote probe and Disconnect are the only safe
		// choices while authentication is outstanding.
		a.Log("Authentication pending; no further Extended Interface request sent")
		return nil
	}
	if a.client != nil {
		a.Log("STAGE iAP1: retrying database without USB reconnect")
		if err := a.load(); err != nil {
			return fmt.Errorf("iAP database retry: %w", err)
		}
		a.mu.Lock()
		a.s.Status = "iPod connected; music library ready"
		a.mu.Unlock()
		return nil
	}
	a.Scan()
	s := a.Snapshot()
	a.mu.Lock()
	a.s.Connected = false
	a.s.Name = ""
	a.s.Entries = nil
	a.s.Total = 0
	a.s.Status = "USB: enumerating iPod devices"
	a.s.AuthRequired = false
	a.s.AuthDetail = ""
	a.s.AuthStage = ""
	a.s.BasicRemote = false
	a.s.BasicLast = ""
	a.s.AccessoryProbe = false
	a.s.ProbeResult = ""
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
		return fmt.Errorf("USB HID setup: %w", e)
	}
	c := iap.New(w, a.Log)
	name, e := c.Handshake()
	if e != nil {
		if errors.Is(e, iap.ErrAuthentication) {
			a.client = c
			a.recordAuthentication(e, true)
			return nil
		}
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
	a.Log("STAGE iAP1: loading music database")
	if err := a.load(); err != nil {
		if errors.Is(err, iap.ErrAuthentication) {
			a.recordAuthentication(err, true)
			return nil
		}
		a.mu.Lock()
		a.s.Status = "iPod control connected; database access failed"
		a.mu.Unlock()
		return fmt.Errorf("iAP database browsing: %w", err)
	}
	a.mu.Lock()
	a.s.Status = "iPod connected; music library ready"
	a.mu.Unlock()
	return nil
}
func (a *App) load() error {
	s := a.Snapshot()
	if a.library!=nil {return a.loadStorage(s)}
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
	if a.library!=nil && s.Category=="Albums" && index<len(s.Cards) {
		return a.openAlbum(s.Cards[index].Key)
	}
	if s.Category == "Songs" {
		if a.library!=nil {return a.playStorage(entry.Index)}
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
	if s.TrayOpen {return a.Action(Action{Name:"tray-close"})}
	if s.StorageMode {
		if s.Screen=="home" {return nil}
		if s.Screen=="library" && len(s.Path)==0 {
			return a.Action(Action{Name:"home"})
		}
	}
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
	if a.library!=nil {
		if a.Snapshot().Settings.AudioMode=="simulated" {
			position,playing:=a.sim.State()
			a.mu.Lock()
			a.s.Now.Position=position
			a.s.Now.Playing=playing
			a.mu.Unlock()
			return nil
		}
		track,position,playing,errText:=a.player.State()
		a.mu.Lock()
		a.s.Now.Position=position
		a.s.Now.Playing=playing
		if errText!="" {a.s.Error=errText}
		_ = track
		a.mu.Unlock()
		return nil
	}
	if a.client == nil {
		return nil
	}
	n, e := a.client.Now()
	if e != nil {
		return e
	}
	a.mu.Lock()
	// Artwork always belongs to the displayed track. Never carry the last
	// album's colors or cover into the next track when they change.
	if a.s.Now.Title != n.Title || a.s.Now.Album != n.Album || a.s.Now.Artist != n.Artist {
		a.s.Accent = [3]uint8{96, 154, 255}
		a.s.Artwork = nil
		a.s.HasArtwork = false
	}
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
	if a.library != nil {
		_ = a.refresh()
		if a.Snapshot().Settings.AudioMode=="simulated" {
			a.simulatedTick()
			return
		}
		if a.player.TakeCompletion() {
			state:=a.Snapshot()
			if state.Now.Repeat==1 {
				if e:=a.playStorage(uint32(a.activeTrack));e!=nil {a.Log("Auto-repeat failed: "+e.Error())}
			} else if e:=a.storageControl("next");e!=nil {
				a.mu.Lock()
				a.s.Status="Finished playlist"
				a.mu.Unlock()
			}
		}
		return
	}
	if a.client != nil && s.Connected && !s.BasicRemote {
		if e := a.refresh(); e != nil {
			if errors.Is(e, iap.ErrAuthentication) {
				a.recordAuthentication(e, true)
				return
			}
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
	if v.Name=="tray" {
		a.mu.Lock()
		a.s.TrayOpen=!a.s.TrayOpen
		a.mu.Unlock()
		return nil
	}
	if v.Name=="tray-close" {
		a.mu.Lock();a.s.TrayOpen=false;a.mu.Unlock()
		return nil
	}
	if strings.HasPrefix(v.Name, "preset") {
		i := int(v.Name[len(v.Name)-1] - '1')
		if i < 0 || i > 3 {
			return errors.New("invalid preset")
		}
		s := a.Snapshot()
		a.mu.Lock();a.s.TrayOpen=false;a.mu.Unlock()
		return a.binding(s.Settings.Buttons[i])
	}
	if v.Name == "scroll" {
		return a.scroll(v.Value)
	}
	if v.Name=="home" {
		a.mu.Lock();a.s.Screen="home";a.s.TrayOpen=false;a.s.Focus=0;a.mu.Unlock()
		return nil
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
		a.s.TrayOpen=false
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
		case "audio-mode":
			if v.Text!="simulated" && v.Text!="usb" {return errors.New("unknown playback output mode")}
			if a.library!=nil {
				if err:=a.player.Stop();err!=nil{return err}
				a.sim.Reset()
			}
			a.mu.Lock()
			a.s.Settings.AudioMode=v.Text
			a.s.Now.Playing=false
			a.s.Now.Position=0
			a.mu.Unlock()
			return a.save()
		case "sort-mode":
			return a.changeSort()
		case "view-mode":
			a.mu.Lock();a.s.GridView=!a.s.GridView;a.mu.Unlock()
			return nil
		case "play-recent":
			return a.playRecent(uint32(v.Value))
		case "open-album":
			return a.openAlbum(v.Text)
		case "storage-open":
			return a.openStorage()
		case "storage-eject":
			return a.ejectStorage()
		case "storage-seek":
			return a.storageSeek(v.Value)
		case "connect":
			return a.connect()
		case "probe-basic-fresh":
			return a.startProbe(true)
		case "probe-accessory":
			return a.startProbe(false)
		case "probe-basic":
			if s.Demo {
				return errors.New("Simple Remote testing requires a real iPod")
			}
			if a.client == nil || !s.AuthRequired {
				return errors.New("connect and capture an authentication request first")
			}
			if err := a.client.BeginSimpleRemote(); err != nil { return err }
			a.mu.Lock()
			a.s.BasicRemote = true
			a.s.BasicLast = ""
			a.s.Status = "Simple Remote identified; send a test key and observe the iPod"
			a.mu.Unlock()
			return nil
		case "disconnect":
			if a.volume!=nil {return a.ejectStorage()}
			a.audio.Stop()
			if a.client != nil {
				e := a.client.Close()
				a.client = nil
				a.mu.Lock()
				a.s.Connected = false
				a.s.Status = "iPod disconnected"
				a.s.AuthRequired = false
				a.s.AuthDetail = ""
				a.s.AuthStage = ""
				a.s.BasicRemote = false
				a.s.BasicLast = ""
				a.s.AccessoryProbe = false
				a.s.ProbeResult = ""
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
			if a.library!=nil {return a.storageControl(v.Name)}
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
			if s.BasicRemote {
				if err := a.client.SimpleRemoteKey(v.Name); err != nil { return err }
				a.mu.Lock()
				a.s.BasicLast = v.Name
				a.s.Status = "Sent " + v.Name + " (no ACK); confirm the iPod responded"
				a.mu.Unlock()
				return nil
			}
			if s.AuthRequired && !s.Connected {
				return errors.New("MFi required for extended controls; choose Try Simple Remote in Connection")
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
			// Save while the host-mode USB inventory, iAP state and log
			// are still available, before Close or SetHostRole.
			if v.Name=="usb-recovery" && s.HostRole!="device" {
				if err:=a.saveSessionReport();err!=nil {return fmt.Errorf("save session before recovery: %w",err)}
			}
			if a.volume!=nil {
				if err:=a.ejectStorage();err!=nil {return fmt.Errorf("cannot change USB role until music is safely ejected: %w",err)}
			}
			a.audio.Stop()
			if a.client != nil {
				_ = a.client.Close()
				a.client = nil
			}
			a.mu.Lock()
			a.s.Connected = false
			a.s.AuthRequired = false
			a.s.AuthDetail = ""
			a.s.AuthStage = ""
			a.s.BasicRemote = false
			a.s.BasicLast = ""
			a.s.AccessoryProbe = false
			a.s.ProbeResult = ""
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
			if s.AuthRequired && !s.Connected {
				return errors.New("MFi authentication required before digital audio")
			}
			if s.BasicRemote { return errors.New("Simple Remote mode does not support USB digital audio") }
			if s.Demo {
				return errors.New("digital audio requires a real iPod")
			}
			if a.client == nil {
				return errors.New("connect the iPod first")
			}
			if e := a.client.RequestDigitalAudio(); e != nil {
				if errors.Is(e, iap.ErrAuthentication) { a.recordAuthentication(e, false); return nil }
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
			if a.library!=nil {return errors.New("Storage playback uses the selected USB DAC directly; select a song")}
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
			if e:=a.saveSessionReport();e!=nil {return e}
			a.Log("Saved last-session.json for recovery download")
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
	if s.StorageMode && s.Screen=="library" && s.Category=="Albums" && s.GridView {
		return a.run(func()error {
			a.mu.Lock()
			offset:=int64(a.s.Offset)+int64(delta)*4
			offset=max(0,min(offset,max(0,int64(a.s.Total)-8)))
			a.s.Offset=uint32(offset)
			a.s.Focus=0
			a.mu.Unlock()
			return a.load()
		})
	}
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
