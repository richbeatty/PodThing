package player

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/richbeatty/PodThing/internal/iap"
	"github.com/richbeatty/PodThing/internal/storage"
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

func TestNestedSettingsNavigation(t *testing.T) {
	a := demoApp(t)
	action(t,a,"settings",0,"")
	if a.Snapshot().Screen!="settings" {t.Fatal("settings page unavailable")}
	action(t,a,"settings-page",0,"settings-controls")
	if a.Snapshot().Screen!="settings-controls" {t.Fatal("controls not nested")}
	action(t,a,"back",0,"")
	if a.Snapshot().Screen!="settings" {t.Fatal("back did not return to settings")}
	action(t,a,"settings-page",0,"settings-about")
	if a.Snapshot().Screen!="settings-about" {t.Fatal("missing about/credits")}
	action(t,a,"back",0,"")
	action(t,a,"back",0,"")
	if a.Snapshot().Screen!="library" {t.Fatal("settings did not return to library")}
}
func TestSettingsRemovedFromTopRight(t *testing.T) {
	a := demoApp(t)
	v:=a.View()
	for _,hit:=range v.Hits {
		if hit.X<=735 && hit.X+hit.W>735 && hit.Y<=26 && hit.Y+hit.H>26 && hit.Action.Name=="settings" {
			t.Fatal("old top right settings button still active")
		}
	}
	action(t,a,"tap",0,"")
	if a.Snapshot().Screen!="library" {t.Fatal("unexpected navigation")}
	if err:=a.Action(Action{Name:"tap",X:29,Y:443});err!=nil {t.Fatal(err)}
	if a.Snapshot().Screen!="settings" {t.Fatal("footer settings button not active")}
}

func TestAuthDiagnosticDoesNotClaimConnection(t *testing.T) {
	a:=demoApp(t)
	err:=&iap.AuthenticationRequest{Command:0x14}
	a.mu.Lock()
	a.s.Status="iAP1: checking remote UI mode"
	a.mu.Unlock()
	a.recordAuthentication(err,true)
	s:=a.Snapshot()
	if s.Connected || !s.AuthRequired || s.BasicRemote || s.AuthStage!="iAP1: checking remote UI mode" {
		t.Fatalf("invalid auth diagnostic state: %+v",s)
	}
	if s.Screen!="settings-usb" || !strings.Contains(s.AuthDetail,"0014") {t.Fatal("authentication request not visible in Connection settings")}
	if err:=a.Action(Action{Name:"probe-basic"});err==nil {t.Fatal("demo must not access physical USB Simple Remote")}
}

func TestSavedSessionRetainedThroughRecoveryAndRestart(t *testing.T) {
	dir:=t.TempDir()
	a,err:=New(true,dir)
	if err!=nil{t.Fatal(err)}
	a.Log("AUTH 00/0014: certificate requested")
	a.mu.Lock()
	a.s.AuthRequired=true
	a.s.AuthDetail="00/0014: certificate requested"
	a.s.HostRole="host"
	a.mu.Unlock()
	if err=a.saveSessionReport();err!=nil{t.Fatal(err)}
	a.mu.Lock()
	a.s.AuthRequired=false
	a.s.AuthDetail=""
	a.s.HostRole="device"
	a.s.Log=nil
	a.mu.Unlock()
	b,err:=a.SavedSessionReport()
	if err!=nil{t.Fatal(err)}
	var saved struct{State Snapshot `json:"state"`}
	if err=json.Unmarshal(b,&saved);err!=nil{t.Fatal(err)}
	if saved.State.HostRole!="host"||!saved.State.AuthRequired||!strings.Contains(strings.Join(saved.State.Log," "),"certificate requested"){t.Fatalf("saved report lacked host evidence: %+v",saved.State)}
	frames:=&Frames{}
	frames.PNG.Store([]byte{1})
	w:=httptest.NewRecorder()
	a.Handler(frames).ServeHTTP(w,httptest.NewRequest("GET","http://localhost/api/report/saved",nil))
	if w.Code!=200||!bytes.Equal(w.Body.Bytes(),b){t.Fatalf("wrong saved HTTP report: %d",w.Code)}
	a.Close()
	second,err:=New(true,dir)
	if err!=nil{t.Fatal(err)}
	defer second.Close()
	b2,err:=second.SavedSessionReport()
	if err!=nil||!bytes.Equal(b2,b){t.Fatalf("restart lost saved report: %v",err)}
}
func TestMissingSavedSessionShows404(t *testing.T){
	a:=demoApp(t)
	frames:=&Frames{}
	frames.PNG.Store([]byte{1})
	w:=httptest.NewRecorder()
	a.Handler(frames).ServeHTTP(w,httptest.NewRequest("GET","http://localhost/api/report/saved",nil))
	if w.Code!=404{t.Fatalf("saved report should not silently substitute live snapshot: %d",w.Code)}
}
func TestFreshProbesAreNeverHardwareActionsInDemo(t *testing.T){
	a:=demoApp(t)
	for _,name:=range []string{"probe-basic-fresh","probe-accessory"}{
		if err:=a.Action(Action{Name:name});err==nil{t.Errorf("demo allowed %s",name)}
	}
}
func TestConnectionMenuUsesOnlyReadOnlyStorageAndDiagnostics(t *testing.T){
	a:=demoApp(t)
	action(t,a,"diagnostics",0,"")
	rows:=a.View().Rows
	got:=map[string]int{}
	for _,row:=range rows{got[row.Action.Name]++}
	for _,name:=range []string{"storage-open","rescan","usb-host","export","usb-recovery"} {
		if got[name]!=1 {t.Fatalf("%s missing or duplicated in connection menu: %+v",name,rows)}
	}
	for _,name:=range []string{"connect","probe-basic-fresh","probe-accessory","digital","capture","audio-start","storage-eject"} {
		if got[name]!=0 {t.Fatalf("obsolete or duplicate connection option %q still shown: %+v",name,rows)}
	}
}

func TestReadOnlyStorageNavigationAndAudioPrerequisite(t *testing.T) {
	a:=demoApp(t)
	a.library=&storage.Library{Root:t.TempDir(),Tracks:[]storage.Track{
		{ID:10,Artist:"Apple",Album:"First",Title:"Alpha",Location:":iPod_Control:Music:F00:A.mp3"},
		{ID:11,Artist:"Apple",Album:"First",Title:"Beta",Location:":iPod_Control:Music:F00:B.mp3"},
		{ID:12,Artist:"Beatles",Album:"Other",Title:"Gamma",Location:":iPod_Control:Music:F00:C.mp3"},
	},Playlists:[]storage.Playlist{{Name:"Road Music",IDs:[]uint32{12,10}}}}
	a.mu.Lock()
	a.s.StorageMode=true
	a.s.Connected=true
	a.s.Demo=false
	a.s.Path=nil
	a.s.Offset=0
	a.s.Focus=0
	a.s.Category="Artists"
	a.mu.Unlock()
	if err:=a.load();err!=nil {t.Fatal(err)}
	if got:=a.Snapshot();got.Total!=2||got.Entries[0].Name!="Apple"{t.Fatalf("artist list %+v",got.Entries)}
	if err:=a.activate(0);err!=nil {t.Fatal(err)}
	s:=a.Snapshot()
	if s.Category!="Albums"||s.Total!=1||s.Entries[0].Name!="First"{t.Fatalf("album list %+v",s)}
	if err:=a.activate(0);err!=nil {t.Fatal(err)}
	s=a.Snapshot()
	if s.Category!="Songs"||s.Total!=2||s.Entries[0].Name!="Alpha"{t.Fatalf("songs %+v",s)}
	// Explicit USB mode must still refuse to claim playback without
	// a physical output; silent simulation is covered separately.
	a.mu.Lock();a.s.Settings.AudioMode="usb";a.mu.Unlock()
	if err:=a.playStorage(s.Entries[0].Index);err==nil||!strings.Contains(err.Error(),"audio output"){
		t.Fatalf("unavailable audio unexpectedly started: %v",err)
	}
	a.mu.Lock()
	a.s.Path=nil
	a.s.Category="Playlists"
	a.s.Offset=0
	a.mu.Unlock()
	if err:=a.load();err!=nil {t.Fatal(err)}
	if a.Snapshot().Entries[0].Name!="Road Music"{t.Fatal("playlist navigation unavailable")}
	if err:=a.activate(0);err!=nil {t.Fatal(err)}
	s=a.Snapshot()
	if s.Category!="Songs"||s.Total!=2||s.Entries[0].Name!="Gamma"||s.Entries[1].Name!="Alpha"{
		t.Fatalf("playlist order not honored: %+v",s.Entries)
	}
}
func TestSafeEjectRefusesUnmountError(t *testing.T) {
	a:=demoApp(t)
	if err:=a.ejectStorage();err==nil {t.Fatal("eject should refuse an absent mount")}
	if a.Snapshot().SafeToDisconnect {t.Fatal("never claim safe ejection without a successful unmount")}
}

func TestSimulatedPlaybackWorksWithoutFilesOrDAC(t *testing.T) {
	a:=demoApp(t)
	a.mu.Lock()
	a.s.Demo=false
	a.s.StorageMode=true
	a.s.Connected=true
	a.s.Settings.AudioMode="simulated"
	a.mu.Unlock()
	a.library=&storage.Library{Root:t.TempDir(),Tracks:[]storage.Track{
		{ID:11,Artist:"Artist",Album:"A",Title:"One",Location:":iPod_Control:Music:missing.m4a",Duration:180000},
		{ID:12,Artist:"Artist",Album:"A",Title:"Two",Location:":iPod_Control:Music:missing2.m4a",Duration:190000},
	}}
	if err:=a.playStorage(0);err!=nil{t.Fatal(err)}
	state:=a.Snapshot()
	if state.Now.Title!="One"||!state.Now.Playing||state.Settings.AudioMode!="simulated"{t.Fatalf("incorrect simulator state %+v",state.Now)}
	a.sim.started=a.sim.started.Add(-2*time.Second)
	a.Tick()
	if a.Snapshot().Now.Position<1900 {t.Fatalf("clock did not advance %d",a.Snapshot().Now.Position)}
	if err:=a.storageControl("play");err!=nil{t.Fatal(err)}
	paused:=a.Snapshot().Now.Position
	if a.Snapshot().Now.Playing {t.Fatal("simulator did not pause")}
	if err:=a.storageSeek(50);err!=nil{t.Fatal(err)}
	if a.Snapshot().Now.Position!=90000{t.Fatal("simulation seek failed")}
	if err:=a.storageControl("next");err!=nil{t.Fatal(err)}
	if a.Snapshot().Now.Title!="Two"{t.Fatal("simulated next failed")}
	if len(a.Snapshot().Recent)!=2 || a.Snapshot().Recent[0].Title!="Two"{t.Fatalf("history not updated %+v",a.Snapshot().Recent)}
	if paused==0{t.Fatal("pause should preserve elapsed progress")}
}
func TestModernAlbumGridSortingRecentsAndTray(t *testing.T) {
	a:=demoApp(t)
	a.library=&storage.Library{Root:t.TempDir(),Tracks:[]storage.Track{
		{ID:5,Artist:"Zed",Album:"Zebra",Title:"Last",Location:":iPod_Control:Music:x.mp3",Duration:120000},
		{ID:6,Artist:"Able",Album:"Abbey",Title:"First",Location:":iPod_Control:Music:y.mp3",Duration:125000},
	}}
	a.mu.Lock()
	a.s.StorageMode=true
	a.s.Connected=true
	a.s.Demo=false
	a.s.Settings.AudioMode="simulated"
	a.s.Screen="library"
	a.s.Category="Albums"
	a.s.GridView=true
	a.s.SortMode="A-Z"
	a.mu.Unlock()
	if err:=a.load();err!=nil{t.Fatal(err)}
	s:=a.Snapshot()
	if len(s.Cards)!=2||s.Cards[0].Name!="Abbey"{t.Fatalf("album A-Z grid %+v",s.Cards)}
	view:=a.View()
	if len(view.Tiles)!=2 || view.Tiles[0].Action.Name!="open-album"{t.Fatalf("album tiles %+v",view.Tiles)}
	if err:=a.Action(Action{Name:"sort-mode"});err!=nil{t.Fatal(err)}
	s=a.Snapshot()
	if s.SortMode!="Z-A"||s.Cards[0].Name!="Zebra"{t.Fatalf("album reverse sorting %+v",s)}
	if err:=a.Action(Action{Name:"open-album",Text:s.Cards[0].Key});err!=nil{t.Fatal(err)}
	if a.Snapshot().Category!="Songs"||a.Snapshot().Total!=1{t.Fatal("album card did not open correct album")}
	if err:=a.playStorage(0);err!=nil{t.Fatal(err)}
	if err:=a.Action(Action{Name:"home"});err!=nil{t.Fatal(err)}
	if a.Snapshot().Screen!="home"{t.Fatal("home navigation failed")}
	if !a.View().State.StorageMode || a.View().Tiles[0].Action.Name!="play-recent" {t.Fatal("recent tile missing")}
	if err:=a.Action(Action{Name:"tray"});err!=nil{t.Fatal(err)}
	if !a.Snapshot().TrayOpen{t.Fatal("tray failed to open")}
	if err:=a.Action(Action{Name:"tray-close"});err!=nil{t.Fatal(err)}
	if a.Snapshot().TrayOpen{t.Fatal("tray failed to close")}
	p,err:=NewPainter();if err!=nil{t.Fatal(err)}
	img:=p.Render(a.View())
	if img.Bounds().Dx()!=800||img.Bounds().Dy()!=480{t.Fatal("modern UI wrong screen size")}
}

func TestSafeToDisconnectRemainsSafeOnSecondEject(t *testing.T) {
	a:=demoApp(t)
	a.mu.Lock()
	a.s.SafeToDisconnect=true
	a.s.Screen="unmounted"
	a.s.StorageMode=false
	a.mu.Unlock()
	if err:=a.ejectStorage();err!=nil{t.Fatalf("second eject should be harmless after confirmed unmount: %v",err)}
	state:=a.Snapshot()
	if !state.SafeToDisconnect||state.Screen!="unmounted"{t.Fatal("idempotent eject lost safe-to-disconnect state")}
	if len(a.View().Tiles)!=1||a.View().Tiles[0].Action.Name!="storage-open"{t.Fatal("safe-to-disconnect screen missing reconnect option")}
}
func TestMinimalUIIsNeutralAndCriticalButtonsClearOfKnob(t *testing.T) {
	a:=demoApp(t)
	a.mu.Lock()
	a.s.StorageMode=true
	a.s.Demo=false
	a.s.Settings.AudioMode="simulated"
	a.s.Screen="playing"
	a.s.Now.Title="Hang Loose — Live At KCRW"
	a.s.Now.Artist="Alabama Shakes"
	a.s.Now.Duration=180000
	a.s.Now.Position=30000
	a.mu.Unlock()
	view:=a.View()
	p,e:=NewPainter();if e!=nil{t.Fatal(e)}
	img:=p.Render(view)
	if img.Bounds().Dx()!=800||img.Bounds().Dy()!=480{t.Fatal("bad Car Thing geometry")}
	r,g,b,_:=img.At(775,170).RGBA()
	if b>r+2000||b>g+2000{t.Fatalf("background should not have a saturated blue wash: rgb16 %d,%d,%d",r,g,b)}
	for _,name:=range []string{"play","next","previous","storage-eject"}{
		found:=false
		for _,hit:=range view.Hits {
			if hit.Action.Name==name&&hit.X+hit.W<=716 {found=true}
		}
		// The synthetic playing test isn't mounted; Eject only appears
		// for a verified mounted storage session, not by screen name.
		if name=="storage-eject" && !view.State.StorageMode {continue}
		if !found{t.Fatalf("%s is not reachable left of hardware dial",name)}
	}
	// Presets are shown by the swipe gesture, never a duplicate More button.
	for _,hit:=range view.Hits {if hit.Action.Name=="tray"{t.Fatal("preset tray must be swipe-only")}}
	if err:=a.Action(Action{Name:"tray"});err!=nil{t.Fatal(err)}
	if !a.Snapshot().TrayOpen{t.Fatal("swipe-down action did not open presets")}
}

func TestUnifiedModernUIAcrossConnectionStates(t *testing.T) {
	a:=demoApp(t)
	p,err:=NewPainter()
	if err!=nil{t.Fatal(err)}
	check:=func(screen string, mounted, connected, safe bool){
		t.Helper()
		a.mu.Lock()
		a.s.Demo=false
		a.s.StorageMode=mounted
		a.s.Connected=connected
		a.s.SafeToDisconnect=safe
		a.s.Screen=screen
		a.s.TrayOpen=false
		a.mu.Unlock()
		v:=a.View()
		img:=p.Render(v)
		if img.Bounds().Dx()!=800 || img.Bounds().Dy()!=480{t.Fatal("wrong modern display geometry")}
		// All states share the same charcoal header, not a legacy blue design.
		r,g,b,_:=img.At(5,5).RGBA()
		if b>r+1500 || b>g+1500{t.Fatalf("%s switched to old blue UI: rgb %d,%d,%d",screen,r,g,b)}
		eject:=0
		for _,h:=range v.Hits {
			if h.Action.Name=="storage-eject"{
				eject++
				if h.X+h.W>716{t.Fatal("eject overlaps hardware dial")}
			}
			if h.Action.Name=="connect"||h.Action.Name=="probe-accessory"||h.Action.Name=="probe-basic-fresh"{
				t.Fatalf("%s showed an obsolete iAP control",screen)
			}
		}
		if mounted && eject!=1 {t.Fatalf("%s has %d eject actions, want exactly one",screen,eject)}
		if !mounted && eject!=0 {t.Fatalf("%s has eject action without mounted storage",screen)}
	}
	check("library",false,false,false)
	check("settings",false,false,false)
	check("home",true,true,false)
	check("library",true,true,false)
	check("settings-usb",true,true,false)
	check("playing",true,true,false)
	check("unmounted",false,false,true)
}

func TestStorageConnectionIsDefaultAndEjectIsNotHiddenInSettings(t *testing.T){
	a:=demoApp(t)
	a.mu.Lock()
	a.s.Demo=false
	a.s.Screen="library"
	a.s.Connected=false
	a.s.StorageMode=false
	a.s.HostRole="host"
	a.mu.Unlock()
	v:=a.View()
	hasOpen:=false
	for _,h:=range v.Hits{
		if h.Action.Name=="storage-open"{hasOpen=true}
		if h.Action.Name=="connect"{t.Fatal("legacy accessory connect must not be main action")}
	}
	if !hasOpen{t.Fatal("no read-only library connect action")}
	a.mu.Lock()
	a.s.StorageMode=true
	a.s.Connected=true
	a.s.Screen="settings-usb"
	a.mu.Unlock()
	v=a.View()
	for _,row:=range v.Rows{
		if row.Action.Name=="storage-eject"{t.Fatal("eject must be single global action, not duplicated in settings")}
	}
	eject:=0
	for _,h:=range v.Hits{if h.Action.Name=="storage-eject"{eject++}}
	if eject!=1{t.Fatalf("eject missing in settings, count=%d",eject)}
}
