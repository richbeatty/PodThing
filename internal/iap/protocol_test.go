package iap

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPacketGoldenVectors(t *testing.T) {
	// Independently specified wire bytes, including length in the checksum.
	for _, tt := range []struct {
		p   Packet
		hex string
	}{{Packet{Lingo: 0, Command: 7}, "55020007f7"}, {Packet{Lingo: 0, Command: 1, Data: []byte{4}}, "5503000104f8"}, {Packet{Lingo: 4, Command: 0x18, Data: []byte{2}}, "550404001802de"}, {Packet{Lingo: 4, Command: 0x29, Data: []byte{1}}, "550404002901ce"}} {
		want, _ := hex.DecodeString(tt.hex)
		got := Encode(tt.p)
		if !bytes.Equal(got, want) {
			t.Fatalf("encode %x; want %x", got, want)
		}
		p, e := Decode(append(got, 0, 0, 0))
		if e != nil || p.Lingo != tt.p.Lingo || p.Command != tt.p.Command || !bytes.Equal(p.Data, tt.p.Data) {
			t.Fatalf("decode: %+v %v", p, e)
		}
	}
}
func TestPacketBoundaryAndMalformed(t *testing.T) {
	for _, n := range []int{0, 252, 253, 254, 255, 256, 1000} {
		p := Packet{Lingo: 4, Command: 0x1b, Data: bytes.Repeat([]byte{0xa5}, n)}
		b := Encode(p)
		q, e := Decode(b)
		if e != nil || !bytes.Equal(q.Data, p.Data) {
			t.Fatalf("length %d: %v", n, e)
		}
		for i := 0; i < len(b); i++ {
			_, _ = Decode(b[:i])
		}
		b[len(b)-1] ^= 1
		if _, e = Decode(b); e == nil {
			t.Fatal("accepted corrupt checksum")
		}
	}
}
func TestHIDDescriptorAndFragmentation(t *testing.T) {
	descriptor := []byte{0x75, 8, 0x85, 0x0d, 0x95, 9, 0x91, 0, 0x85, 1, 0x95, 13, 0x81, 0}
	defs, e := ParseReports(descriptor)
	if e != nil {
		t.Fatal(e)
	}
	var output Report
	for _, d := range defs {
		if d.Output {
			output = d
		}
	}
	if output.ID != 13 || output.Bytes != 9 {
		t.Fatalf("descriptor: %+v", defs)
	}
	frame := Encode(Packet{Lingo: 4, Command: 0x1a, Data: []byte{2, 0, 0, 0, 0, 0, 0, 0, 25}})
	reports, e := Reports(frame, defs)
	if e != nil {
		t.Fatal(e)
	}
	if len(reports) != 2 || reports[0][1] != 2 || reports[1][1] != 1 {
		t.Fatalf("bad fragment flags: %x", reports)
	}
	r := Reassembler{}
	var data []byte
	for _, v := range reports {
		data, e = r.Feed(v)
		if e != nil {
			t.Fatal(e)
		}
	}
	if !bytes.Equal(data[:len(frame)], frame) {
		t.Fatalf("reassembly %x", data)
	}
	r = Reassembler{}
	if _, e = r.Feed([]byte{1, 1, 0}); e == nil {
		t.Fatal("accepted orphan continuation")
	}
}

type exchange struct {
	request   Packet
	responses []Packet
}
type scriptWire struct {
	t        *testing.T
	script   []exchange
	incoming [][]byte
	closed   bool
}

func (w *scriptWire) Send(b []byte) error {
	w.t.Helper()
	p, e := Decode(b)
	if e != nil {
		w.t.Fatal(e)
	}
	if len(w.script) == 0 {
		w.t.Fatalf("unexpected command %+v", p)
	}
	x := w.script[0]
	w.script = w.script[1:]
	if !bytes.Equal(Encode(p), Encode(x.request)) {
		w.t.Fatalf("request %x; expected %x", Encode(p), Encode(x.request))
	}
	for _, r := range x.responses {
		w.incoming = append(w.incoming, Encode(r))
	}
	return nil
}
func (w *scriptWire) Receive(ctx context.Context) ([]byte, error) {
	if len(w.incoming) == 0 {
		return nil, context.DeadlineExceeded
	}
	p := w.incoming[0]
	w.incoming = w.incoming[1:]
	return p, nil
}
func (w *scriptWire) Close() error { w.closed = true; return nil }
func ack(lingo byte, cmd uint16) Packet {
	if lingo == 4 {
		return Packet{Lingo: 4, Command: 1, Data: []byte{0, byte(cmd >> 8), byte(cmd)}}
	}
	return Packet{Lingo: 0, Command: 2, Data: []byte{0, byte(cmd)}}
}
func TestAccessoryHandshakeAndPagedBrowsing(t *testing.T) {
	w := &scriptWire{t: t, script: []exchange{
		{Packet{Lingo: 0, Command: 1, Data: []byte{4}}, nil}, {Packet{Lingo: 0, Command: 3}, []Packet{{Lingo: 0, Command: 4, Data: []byte{0}}}}, {Packet{Lingo: 0, Command: 5}, []Packet{{Lingo: 0, Command: 2, Data: []byte{6, 5, 0, 0, 3, 0xe8}}, ack(0, 5)}}, {Packet{Lingo: 0, Command: 7}, []Packet{{Lingo: 0, Command: 8, Data: []byte("Test iPod\x00")}}},
		{Packet{Lingo: 4, Command: 0x16}, []Packet{ack(4, 0x16)}}, {Packet{Lingo: 4, Command: 0x17, Data: []byte{2, 0, 0, 0, 7}}, []Packet{ack(4, 0x17)}}, {Packet{Lingo: 4, Command: 0x18, Data: []byte{3}}, []Packet{{Lingo: 4, Command: 0x19, Data: U32(27)}}},
		{Packet{Lingo: 4, Command: 0x1a, Data: []byte{3, 0, 0, 0, 25, 0, 0, 0, 2}}, []Packet{{Lingo: 4, Command: 0x1b, Data: append(U32(25), []byte("First\x00")...)}, {Lingo: 4, Command: 0x1b, Data: append(U32(25), []byte("duplicate\x00")...)}, {Lingo: 4, Command: 0x1b, Data: append(U32(26), []byte("Second\x00")...)}}},
	}}
	c := New(w, nil)
	name, e := c.Handshake()
	if e != nil || name != "Test iPod" {
		t.Fatalf("handshake %q %v", name, e)
	}
	rows, total, e := c.Browse([]Selection{{Category: 2, Index: 7}}, 3, 25, 25)
	if e != nil || total != 27 || len(rows) != 2 || rows[1].Name != "Second" {
		t.Fatalf("browse %+v %d %v", rows, total, e)
	}
	if len(w.script) != 0 {
		t.Fatal("unused exchanges")
	}
}
func TestAuthenticationChallengeIsNotFaked(t *testing.T) {
	w := &scriptWire{t: t, script: []exchange{{Packet{Lingo: 0, Command: 1, Data: []byte{4}}, nil}, {Packet{Lingo: 0, Command: 3}, []Packet{{Lingo: 0, Command: 0x14}}}}}
	c := New(w, nil)
	_, e := c.Handshake()
	if !errors.Is(e, ErrAuthentication) {
		t.Fatalf("wanted authentication blocker, got %v", e)
	}
}
func TestCommandRejection(t *testing.T) {
	w := &scriptWire{t: t, script: []exchange{{Packet{Lingo: 4, Command: 0x29, Data: []byte{3}}, []Packet{{Lingo: 4, Command: 1, Data: []byte{4, 0, 0x29}}}}}}
	if e := New(w, nil).Control(3); e == nil {
		t.Fatal("ignored iPod rejection")
	}
}
func TestDigitalAudioCapabilities(t *testing.T) {
	identify := append(U32(1|1<<4|1<<10), make([]byte, 8)...)
	w := &scriptWire{t: t, script: []exchange{{Packet{Lingo: 0, Command: 0x13, Data: identify}, []Packet{ack(0, 0x13), {Lingo: 10, Command: 2}}}, {Packet{Lingo: 10, Command: 3, Data: U32(44100)}, []Packet{{Lingo: 10, Command: 4, Data: append(U32(44100), make([]byte, 8)...)}}}, {Packet{Lingo: 10, Command: 0, Data: []byte{0, 4}}, nil}}}
	c := New(w, nil)
	if e := c.RequestDigitalAudio(); e != nil {
		t.Fatal(e)
	}
	if len(w.script) != 0 {
		t.Fatal("audio negotiation incomplete")
	}
}

// A real iPod need not acknowledge legacy Identify, only EnterRemoteUIMode.
func TestHandshakeWithoutIdentifyAck(t *testing.T) {
	w := &scriptWire{t: t, script: []exchange{
		{Packet{Lingo: 0, Command: 1, Data: []byte{4}}, nil},
		{Packet{Lingo: 0, Command: 3}, []Packet{{Lingo: 0, Command: 4, Data: []byte{0}}}},
		{Packet{Lingo: 0, Command: 5}, []Packet{ack(0, 5)}},
		{Packet{Lingo: 0, Command: 7}, []Packet{{Lingo: 0, Command: 8, Data: []byte("Classic\x00")}}},
	}}
	name, err := New(w, nil).Handshake()
	if err != nil || name != "Classic" || len(w.script) != 0 {
		t.Fatalf("handshake = %q, %v, unused steps %d", name, err, len(w.script))
	}
}
type restoreWire struct { closed, restored bool }
func (w *restoreWire) Send([]byte) error { return nil }
func (w *restoreWire) Receive(context.Context) ([]byte, error) { return nil, context.DeadlineExceeded }
func (w *restoreWire) Close() error { w.closed = true; return nil }
func (w *restoreWire) RestoreOriginalOnClose() { w.restored = true }
func TestAbortPreservesAccessoryMode(t *testing.T) {
	w := &restoreWire{}
	if err := New(w, nil).Abort(); err != nil || !w.closed || w.restored {
		t.Fatalf("abort should close without restoring storage: %+v, %v", w, err)
	}
}
func TestExplicitDisconnectRestoresStorage(t *testing.T) {
	w := &restoreWire{}
	if err := New(w, nil).Close(); err != nil || !w.closed || !w.restored {
		t.Fatalf("explicit close should request original configuration: %+v, %v", w, err)
	}
}

func TestIdentifyAlreadyEnteredRemoteMode(t *testing.T) {
	w := &scriptWire{t: t, script: []exchange{
		{Packet{Lingo: 0, Command: 1, Data: []byte{4}}, nil},
		{Packet{Lingo: 0, Command: 3}, []Packet{{Lingo: 0, Command: 4, Data: []byte{1}}}},
		{Packet{Lingo: 0, Command: 7}, []Packet{{Lingo: 0, Command: 8, Data: []byte("iPod Classic\x00")}}},
	}}
	name, err := New(w, nil).Handshake()
	if err != nil || name != "iPod Classic" || len(w.script) != 0 {
		t.Fatalf("already-remote handshake: %q %v remaining %d", name, err, len(w.script))
	}
}

func TestMissingNameDoesNotResetRemoteMode(t *testing.T) {
	w := &scriptWire{t: t, script: []exchange{
		{Packet{Lingo: 0, Command: 1, Data: []byte{4}}, nil},
		{Packet{Lingo: 0, Command: 3}, []Packet{{Lingo: 0, Command: 4, Data: []byte{1}}}},
		{Packet{Lingo: 0, Command: 7}, nil},
	}}
	name, err := New(w, nil).Handshake()
	if err != nil || name != "iPod" { t.Fatalf("name fallback = %q %v", name, err) }
}

func TestQueryRemoteModeMalformed(t *testing.T) {
	w := &scriptWire{t: t, script: []exchange{
		{Packet{Lingo: 0, Command: 1, Data: []byte{4}}, nil},
		{Packet{Lingo: 0, Command: 3}, []Packet{{Lingo: 0, Command: 4}}},
	}}
	_, err := New(w, nil).Handshake()
	if err == nil || !strings.Contains(err.Error(), "no state byte") { t.Fatalf("malformed response: %v", err) }
}

func TestHIDOutputReportSelectionIndependentOfDescriptorOrder(t *testing.T) {
	frame := []byte{1,2,3,4,5,6,7}
	for _,defs := range [][]Report{
		{{ID: 1, Bytes: 8, Output: true}, {ID: 2, Bytes: 4, Output: true}, {ID: 3, Bytes: 12, Output: true}},
		{{ID: 3, Bytes: 12, Output: true}, {ID: 2, Bytes: 4, Output: true}, {ID: 1, Bytes: 8, Output: true}},
	} {
		reports,err:=Reports(frame,defs)
		if err!=nil||len(reports)!=1||reports[0][0]!=1 {
			t.Fatalf("wanted smallest fitting output report, got %x: %v",reports,err)
		}
	}
	defs:=[]Report{{ID: 1, Bytes: 4, Output: true},{ID: 2, Bytes: 8, Output: true}}
	reports,err:=Reports(bytes.Repeat([]byte{9},22),defs)
	if err!=nil||len(reports)!=4||reports[0][0]!=2 {
		t.Fatalf("wanted fragmentation using largest report, got %d: %v",len(reports),err)
	}
}

func TestAuthenticationDetailsDoNotLogChallenge(t *testing.T) {
	secret := []byte{11,22,33,44,55}
	w := &scriptWire{t:t,script:[]exchange{
		{Packet{Lingo:0,Command:1,Data:[]byte{4}},nil},
		{Packet{Lingo:0,Command:3},[]Packet{{Lingo:0,Command:0x17,Data:secret}}},
	}}
	var logs []string
	_,err:=New(w,func(s string){logs=append(logs,s)}).Handshake()
	var challenge *AuthenticationRequest
	if !errors.As(err,&challenge) || challenge.Command!=0x17 || challenge.Length!=len(secret) || !errors.Is(err,ErrAuthentication) {
		t.Fatalf("wanted typed MFi signature request, got %v",err)
	}
	for _,l:=range logs {
		if strings.Contains(l,"11 22 33") || strings.Contains(l,"0b1621") {t.Fatal("authentication challenge leaked to logs")}
	}
}
func TestSimpleRemoteCompatibilitySequence(t *testing.T) {
	w:=&scriptWire{t:t,script:[]exchange{
		{Packet{Lingo:0,Command:1,Data:[]byte{2}},nil},
		{Packet{Lingo:2,Command:0,Data:[]byte{0x01}},nil},
		{Packet{Lingo:2,Command:0,Data:[]byte{0}},nil},
		{Packet{Lingo:2,Command:0,Data:[]byte{0x08}},nil},
		{Packet{Lingo:2,Command:0,Data:[]byte{0}},nil},
		{Packet{Lingo:2,Command:0,Data:[]byte{0x10}},nil},
		{Packet{Lingo:2,Command:0,Data:[]byte{0}},nil},
	}}
	c:=New(w,nil)
	c.simpleHold=0
	if err:=c.BeginSimpleRemote();err!=nil{t.Fatal(err)}
	for _,k:=range []string{"play","next","previous"}{if err:=c.SimpleRemoteKey(k);err!=nil{t.Fatal(err)}}
	if len(w.script)!=0 {t.Fatalf("unused commands %d",len(w.script))}
	if err:=c.SimpleRemoteKey("artwork");err==nil{t.Fatal("accepted non-simple remote command")}
}

func TestModernAccessoryProbeNoCredentials(t *testing.T) {
	// 0x1d: General, Simple, Display and Extended. No Digital Audio,
	// auth options, device ID, or fictitious MFi certificate.
	data:=append(U32(0x1d),U32(0)...)
	data=append(data,U32(0)...)
	w:=&scriptWire{t:t,script:[]exchange{
		{Packet{Lingo:0,Command:0x13,Data:data},[]Packet{ack(0,0x13)}},
		{Packet{Lingo:0,Command:7},[]Packet{{Lingo:0,Command:8,Data:[]byte("Classic\x00")}}},
		{Packet{Lingo:0,Command:3},[]Packet{{Lingo:0,Command:4,Data:[]byte{0}}}},
	}}
	name,err:=New(w,nil).ProbeAccessory()
	if err!=nil||name!="Classic"||len(w.script)!=0 {t.Fatalf("probe %q %v remaining %d",name,err,len(w.script))}
}
func TestModernAccessoryProbeStopsOnAuthentication(t *testing.T) {
	data:=append(U32(0x1d),U32(0)...)
	data=append(data,U32(0)...)
	w:=&scriptWire{t:t,script:[]exchange{
		{Packet{Lingo:0,Command:0x13,Data:data},[]Packet{{Lingo:0,Command:0x14}}},
	}}
	_,err:=New(w,nil).ProbeAccessory()
	if !errors.Is(err,ErrAuthentication)||len(w.script)!=0 {t.Fatalf("no key means no fake auth: %v",err)}
}
func TestSimpleRemoteHeldFor150ms(t *testing.T) {
	w:=&scriptWire{t:t,script:[]exchange{
		{Packet{Lingo:2,Command:0,Data:[]byte{8}},nil},
		{Packet{Lingo:2,Command:0,Data:[]byte{0}},nil},
	}}
	c:=New(w,nil)
	start:=time.Now()
	if err:=c.SimpleRemoteKey("next");err!=nil {t.Fatal(err)}
	if time.Since(start)<140*time.Millisecond {t.Fatal("released button too soon")}
}
