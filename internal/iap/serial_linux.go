//go:build linux

package iap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// UART iAP1 is a different physical connection from USB iAP HID.
// A 30-pin USB charging/data cable does not expose the iPod's serial pins.
// This transport never touches the iPod's mass-storage filesystem.
const serialPayloadLimit = 16384

// SerialPorts returns USB-UART adapters only, never console UARTs or disks.
func SerialPorts() []string {
	ports := []string{}
	for _, pattern := range []string{"/dev/serial/by-id/*", "/dev/ttyUSB*", "/dev/ttyACM*"} {
		matches, _ := filepath.Glob(pattern)
		sort.Strings(matches)
		for _, path := range matches {
			target, err := filepath.EvalSymlinks(path)
			if err != nil || !(strings.HasPrefix(target, "/dev/ttyUSB") || strings.HasPrefix(target, "/dev/ttyACM")) {
				continue
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode()&os.ModeCharDevice == 0 {
				continue
			}
			duplicate := false
			for _, saved := range ports {
				old, _ := filepath.EvalSymlinks(saved)
				if old == target { duplicate = true; break }
			}
			if !duplicate { ports = append(ports, path) }
		}
	}
	return ports
}

// SerialFrames converts the UART wire format FF 55 + length + iAP frame
// into the same 55-prefixed packets expected by Client and Decode.
// Data is untrusted, so both total buffering and packet size are limited.
type SerialFrames struct { pending []byte }

func (f *SerialFrames) Feed(b []byte) error {
	if len(f.pending)+len(b) > 2*serialPayloadLimit+16 {
		f.pending = nil
		return errors.New("serial iAP input exceeded buffer limit")
	}
	f.pending = append(f.pending, b...)
	return nil
}

func (f *SerialFrames) Next() ([]byte, error) {
	for len(f.pending)>0 {
		if f.pending[0]!=0xff {
			f.pending=f.pending[1:]
			continue
		}
		if len(f.pending)==1 { return nil,nil }
		if f.pending[1]!=0x55 {
			f.pending=f.pending[1:]
			continue
		}
		if len(f.pending)<3 { return nil,nil }
		n, header := int(f.pending[2]), 3
		if n==0 {
			if len(f.pending)<5 {return nil,nil}
			n=int(f.pending[3])<<8|int(f.pending[4])
			header=5
		}
		if n<2 || n>serialPayloadLimit {
			f.pending=f.pending[2:]
			return nil,fmt.Errorf("invalid serial iAP payload length %d",n)
		}
		end:=header+n+1
		if len(f.pending)<end {return nil,nil}
		raw:=append([]byte(nil),f.pending[1:end]...)
		f.pending=f.pending[end:]
		if _,e:=Decode(raw);e!=nil{return nil,fmt.Errorf("bad serial iAP packet: %w",e)}
		return raw,nil
	}
	return nil,nil
}

type SerialTransport struct {
	fd int
	original syscall.Termios
	rx sync.Mutex
	tx sync.Mutex
	mu sync.Mutex
	closed bool
	frames SerialFrames
}

func termios(fd int, attr *syscall.Termios, request uintptr) error {
	_,_,errno:=syscall.Syscall(syscall.SYS_IOCTL,uintptr(fd),request,uintptr(unsafe.Pointer(attr)))
	if errno!=0 {return errno}
	return nil
}

func OpenSerial(path string, baud int) (*SerialTransport,error) {
	var speed uint32
	switch baud {
	case 19200: speed=syscall.B19200
	case 57600: speed=syscall.B57600
	default: return nil,fmt.Errorf("unsupported iAP serial rate %d: use 19200 or 57600",baud)
	}
	allowed:=false
	for _,port:=range SerialPorts(){if port==path {allowed=true;break}}
	if !allowed {return nil,fmt.Errorf("no recognized USB serial adapter at %q (requires a 3.3V USB-UART 30-pin breakout, not a standard USB-to-30-pin cable)",path)}
	fd,e:=syscall.Open(path,syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK|syscall.O_CLOEXEC,0)
	if e!=nil{return nil,fmt.Errorf("open serial port: %w",e)}
	var original syscall.Termios
	if e=termios(fd,&original,syscall.TCGETS);e!=nil{syscall.Close(fd);return nil,fmt.Errorf("get serial settings: %w",e)}
	raw:=original
	raw.Iflag=0
	raw.Oflag=0
	raw.Lflag=0
	raw.Cflag=syscall.CS8|syscall.CLOCAL|syscall.CREAD|speed
	raw.Ispeed=speed
	raw.Ospeed=speed
	raw.Cc[syscall.VMIN]=0
	raw.Cc[syscall.VTIME]=0
	if e=termios(fd,&raw,syscall.TCSETS);e!=nil{syscall.Close(fd);return nil,fmt.Errorf("set serial 8N1: %w",e)}
	return &SerialTransport{fd:fd,original:original},nil
}
func (s *SerialTransport) fileDescriptor() (int,error) {
	s.mu.Lock();defer s.mu.Unlock()
	if s.closed {return -1,errors.New("serial port closed")}
	return s.fd,nil
}
func (s *SerialTransport) Send(frame []byte) error {
	if _,e:=Decode(frame);e!=nil{return fmt.Errorf("reject invalid outbound iAP frame: %w",e)}
	s.tx.Lock();defer s.tx.Unlock()
	fd,e:=s.fileDescriptor();if e!=nil{return e}
	b:=append([]byte{0xff},frame...)
	deadline:=time.Now().Add(2*time.Second)
	for len(b)>0 {
		n,err:=syscall.Write(fd,b)
		if n>0 {b=b[n:]}
		if err==nil {if n==0 {time.Sleep(5*time.Millisecond)};continue}
		if err==syscall.EINTR {continue}
		if err==syscall.EAGAIN || err==syscall.EWOULDBLOCK {
			if time.Now().After(deadline) {return errors.New("serial iAP write timeout")}
			time.Sleep(5*time.Millisecond)
			continue
		}
		return fmt.Errorf("serial iAP write: %w",err)
	}
	return nil
}
func (s *SerialTransport) Receive(ctx context.Context) ([]byte,error) {
	s.rx.Lock();defer s.rx.Unlock()
	buf:=make([]byte,512)
	for {
		if e:=ctx.Err();e!=nil {return nil,e}
		if frame,e:=s.frames.Next();frame!=nil||e!=nil{return frame,e}
		fd,e:=s.fileDescriptor();if e!=nil{return nil,e}
		n,err:=syscall.Read(fd,buf)
		if n>0 {
			if e:=s.frames.Feed(buf[:n]);e!=nil{return nil,e}
			continue
		}
		if err!=nil&&err!=syscall.EAGAIN&&err!=syscall.EWOULDBLOCK&&err!=syscall.EINTR {
			return nil,fmt.Errorf("serial iAP read: %w",err)
		}
		select {case <-ctx.Done():return nil,ctx.Err();case <-time.After(5*time.Millisecond):}
	}
}
func (s *SerialTransport) Close() error {
	s.mu.Lock();defer s.mu.Unlock()
	if s.closed{return nil}
	s.closed=true
	_ = termios(s.fd,&s.original,syscall.TCSETS)
	return syscall.Close(s.fd)
}
