package player

import (
	"errors"
	"fmt"
	"strings"

	"github.com/richbeatty/PodThing/internal/iap"
)

// Serial testing uses a 3.3V 30-pin UART adapter. It is independent
// from the USB HID and read-only storage connection modes.
func (a *App) openSerial() error {
	if a.s.Demo {return errors.New("serial testing requires a real 30-pin UART adapter and iPod")}
	if a.volume!=nil || a.client!=nil {
		return errors.New("close the existing iPod USB session before opening serial iAP")
	}
	if a.serial!=nil{return errors.New("serial test is already active; close it first")}
	if a.Snapshot().HostRole!="host" {
		return errors.New("enable USB host before using a USB-to-UART adapter")
	}
	ports:=iap.SerialPorts()
	if len(ports)==0 {
		return errors.New("no USB-UART adapter found; a regular USB-to-30-pin iPod cable does not expose the 30-pin serial pins")
	}
	if len(ports)>1 {
		return fmt.Errorf("%d USB-UART adapters found; connect only one for this test",len(ports))
	}
	transport,e:=iap.OpenSerial(ports[0],19200)
	if e!=nil{return e}
	c:=iap.New(transport,a.Log)
	if e=c.BeginSimpleRemote();e!=nil {
		_=c.Abort()
		return fmt.Errorf("serial iAP Simple Remote identification: %w",e)
	}
	a.serial=c
	a.mu.Lock()
	a.s.SerialPort=ports[0]
	a.s.Status="UART Simple Remote test opened at 19200 baud; keys are unacknowledged. Observe the physical iPod"
	a.mu.Unlock()
	a.Log("SERIAL test: opened "+ports[0]+" at 19200 8N1, no iPod disk mount or MFi claim")
	return nil
}

func (a *App) serialKey(name string) error {
	if a.serial==nil{return errors.New("select Serial dock test first; connect a real 3.3V USB-UART breakout")}
	key:=strings.TrimPrefix(name,"serial-")
	if key!="play"&&key!="next"&&key!="previous" {
		return fmt.Errorf("unknown serial key %q",key)
	}
	if e:=a.serial.SimpleRemoteKey(key);e!=nil{return e}
	a.mu.Lock()
	a.s.Status="UART sent "+key+"; unacknowledged by protocol. Check the iPod's actual screen/audio"
	a.mu.Unlock()
	return nil
}

// Explicit, optional compatibility probe. This changes the iPod's remote
// UI state and may request MFi; no automatic retries, auth spoofing or
// filesystem reads. The operator must observe the physical iPod.
func (a *App) serialBrowse() error {
	if a.serial==nil {return errors.New("open the serial test first")}
	a.Log("SERIAL Extended Interface probe explicitly requested")
	name,e:=a.serial.Handshake()
	if e!=nil {
		if errors.Is(e,iap.ErrAuthentication) {
			return fmt.Errorf("serial Extended Interface requires MFi authentication: %v",e)
		}
		return fmt.Errorf("serial Extended Interface did not complete: %w",e)
	}
	rows,total,e:=a.serial.Browse(nil,2,0,5) // 2 = Artists
	if e!=nil {
		if errors.Is(e,iap.ErrAuthentication) {
			return fmt.Errorf("serial artist browsing requires MFi authentication: %v",e)
		}
		return fmt.Errorf("serial artist query failed: %w",e)
	}
	example:=make([]string,0,len(rows))
	for _,row:=range rows {if len(example)<3 {example=append(example,row.Name)}}
	a.mu.Lock()
	a.s.Status=fmt.Sprintf("Serial browse answered: %s, %d artists, examples: %s",name,total,strings.Join(example,", "))
	a.mu.Unlock()
	a.Log(fmt.Sprintf("SERIAL Extended Interface response: %d artist records (not playback authentication)",total))
	return nil
}

func (a *App) closeSerial() error {
	if a.serial==nil{return nil}
	e:=a.serial.Abort() // No extra iAP commands or USB resets during close.
	a.serial=nil
	a.mu.Lock()
	a.s.SerialPort=""
	a.s.Status="UART serial test closed; no USB storage was mounted"
	a.mu.Unlock()
	a.Log("SERIAL test closed")
	return e
}
