package player

import (
    "bytes"
    "errors"
    "image"
    _ "image/gif"
    _ "image/jpeg"
    _ "image/png"
)

// ArtworkAccent derives a vibrant accent from a real album cover. It
// samples evenly across the image and ignores white, black and dull pixels.
// No track title hashing or fabricated album-color data is used.
func ArtworkAccent(src image.Image) [3]uint8 {
    fallback:=[3]uint8{96,154,255}
    if src==nil {return fallback}
    b:=src.Bounds()
    if b.Dx()<1||b.Dy()<1 {return fallback}
    var total,rr,gg,bb uint64
    for iy:=0;iy<24;iy++ {
        y:=b.Min.Y+(iy*b.Dy()+b.Dy()/2)/24
        if y>=b.Max.Y {y=b.Max.Y-1}
        for ix:=0;ix<24;ix++ {
            x:=b.Min.X+(ix*b.Dx()+b.Dx()/2)/24
            if x>=b.Max.X {x=b.Max.X-1}
            r,g,bl,a:=src.At(x,y).RGBA()
            if a<0xa000 {continue}
            red,green,blue:=int(r>>8),int(g>>8),int(bl>>8)
            maxv:=max(red,max(green,blue))
            minv:=min(red,min(green,blue))
            saturation:=maxv-minv
            if saturation<29||maxv<40||minv>223 {continue}
            weight:=uint64(saturation*saturation)
            rr+=uint64(red)*weight
            gg+=uint64(green)*weight
            bb+=uint64(blue)*weight
            total+=weight
        }
    }
    if total==0 {return fallback}
    brighten:=func(v uint64) uint8 {
        value:=int(v/total)
        return uint8(max(74,min(value,238)))
    }
    return [3]uint8{brighten(rr),brighten(gg),brighten(bb)}
}

// ApplyArtwork is the safe handoff for decoded iPod cover data once the
// accessory transport supports its artwork commands. Until then, the UI
// uses a neutral placeholder and a stable blue accent.
func (a *App) ApplyArtwork(encoded []byte) error {
    if len(encoded)==0||len(encoded)>4*1024*1024 {return errors.New("artwork must be 1 to 4 MiB")}
    cfg,_,err:=image.DecodeConfig(bytes.NewReader(encoded))
    if err!=nil {return err}
    if cfg.Width<1||cfg.Height<1||cfg.Width>2048||cfg.Height>2048 {return errors.New("artwork dimensions out of range")}
    img,_,err:=image.Decode(bytes.NewReader(encoded))
    if err!=nil {return err}
    ac:=ArtworkAccent(img)
    a.mu.Lock()
    a.s.Accent=ac
    a.s.Artwork=img
    a.s.HasArtwork=true
    a.mu.Unlock()
    return nil
}
