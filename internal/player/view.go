package player

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
	xdraw "golang.org/x/image/draw"
)

type Hit struct {
	X, Y, W, H int
	Action     Action
}
type Row struct {
	Title, Detail string
	Action        Action
}
type View struct {
	State          Snapshot
	Title, Caption string
	Rows           []Row
	Tiles          []ModernTile
	Hits           []Hit
	Start          int
}

func (a *App) View() View { return BuildView(a.Snapshot()) }
func BuildView(s Snapshot) View {
	// Connection status must never select a different visual design.
	return BuildModernView(s)
}
func buildLegacyView(s Snapshot) View {
	v := View{State:s,Title:s.Category}
	switch s.Screen {
	case "library":
		v.Caption="Browse your music"
		if s.Total>0 {v.Caption=fmt.Sprintf("%d items",s.Total)}
		if len(s.Path)>0 {var names []string;for _,p:=range s.Path {names=append(names,p.Name)};v.Caption=strings.Join(names," / ")}
		for i,c:=range Categories {v.Hits=append(v.Hits,Hit{16+i*194,83,186,43,Action{Name:"navigate",Text:c}})}
		for i,e:=range s.Entries {v.Rows=append(v.Rows,Row{Title:e.Name,Detail:fmt.Sprintf("%d",int(s.Offset)+i+1),Action:Action{Name:"open",Value:i}})}
	case "settings":
		v.Title,v.Caption="Settings","Music, sound and device"
		v.Rows=[]Row{
			{"iPod & USB","Connection, eject status, diagnostics",Action{Name:"settings-page",Text:"settings-usb"}},
			{"Audio output","Real output or silent preview",Action{Name:"settings-page",Text:"settings-audio"}},
			{"Controls","Wheel and swipe-down presets",Action{Name:"settings-page",Text:"settings-controls"}},
			{"About","Version and credits",Action{Name:"settings-page",Text:"settings-about"}},
		}
	case "settings-usb","diagnostics":
		v.Title,v.Caption="iPod & USB","Storage is always mounted read-only"
		v.Rows=[]Row{
			{"Rescan USB devices",fmt.Sprintf("%d detected",len(s.USB)),Action{Name:"rescan"}},
			{"Enable USB host","Required to read your iPod",Action{Name:"usb-host"}},
			{"Save diagnostics","Report for troubleshooting",Action{Name:"export"}},
			{"Computer recovery","USB network recovery (ejects first)",Action{Name:"usb-recovery"}},
		}
		if s.StorageMode {
			// Eject is the single persistent header action. Do not add
			// a second, scroll-hidden Eject menu item.
			v.Caption="iPod mounted read-only · Eject in top bar"
		} else if s.SafeToDisconnect {
			v.Caption="iPod unmounted. Safe to disconnect."
		} else {
			v.Rows=append([]Row{{"Open iPod music","Read-only library",Action{Name:"storage-open"}}},v.Rows...)
		}
		if s.Error!="" {v.Rows=append(v.Rows,Row{"Last error",s.Error,Action{Name:"rescan"}})}
	case "settings-audio":
		v.Title,v.Caption="Audio","Playback from Car Thing to an output device"
		output:=s.Settings.Output
		if output=="" {output="Select a USB DAC / hub output"}
		v.Rows=[]Row{
			{"Output device",output,Action{Name:"output"}},
			{"Test output","Three-second tone",Action{Name:"tone"}},
		}
	case "settings-controls":
		v.Title,v.Caption="Controls","Customize your hardware buttons"
		for i,b:=range s.Settings.Buttons {v.Rows=append(v.Rows,Row{fmt.Sprintf("Preset %d",i+1),b,Action{Name:"remap",Value:i}})}
		direction:="Normal";if s.Settings.InvertWheel {direction="Reversed"}
		v.Rows=append(v.Rows,Row{"Wheel direction",direction,Action{Name:"invert"}},Row{"Back","",Action{Name:"back"}})
	case "settings-about":
		v.Title,v.Caption="About","Firmware and acknowledgments"
		v.Rows=[]Row{
			{"Version",s.Version,Action{Name:"settings-page",Text:"settings-about"}},
			{"Music source","Read-only iPod storage",Action{Name:"settings-page",Text:"settings-about"}},
			{"Artwork","ArtworkDB, then embedded cover",Action{Name:"settings-page",Text:"settings-about"}},
		}
	case "playing":
		v.Title,v.Caption="Now Playing","Playback on your iPod"
		v.Rows=[]Row{
			{"Play / Pause","",Action{Name:"play"}},
			{"Previous track","",Action{Name:"previous"}},
			{"Next track","",Action{Name:"next"}},
		}
	}
	if s.Screen!="library" {v.Hits=append(v.Hits,Hit{12,10,52,49,Action{Name:"back"}})}
	if s.Screen=="playing" && s.StorageMode && s.Now.Duration>0 {
		for i:=0;i<20;i++ {
			v.Hits=append(v.Hits,Hit{315+i*22,275,22,33,Action{Name:"storage-seek",Value:i*5}})
		}
	}
	if s.Screen=="library"&&!s.Connected {
		action:="storage-open";if s.HostRole=="device" {action="usb-host"}
		if s.AuthRequired && !s.BasicRemote {action="diagnostics"}
		v.Hits=append(v.Hits,Hit{240,267,320,55,Action{Name:action}})
	}
	if s.Screen!="playing" {
		y,step,visible:=101,50,6
		if s.Screen=="library" {y,step,visible=134,52,5}
		v.Start=max(0,min(s.Focus-2,len(v.Rows)-visible))
		for i:=v.Start;i<min(len(v.Rows),v.Start+visible);i++ {
			v.Hits=append(v.Hits,Hit{18,y+(i-v.Start)*step,762,step-3,v.Rows[i].Action})
		}
	}
	v.Hits=append(v.Hits,Hit{14,419,53,52,Action{Name:"settings"}},
		Hit{72,418,490,57,Action{Name:"playing"}},
		Hit{568,420,61,54,Action{Name:"previous"}},
		Hit{635,418,73,56,Action{Name:"play"}},
		Hit{714,420,62,54,Action{Name:"next"}})
	return v
}

var bg = color.RGBA{10, 14, 21, 255}
var panel = color.RGBA{28, 35, 45, 255}
var line = color.RGBA{55, 64, 76, 255}
var textColor = color.RGBA{242, 244, 248, 255}
var muted = color.RGBA{151, 166, 184, 255}
var accent = color.RGBA{96, 154, 255, 255}
var green = color.RGBA{115, 224, 172, 255}

type Painter struct {
	mu           sync.Mutex
	normal, bold *opentype.Font
	faces        map[string]font.Face
}

func NewPainter() (*Painter, error) {
	n, e := opentype.Parse(goregular.TTF)
	if e != nil {
		return nil, e
	}
	b, e := opentype.Parse(gobold.TTF)
	if e != nil {
		return nil, e
	}
	return &Painter{normal: n, bold: b, faces: map[string]font.Face{}}, nil
}
func (p *Painter) face(size int, bold bool) font.Face {
	k := fmt.Sprintf("%d-%t", size, bold)
	if f := p.faces[k]; f != nil {
		return f
	}
	fnt := p.normal
	if bold {
		fnt = p.bold
	}
	f, _ := opentype.NewFace(fnt, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
	p.faces[k] = f
	return f
}
func fill(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	draw.Draw(img, image.Rect(x, y, x+w, y+h), &image.Uniform{C: c}, image.Point{}, draw.Src)
}
func round(img *image.RGBA,x,y,w,h,r int,c color.RGBA) {
	if w<=0||h<=0 {return}
	r=min(r,min(w,h)/2)
	if r<=0 {fill(img,x,y,w,h,c);return}
	vr:=vector.NewRasterizer(w,h)
	fw,fh,fr:=float32(w),float32(h),float32(r)
	k:=fr*0.55228475
	vr.MoveTo(fr,0)
	vr.LineTo(fw-fr,0)
	vr.CubeTo(fw-fr+k,0,fw,fr-k,fw,fr)
	vr.LineTo(fw,fh-fr)
	vr.CubeTo(fw,fh-fr+k,fw-fr+k,fh,fw-fr,fh)
	vr.LineTo(fr,fh)
	vr.CubeTo(fr-k,fh,0,fh-fr+k,0,fh-fr)
	vr.LineTo(0,fr)
	vr.CubeTo(0,fr-k,fr-k,0,fr,0)
	vr.ClosePath()
	vr.Draw(img,image.Rect(x,y,x+w,y+h),&image.Uniform{C:c},image.Point{})
}
func (p *Painter) text(img *image.RGBA, x, y, size int, bold bool, c color.RGBA, s string, maxWidth int) {
	f := p.face(size, bold)
	if maxWidth > 0 {
		r := []rune(s)
		for len(r) > 0 && font.MeasureString(f, string(r)).Ceil() > maxWidth {
			r = r[:len(r)-1]
		}
		if len(r) < len([]rune(s)) {
			for len(r) > 0 && font.MeasureString(f, string(r)+"…").Ceil() > maxWidth {
				r = r[:len(r)-1]
			}
			s = string(r) + "…"
		}
	}
	d := font.Drawer{Dst: img, Src: &image.Uniform{C: c}, Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

func themeAccent(s Snapshot) color.RGBA {
	if s.Accent==[3]uint8{} {return accent}
	return color.RGBA{s.Accent[0],s.Accent[1],s.Accent[2],255}
}
func tintMix(c color.RGBA,n int) color.RGBA {
	n=max(0,min(255,n))
	return color.RGBA{
		uint8((int(c.R)*n+28*(255-n))/255),
		uint8((int(c.G)*n+35*(255-n))/255),
		uint8((int(c.B)*n+45*(255-n))/255),255}
}
func (p *Painter) glyph(img *image.RGBA,x,y int,name string,c color.RGBA) {
	r:=vector.NewRasterizer(32,32)
	poly:=func(points ...float32) {
		if len(points)<6 {return}
		r.MoveTo(points[0],points[1])
		for i:=2;i<len(points);i+=2 {r.LineTo(points[i],points[i+1])}
		r.ClosePath()
	}
	switch name {
	case "play": poly(10,5,27,16,10,27)
	case "pause": poly(9,6,14,6,14,26,9,26);poly(19,6,24,6,24,26,19,26)
	case "next": poly(5,6,19,16,5,26);poly(20,6,25,6,25,26,20,26)
	case "previous": poly(27,6,13,16,27,26);poly(7,6,12,6,12,26,7,26)
	case "back": poly(19,5,22,9,13,16,22,23,19,27,7,16)
	case "gear": poly(14,4,18,4,19,9,23,11,27,9,29,14,25,17,25,21,21,22,19,28,13,28,11,23,7,21,4,23,3,17,7,14,8,10,12,9)
	case "music": poly(17,6,26,4,26,22,23,22,23,10,20,11,20,24,17,24);poly(9,19,16,17,18,20,16,25,9,27,6,23)
	case "chevron": poly(13,8,17,8,24,16,17,24,13,24,20,16)
	}
	r.Draw(img,image.Rect(x,y,x+32,y+32),&image.Uniform{C:c},image.Point{})
}
func (p *Painter) Render(v View) *image.RGBA {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.renderModern(v)
}

func (p *Painter) ipod(img *image.RGBA, x, y int, scale float64) {
	w, h := int(105*scale), int(157*scale)
	round(img, x, y, w, h, 12, color.RGBA{182, 189, 197, 255})
	round(img, x+8, y+10, w-16, h/3, 5, color.RGBA{46, 60, 67, 255})
	cx, cy, r := x+w/2, y+h*3/4, w/3
	for yy := -r; yy <= r; yy++ {
		for xx := -r; xx <= r; xx++ {
			if xx*xx+yy*yy <= r*r {
				img.SetRGBA(cx+xx, cy+yy, textColor)
			}
		}
	}
	rr := r / 3
	for yy := -rr; yy <= rr; yy++ {
		for xx := -rr; xx <= rr; xx++ {
			if xx*xx+yy*yy <= rr*rr {
				img.SetRGBA(cx+xx, cy+yy, color.RGBA{182, 189, 197, 255})
			}
		}
	}
}
func (p *Painter) playing(img *image.RGBA,s Snapshot) {
	ac:=themeAccent(s)
	if s.HasArtwork && s.Artwork != nil {
		// Catmull-Rom scaling avoids blocky nearest-neighbor cover art.
		cover := image.NewRGBA(image.Rect(0,0,230,230))
		xdraw.CatmullRom.Scale(cover,cover.Bounds(),s.Artwork,s.Artwork.Bounds(),xdraw.Over,nil)
		draw.Draw(img,image.Rect(35,115,265,345),cover,image.Point{},draw.Over)
	} else {
		round(img,35,115,230,230,22,tintMix(ac,78))
		round(img,58,138,184,184,18,tintMix(ac,35))
		p.glyph(img,134,205,"music",textColor)
	}
	track:=s.Now.Title
	if track=="" {track="Nothing playing"}
	p.text(img,315,161,30,true,textColor,track,450)
	p.text(img,315,201,21,false,muted,s.Now.Artist,450)
	p.text(img,315,237,17,false,muted,s.Now.Album,450)
	round(img,315,285,439,6,3,line)
	if s.Now.Duration>0 {
		portion:=uint64(min(s.Now.Position,s.Now.Duration))*439/uint64(s.Now.Duration)
		if portion>0 {round(img,315,285,int(portion),6,3,ac)}
	}
	p.text(img,315,321,14,false,muted,clock(s.Now.Position),120)
	p.text(img,698,321,14,false,muted,clock(s.Now.Duration),60)
	p.text(img,315,374,15,false,muted,"Use the controls below for playback",450)
}
func clock(ms uint32) string { return fmt.Sprintf("%d:%02d", ms/60000, ms/1000%60) }
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
