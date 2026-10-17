package player

import (
 "bufio"
 "bytes"
 "errors"
 "fmt"
 "image"
 "image/png"
 "io"
 "os"
 "os/exec"
 "strings"
 "sync"
 "time"
)

// The system libgpod decoder runs in an isolated Python/ctypes process.
// A crash in legacy artwork decoding must never terminate the Car Thing
// player or interrupt its read-only iPod session.
type classicArtwork struct {
 mu sync.Mutex
 root string
 cmd *exec.Cmd
 stdin io.WriteCloser
 reader *bufio.Reader
 tried bool
 readErr error
}
func (c *classicArtwork) prepare(root string) error {
 if c.tried {return c.readErr}
 c.tried=true
 c.root=root
 cmd:=exec.Command("/usr/bin/python3","/usr/lib/podthing/artwork.py",root)
 stdin,e:=cmd.StdinPipe()
 if e!=nil{c.readErr=e;return e}
 stdout,e:=cmd.StdoutPipe()
 if e!=nil{c.readErr=e;return e}
 // stdout is a single protocol line; stderr goes to the boot journal.
 if e=cmd.Start();e!=nil{c.readErr=e;return e}
 c.cmd=cmd
 c.stdin=stdin
 c.reader=bufio.NewReaderSize(stdout,4096)
 line,e:=c.readLine(10*time.Second)
 if e!=nil {c.closeLocked();c.readErr=e;return e}
 if line!="READY" {
  c.closeLocked()
  c.readErr=fmt.Errorf("libgpod: %s",line)
  return c.readErr
 }
 return nil
}
func (c *classicArtwork) readLine(timeout time.Duration)(string,error){
 type outcome struct {s string;e error}
 ch:=make(chan outcome,1)
 go func(){s,e:=c.reader.ReadString('\n');ch<-outcome{strings.TrimSpace(s),e}}()
 select {
 case a:=<-ch:return a.s,a.e
 case <-time.After(timeout):return "",errors.New("iPod artwork decoder timed out")
 }
}
func (c *classicArtwork) Cover(root string,id uint32)(image.Image,error){
 c.mu.Lock()
 defer c.mu.Unlock()
 if e:=c.prepare(root);e!=nil{return nil,e}
 if c.root!=root {return nil,errors.New("artwork worker attached to different iPod")}
 if _,e:=fmt.Fprintf(c.stdin,"%d\n",id);e!=nil{c.closeLocked();c.readErr=e;return nil,e}
 line,e:=c.readLine(5*time.Second)
 if e!=nil{c.closeLocked();c.readErr=e;return nil,e}
 if line=="NONE"{return nil,errors.New("no indexed iPod artwork for this track")}
 if !strings.HasPrefix(line,"OK /tmp/podthing-art-") {
  c.closeLocked()
  c.readErr=fmt.Errorf("iPod artwork worker: %s",line)
  return nil,c.readErr
 }
 path:=strings.TrimPrefix(line,"OK ")
 defer os.Remove(path)
 f,e:=os.Open(path)
 if e!=nil{return nil,e}
 defer f.Close()
 info,e:=f.Stat()
 if e!=nil{return nil,e}
 if info.Size()<32||info.Size()>2<<20{return nil,errors.New("decoded artwork exceeds safe file size")}
 b,e:=io.ReadAll(io.LimitReader(f,2<<20))
 if e!=nil{return nil,e}
 cfg,e:=png.DecodeConfig(bytes.NewReader(b))
 if e!=nil{return nil,e}
 if cfg.Width<1||cfg.Height<1||cfg.Width>1024||cfg.Height>1024{
  return nil,errors.New("decoded iPod artwork dimensions are invalid")
 }
 return png.Decode(bytes.NewReader(b))
}
func (c *classicArtwork) closeLocked(){
 if c.stdin!=nil {_,_=io.WriteString(c.stdin,"QUIT\n");_=c.stdin.Close();c.stdin=nil}
 if c.cmd!=nil {
  cmd:=c.cmd
  done:=make(chan struct{},1)
  go func(){_=cmd.Wait();done<-struct{}{}}()
  select{
  case <-done:
  case <-time.After(2*time.Second):
   if cmd.Process!=nil{_=cmd.Process.Kill()}
   <-done
  }
  c.cmd=nil
 }
 c.reader=nil
}
func (c *classicArtwork) Close(){
 c.mu.Lock()
 defer c.mu.Unlock()
 c.closeLocked()
 c.tried=false
 c.root=""
 c.readErr=nil
}
