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
	Hits           []Hit
	Start          int
}

func (a *App) View() View { return BuildView(a.Snapshot()) }
func BuildView(s Snapshot) View {
	v := View{State:s,Title:s.Category}
	switch s.Screen {
	case "library":
		v.Caption="Browse your music"
		if s.Total>0 {v.Caption=fmt.Sprintf("%d items",s.Total)}
		if len(s.Path)>0 {var names []string;for _,p:=range s.Path {names=append(names,p.Name)};v.Caption=strings.Join(names," / ")}
		for i,c:=range Categories {v.Hits=append(v.Hits,Hit{16+i*194,83,186,43,Action{Name:"navigate",Text:c}})}
		for i,e:=range s.Entries {v.Rows=append(v.Rows,Row{Title:e.Name,Detail:fmt.Sprintf("%d",int(s.Offset)+i+1),Action:Action{Name:"open",Value:i}})}
	case "settings":
		v.Title,v.Caption="Settings","Connection, audio and preferences"
		v.Rows=[]Row{
			{"Connection & USB","iPod and recovery",Action{Name:"settings-page",Text:"settings-usb"}},
			{"Audio","Output and digital audio",Action{Name:"settings-page",Text:"settings-audio"}},
			{"Controls","Wheel and presets",Action{Name:"settings-page",Text:"settings-controls"}},
			{"About","Version and credits",Action{Name:"settings-page",Text:"settings-about"}},
			{"Back to music","",Action{Name:"back"}},
		}
	case "settings-usb","diagnostics":
		v.Title,v.Caption="Connection","USB host, iPod and recovery"
		v.Rows=[]Row{
			{"Connect iPod",s.Name,Action{Name:"connect"}},
			{"Rescan devices",fmt.Sprintf("%d USB devices",len(s.USB)),Action{Name:"rescan"}},
			{"Enable USB host","Normal iPod use",Action{Name:"usb-host"}},
			{"Computer recovery","USB diagnostic download",Action{Name:"usb-recovery"}},
			{"Save diagnostic report","Stored on device",Action{Name:"export"}},
			{"Disconnect iPod","Release accessory mode",Action{Name:"disconnect"}},
		}
		if s.Error!="" {v.Rows=append(v.Rows,Row{"Last error",s.Error,Action{Name:"rescan"}})}
		for _,d:=range s.USB {if d.IPod {v.Rows=append(v.Rows,Row{"Detected iPod",d.Product+fmt.Sprintf(" / cfg %d",d.Active),Action{Name:"rescan"}})}}
		v.Rows=append(v.Rows,Row{"Back","",Action{Name:"back"}})
	case "settings-audio":
		v.Title,v.Caption="Audio","Playback and output settings"
		capture,output:=s.Settings.Capture,s.Settings.Output
		if capture=="" {capture="Not selected"}
		if output=="" {output="Not selected"}
		v.Rows=[]Row{
			{"Request digital audio","iPod accessory mode required",Action{Name:"digital"}},
			{"iPod audio capture",capture,Action{Name:"capture"}},
			{"Hub / DAC output",output,Action{Name:"output"}},
			{"Test DAC tone","3 second test",Action{Name:"tone"}},
			{"Start audio route","iPod to USB output",Action{Name:"audio-start"}},
			{"Stop audio",s.AudioStatus,Action{Name:"audio-stop"}},
			{"Back","",Action{Name:"back"}},
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
			{"Bootloader","ThingLabsOSS Superbird U-Boot",Action{Name:"settings-page",Text:"settings-about"}},
			{"Kernel and BSP","Joey Eamigh / yocto-superbird",Action{Name:"settings-page",Text:"settings-about"}},
			{"Artwork colors","Auto when iPod artwork is available",Action{Name:"settings-page",Text:"settings-about"}},
			{"Back","",Action{Name:"back"}},
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
	if s.Screen=="library"&&!s.Connected {
		action:="connect";if s.HostRole=="device" {action="usb-host"}
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
	st:=v.State
	img:=image.NewRGBA(image.Rect(0,0,800,480))
	ac:=themeAccent(st)
	fill(img,0,0,800,480,bg)
	for y:=0;y<415;y+=5 {
		factor:=max(0,24-y/23)
		if y<65 {factor+=8}
		fill(img,0,y,800,5,tintMix(ac,factor))
	}
	if st.Screen!="library" {
		round(img,16,10,44,45,13,panel)
		p.glyph(img,22,16,"back",textColor)
	}
	titleX:=25
	if st.Screen!="library" {titleX=78}
	p.text(img,titleX,44,29,true,textColor,v.Title,502)
	p.text(img,titleX,70,13,false,muted,v.Caption,573)
	status:="NOT CONNECTED"
	if st.Connected {status="IPOD CONNECTED"}
	if st.Busy {status="CONNECTING"}
	statusColor:=muted
	if st.Connected {statusColor=green}
	if st.Busy {statusColor=ac}
	round(img,619,13,167,36,18,panel)
	p.text(img,632,37,13,true,statusColor,status,151)
	if st.Screen=="library" {
		for i,category:=range Categories {
			x:=16+i*194
			selected:=category==st.Category
			col,fg:=panel,muted
			if selected {col,fg=tintMix(ac,70),textColor}
			round(img,x,83,186,43,13,col)
			p.text(img,x+16,111,17,selected,fg,category,157)
		}
	}
	if st.Screen=="playing" {
		p.playing(img,st)
	} else if st.Screen=="library"&&!st.Connected {
		round(img,24,146,752,251,21,panel)
		round(img,76,183,88,88,24,tintMix(ac,80))
		p.glyph(img,104,210,"music",textColor)
		p.text(img,194,209,27,true,textColor,"Your music, ready to go.",550)
		p.text(img,194,244,16,false,muted,"Connect your iPod to browse and play.",550)
		round(img,240,267,320,55,17,ac)
		label:="Connect iPod"
		if st.HostRole=="device" {label="Enable USB host"}
		p.text(img,327,302,20,true,bg,label,215)
		p.text(img,244,366,15,false,muted,"Use Settings for USB and audio options",490)
	} else {
		y,step,count:=101,50,6
		if st.Screen=="library" {y,step,count=134,52,5}
		if len(v.Rows)==0 {p.text(img,36,213,22,false,muted,"Nothing to show yet",690)}
		for i:=v.Start;i<min(len(v.Rows),v.Start+count);i++ {
			item:=v.Rows[i]
			yy:=y+(i-v.Start)*step
			col:=panel
			if i==st.Focus {col=tintMix(ac,46)}
			round(img,18,yy,762,step-3,14,col)
			if i==st.Focus {round(img,20,yy+9,3,step-21,2,ac)}
			p.text(img,36,yy+31,18,i==st.Focus,textColor,item.Title,499)
			p.text(img,550,yy+30,13,false,muted,item.Detail,178)
			p.glyph(img,743,yy+8,"chevron",muted)
		}
	}
	message:=st.Status
	messageColor:=muted
	if st.Error!="" {message=st.Error;messageColor=color.RGBA{255,148,149,255}}
	if st.Screen!="playing"&&message!="" {p.text(img,22,410,12,false,messageColor,message,752)}
	fill(img,0,418,800,62,panel)
	fill(img,0,418,800,1,line)
	round(img,16,424,44,43,13,tintMix(ac,58))
	p.glyph(img,22,429,"gear",textColor)
	track:=st.Now.Title
	if track=="" {track="Nothing playing"}
	p.text(img,80,442,18,true,textColor,track,470)
	artist:=st.Now.Artist
	if artist=="" {artist="Your iPod music"}
	p.text(img,80,464,13,false,muted,artist,470)
	p.glyph(img,582,431,"previous",textColor)
	round(img,641,421,65,51,16,ac)
	if st.Now.Playing {p.glyph(img,659,430,"pause",bg)} else {p.glyph(img,659,430,"play",bg)}
	p.glyph(img,729,431,"next",textColor)
	return img
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
