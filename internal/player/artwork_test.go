package player

import (
    "bytes"
    "image"
    "image/color"
    "image/png"
    "testing"
)

func TestArtworkAccentFromActualPixels(t *testing.T) {
    img:=image.NewRGBA(image.Rect(0,0,64,64))
    for y:=0;y<64;y++ {for x:=0;x<64;x++ {img.Set(x,y,color.RGBA{215,43,64,255})}}
    ac:=ArtworkAccent(img)
    if ac[0]<180||ac[1]>110||ac[2]>120 {t.Fatalf("unexpected artwork color: %v",ac)}
}
func TestApplyArtworkUpdatesAndRejectsInvalidBytes(t *testing.T) {
    app:=demoApp(t)
    img:=image.NewRGBA(image.Rect(0,0,20,20))
    for y:=0;y<20;y++ {for x:=0;x<20;x++ {img.Set(x,y,color.RGBA{24,125,220,255})}}
    var encoded bytes.Buffer
    if err:=png.Encode(&encoded,img);err!=nil {t.Fatal(err)}
    if err:=app.ApplyArtwork(encoded.Bytes());err!=nil {t.Fatal(err)}
    state:=app.Snapshot()
    if !state.HasArtwork||state.Artwork==nil||state.Accent==[3]uint8{96,154,255} {t.Fatalf("theme not updated: %+v",state.Accent)}
    if err:=app.ApplyArtwork([]byte("invalid"));err==nil {t.Fatal("accepted invalid artwork")}
    if !app.Snapshot().HasArtwork {t.Fatal("invalid artwork discarded prior valid cover")}
}
