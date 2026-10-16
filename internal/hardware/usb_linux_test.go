package hardware

import (
	"testing"
	"unsafe"
)

func TestDescriptorConfigurations(t *testing.T) {
	b := []byte{
		18, 1, 0, 2, 0, 0, 0, 64, 0xac, 5, 0x61, 0x12, 0, 1, 1, 2, 3, 2,
		9, 2, 25, 0, 1, 1, 0, 0x80, 50, 9, 4, 0, 0, 1, 8, 6, 0x50, 0, 7, 5, 0x81, 2, 0, 2, 0,
		9, 2, 34, 0, 1, 2, 0, 0x80, 50, 9, 4, 2, 0, 1, 3, 0, 0, 0, 9, 0x21, 0x11, 1, 0, 1, 0x22, 0x30, 0, 7, 5, 0x83, 3, 64, 0, 4,
	}
	c, e := ParseDescriptors(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(c) != 2 || c[1].Value != 2 || c[1].Interfaces[0].ReportLength != 48 || c[1].Interfaces[0].Endpoints[0].Address != 0x83 {
		t.Fatalf("bad parse: %+v", c)
	}
	for _, bad := range [][]byte{{0, 2}, {9, 4, 0}, {2, 5}, {1, 2}} {
		if _, e := ParseDescriptors(bad); e == nil {
			t.Fatalf("accepted %x", bad)
		}
	}
}
func TestLinuxUSBIOCTLLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		if unsafe.Sizeof(controlTransfer{}) != 24 || unsafe.Sizeof(urb{}) != 56 || unsafe.Offsetof(urb{}.Buffer) != 16 {
			t.Fatal("incorrect 64-bit Linux USB ABI")
		}
		if ioctlCode(3, 0, 24) != 0xc0185500 || ioctlCode(2, 10, 56) != 0x8038550a || ioctlCode(1, 13, 8) != 0x4008550d {
			t.Fatal("incorrect USB ioctl numbers")
		}
	}
}
