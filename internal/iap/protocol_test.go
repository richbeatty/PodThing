package iap

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"
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
		{Packet{Lingo: 0, Command: 1, Data: []byte{4}}, []Packet{ack(0, 1)}}, {Packet{Lingo: 0, Command: 5}, []Packet{ack(0, 5)}}, {Packet{Lingo: 0, Command: 7}, []Packet{{Lingo: 0, Command: 8, Data: []byte("Test iPod\x00")}}},
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
	w := &scriptWire{t: t, script: []exchange{{Packet{Lingo: 0, Command: 1, Data: []byte{4}}, []Packet{{Lingo: 0, Command: 0x14}}}}}
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
