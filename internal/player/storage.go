package player

import (
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"math/rand"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/richbeatty/PodThing/internal/hardware"
	"github.com/richbeatty/PodThing/internal/iap"
	"github.com/richbeatty/PodThing/internal/storage"
)

func (a *App) filteredStorageTracks(s Snapshot) []int {
	if a.library==nil{return nil}
	artist,album:="",""
	var playlist *storage.Playlist
	for _,p:=range s.Path {
		switch p.Category {
		case 2:artist=p.Name
		case 3:album=p.Name
		case 1:
			if int(p.Index)<len(a.library.Playlists) {
				playlist=&a.library.Playlists[p.Index]
			}
		}
	}
	out:=[]int{}
	if playlist!=nil {
		ids:=map[uint32]int{}
		for i,t:=range a.library.Tracks{ids[t.ID]=i}
		for _,id:=range playlist.IDs {
			if i,ok:=ids[id];ok{out=append(out,i)}
		}
		return out
	}
	for i,t:=range a.library.Tracks {
		if artist!=""&&t.Artist!=artist{continue}
		if album!=""&&t.Album!=album{continue}
		out=append(out,i)
	}
	return out
}
func (a *App) loadStorage(s Snapshot) error {
	if a.library==nil{return errors.New("no read-only iPod library")}
	rows:=[]iap.Entry{}
	cards:=[]MusicCard{}
	switch s.Category {
	case "Artists","Albums":
		names:=map[string]bool{}
		for _,index:=range a.filteredStorageTracks(s) {
			t:=a.library.Tracks[index]
			name:=t.Artist
			if s.Category=="Albums" {name=t.Album}
			names[name]=true
		}
		var keys []string
		for k:=range names{keys=append(keys,k)}
		sort.Slice(keys,func(i,j int)bool{return sortText(s.SortMode,keys[i],keys[j])})
		if s.Category=="Albums" {
			indices:=a.filteredStorageTracks(s)
			cards=sortedCards(a.library,indices,s.SortMode)
			for i,card:=range cards {rows=append(rows,iap.Entry{Index:uint32(i),Name:card.Name})}
		}else{
			for i,k:=range keys {rows=append(rows,iap.Entry{Index:uint32(i),Name:k})}
		}
	case "Songs":
		indices:=a.filteredStorageTracks(s)
		// Preserve source playlist order by default, while allowing a
		// user-selected alphabetical/artist/album sort.
		if len(s.Path)==0 || s.SortMode!="A-Z" {
			sort.SliceStable(indices,func(i,j int)bool {
				a,b:=a.library.Tracks[indices[i]],a.library.Tracks[indices[j]]
				switch s.SortMode {
				case "Artist":
					if a.Artist!=b.Artist{return sortText("A-Z",a.Artist,b.Artist)}
				case "Album":
					if a.Album!=b.Album{return sortText("A-Z",a.Album,b.Album)}
				}
				return sortText(s.SortMode,a.Title,b.Title)
			})
		}
		for _,i:=range indices {
			t:=a.library.Tracks[i]
			rows=append(rows,iap.Entry{Index:uint32(i),Name:t.Title})
		}
	case "Playlists":
		for i,p:=range a.library.Playlists {
			rows=append(rows,iap.Entry{Index:uint32(i),Name:p.Name})
		}
		sort.Slice(rows,func(i,j int)bool{return sortText(s.SortMode,rows[i].Name,rows[j].Name)})
	default:return fmt.Errorf("unsupported library category %q",s.Category)
	}
	start:=min(int(s.Offset),len(rows))
	end:=min(start+PageSize,len(rows))
	a.mu.Lock()
	a.s.Entries=append([]iap.Entry(nil),rows[start:end]...)
	a.s.Cards=nil
	if len(cards)>0 {
		a.s.Cards=append([]MusicCard(nil),cards[start:end]...)
	}
	a.s.Total=uint32(len(rows))
	if a.s.Focus>=len(a.s.Entries){a.s.Focus=max(0,len(a.s.Entries)-1)}
	a.mu.Unlock()
	return nil
}
func (a *App) playStorage(index uint32) error {
	if a.library==nil {return errors.New("the iPod is not mounted")}
	if int(index)>=len(a.library.Tracks){return errors.New("selected track is not in the iPod database")}
	s:=a.Snapshot()
	output:=s.Settings.Output
	t:=a.library.Tracks[index]
	if s.Settings.AudioMode=="simulated" {
		if err:=a.player.Stop();err!=nil{return err}
		a.sim.Start(t.Duration)
	}else{
		if output=="" {return errors.New("no audio output selected. Choose Audio > Hub / DAC output first")}
		found:=false
		for _,p:=range hardware.ScanAudio() {
			if !p.Capture && p.ID==output && p.USB && !p.IPod {found=true}
		}
		if !found{return errors.New("selected USB DAC is not available. Music cannot play without a real audio output")}
		path,e:=a.library.ResolveFile(t)
		if e!=nil{return fmt.Errorf("music file cannot be opened safely: %w",e)}
		if e:=a.player.Start(path,t,output,0);e!=nil{return e}
		a.sim.Reset()
	}
	queue:=a.filteredStorageTracks(s)
	if len(queue)==0 {
		for i:=range a.library.Tracks{queue=append(queue,i)}
	}
	a.queue=queue
	a.activeTrack=int(index)
	a.mu.Lock()
	shuffle,repeat:=a.s.Now.Shuffle,a.s.Now.Repeat
	a.s.Now=iap.NowPlaying{Title:t.Title,Artist:t.Artist,Album:t.Album,Duration:max(t.Duration,1),Playing:true,Shuffle:shuffle,Repeat:repeat}
	if s.Settings.AudioMode=="simulated" && t.Duration==0 {a.s.Now.Duration=240000}
	a.s.Screen="playing"
	a.s.Focus=0
	a.s.Status="Playing from iPod USB storage (read-only)"
	if s.Settings.AudioMode=="simulated" {a.s.Status="Simulated playback — no sound output"}
	a.s.Artwork=nil
	a.s.HasArtwork=false
	a.mu.Unlock()
	// Embedded cover images are optional; iPod ArtworkDB is a separate
	// container and may hold artwork unavailable in the music files.
	if s.Settings.AudioMode=="usb" {
		path,e:=a.library.ResolveFile(t)
		if e==nil {a.extractEmbeddedArt(path,int(index))}
	}
	a.recordRecent(t)
	if s.Settings.AudioMode=="simulated" {
		a.Log("STORAGE simulated playback (no decoder): "+t.Artist+" / "+t.Title)
	}else{
		a.Log("STORAGE local decode: "+t.Artist+" / "+t.Title)
	}
	return nil
}
func (a *App) extractEmbeddedArt(path string, trackIndex int) {
	ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second)
	defer cancel()
	cmd:=exec.CommandContext(ctx,"/usr/bin/ffmpeg","-nostdin","-v","error","-i",path,
		"-an","-map","0:v:0","-frames:v","1","-f","image2pipe","-vcodec","mjpeg","pipe:1")
	pipe,e:=cmd.StdoutPipe()
	if e!=nil{return}
	if e=cmd.Start();e!=nil{return}
	data,e:=io.ReadAll(io.LimitReader(pipe,4<<20))
	_ = cmd.Wait()
	if e!=nil||len(data)==0||len(data)>=4<<20 {return}
	img,e:=jpeg.Decode(strings.NewReader(string(data)))
	if e!=nil{return}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.library!=nil&&a.activeTrack==trackIndex {
		a.s.Artwork=img
		a.s.HasArtwork=true
	}
}
func (a *App) storageSeek(percent int) error {
	if a.library==nil {return errors.New("music library is not open")}
	if percent<0||percent>100{return errors.New("seek position must be 0-100")}
	t:=a.library.Tracks[a.activeTrack]
	if t.Duration==0 {return errors.New("this track has no known duration")}
	pos:=uint32(uint64(t.Duration)*uint64(percent)/100)
	if a.Snapshot().Settings.AudioMode=="simulated" {
		a.sim.Seek(pos)
	}else{
		path,e:=a.library.ResolveFile(t)
		if e!=nil{return e}
		out:=a.Snapshot().Settings.Output
		if e=a.player.Start(path,t,out,pos);e!=nil{return e}
	}
	a.mu.Lock()
	a.s.Now.Position=pos
	a.s.Now.Playing=true
	a.mu.Unlock()
	return nil
}
func (a *App) storageControl(name string) error {
	if a.library==nil{return errors.New("read-only library not mounted")}
	if name=="play" {
		if a.Snapshot().Settings.AudioMode=="simulated" && a.sim.duration>0 {
			playing:=a.sim.Toggle()
			a.mu.Lock()
			a.s.Now.Playing=playing
			a.mu.Unlock()
			return nil
		}
		if a.player.Active() {
			playing,e:=a.player.Toggle()
			if e!=nil{return e}
			a.mu.Lock()
			a.s.Now.Playing=playing
			a.mu.Unlock()
			return nil
		}
		return a.playStorage(uint32(a.activeTrack))
	}
	if name!="next" && name!="previous" {return errors.New("unknown playback command")}
	if len(a.queue)==0{
		for i:=range a.library.Tracks{a.queue=append(a.queue,i)}
	}
	current:=-1
	for i,id:=range a.queue{if id==a.activeTrack {current=i;break}}
	if current<0{return errors.New("current track not in queue")}
	s:=a.Snapshot()
	if name=="next" && s.Now.Shuffle!=0 && len(a.queue)>1 {
		next:=rand.Intn(len(a.queue)-1)
		if next>=current{next++}
		return a.playStorage(uint32(a.queue[next]))
	}
	advance:=1
	if name=="previous"{advance=-1}
	n:=current+advance
	if n<0||n>=len(a.queue){
		if s.Now.Repeat==2 {
			if n<0{n=len(a.queue)-1}else{n=0}
		}else{
			return errors.New("end of current playlist")
		}
	}
	return a.playStorage(uint32(a.queue[n]))
}
