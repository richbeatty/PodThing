package storage

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func put32(b []byte, at int, value int){binary.LittleEndian.PutUint32(b[at:],uint32(value))}
func chunk(kind string, header int, body []byte) []byte {
	b:=make([]byte,header+len(body))
	copy(b,kind)
	put32(b,4,header)
	put32(b,8,len(b))
	copy(b[header:],body)
	return b
}
func stringChunk(kind int, content string) []byte {
	var raw []byte
	for _,v:=range utf16.Encode([]rune(content)){
		raw=append(raw,byte(v),byte(v>>8))
	}
	b:=chunk("mhod",40,raw)
	put32(b,12,kind)
	put32(b,28,len(raw))
	return b
}
func sampleTrack(id int, title, artist, album, path string) []byte {
	fields:=append(stringChunk(1,title),stringChunk(4,artist)...)
	fields=append(fields,stringChunk(3,album)...)
	fields=append(fields,stringChunk(2,path)...)
	b:=chunk("mhit",64,fields)
	put32(b,12,4)
	put32(b,16,id)
	put32(b,40,180000)
	return b
}
func fixture() []byte {
	mhit1:=sampleTrack(11,"First Song","Artist","Album",":iPod_Control:Music:F00:AAAA.mp3")
	mhit2:=sampleTrack(12,"Second Song","Artist","Album",":iPod_Control:Music:F00:BBBB.mp3")
	mhlt:=make([]byte,12)
	copy(mhlt,"mhlt")
	put32(mhlt,4,12)
	put32(mhlt,8,2)
	tracks:=append(mhlt,mhit1...)
	tracks=append(tracks,mhit2...)
	d1:=chunk("mhsd",24,tracks)
	put32(d1,12,1)
	mhip:=chunk("mhip",32,nil)
	put32(mhip,24,12)
	pl:=chunk("mhyp",48,append(stringChunk(1,"Road Trip"),mhip...))
	d2:=chunk("mhsd",24,append([]byte("mhlp"),make([]byte,8)...))
	put32(d2,12,2)
	// The actual mhlp has a 12-byte header with number of playlists.
	mhlp:=make([]byte,12)
	copy(mhlp,"mhlp")
	put32(mhlp,4,12)
	put32(mhlp,8,1)
	d2=chunk("mhsd",24,append(mhlp,pl...))
	put32(d2,12,2)
	root:=chunk("mhbd",64,append(d1,d2...))
	put32(root,16,0x19)
	return root
}
func TestParseClassicMusicAndPlaylists(t *testing.T){
	root:=t.TempDir()
	lib,e:=Parse(fixture(),root)
	if e!=nil{t.Fatal(e)}
	if len(lib.Tracks)!=2||lib.Tracks[0].Title!="First Song"||lib.Tracks[1].Location!=":iPod_Control:Music:F00:BBBB.mp3"{
		t.Fatalf("parsed tracks %+v",lib.Tracks)
	}
	if len(lib.Playlists)!=1||lib.Playlists[0].Name!="Road Trip"||len(lib.Playlists[0].IDs)!=1||lib.Playlists[0].IDs[0]!=12{
		t.Fatalf("parsed playlists %+v",lib.Playlists)
	}
	dir:=filepath.Join(root,"iPod_Control","Music","F00")
	if e=os.MkdirAll(dir,0700);e!=nil{t.Fatal(e)}
	if e=os.WriteFile(filepath.Join(dir,"AAAA.mp3"),[]byte("dummy"),0600);e!=nil{t.Fatal(e)}
	path,e:=lib.ResolveFile(lib.Tracks[0])
	if e!=nil||path!=filepath.Join(dir,"AAAA.mp3"){t.Fatalf("file path %q %v",path,e)}
}
func TestRejectMalformedDBAndUnsafePaths(t *testing.T){
	for _,data:=range [][]byte{
		{},
		[]byte("mhbd"),
		func()[]byte{b:=fixture();put32(b,8,len(b)+1000);return b}(),
		func()[]byte{b:=fixture();put32(b,4,0);return b}(),
	} {
		if _,e:=Parse(data,t.TempDir());e==nil{t.Fatal("accepted invalid DB")}
	}
	lib:=&Library{Root:t.TempDir()}
	for _,p:=range []string{":iPod_Control:..:etc:passwd",":iPod_Control:/etc/passwd",":iPod_Control:Music:../../evil"}{
		_,e:=lib.ResolveFile(Track{Location:p})
		if e==nil{t.Fatalf("unsafe path accepted: %s",p)}
	}
}
func TestReadNeverWritesDatabase(t *testing.T){
	root:=t.TempDir()
	dir:=filepath.Join(root,"iPod_Control","iTunes")
	if e:=os.MkdirAll(dir,0500);e!=nil{t.Fatal(e)}
	path:=filepath.Join(dir,"iTunesDB")
	data:=fixture()
	if e:=os.WriteFile(path,data,0400);e!=nil{t.Fatal(e)}
	lib,e:=Read(root)
	if e!=nil{t.Fatal(e)}
	if len(lib.Tracks)!=2{t.Fatal("wrong track count")}
	after,e:=os.ReadFile(path)
	if e!=nil{t.Fatal(e)}
	if string(after)!=string(data) {t.Fatal("reader modified on-device database")}
}
func TestSafeEjectStateWhenNotMounted(t *testing.T){
	v:=&Volume{}
	if e:=v.Eject();e!=nil{t.Fatal(e)}
	if !strings.Contains(v.Status(),"Ejected"){t.Fatal("incorrect eject state")}
}
