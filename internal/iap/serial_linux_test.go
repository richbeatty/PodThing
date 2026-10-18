//go:build linux

package iap

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestSerialFramesFragmentedSyncAndPackets(t *testing.T) {
	first:=Encode(Packet{Lingo:0,Command:7})
	second:=Encode(Packet{Lingo:4,Command:0x18,Data:[]byte{2}})
	data:=append([]byte{0x11,0xff,0x33,0xff}, append(first,append([]byte{0xff},second...)...)...)
	// Preserve a full FF prefix before each packet.
	f:=&SerialFrames{}
	if e:=f.Feed(data[:4]);e!=nil{t.Fatal(e)}
	if p,e:=f.Next();p!=nil||e!=nil{t.Fatalf("partial returned %x %v",p,e)}
	if e:=f.Feed(data[4:]);e!=nil{t.Fatal(e)}
	p,e:=f.Next()
	if e!=nil||!bytes.Equal(p,first){t.Fatalf("first frame %x %v",p,e)}
	q,e:=f.Next()
	if e!=nil||!bytes.Equal(q,second){t.Fatalf("second frame %x %v",q,e)}
	if p,e:=f.Next();p!=nil||e!=nil{t.Fatalf("expected exhausted frames: %x %v",p,e)}
}

func TestSerialFramesExtendedAndCorrupt(t *testing.T) {
	long:=Encode(Packet{Lingo:4,Command:0x1b,Data:bytes.Repeat([]byte{0x22},300)})
	f:=&SerialFrames{}
	b:=append([]byte{0xff},long...)
	for _,v:=range b {
		if e:=f.Feed([]byte{v});e!=nil {t.Fatal(e)}
	}
	p,e:=f.Next()
	if e!=nil||!bytes.Equal(p,long){t.Fatalf("large frame %d %v",len(p),e)}
	bad:=append([]byte{0xff},append([]byte(nil),long...)...)
	bad[len(bad)-1]^=1
	if e:=f.Feed(bad);e!=nil{t.Fatal(e)}
	if _,e:=f.Next();e==nil||!strings.Contains(e.Error(),"bad serial"){t.Fatalf("expected checksum failure: %v",e)}
	if e:=f.Feed(append([]byte{0xff},Encode(Packet{Lingo:0,Command:7})...));e!=nil{t.Fatal(e)}
	if p,e:=f.Next();e!=nil||len(p)==0{t.Fatalf("corrupt frame poisoned parser: %v",e)}
}

func TestSerialFramesInvalidSizesAndBounds(t *testing.T) {
	f:=&SerialFrames{}
	if e:=f.Feed([]byte{0xff,0x55,0,0xff,0xff});e!=nil{t.Fatal(e)}
	if _,e:=f.Next();e==nil{t.Fatal("oversized frame accepted")}
	if e:=f.Feed(bytes.Repeat([]byte{0xff},2*serialPayloadLimit+17));e==nil{t.Fatal("unbounded buffer accepted")}
}

func TestOpenSerialRejectsUnsafePaths(t *testing.T) {
	if _,e:=OpenSerial("/dev/sda",19200);e==nil{t.Fatal("disk opened as serial port")}
	if _,e:=OpenSerial("/dev/ttyUSB0",9600);e==nil{t.Fatal("unsupported baud accepted")}
	ctx,cancel:=context.WithTimeout(context.Background(),time.Millisecond)
	defer cancel()
	_ = ctx // Physical serial receive is exercised only with the proper dock.
}
