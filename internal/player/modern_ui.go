package player

import (
 "fmt"
 "image"
 "image/color"
 "image/draw"
 "strings"

 xdraw "golang.org/x/image/draw"
)

// Touch-first compact layout. No feature requires the Car Thing's dedicated
// preset buttons; the controls stay reachable by touch/swipe.
type ModernTile struct {
 X,Y,W,H int
 Title,Subtitle string
 Art image.Image
 Action Action
}
var (
 ink=color.RGBA{15,16,18,255}
 raised=color.RGBA{27,28,30,255}
 pale=color.RGBA{242,241,236,255}
 soft=color.RGBA{155,157,159,255}
 hair=color.RGBA{57,58,61,255}
 warm=color.RGBA{201,211,199,255}
)
func tastefulAccent(s Snapshot) color.RGBA {
 if s.HasArtwork && s.Artwork!=nil {
  ac:=ArtworkAccent(s.Artwork)
  return color.RGBA{
   uint8((int(ac[0])+2*int(warm.R))/3),
   uint8((int(ac[1])+2*int(warm.G))/3),
   uint8((int(ac[2])+2*int(warm.B))/3),255}
 }
 return warm
}
func BuildModernView(s Snapshot) View {
 v:=View{State:s,Title:"PodThing"}
 // Header and transport live inside the safe 0..700px touch region.
 if !s.TrayOpen {
  v.Hits=append(v.Hits,
   Hit{16,5,157,61,Action{Name:"home"}},
   Hit{390,5,124,61,Action{Name:"navigate",Text:"Albums"}},
   Hit{516,5,104,61,Action{Name:"playing"}},
   Hit{637,5,79,61,Action{Name:"tray"}},
  )
 }
 switch s.Screen {
 case "home":
  v.Title="Home"
  if len(s.Recent)>0 {
   v.Caption="Recently played"
   for i,r:=range s.Recent {
    if i>=4{break}
    v.Tiles=append(v.Tiles,ModernTile{X:25+i*174,Y:145,W:161,H:174,
     Title:r.Title,Subtitle:r.Artist,Art:s.Covers[albumKey(r.Artist,r.Album)],
     Action:Action{Name:"play-recent",Value:int(r.ID)}})
   }
  } else {
   v.Caption="Albums"
   for i,c:=range s.HomeAlbums {
    if i>=4{break}
    v.Tiles=append(v.Tiles,ModernTile{X:25+i*174,Y:145,W:161,H:174,
     Title:c.Name,Subtitle:c.Detail,Art:s.Covers[c.Key],
     Action:Action{Name:"open-album",Text:c.Key}})
   }
  }
  for i,c:=range Categories {
   v.Tiles=append(v.Tiles,ModernTile{X:25+i*174,Y:353,W:161,H:48,Title:c,Action:Action{Name:"navigate",Text:c}})
  }
 case "library":
  v.Title=s.Category
  v.Caption=fmt.Sprintf("%d %s",s.Total,strings.ToLower(s.Category))
  for i,c:=range Categories{
   v.Hits=append(v.Hits,Hit{26+i*174,84,161,47,Action{Name:"navigate",Text:c}})
  }
  v.Hits=append(v.Hits,Hit{486,128,133,42,Action{Name:"sort-mode"}})
  if s.Category=="Albums" {v.Hits=append(v.Hits,Hit{635,128,80,42,Action{Name:"view-mode"}})}
  if s.Category=="Albums" && s.GridView {
   for i,c:=range s.Cards{
    if i>=8{break}
    x,y:=24+i%4*174,177+i/4*112
    v.Tiles=append(v.Tiles,ModernTile{X:x,Y:y,W:158,H:106,Title:c.Name,Subtitle:c.Detail,Art:s.Covers[c.Key],Action:Action{Name:"open-album",Text:c.Key}})
   }
  }else{
   visible:=4
   start:=max(0,min(s.Focus-1,len(s.Entries)-visible))
   v.Start=start
   for i:=start;i<min(start+visible,len(s.Entries));i++ {
    entry:=s.Entries[i]
    subtitle:=""
    if s.Category=="Songs" {subtitle=s.TrackSummaries[entry.Index]}
    action:=Action{Name:"open",Value:i}
    if s.Category=="Albums"&&i<len(s.Cards){action=Action{Name:"open-album",Text:s.Cards[i].Key}}
    v.Tiles=append(v.Tiles,ModernTile{X:26,Y:181+(i-start)*55,W:670,H:52,Title:entry.Name,Subtitle:subtitle,Action:action})
   }
  }
 case "unmounted":
  v.Title="Safe to disconnect"
  v.Tiles=append(v.Tiles,ModernTile{X:30,Y:285,W:380,H:58,Title:"Reconnect iPod library",Action:Action{Name:"storage-open"}})
 case "playing":
  v.Title="Now playing"
  if s.Now.Duration>0{
   for i:=0;i<24;i++{
    v.Hits=append(v.Hits,Hit{342+i*14,266,14,49,Action{Name:"storage-seek",Value:i*100/23}})
   }
  }
  v.Hits=append(v.Hits,
   Hit{362,328,65,65,Action{Name:"previous"}},
   Hit{459,318,87,85,Action{Name:"play"}},
   Hit{572,328,65,65,Action{Name:"next"}},
  )
 case "settings","settings-usb","settings-audio","settings-controls","settings-about":
  base:=buildLegacyView(s)
  v.Title=base.Title
  v.Caption=base.Caption
  v.Rows=base.Rows
  visible:=5
  v.Start=max(0,min(s.Focus-1,len(base.Rows)-visible))
  for i:=v.Start;i<min(v.Start+visible,len(base.Rows));i++{
   row:=base.Rows[i]
   v.Tiles=append(v.Tiles,ModernTile{X:23,Y:90+(i-v.Start)*66,W:681,H:61,
    Title:row.Title,Subtitle:row.Detail,Action:row.Action})
  }
  if s.Screen=="settings-audio" {
   next:="usb"
   label:="Simulated · silent"
   if s.Settings.AudioMode=="usb"{next="simulated";label="USB audio"}
   v.Tiles=append([]ModernTile{{X:23,Y:84,W:681,H:54,Title:"Playback · "+label,Subtitle:"Tap to change",Action:Action{Name:"audio-mode",Text:next}}},v.Tiles...)
   for i:=1;i<len(v.Tiles);i++{v.Tiles[i].Y=144+(i-1)*52;v.Tiles[i].H=47}
  }
 }
 for _,tile:=range v.Tiles {
  v.Hits=append(v.Hits,Hit{tile.X,tile.Y,tile.W,tile.H,tile.Action})
  if s.Screen=="home" {v.Rows=append(v.Rows,Row{Title:tile.Title,Detail:tile.Subtitle,Action:tile.Action})}
 }
 // Keep the previous/next and play buttons reachable on other pages.
 if s.Screen!="playing" && !s.TrayOpen {
  v.Hits=append(v.Hits,
   Hit{12,420,57,60,Action{Name:"settings"}},
   Hit{75,421,441,59,Action{Name:"playing"}},
   Hit{526,420,57,60,Action{Name:"previous"}},
   Hit{592,420,65,60,Action{Name:"play"}},
   Hit{662,420,54,60,Action{Name:"next"}})
 }
 if s.TrayOpen {
  // A modal, four-key tray. Always safe to dismiss; eject only when
  // the storage session is actually mounted.
  v.Hits=[]Hit{{0,280,800,200,Action{Name:"tray-close"}},{636,0,80,62,Action{Name:"tray-close"}}}
  for i:=0;i<4;i++ {v.Hits=append(v.Hits,Hit{25+i*174,100,160,69,Action{Name:fmt.Sprintf("preset%d",i+1)}})}
  v.Hits=append(v.Hits,Hit{25,199,154,52,Action{Name:"shuffle"}},Hit{196,199,154,52,Action{Name:"repeat"}})
  if s.StorageMode {v.Hits=append(v.Hits,Hit{365,199,167,52,Action{Name:"storage-eject"}})}
  v.Hits=append(v.Hits,Hit{550,199,150,52,Action{Name:"settings"}})
 }
 return v
}
func (p *Painter) minimalCover(img *image.RGBA,x,y,w,h int,src image.Image) {
 round(img,x,y,w,h,7,raised)
 if src!=nil {
  dst:=image.NewRGBA(image.Rect(0,0,w,h))
  xdraw.ApproxBiLinear.Scale(dst,dst.Bounds(),src,src.Bounds(),draw.Over,nil)
  draw.Draw(img,image.Rect(x,y,x+w,y+h),dst,image.Point{},draw.Src)
 }else{
  p.glyph(img,x+(w-32)/2,y+(h-32)/2,"music",soft)
 }
}
func (p *Painter) minimalTile(img *image.RGBA,t ModernTile,s Snapshot) {
 if s.Screen=="home" && t.H>80{
  p.minimalCover(img,t.X,t.Y,118,118,t.Art)
  p.text(img,t.X,t.Y+140,15,true,pale,t.Title,t.W)
  p.text(img,t.X,t.Y+161,12,false,soft,t.Subtitle,t.W)
  return
 }
 if s.Screen=="library" && s.Category=="Albums" && s.GridView {
  p.minimalCover(img,t.X,t.Y,85,85,t.Art)
  p.text(img,t.X+94,t.Y+37,14,true,pale,t.Title,65)
  p.text(img,t.X+94,t.Y+57,11,false,soft,t.Subtitle,65)
  return
 }
 if s.Screen=="home" {
  round(img,t.X,t.Y,t.W,t.H,9,raised)
  p.text(img,t.X+14,t.Y+31,16,false,pale,t.Title,t.W-22)
  return
 }
 fill(img,t.X,t.Y+t.H-1,t.W,1,hair)
 p.text(img,t.X+12,t.Y+35,18,true,pale,t.Title,t.W-35)
 if t.Subtitle!="" {p.text(img,t.X+396,t.Y+35,12,false,soft,t.Subtitle,t.W-410)}
}
func (p *Painter) renderModern(v View) *image.RGBA{
 s:=v.State
 img:=image.NewRGBA(image.Rect(0,0,800,480))
 fill(img,0,0,800,480,ink)
 // Never flood the entire interface with album accent or saturated blue.
 ac:=tastefulAccent(s)
 p.text(img,27,44,21,true,pale,"PodThing",174)
 p.text(img,398,43,15,false,soft,"Library",100)
 p.text(img,530,43,15,false,soft,"Playing",100)
 p.text(img,650,43,20,false,pale,"⌄",45)
 fill(img,26,67,675,1,hair)
 if s.Screen=="home" {
  p.text(img,27,113,29,true,pale,"Your music",450)
  p.text(img,27,137,14,false,soft,v.Caption,500)
  p.text(img,27,342,12,false,soft,"BROWSE",270)
 }else if s.Screen=="library"{
  for i,c:=range Categories{
   x:=26+i*174
   if c==s.Category{
    p.text(img,x+3,110,19,true,pale,c,154)
    fill(img,x,125,145,2,ac)
   }else{p.text(img,x+3,110,17,false,soft,c,154)}
  }
  p.text(img,27,159,14,false,soft,v.Caption,410)
  p.text(img,491,157,14,false,pale,"Sort: "+s.SortMode,130)
  if s.Category=="Albums"{
   label:="Grid";if !s.GridView{label="List"}
   p.text(img,642,157,14,false,pale,label,65)
  }
 }else if s.Screen=="unmounted" {
  p.text(img,30,150,37,true,pale,"Safe to disconnect",510)
  p.text(img,30,194,17,false,soft,"The iPod storage was unmounted successfully.",650)
  p.text(img,30,229,15,false,soft,"You can unplug the cable. No files were changed.",650)
 }else if s.Screen=="playing"{
  p.minimalCover(img,35,91,271,271,s.Artwork)
  p.text(img,345,120,12,false,soft,"NOW PLAYING",315)
  name:=s.Now.Title
  if name==""{name="Nothing selected"}
  p.text(img,345,173,29,true,pale,name,355)
  p.text(img,345,206,17,false,soft,s.Now.Artist,353)
  p.text(img,345,231,14,false,soft,s.Now.Album,352)
  fill(img,346,285,325,3,hair)
  if s.Now.Duration>0{
   w:=int(uint64(min(s.Now.Position,s.Now.Duration))*325/uint64(s.Now.Duration))
   if w>0{fill(img,346,285,w,3,ac)}
  }
  p.text(img,346,311,12,false,soft,clock(s.Now.Position),80)
  p.text(img,625,311,12,false,soft,clock(s.Now.Duration),68)
  p.glyph(img,385,342,"previous",pale)
  round(img,466,331,66,60,28,pale)
  iconName:="play";if s.Now.Playing{iconName="pause"}
  p.glyph(img,482,342,iconName,ink)
  p.glyph(img,590,342,"next",pale)
  if s.Settings.AudioMode=="simulated"{
   p.text(img,37,396,13,false,soft,"SIMULATED  ·  NO AUDIO OUTPUT",290)
  }
 }else if s.Screen=="settings"||strings.HasPrefix(s.Screen,"settings-"){
  p.text(img,27,112,30,true,pale,v.Title,520)
  p.text(img,27,139,14,false,soft,v.Caption,540)
 }
 for _,tile:=range v.Tiles {p.minimalTile(img,tile,s)}
 if s.Screen!="playing" && s.Screen!="unmounted" {
  fill(img,0,416,720,64,raised)
  p.glyph(img,25,432,"gear",soft)
  name:=s.Now.Title
  if name==""{name="Choose a song"}
  p.text(img,86,443,17,true,pale,name,416)
  p.text(img,86,463,12,false,soft,s.Now.Artist,420)
  p.glyph(img,542,432,"previous",pale)
  round(img,598,427,53,46,23,pale)
  iconName:="play";if s.Now.Playing{iconName="pause"}
  p.glyph(img,609,435,iconName,ink)
  p.glyph(img,672,432,"next",pale)
 }
 if s.TrayOpen {
  fill(img,0,0,723,277,raised)
  p.text(img,27,41,23,true,pale,"Quick actions",450)
  p.text(img,650,42,19,false,soft,"×",40)
  for i,bind:=range s.Settings.Buttons{
   x:=25+i*174
   round(img,x,99,160,71,11,ink)
   p.text(img,x+14,120,12,false,soft,fmt.Sprintf("PRESET %d",i+1),133)
   p.text(img,x+14,149,16,true,pale,bind,135)
  }
  for i,label:=range []string{"Shuffle","Repeat"}{
   x:=25+i*171
   p.text(img,x+8,231,17,false,pale,label,132)
  }
  if s.StorageMode{p.text(img,374,231,17,false,pale,"Eject iPod",146)}
  p.text(img,562,231,17,false,pale,"Settings",134)
  fill(img,0,276,720,1,hair)
 }
 if s.Error!=""{
  round(img,21,370,684,40,8,color.RGBA{83,40,42,255})
  p.text(img,33,396,15,true,pale,s.Error,650)
 }
 return img
}
