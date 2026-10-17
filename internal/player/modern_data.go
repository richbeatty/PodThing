package player

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/richbeatty/PodThing/internal/iap"
	"github.com/richbeatty/PodThing/internal/storage"
)

// Recents are stored on the Car Thing, never on the iPod.
type RecentItem struct {
	ID uint32 `json:"id"`
	Title string `json:"title"`
	Artist string `json:"artist"`
	Album string `json:"album"`
}
type MusicCard struct {
	Name string `json:"name"`
	Detail string `json:"detail"`
	Key string `json:"key"`
	TrackIndex uint32 `json:"track_index"`
}
func (a *App) loadRecent() {
	b,err:=os.ReadFile(filepath.Join(a.stateDir,"recents.json"))
	if err!=nil{return}
	var r []RecentItem
	if json.Unmarshal(b,&r)!=nil{return}
	if len(r)>40 {r=r[:40]}
	a.s.Recent=r
}
func (a *App) recordRecent(t storage.Track) {
	s:=a.Snapshot()
	out:=[]RecentItem{{ID:t.ID,Title:t.Title,Artist:t.Artist,Album:t.Album}}
	for _,r:=range s.Recent {
		if r.ID!=t.ID && len(out)<40 {out=append(out,r)}
	}
	a.mu.Lock()
	a.s.Recent=out
	a.mu.Unlock()
	data,err:=json.MarshalIndent(out,"","  ")
	if err!=nil {return}
	tmp:=filepath.Join(a.stateDir,"recents.json.tmp")
	dst:=filepath.Join(a.stateDir,"recents.json")
	if err=os.WriteFile(tmp,data,0600);err==nil {
		if err=os.Rename(tmp,dst);err!=nil {a.Log("Recent history couldn't be saved: "+err.Error())}
	}else{a.Log("Recent history couldn't be saved: "+err.Error())}
	a.rebuildHomeAlbums()
}
func albumKey(artist,album string) string {return artist+""+album}
func (a *App) rebuildHomeAlbums() {
	if a.library==nil {return}
	cards:=make([]MusicCard,0,12)
	seen:=map[string]bool{}
	add:=func(index int) {
		if index<0||index>=len(a.library.Tracks){return}
		t:=a.library.Tracks[index]
		key:=albumKey(t.Artist,t.Album)
		if seen[key]||len(cards)>=12{return}
		seen[key]=true
		cards=append(cards,MusicCard{Name:t.Album,Detail:t.Artist,Key:key,TrackIndex:uint32(index)})
	}
	for _,recent:=range a.Snapshot().Recent {
		for i,t:=range a.library.Tracks {
			if t.ID==recent.ID {add(i);break}
		}
		if len(cards)>=12 {break}
	}
	for i:=range a.library.Tracks {
		if len(cards)>=12 {break}
		add(i)
	}
	a.mu.Lock()
	a.s.HomeAlbums=cards
	a.mu.Unlock()
}
func (a *App) openAlbum(key string) error {
	if a.library==nil{return fmt.Errorf("open an iPod library first")}
	parts:=strings.SplitN(key,"",2)
	if len(parts)!=2{return fmt.Errorf("invalid album selection")}
	a.mu.Lock()
	a.s.Screen="library"
	a.s.TrayOpen=false
	a.s.Category="Songs"
	a.s.Path=[]iap.Selection{{Category:2,Name:parts[0]},{Category:3,Name:parts[1]}}
	a.s.Offset=0
	a.s.Focus=0
	a.mu.Unlock()
	return a.load()
}
func (a *App) playRecent(id uint32) error {
	if a.library==nil{return fmt.Errorf("connect a music library first")}
	for i,t:=range a.library.Tracks {
		if t.ID==id {
			// Select a consistent whole-library queue; don't accidentally
			// inherit the previous album/playlist filtering.
			a.mu.Lock()
			a.s.Path=nil
			a.s.Category="Songs"
			a.mu.Unlock()
			return a.playStorage(uint32(i))
		}
	}
	return fmt.Errorf("recent track is not on the connected iPod")
}
func (a *App) changeSort() error {
	choices:=[]string{"A-Z","Z-A","Artist","Album"}
	a.mu.Lock()
	current:=a.s.SortMode
	index:=0
	for i,v:=range choices {if v==current {index=(i+1)%len(choices);break}}
	a.s.SortMode=choices[index]
	a.s.Offset=0
	a.s.Focus=0
	a.mu.Unlock()
	if a.library!=nil || a.Snapshot().Connected {return a.load()}
	return nil
}
func sortText(mode string,a,b string)bool {
	if mode=="Z-A"{return strings.ToLower(a)>strings.ToLower(b)}
	return strings.ToLower(a)<strings.ToLower(b)
}
func sortedCards(lib *storage.Library,indices []int,mode string) []MusicCard {
	m:=map[string]MusicCard{}
	for _,i:=range indices {
		t:=lib.Tracks[i]
		key:=albumKey(t.Artist,t.Album)
		if _,ok:=m[key];!ok {m[key]=MusicCard{Name:t.Album,Detail:t.Artist,Key:key,TrackIndex:uint32(i)}}
	}
	out:=make([]MusicCard,0,len(m))
	for _,c:=range m{out=append(out,c)}
	sort.Slice(out,func(i,j int)bool {
		a,b:=out[i],out[j]
		if mode=="Artist" {
			if a.Detail!=b.Detail{return sortText("A-Z",a.Detail,b.Detail)}
		}
		if a.Name!=b.Name{return sortText(mode,a.Name,b.Name)}
		return sortText("A-Z",a.Detail,b.Detail)
	})
	return out
}
