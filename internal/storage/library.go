package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
)

type Track struct {
	ID uint32
	Title string
	Artist string
	Album string
	Location string
	Duration uint32
	TrackNumber uint32
}
type Playlist struct {
	Name string
	IDs []uint32
}
type Library struct {
	Root string
	Tracks []Track
	Playlists []Playlist
}

func u32(b []byte, at int) uint32 {
	if at < 0 || at+4 > len(b) { return 0 }
	return binary.LittleEndian.Uint32(b[at:at+4])
}

// sized checks both the chunk and its enclosing allocation. All sizes are
// untrusted bytes from a removable filesystem.
func sized(b []byte, pos, end int, signature string) (header, total int, err error) {
	if pos < 0 || end > len(b) || pos+12 > end { return 0,0,errors.New("truncated iTunesDB record") }
	if string(b[pos:pos+4])!=signature { return 0,0,fmt.Errorf("expected %s at offset %d",signature,pos) }
	header,total=int(u32(b,pos+4)),int(u32(b,pos+8))
	if header<12 || total<header || total>end-pos { return 0,0,fmt.Errorf("invalid %s length at offset %d",signature,pos) }
	return
}
func decodeText(b []byte) string {
	if len(b)<2 {return ""}
	// iPod mhod text is UTF-16LE; discard trailing terminators.
	if len(b)%2!=0 {b=b[:len(b)-1]}
	w:=make([]uint16,len(b)/2)
	for i:=range w {w[i]=binary.LittleEndian.Uint16(b[2*i:])}
	return strings.TrimSpace(strings.TrimRight(string(utf16.Decode(w)),"\x00"))
}
func obj(b []byte,pos,end int) (kind uint32, value string, size int, err error) {
	h,t,e:=sized(b,pos,end,"mhod")
	if e!=nil {return 0,"",0,e}
	kind=u32(b,pos+12)
	if h>=24 && t>=40 {
		n:=int(u32(b,pos+28))
		if n>0 && n<=t-40 && n<=1<<20 {value=decodeText(b[pos+40:pos+40+n])}
	}
	return kind,value,t,nil
}
func parseTrack(b []byte, pos,end int) (Track,int,error) {
	h,t,e:=sized(b,pos,end,"mhit")
	if e!=nil {return Track{},0,e}
	if h<24 {return Track{},0,errors.New("short mhit header")}
	z:=Track{ID:u32(b,pos+16)}
	if h>=44 {z.Duration=u32(b,pos+40)}
	if h>=48 {z.TrackNumber=u32(b,pos+44)}
	cursor:=pos+h
	for cursor<pos+t && stringAt(b,cursor,"mhod") {
		kind,v,n,e:=obj(b,cursor,pos+t)
		if e!=nil{return Track{},0,e}
		switch kind {
		case 1:z.Title=v
		case 2:z.Location=v
		case 3:z.Album=v
		case 4:z.Artist=v
		}
		cursor+=n
	}
	return z,t,nil
}
func stringAt(b []byte,at int,s string) bool {
	return at>=0 && at+len(s)<=len(b) && string(b[at:at+len(s)])==s
}
func parseTracks(b []byte,start,end int) ([]Track,error) {
	if start+12>end || !stringAt(b,start,"mhlt") {return nil,errors.New("missing mhlt")}
	h:=int(u32(b,start+4))
	if h<12 || h>end-start {return nil,errors.New("bad mhlt header")}
	count:=int(u32(b,start+8))
	if count>500000 {return nil,errors.New("unreasonable music library size")}
	tracks:=make([]Track,0,min(count,10000))
	cursor:=start+h
	for i:=0;i<count;i++ {
		z,n,e:=parseTrack(b,cursor,end)
		if e!=nil{return nil,e}
		if z.Location!="" {
			if z.Title=="" {z.Title="Unknown title"}
			if z.Artist=="" {z.Artist="Unknown artist"}
			if z.Album=="" {z.Album="Unknown album"}
			tracks=append(tracks,z)
		}
		cursor+=n
	}
	return tracks,nil
}
func parsePlaylists(b []byte,start,end int) ([]Playlist,error) {
	if start+12>end || !stringAt(b,start,"mhlp") {return nil,errors.New("missing mhlp")}
	h:=int(u32(b,start+4))
	if h<12 || h>end-start {return nil,errors.New("bad mhlp header")}
	count:=int(u32(b,start+8))
	if count>100000 {return nil,errors.New("unreasonable playlist count")}
	cursor:=start+h
	out:=[]Playlist{}
	for i:=0;i<count;i++ {
		hp,tp,e:=sized(b,cursor,end,"mhyp")
		if e!=nil{return nil,e}
		p:=Playlist{}
		child:=cursor+hp
		for child<cursor+tp {
			if stringAt(b,child,"mhod") {
				kind,value,n,e:=obj(b,child,cursor+tp)
				if e!=nil{return nil,e}
				if kind==1 && p.Name=="" {p.Name=value}
				child+=n
			} else if stringAt(b,child,"mhip") {
				hh,tt,e:=sized(b,child,cursor+tp,"mhip")
				if e!=nil{return nil,e}
				if hh>=28 {p.IDs=append(p.IDs,u32(b,child+24))}
				child+=tt
			} else {break}
		}
		// Exclude the built-in master playlist; its track list duplicates
		// the entire library and is not a user-created playlist.
		if u32(b,cursor+20)!=1 && p.Name!="" && len(p.IDs)>0 && len(out)<10000 {
			out=append(out,p)
		}
		cursor+=tp
	}
	return out,nil
}
// Parse checks the stock iTunesDB structure without modifying the original.
func Parse(data []byte, root string) (*Library,error) {
	h,t,e:=sized(data,0,len(data),"mhbd")
	if e!=nil{return nil,e}
	out:=&Library{Root:root}
	for pos:=h;pos<t; {
		dh,dt,e:=sized(data,pos,t,"mhsd")
		if e!=nil{return nil,e}
		if dh<16 {return nil,errors.New("short mhsd")}
		switch u32(data,pos+12) {
		case 1:
			tracks,e:=parseTracks(data,pos+dh,pos+dt)
			if e!=nil{return nil,e}
			out.Tracks=tracks
		case 2:
			playlists,e:=parsePlaylists(data,pos+dh,pos+dt)
			if e==nil {out.Playlists=playlists}
		}
		pos+=dt
	}
	if len(out.Tracks)==0 {return nil,errors.New("no tracks with file locations in iTunesDB")}
	sort.Slice(out.Tracks,func(i,j int)bool{
		x,y:=out.Tracks[i],out.Tracks[j]
		if x.Artist!=y.Artist{return strings.ToLower(x.Artist)<strings.ToLower(y.Artist)}
		if x.Album!=y.Album{return strings.ToLower(x.Album)<strings.ToLower(y.Album)}
		if x.TrackNumber!=y.TrackNumber{return x.TrackNumber<y.TrackNumber}
		return strings.ToLower(x.Title)<strings.ToLower(y.Title)
	})
	return out,nil
}
func Read(root string) (*Library,error) {
	path:=filepath.Join(root,"iPod_Control","iTunes","iTunesDB")
	f,e:=os.Open(path)
	if e!=nil{return nil,fmt.Errorf("open iTunesDB: %w",e)}
	defer f.Close()
	st,e:=f.Stat()
	if e!=nil{return nil,e}
	// A 512 GB collection may have a sizable iTunesDB. Bound memory use
	// rather than allowing a malformed size or huge database to OOM Car Thing.
	if st.Size()<24||st.Size()>160<<20 {return nil,fmt.Errorf("iTunesDB size %d is outside safe range (160 MiB limit)",st.Size())}
	b:=make([]byte,st.Size())
	if _,e=f.ReadAt(b,0);e!=nil{return nil,e}
	return Parse(b,root)
}
// ResolveFile accepts only colon-separated, relative iPod_Control paths;
// reject traversal and symlinks escaping the read-only mount.
func (l *Library) ResolveFile(t Track) (string,error) {
	raw:=strings.Trim(t.Location,":")
	if raw==""||strings.Contains(raw,"/")||strings.Contains(raw,"\\") {return "",errors.New("invalid track path")}
	parts:=strings.Split(raw,":")
	for _,part:=range parts {
		if part==""||part=="."||part==".." {return "",errors.New("invalid track path component")}
	}
	path:=filepath.Join(append([]string{l.Root},parts...)...)
	cleanRoot,e:=filepath.EvalSymlinks(l.Root)
	if e!=nil{return "",e}
	real,e:=filepath.EvalSymlinks(path)
	if e!=nil{return "",e}
	if !strings.HasPrefix(real,cleanRoot+string(os.PathSeparator)) {return "",errors.New("track path escapes iPod mount")}
	return real,nil
}
