package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Playback struct {
	mu sync.Mutex
	cmd *exec.Cmd
	done chan error
	track Track
	output string
	seekMS uint32
	start time.Time
	elapsed time.Duration
	paused bool
	ended bool
	errorText string
	decoderChecked bool
}
// decoderFailure preserves the dynamic-linker's original stderr. Exit 127
// commonly means a missing shared library/ELF loader, not an iPod file error.
func decoderFailure(err error, output string) error {
	detail:=strings.TrimSpace(output)
	if len(detail)>1500 {detail=detail[len(detail)-1500:]}
	var exit *exec.ExitError
	if errors.As(err,&exit) && exit.ExitCode()==127 {
		if detail=="" {detail="the decoder exited before reporting a reason; inspect the FFmpeg runtime check in the firmware build log"}
		return fmt.Errorf("FFmpeg runtime failed (exit 127, likely a missing runtime library or loader): %s",detail)
	}
	if detail!="" {return fmt.Errorf("decoder: %w: %s",err,detail)}
	return fmt.Errorf("decoder: %w",err)
}

// FFmpeg is shipped dynamically linked inside the appliance. Check its
// dependencies *on the device* before claiming the track started. Only the
// first successful check is cached; a failed preflight can be retried.
func (p *Playback) checkDecoder() error {
	p.mu.Lock()
	ready:=p.decoderChecked
	p.mu.Unlock()
	if ready {return nil}
	ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
	defer cancel()
	cmd:=exec.CommandContext(ctx,"/usr/bin/ffmpeg","-hide_banner","-version")
	out,e:=cmd.CombinedOutput()
	if e!=nil {
		if ctx.Err()!=nil {return fmt.Errorf("FFmpeg decoder check timed out: %w",ctx.Err())}
		return fmt.Errorf("FFmpeg cannot run on this firmware: %w",decoderFailure(e,string(out)))
	}
	p.mu.Lock()
	p.decoderChecked=true
	p.mu.Unlock()
	return nil
}

func (p *Playback) Start(file string, track Track, output string, offsetMS uint32) error {
	if output=="" {return errors.New("choose a USB DAC output under Audio settings before playback")}
	if _,e:=os.Stat(file);e!=nil{return e}
	if e:=p.Stop();e!=nil{return e}
	if e:=p.checkDecoder();e!=nil{return e}
	// Decoder reads its input file and sends PCM to the selected ALSA
	// playback device. No iPod USB audio or MFi session is involved.
	args:=[]string{"-nostdin","-hide_banner","-loglevel","error"}
	if offsetMS>0 {
		args=append(args,"-ss",fmt.Sprintf("%.3f",float64(offsetMS)/1000))
	}
	args=append(args,"-i",file,"-vn","-sn","-map","0:a:0","-ar","44100","-ac","2","-f","alsa",output)
	cmd:=exec.Command("/usr/bin/ffmpeg",args...)
	cmd.SysProcAttr=&syscall.SysProcAttr{Setpgid:true}
	var stderr boundedAudioError
	cmd.Stderr=&stderr
	if e:=cmd.Start();e!=nil{return fmt.Errorf("start ffmpeg: %w",e)}
	done:=make(chan error,1)
	p.mu.Lock()
	p.cmd=cmd
	p.done=done
	p.track=track
	p.output=output
	p.seekMS=offsetMS
	p.elapsed=0
	p.start=time.Now()
	p.paused=false
	p.errorText=""
	p.ended=false
	p.mu.Unlock()
	go func() {
		e:=cmd.Wait()
		if e!=nil {e=decoderFailure(e,stderr.String())}
		p.mu.Lock()
		if p.cmd==cmd {
			if !p.paused {p.elapsed+=time.Since(p.start)}
			p.cmd=nil
			if e!=nil {p.errorText=e.Error()} else {p.ended=true}
		}
		p.mu.Unlock()
		done<-e
	}()
	return nil
}
type boundedAudioError struct {
	mu sync.Mutex
	b []byte
}
func (b *boundedAudioError) Write(p []byte)(int,error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.b=append(b.b,p...)
	if len(b.b)>2048 {b.b=b.b[len(b.b)-2048:]}
	return len(p),nil
}
func (b *boundedAudioError) String()string{
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.b))
}
func (p *Playback) Stop() error {
	p.mu.Lock()
	cmd,done:=p.cmd,p.done
	p.cmd=nil
	p.paused=false
	p.mu.Unlock()
	if cmd==nil {return nil}
	if cmd.Process!=nil { _=syscall.Kill(-cmd.Process.Pid,syscall.SIGCONT);_=syscall.Kill(-cmd.Process.Pid,syscall.SIGTERM)}
	select {
	case <-done:return nil
	case <-time.After(3*time.Second):
		if cmd.Process!=nil { _=syscall.Kill(-cmd.Process.Pid,syscall.SIGKILL)}
		select {
		case <-done: return nil
		case <-time.After(2*time.Second): return errors.New("decoder did not exit; refusing to unmount music files")
		}
	}
}
func (p *Playback) Toggle() (bool,error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd==nil{return false,errors.New("no music track active")}
	if p.paused {
		if e:=syscall.Kill(-p.cmd.Process.Pid,syscall.SIGCONT);e!=nil{return false,e}
		p.start=time.Now()
		p.paused=false
		return true,nil
	}
	if e:=syscall.Kill(-p.cmd.Process.Pid,syscall.SIGSTOP);e!=nil{return false,e}
	p.elapsed+=time.Since(p.start)
	p.paused=true
	return false,nil
}
func (p *Playback) State() (Track,uint32,bool,string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	duration:=p.elapsed
	if p.cmd!=nil && !p.paused {duration+=time.Since(p.start)}
	ms:=uint64(p.seekMS)+uint64(duration/time.Millisecond)
	if ms>uint64(^uint32(0)) {ms=uint64(^uint32(0))}
	return p.track,uint32(ms),p.cmd!=nil&&!p.paused,p.errorText
}
// TakeCompletion reports natural end only once; deliberate Stop does not
// advance the queue.
func (p *Playback) TakeCompletion() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	e:=p.ended
	p.ended=false
	return e
}
func (p *Playback) Active() bool {
	p.mu.Lock();defer p.mu.Unlock();return p.cmd!=nil
}
