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
	v := View{State: s, Title: s.Category}
	v.Hits = append(v.Hits, Hit{690, 12, 94, 36, Action{Name: "settings"}})
	for i, c := range Categories {
		v.Hits = append(v.Hits, Hit{16 + i*194, 62, 186, 40, Action{Name: "navigate", Text: c}})
	}
	switch s.Screen {
	case "library":
		v.Caption = fmt.Sprintf("%d items", s.Total)
		if len(s.Path) > 0 {
			var names []string
			for _, p := range s.Path {
				names = append(names, p.Name)
			}
			v.Caption = strings.Join(names, " / ")
		}
		for i, e := range s.Entries {
			v.Rows = append(v.Rows, Row{Title: e.Name, Detail: fmt.Sprintf("%d", e.Index+1), Action: Action{Name: "open", Value: i}})
		}
		if len(s.Path) > 0 {
			v.Hits = append(v.Hits, Hit{16, 111, 45, 40, Action{Name: "back"}})
		}
	case "settings":
		v.Title = "Settings"
		v.Caption = "Press a row to change its value"
		for i, b := range s.Settings.Buttons {
			v.Rows = append(v.Rows, Row{fmt.Sprintf("Top button %d", i+1), b, Action{Name: "remap", Value: i}})
		}
		direction := "Normal"
		if s.Settings.InvertWheel {
			direction = "Reversed"
		}
		v.Rows = append(v.Rows, Row{"Wheel direction", direction, Action{Name: "invert"}}, Row{"USB & audio diagnostics", "Connection, DAC, reports", Action{Name: "diagnostics"}}, Row{"Back to library", "", Action{Name: "back"}})
	case "diagnostics":
		v.Title = "USB & Audio"
		v.Caption = "Scroll for audio controls and reports"
		capture, output := s.Settings.Capture, s.Settings.Output
		if capture == "" {
			capture = "Not selected"
		}
		if output == "" {
			output = "Not selected"
		}
		v.Rows = []Row{{"Connect iPod", s.Name, Action{Name: "connect"}}, {"Rescan devices", fmt.Sprintf("%d USB / %d PCM", len(s.USB), len(s.Audio)), Action{Name: "rescan"}}, {"Request iPod digital audio", "Authentication may be required", Action{Name: "digital"}}, {"iPod audio capture", capture, Action{Name: "capture"}}, {"Hub / DAC output", output, Action{Name: "output"}}, {"Test DAC tone (3 seconds)", "Tests output only; start with car volume low", Action{Name: "tone"}}, {"Start iPod audio route", "Selected capture to selected DAC", Action{Name: "audio-start"}}, {"Stop audio", s.AudioStatus, Action{Name: "audio-stop"}}, {"Save diagnostic report", "Saved locally; download from browser", Action{Name: "export"}}, {"Disconnect iPod", "Release accessory mode", Action{Name: "disconnect"}}}
		v.Rows = append(v.Rows, Row{"Enable USB host", "Use a powered OTG hub; role: " + s.HostRole, Action{Name: "usb-host"}}, Row{"USB recovery console", "Connect computer, then open 10.42.1.2:8787", Action{Name: "usb-recovery"}})
		for _, d := range s.USB {
			title := d.Product
			if title == "" {
				title = d.Vendor + ":" + d.ProductID
			}
			v.Rows = append(v.Rows, Row{title, fmt.Sprintf("USB %s / cfg %d", d.Name, d.Active), Action{Name: "rescan"}})
		}
		for _, p := range s.Audio {
			kind := "Output"
			if p.Capture {
				kind = "Capture"
			}
			v.Rows = append(v.Rows, Row{p.Name, kind + " " + p.ID, Action{Name: "rescan"}})
		}
		if s.Error != "" {
			v.Rows = append(v.Rows, Row{"Last connection error", s.Error, Action{Name: "diagnostics"}})
		}
		v.Rows = append(v.Rows, Row{"Back to library", "USB role: " + s.HostRole, Action{Name: "back"}})
	case "playing":
		v.Title = "Now Playing"
		v.Caption = "Playback stays on your iPod"
		v.Rows = []Row{{"Play / Pause", "", Action{Name: "play"}}, {"Previous track", "", Action{Name: "previous"}}, {"Next track", "", Action{Name: "next"}}, {"Shuffle", []string{"Off", "Songs", "Albums"}[min(int(s.Now.Shuffle), 2)], Action{Name: "shuffle"}}, {"Repeat", []string{"Off", "One", "All"}[min(int(s.Now.Repeat), 2)], Action{Name: "repeat"}}}
	}
	if s.Screen != "playing" {
		v.Start = max(0, min(s.Focus-2, len(v.Rows)-5))
		for i := v.Start; i < min(len(v.Rows), v.Start+5); i++ {
			v.Hits = append(v.Hits, Hit{16, 154 + (i-v.Start)*44, 768, 42, v.Rows[i].Action})
		}
	} else {
		for i := 0; i < len(v.Rows); i++ {
			v.Hits = append(v.Hits, Hit{475, 148 + i*44, 309, 42, v.Rows[i].Action})
		}
	}
	if s.Screen == "library" && !s.Connected {
		connectAction := "connect"
		if s.HostRole == "device" {
			connectAction = "usb-host"
		}
		v.Hits = append(v.Hits, Hit{283, 283, 234, 48, Action{Name: connectAction}}, Hit{283, 337, 234, 36, Action{Name: "diagnostics"}})
	}
	v.Hits = append(v.Hits, Hit{0, 404, 550, 76, Action{Name: "playing"}}, Hit{556, 417, 64, 48, Action{Name: "previous"}}, Hit{628, 411, 76, 60, Action{Name: "play"}}, Hit{712, 417, 64, 48, Action{Name: "next"}})
	return v
}

var bg = color.RGBA{12, 15, 19, 255}
var panel = color.RGBA{23, 28, 35, 255}
var line = color.RGBA{39, 45, 55, 255}
var textColor = color.RGBA{242, 244, 248, 255}
var muted = color.RGBA{144, 157, 174, 255}
var accent = color.RGBA{255, 166, 76, 255}
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
func round(img *image.RGBA, x, y, w, h, r int, c color.RGBA) {
	fill(img, x+r, y, w-2*r, h, c)
	fill(img, x, y+r, w, h-2*r, c)
	for yy := 0; yy < r; yy++ {
		for xx := 0; xx < r; xx++ {
			if (xx-r)*(xx-r)+(yy-r)*(yy-r) <= r*r {
				img.SetRGBA(x+xx, y+yy, c)
				img.SetRGBA(x+w-1-xx, y+yy, c)
				img.SetRGBA(x+xx, y+h-1-yy, c)
				img.SetRGBA(x+w-1-xx, y+h-1-yy, c)
			}
		}
	}
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
func (p *Painter) Render(v View) *image.RGBA {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := v.State
	img := image.NewRGBA(image.Rect(0, 0, 800, 480))
	fill(img, 0, 0, 800, 480, bg)
	round(img, 16, 17, 10, 25, 5, accent)
	p.text(img, 36, 40, 27, true, textColor, "PodThing", 210)
	status := "DISCONNECTED"
	c := muted
	if s.Connected {
		status = "IPOD CONNECTED"
		c = green
	}
	if s.Demo {
		status = "DEMO · NO AUDIO"
		c = accent
	}
	if s.Busy {
		status = "WORKING…"
		c = accent
	}
	p.text(img, 261, 36, 13, true, c, status, 300)
	round(img, 690, 12, 94, 36, 10, panel)
	p.text(img, 710, 36, 17, false, textColor, "Settings", 78)
	for i, cat := range Categories {
		x := 16 + i*194
		active := s.Screen == "library" && s.Category == cat
		col := panel
		fg := muted
		if active {
			col = color.RGBA{57, 40, 26, 255}
			fg = accent
		}
		round(img, x, 62, 186, 40, 10, col)
		p.text(img, x+18, 89, 19, active, fg, cat, 150)
	}
	titleX := 20
	if len(s.Path) > 0 && s.Screen == "library" {
		p.text(img, 22, 139, 26, false, accent, "‹", 30)
		titleX = 56
	}
	p.text(img, titleX, 138, 22, true, textColor, v.Title, 310)
	p.text(img, 348, 137, 14, false, muted, v.Caption, 430)
	if s.Screen == "playing" {
		p.playing(img, s)
		for i, row := range v.Rows {
			y := 148 + i*44
			col := panel
			if i == s.Focus {
				col = color.RGBA{57, 40, 26, 255}
			}
			round(img, 475, y, 309, 40, 9, col)
			p.text(img, 489, y+26, 17, false, textColor, row.Title, 190)
			p.text(img, 698, y+26, 14, false, accent, row.Detail, 74)
		}
	} else if s.Screen == "library" && !s.Connected {
		p.ipod(img, 176, 181, 0.65)
		p.text(img, 283, 211, 28, true, textColor, "Your library. Your iPod.", 470)
		p.text(img, 283, 243, 17, false, muted, "Attach a powered hub and iPod data cable.", 470)
		round(img, 283, 283, 234, 48, 12, accent)
		button := "Connect iPod"
		if s.HostRole == "device" {
			button = "Enable USB host"
		}
		p.text(img, 307, 314, 20, true, bg, button, 205)
		p.text(img, 307, 361, 16, false, muted, "USB & audio diagnostics", 300)
	} else if len(v.Rows) == 0 {
		p.text(img, 30, 220, 24, false, muted, "No items returned", 700)
	} else {
		for i := v.Start; i < min(len(v.Rows), v.Start+5); i++ {
			row := v.Rows[i]
			y := 154 + (i-v.Start)*44
			col := panel
			fg := textColor
			if i == s.Focus {
				col = color.RGBA{50, 39, 29, 255}
				fg = accent
			}
			round(img, 16, y, 768, 41, 9, col)
			if i == s.Focus {
				round(img, 16, y+8, 4, 25, 2, accent)
			}
			p.text(img, 34, y+27, 20, i == s.Focus, fg, row.Title, 490)
			p.text(img, 536, y+26, 14, false, muted, row.Detail, 206)
			p.text(img, 758, y+27, 23, false, muted, "›", 18)
		}
		if len(v.Rows) > 5 {
			h := max(10, 220*5/len(v.Rows))
			y := 154 + (220-h)*v.Start/max(1, len(v.Rows)-5)
			round(img, 791, y, 3, h, 1, muted)
		}
	}
	message := s.Status
	mc := muted
	if s.Error != "" {
		message = s.Error
		mc = color.RGBA{255, 137, 123, 255}
	}
	if s.Screen == "library" && s.Total > 0 && s.Error == "" {
		message = fmt.Sprintf("%d–%d of %d  ·  Turn to browse · Press to select", s.Offset+1, s.Offset+uint32(len(s.Entries)), s.Total)
	}
	p.text(img, 20, 394, 13, false, mc, message, 760)
	fill(img, 0, 404, 800, 76, panel)
	round(img, 16, 417, 48, 48, 10, color.RGBA{57, 40, 26, 255})
	p.text(img, 29, 450, 29, false, accent, "♪", 35)
	title := s.Now.Title
	if title == "" {
		title = "Nothing playing"
	}
	p.text(img, 80, 437, 18, true, textColor, title, 456)
	subtitle := s.Now.Artist
	if s.Now.Album != "" {
		subtitle += " · " + s.Now.Album
	}
	if subtitle == "" {
		subtitle = "Playback stays on your iPod"
	}
	p.text(img, 80, 460, 14, false, muted, subtitle, 455)
	for _, r := range []struct{ x, w int }{{556, 64}, {628, 76}, {712, 64}} {
		round(img, r.x, 417, r.w, 48, 12, line)
	}
	round(img, 628, 411, 76, 60, 14, accent)
	p.text(img, 574, 449, 25, true, textColor, "‹‹", 50)
	if s.Now.Playing {
		fill(img, 655, 429, 7, 24, bg)
		fill(img, 669, 429, 7, 24, bg)
	} else {
		for y := -14; y <= 14; y++ {
			width := 23 * (14 - absInt(y)) / 14
			fill(img, 657, 441+y, width, 1, bg)
		}
	}
	p.text(img, 729, 449, 25, true, textColor, "››", 50)
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
func (p *Painter) playing(img *image.RGBA, s Snapshot) {
	p.ipod(img, 34, 165, 0.98)
	title := s.Now.Title
	if title == "" {
		title = "Nothing playing"
	}
	p.text(img, 166, 194, 25, true, textColor, title, 292)
	p.text(img, 166, 226, 18, false, muted, s.Now.Artist, 290)
	p.text(img, 166, 252, 16, false, muted, s.Now.Album, 290)
	round(img, 166, 283, 284, 4, 2, line)
	if s.Now.Duration > 0 {
		round(img, 166, 283, max(4, int(uint64(s.Now.Position)*284/uint64(s.Now.Duration))), 4, 2, accent)
	}
	p.text(img, 166, 312, 14, false, muted, clock(s.Now.Position), 100)
	p.text(img, 407, 312, 14, false, muted, clock(s.Now.Duration), 60)
	if s.Demo {
		p.text(img, 34, 355, 14, true, accent, "DEMO · Playback is simulated", 410)
	}
}
func clock(ms uint32) string { return fmt.Sprintf("%d:%02d", ms/60000, ms/1000%60) }
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
