package player

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// ModernTile describes a touch-first card; hits are generated independently
// of any hardware preset buttons.
type ModernTile struct {
	X,Y,W,H int
	Title,Subtitle string
	Art image.Image
	Action Action
}
func BuildModernView(s Snapshot) View {
	v:=View{State:s,Title:"Your music"}
	// Every major destination can be reached by touching the screen.
	v.Hits=append(v.Hits,
		Hit{20,6,164,65,Action{Name:"home"}},
		Hit{225,12,116,51,Action{Name:"home"}},
		Hit{348,12,126,51,Action{Name:"navigate",Text:"Albums"}},
		Hit{493,12,125,51,Action{Name:"playing"}},
		Hit{648,7,62,63,Action{Name:"tray"}},
		Hit{724,7,64,63,Action{Name:"settings"}},
		Hit{12,420,58,60,Action{Name:"settings"}},
		Hit{75,420,474,60,Action{Name:"playing"}},
		Hit{568,420,65,59,Action{Name:"previous"}},
		Hit{638,420,70,59,Action{Name:"play"}},
		Hit{715,420,69,59,Action{Name:"next"}},
	)
	switch s.Screen {
	case "home":
		v.Title="Home"
		if len(s.Recent)>0 {
			v.Caption="Recently played"
			for i,r:=range s.Recent {
				if i==4 {break}
				v.Tiles=append(v.Tiles,ModernTile{X:20+i*194,Y:137,W:183,H:170,
					Title:r.Title,Subtitle:r.Artist,Action:Action{Name:"play-recent",Value:int(r.ID)}})
			}
		}else{
			v.Caption="Albums to explore"
			for i,c:=range s.HomeAlbums {
				if i==4 {break}
				v.Tiles=append(v.Tiles,ModernTile{X:20+i*194,Y:137,W:183,H:170,
					Title:c.Name,Subtitle:c.Detail,Art:s.Covers[c.Key],Action:Action{Name:"open-album",Text:c.Key}})
			}
		}
		for i,c:=range Categories {
			v.Tiles=append(v.Tiles,ModernTile{X:20+i*194,Y:337,W:183,H:61,
				Title:c,Action:Action{Name:"navigate",Text:c}})
		}
	case "library":
		v.Title=s.Category
		v.Caption=fmt.Sprintf("%d %s",s.Total,strings.ToLower(s.Category))
		for i,c:=range Categories {
			v.Hits=append(v.Hits,Hit{18+i*195,79,188,46,Action{Name:"navigate",Text:c}})
		}
		v.Hits=append(v.Hits,Hit{534,130,129,40,Action{Name:"sort-mode"}})
		if s.Category=="Albums" {
			v.Hits=append(v.Hits,Hit{680,130,105,40,Action{Name:"view-mode"}})
		}
		if s.Category=="Albums" && s.GridView {
			// Eight thumbnails are visible per page; rotary/wheel scroll
			// shifts the page, while touch selection opens the album.
			for i,c:=range s.Cards {
				if i>=8 {break}
				x,y:=18+(i%4)*195,172+(i/4)*120
				v.Tiles=append(v.Tiles,ModernTile{X:x,Y:y,W:185,H:114,
					Title:c.Name,Subtitle:c.Detail,Art:s.Covers[c.Key],
					Action:Action{Name:"open-album",Text:c.Key}})
			}
		}else{
			visible:=4
			start:=max(0,min(s.Focus-1,len(s.Entries)-visible))
			v.Start=start
			for i:=start;i<min(len(s.Entries),start+visible);i++{
				entry:=s.Entries[i]
				subtitle:=""
				if s.Category=="Songs" && int(entry.Index)<len(s.TrackSummaries) {
					subtitle=s.TrackSummaries[entry.Index]
				}
				v.Tiles=append(v.Tiles,ModernTile{X:18,Y:175+(i-start)*58,W:762,H:53,
					Title:entry.Name,Subtitle:subtitle,Action:Action{Name:"open",Value:i}})
			}
		}
	case "playing":
		v.Title="Now playing"
		if s.Settings.AudioMode=="simulated" {v.Caption="SIMULATED · no audio output"}else{v.Caption="USB audio"}
		if s.Now.Duration>0 {
			for i:=0;i<=20;i++ {
				v.Hits=append(v.Hits,Hit{320+i*20,278,20,38,Action{Name:"storage-seek",Value:i*5}})
			}
		}
		v.Hits=append(v.Hits,Hit{345,340,104,64,Action{Name:"shuffle"}},
			Hit{549,340,104,64,Action{Name:"repeat"}})
	case "settings","settings-audio","settings-usb","settings-controls","settings-about":
		base:=buildLegacyView(s)
		v.Title=base.Title
		v.Caption=base.Caption
		// Settings retain their existing features but use a bigger touch
		// list, independent of physical navigation.
		visible:=5
		v.Start=max(0,min(s.Focus-1,len(base.Rows)-visible))
		for i:=v.Start;i<min(len(base.Rows),v.Start+visible);i++ {
			row:=base.Rows[i]
			v.Tiles=append(v.Tiles,ModernTile{X:18,Y:103+(i-v.Start)*61,W:764,H:56,
				Title:row.Title,Subtitle:row.Detail,Action:row.Action})
		}
		if s.Screen=="settings-audio" {
			v.Tiles=append([]ModernTile{{X:18,Y:82,W:764,H:44,
				Title:"Playback mode: "+map[bool]string{true:"Simulated (silent)",false:"USB DAC"}[s.Settings.AudioMode=="simulated"],
				Subtitle:"Tap to switch",Action:Action{Name:"audio-mode",Text:map[bool]string{true:"usb",false:"simulated"}[s.Settings.AudioMode=="simulated"]}}},v.Tiles...)
			// Reduce visible settings rows to avoid overlap with playback mode.
			for i:=1;i<len(v.Tiles);i++ {v.Tiles[i].Y=137+(i-1)*52;v.Tiles[i].H=47}
		}
	default:
		v.Title=s.Screen
	}
	for _,tile:=range v.Tiles {
		v.Hits=append(v.Hits,Hit{tile.X,tile.Y,tile.W,tile.H,tile.Action})
	}
	if s.TrayOpen {
		// Modal tray takes the touch surface until dismissed.
		v.Hits=[]Hit{{0,286,800,134,Action{Name:"tray-close"}},
			{716,0,84,79,Action{Name:"tray-close"}}}
		for i:=0;i<4;i++{
			v.Hits=append(v.Hits,Hit{20+i*194,123,181,79,Action{Name:fmt.Sprintf("preset%d",i+1)}})
		}
		v.Hits=append(v.Hits,Hit{20,216,178,53,Action{Name:"shuffle"}},
			Hit{218,216,178,53,Action{Name:"repeat"}},
			Hit{416,216,180,53,Action{Name:"storage-eject"}},
			Hit{617,216,170,53,Action{Name:"settings"}})
	}
	return v
}
func (p *Painter) modernCover(img *image.RGBA,x,y,w,h int,src image.Image,title string,ac color.RGBA) {
	round(img,x,y,w,h,11,tintMix(ac,66))
	if src!=nil {
		dst:=image.NewRGBA(image.Rect(0,0,w,h))
		xdraw.CatmullRom.Scale(dst,dst.Bounds(),src,src.Bounds(),draw.Over,nil)
		draw.Draw(img,image.Rect(x,y,x+w,y+h),dst,image.Point{},draw.Src)
		return
	}
	// Non-deceptive geometric placeholder for artwork the iPod database
	// hasn't exposed; never invent an actual album cover.
	round(img,x+6,y+6,w-12,h-12,9,tintMix(ac,36))
	p.glyph(img,x+(w-32)/2,y+(h-32)/2,"music",textColor)
}
func (p *Painter) modernTile(img *image.RGBA,t ModernTile,s Snapshot,ac color.RGBA) {
	if t.H<=65 {
		round(img,t.X,t.Y,t.W,t.H,14,panel)
		p.text(img,t.X+17,t.Y+37,17,true,textColor,t.Title,t.W-30)
		return
	}
	round(img,t.X,t.Y,t.W,t.H,14,panel)
	if s.Screen=="library" && s.Category=="Albums" && s.GridView {
		p.modernCover(img,t.X+9,t.Y+11,91,91,t.Art,t.Title,ac)
		p.text(img,t.X+111,t.Y+49,16,true,textColor,t.Title,t.W-118)
		p.text(img,t.X+111,t.Y+72,12,false,muted,t.Subtitle,t.W-119)
		return
	}
	if s.Screen=="home" {
		p.modernCover(img,t.X+12,t.Y+11,92,92,t.Art,t.Title,ac)
		p.text(img,t.X+12,t.Y+128,15,true,textColor,t.Title,t.W-23)
		p.text(img,t.X+12,t.Y+149,12,false,muted,t.Subtitle,t.W-23)
		return
	}
	p.text(img,t.X+18,t.Y+32,18,true,textColor,t.Title,t.W-45)
	if t.Subtitle!="" {p.text(img,t.X+430,t.Y+32,13,false,muted,t.Subtitle,t.W-465)}
	p.glyph(img,t.X+t.W-42,t.Y+13,"chevron",muted)
}
func (p *Painter) renderModern(v View) *image.RGBA {
	s:=v.State
	img:=image.NewRGBA(image.Rect(0,0,800,480))
	ac:=themeAccent(s)
	fill(img,0,0,800,480,bg)
	for y:=0;y<414;y+=6 {
		fill(img,0,y,800,6,tintMix(ac,max(0,18-y/20)))
	}
	// Header; tray is accessible by swipe or tapping the small chevron.
	round(img,16,9,181,55,16,panel)
	p.text(img,30,45,23,true,textColor,"PodThing",161)
	for i,item:=range []struct{label string;active bool;x int}{
		{"Home",s.Screen=="home",226},{"Library",s.Screen=="library",349},{"Playing",s.Screen=="playing",495},
	}{
		col:=panel;if item.active {col=tintMix(ac,84)}
		width:=116;if i==1 {width=127}
		round(img,item.x,14,width,49,15,col)
		p.text(img,item.x+15,45,16,item.active,textColor,item.label,width-21)
	}
	round(img,645,13,67,48,15,panel)
	p.text(img,657,44,21,true,textColor,"⌄",40)
	round(img,722,13,66,48,15,panel)
	p.glyph(img,740,20,"gear",textColor)
	if s.Screen=="home" {
		p.text(img,22,112,24,true,textColor,"Your music",430)
		p.text(img,22,132,13,false,muted,v.Caption,480)
		p.text(img,22,329,14,true,textColor,"BROWSE YOUR LIBRARY",300)
	}else if s.Screen=="library" {
		for i,c:=range Categories {
			x:=18+i*195
			col:=panel
			if c==s.Category {col=tintMix(ac,85)}
			round(img,x,79,188,46,14,col)
			p.text(img,x+18,108,17,c==s.Category,textColor,c,165)
		}
		p.text(img,22,157,18,true,textColor,v.Caption,425)
		round(img,532,130,131,40,13,panel)
		p.text(img,545,157,14,false,textColor,s.SortMode+" ↕",106)
		if s.Category=="Albums" {
			round(img,680,130,105,40,13,panel)
			mode:="Grid";if !s.GridView {mode="List"}
			p.text(img,696,157,14,false,textColor,mode+" ▦",90)
		}
	}else if s.Screen=="playing" {
		p.text(img,318,116,13,true,muted,v.Caption,395)
		if s.Artwork!=nil && s.HasArtwork {
			p.modernCover(img,45,114,225,225,s.Artwork,s.Now.Album,ac)
		}else{
			p.modernCover(img,45,114,225,225,nil,s.Now.Album,ac)
		}
		track:=s.Now.Title;if track==""{track="Nothing selected"}
		p.text(img,317,171,30,true,textColor,track,465)
		p.text(img,318,208,18,false,muted,s.Now.Artist+"  ·  "+s.Now.Album,456)
		round(img,320,286,438,7,3,line)
		if s.Now.Duration>0 {
			width:=int(uint64(min(s.Now.Position,s.Now.Duration))*438/uint64(s.Now.Duration))
			if width>0 {round(img,320,286,width,7,3,ac)}
		}
		p.text(img,320,322,13,false,muted,clock(s.Now.Position),80)
		p.text(img,708,322,13,false,muted,clock(s.Now.Duration),72)
		round(img,345,346,104,49,14,panel)
		round(img,549,346,104,49,14,panel)
		p.text(img,361,378,15,false,textColor,fmt.Sprintf("Shuffle %d",s.Now.Shuffle),95)
		p.text(img,567,378,15,false,textColor,fmt.Sprintf("Repeat %d",s.Now.Repeat),85)
	}
	for _,tile:=range v.Tiles {p.modernTile(img,tile,s,ac)}
	fill(img,0,419,800,61,panel)
	p.glyph(img,22,433,"gear",muted)
	title:=s.Now.Title;if title==""{title="Choose something to play"}
	p.text(img,82,442,17,true,textColor,title,450)
	artist:=s.Now.Artist
	if s.Settings.AudioMode=="simulated" && s.StorageMode {artist="SIMULATED  ·  "+artist}
	p.text(img,82,462,12,false,muted,artist,463)
	p.glyph(img,585,432,"previous",textColor)
	round(img,638,425,72,47,14,ac)
	if s.Now.Playing {p.glyph(img,658,432,"pause",bg)}else{p.glyph(img,658,432,"play",bg)}
	p.glyph(img,732,432,"next",textColor)
	if s.TrayOpen {
		round(img,0,0,800,285,18,color.RGBA{32,40,51,255})
		p.text(img,24,41,26,true,textColor,"Quick actions",440)
		p.text(img,24,67,14,false,muted,"Swipe up or tap outside to dismiss",580)
		p.text(img,739,47,24,true,textColor,"×",32)
		for i,bind:=range s.Settings.Buttons {
			x:=20+i*194
			round(img,x,122,181,81,14,tintMix(ac,57))
			p.text(img,x+14,147,13,false,muted,fmt.Sprintf("PRESET %d",i+1),150)
			p.text(img,x+14,179,18,true,textColor,bind,149)
		}
		for i,label:=range []string{"Shuffle","Repeat","Eject iPod","Settings"} {
			x:=20+i*198
			round(img,x,216,180,53,12,panel)
			p.text(img,x+12,250,16,true,textColor,label,151)
		}
	}
	return img
}
