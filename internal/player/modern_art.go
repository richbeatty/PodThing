package player

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"io"
	"os/exec"
	"time"

)

// requestCovers requests only the visible album cards. The worker is
// asynchronous, strictly serialized, and bounded by a decoder timeout;
// eject waits for any active read before unmounting the iPod.
func (a *App) requestCovers(cards []MusicCard) {
	if a.volume==nil{return}
	for _,c:=range cards {
		a.mu.Lock()
		if a.coverTried==nil {a.coverTried=map[string]bool{}}
		tried:=a.coverTried[c.Key]
		if !tried {a.coverTried[c.Key]=true}
		a.mu.Unlock()
		if !tried {go a.fetchCover(c)}
	}
}
func (a *App) fetchCover(card MusicCard) {
	a.artFetch.Lock()
	defer a.artFetch.Unlock()
	a.artUse.RLock()
	defer a.artUse.RUnlock()
	if a.library==nil || a.volume==nil {return}
	if int(card.TrackIndex)>=len(a.library.Tracks) {return}
	t:=a.library.Tracks[card.TrackIndex]
	path,e:=a.library.ResolveFile(t)
	if e!=nil{return}
	img,e:=decodeEmbeddedCover(path)
	if e!=nil || img==nil {return}
	a.mu.Lock()
	if a.library!=nil && a.volume!=nil {
		if a.s.Covers==nil {a.s.Covers=map[string]image.Image{}}
		a.s.Covers[card.Key]=img
		if a.activeTrack>=0 && a.activeTrack<len(a.library.Tracks) {
			current:=a.library.Tracks[a.activeTrack]
			if albumKey(current.Artist,current.Album)==card.Key && a.s.Now.Title==current.Title {
				a.s.Artwork=img
				a.s.HasArtwork=true
				a.s.Accent=ArtworkAccent(img)
			}
		}
	}
	a.mu.Unlock()
}
func decodeEmbeddedCover(path string) (image.Image,error) {
	ctx,cancel:=context.WithTimeout(context.Background(),2*time.Second)
	defer cancel()
	cmd:=exec.CommandContext(ctx,"/usr/bin/ffmpeg",
		"-nostdin","-hide_banner","-v","error","-i",path,
		"-an","-map","0:v:0","-frames:v","1",
		"-vf","scale=160:160:force_original_aspect_ratio=decrease",
		"-f","image2pipe","-vcodec","mjpeg","pipe:1")
	pipe,e:=cmd.StdoutPipe()
	if e!=nil{return nil,e}
	if e=cmd.Start();e!=nil{return nil,e}
	b,e:=io.ReadAll(io.LimitReader(pipe,2<<20))
	_ = cmd.Wait()
	if e!=nil{return nil,e}
	if len(b)==0 || len(b)>=2<<20 {return nil,nil}
	img,e:=jpeg.Decode(bytes.NewReader(b))
	if e!=nil {return nil,e}
	return img,nil
}

