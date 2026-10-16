// Package iap implements the accessory (head-unit) side of legacy iAP1.
// It deliberately does not emulate an iPod or invent accessory credentials.
package iap

import (
	"encoding/binary"
	"errors"
	"fmt"
)

var ErrAuthentication = errors.New("iPod requested accessory authentication; this build has no MFi authentication provider")

type Packet struct {
	Lingo   byte
	Command uint16
	Data    []byte
}

func Encode(p Packet) []byte {
	b := []byte{p.Lingo}
	if p.Lingo == 4 {
		b = append(b, byte(p.Command>>8))
	}
	b = append(b, byte(p.Command))
	b = append(b, p.Data...)
	out := []byte{0x55}
	if len(b) < 256 {
		out = append(out, byte(len(b)))
	} else {
		out = append(out, 0, byte(len(b)>>8), byte(len(b)))
	}
	out = append(out, b...)
	var sum byte
	for _, v := range out[1:] {
		sum += v
	}
	return append(out, -sum)
}

// Decode accepts HID padding but validates the entire iAP packet checksum.
func Decode(b []byte) (Packet, error) {
	if len(b) < 4 || b[0] != 0x55 {
		return Packet{}, errors.New("missing iAP packet header")
	}
	off, n := 2, int(b[1])
	if n == 0 {
		if len(b) < 5 {
			return Packet{}, errors.New("short extended header")
		}
		n = int(binary.BigEndian.Uint16(b[2:4]))
		off = 4
	}
	if n < 2 || len(b) < off+n+1 {
		return Packet{}, errors.New("truncated iAP packet")
	}
	var sum byte
	for _, v := range b[1 : off+n+1] {
		sum += v
	}
	if sum != 0 {
		return Packet{}, errors.New("invalid iAP checksum")
	}
	p := Packet{Lingo: b[off], Command: uint16(b[off+1])}
	start := off + 2
	if p.Lingo == 4 {
		if n < 3 {
			return Packet{}, errors.New("short extended-interface command")
		}
		p.Command = binary.BigEndian.Uint16(b[off+1:])
		start++
	}
	p.Data = append([]byte(nil), b[start:off+n]...)
	return p, nil
}

type Report struct {
	ID     byte
	Bytes  int
	Output bool
}

// ParseReports reads the device's descriptor instead of assuming iPhone or
// emulator report IDs. Bytes excludes the report-ID byte and includes link control.
func ParseReports(b []byte) ([]Report, error) {
	type globals struct {
		size, count int
		id          byte
	}
	g := globals{}
	stack := []globals{}
	type key struct {
		id  byte
		out bool
	}
	bits := map[key]int{}
	for i := 0; i < len(b); {
		t := b[i]
		i++
		if t == 0xfe {
			if i+2 > len(b) {
				return nil, errors.New("short long HID item")
			}
			n := int(b[i])
			i += 2 + n
			if i > len(b) {
				return nil, errors.New("truncated HID item")
			}
			continue
		}
		n := int(t & 3)
		if n == 3 {
			n = 4
		}
		if i+n > len(b) {
			return nil, errors.New("truncated HID descriptor")
		}
		var v uint32
		for j := 0; j < n; j++ {
			v |= uint32(b[i+j]) << uint(j*8)
		}
		i += n
		switch t & 0xfc {
		case 0x74:
			g.size = int(v)
		case 0x94:
			g.count = int(v)
		case 0x84:
			g.id = byte(v)
		case 0xa4:
			stack = append(stack, g)
		case 0xb4:
			if len(stack) == 0 {
				return nil, errors.New("HID global stack underflow")
			}
			g = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case 0x80, 0x90:
			if g.size > 64 || g.count > 65536 {
				return nil, errors.New("oversized HID report")
			}
			bits[key{g.id, t&0xfc == 0x90}] += g.size * g.count
		}
	}
	var out []Report
	for k, n := range bits {
		if n > 8 && n <= 8192*8 {
			out = append(out, Report{k.id, (n + 7) / 8, k.out})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no usable HID input/output reports")
	}
	return out, nil
}

func Reports(frame []byte, defs []Report) ([][]byte, error) {
	var best *Report
	for i := range defs {
		d := &defs[i]
		if !d.Output || d.Bytes < 2 {
			continue
		}
		if best == nil || (best.Bytes-1 < len(frame) && d.Bytes > best.Bytes) || (d.Bytes-1 >= len(frame) && d.Bytes < best.Bytes) {
			best = d
		}
	}
	if best == nil {
		return nil, errors.New("iPod exposes no HID output reports")
	}
	var out [][]byte
	for off := 0; off < len(frame); {
		n := min(best.Bytes-1, len(frame)-off)
		control := byte(0)
		if off > 0 {
			control |= 1
		}
		if off+n < len(frame) {
			control |= 2
		}
		r := make([]byte, best.Bytes+1)
		r[0] = best.ID
		r[1] = control
		copy(r[2:], frame[off:off+n])
		out = append(out, r)
		off += n
	}
	return out, nil
}

type Reassembler struct {
	buf     []byte
	started bool
}

func (r *Reassembler) Feed(report []byte) ([]byte, error) {
	if len(report) < 3 {
		return nil, errors.New("short HID report")
	}
	c := report[1]
	if c > 3 {
		return nil, fmt.Errorf("unknown HID link control %02x", c)
	}
	if c&1 == 0 {
		r.buf = nil
		r.started = true
	} else if !r.started {
		return nil, errors.New("HID continuation without first fragment")
	}
	if len(r.buf)+len(report)-2 > 65540 {
		r.buf = nil
		r.started = false
		return nil, errors.New("oversized iAP frame")
	}
	r.buf = append(r.buf, report[2:]...)
	if c&2 != 0 {
		return nil, nil
	}
	out := r.buf
	r.buf = nil
	r.started = false
	return out, nil
}

func U32(v uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, v); return b }
