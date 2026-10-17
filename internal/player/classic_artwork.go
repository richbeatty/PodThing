package player

import (
    "errors"
    "fmt"
    "image"
    "image/png"
    "os"
    "path/filepath"
    "sync"

    "github.com/ebitengine/purego"
)

// classicArtwork reads the stock iPod ArtworkDB and .ithmb using the mature
// libgpod reader. It never calls any database/thumbnail write API. The
// iTunesDB is parsed once per mount, then released before unmounting.
type classicArtwork struct {
    mu sync.Mutex
    tried bool
    root string
    db uintptr
    readErr error
    parse func(string,*uintptr) uintptr
    free func(uintptr)
    track func(uintptr,uint32) uintptr
    thumbnail func(uintptr,int32,int32) uintptr
    save func(uintptr,string,string,uintptr,uintptr,*uintptr) int32
    unref func(uintptr)
}
func (c *classicArtwork) prepare(root string) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    if c.tried {
        if c.root!=root {return errors.New("artwork library mounted on a different iPod")}
        return c.readErr
    }
    c.tried=true
    c.root=root
    fail:=func(e error)error {c.readErr=e;return e}
    lib,e:=purego.Dlopen("libgpod.so.4",purego.RTLD_NOW|purego.RTLD_LOCAL)
    if e!=nil {return fail(fmt.Errorf("libgpod unavailable: %w",e))}
    pix,e:=purego.Dlopen("libgdk_pixbuf-2.0.so.0",purego.RTLD_NOW|purego.RTLD_LOCAL)
    if e!=nil{return fail(fmt.Errorf("pixbuf unavailable: %w",e))}
    obj,e:=purego.Dlopen("libgobject-2.0.so.0",purego.RTLD_NOW|purego.RTLD_LOCAL)
    if e!=nil{return fail(fmt.Errorf("gobject unavailable: %w",e))}
    purego.RegisterLibFunc(&c.parse,lib,"itdb_parse")
    purego.RegisterLibFunc(&c.free,lib,"itdb_free")
    purego.RegisterLibFunc(&c.track,lib,"itdb_track_by_id")
    purego.RegisterLibFunc(&c.thumbnail,lib,"itdb_track_get_thumbnail")
    purego.RegisterLibFunc(&c.save,pix,"gdk_pixbuf_savev")
    purego.RegisterLibFunc(&c.unref,obj,"g_object_unref")
    var gerr uintptr
    c.db=c.parse(root,&gerr)
    if c.db==0 {return fail(errors.New("libgpod cannot read the mounted ArtworkDB/iTunesDB"))}
    return nil
}
func (c *classicArtwork) Cover(root string, id uint32) (image.Image,error) {
    if err:=c.prepare(root);err!=nil{return nil,err}
    c.mu.Lock()
    defer c.mu.Unlock()
    if c.db==0 {return nil,errors.New("artwork index is closed")}
    track:=c.track(c.db,id)
    if track==0{return nil,errors.New("track not found in libgpod")}
    pixbuf:=c.thumbnail(track,200,200)
    if pixbuf==0{return nil,errors.New("track has no readable iPod artwork")}
    defer c.unref(pixbuf)
    // Do not save to the iPod. The pixbuf PNG is written to the Car Thing's
    // local tmpfs and decoded into a bounded Go image.
    tmp,e:=os.CreateTemp("","podthing-cover-*.png")
    if e!=nil{return nil,e}
    path:=tmp.Name()
    _=tmp.Close()
    defer os.Remove(path)
    var gerr uintptr
    if c.save(pixbuf,path,"png",0,0,&gerr)==0 {
        return nil,errors.New("libgpod could not decode this iPod artwork format")
    }
    f,e:=os.Open(filepath.Clean(path))
    if e!=nil{return nil,e}
    defer f.Close()
    cfg,e:=png.DecodeConfig(f)
    if e!=nil{return nil,e}
    if cfg.Width<1||cfg.Height<1||cfg.Width>1024||cfg.Height>1024 {
        return nil,errors.New("unreasonable decoded artwork dimensions")
    }
    if _,e=f.Seek(0,0);e!=nil{return nil,e}
    return png.Decode(f)
}
func (c *classicArtwork) Close() {
    c.mu.Lock()
    defer c.mu.Unlock()
    if c.db!=0 {c.free(c.db);c.db=0}
    c.tried=false
    c.root=""
    c.readErr=nil
}
