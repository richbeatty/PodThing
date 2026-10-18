//go:build linux

package storage

import (
 "errors"
 "os"
 "strings"
 "testing"
)

func TestInspectManagedMountRejectsUnrelatedOrWritableDisk(t *testing.T) {
 good:=[]byte("35 25 8:2 / /run/podthing-ipod ro,relatime - vfat /dev/sdb2 ro,uid=0\n")
 v,e:=InspectManagedMount(good,[]string{"sdb2"})
 if e!=nil||v==nil||!v.Mounted()||v.FSType!="vfat"||v.Device!="/dev/sdb2"{t.Fatalf("expected verified read-only mount: %+v %v",v,e)}
 bad:=[][]byte{
  []byte("35 25 8:2 / /run/podthing-ipod rw,relatime - vfat /dev/sdb2 rw\n"),
  []byte("35 25 8:2 / /run/podthing-ipod ro,relatime - ext4 /dev/sdb2 ro\n"),
  []byte("35 25 8:2 / /run/podthing-ipod ro,relatime - vfat /dev/sda2 ro\n"),
 }
 for _,mounts:=range bad {
  if _,e:=InspectManagedMount(mounts,[]string{"sdb2"});e==nil{
   t.Fatalf("should reject unsafe or unrelated mount %q",mounts)
  }
 }
 if _,e:=InspectManagedMount([]byte(""),[]string{"sdb2"});!errors.Is(e,os.ErrNotExist) {
  t.Fatalf("absent mount should be distinguished from unsafe one: %v",e)
 }
 if !strings.Contains(v.Status(),"read-only"){t.Fatal("did not preserve read-only status")}
}

func TestUSBEjectNeverDetachesUnverifiedOrMountedDevice(t *testing.T) {
	if e:=(*Volume)(nil).ReleaseUSBConnection();e==nil {
		t.Fatal("nil volume must not deauthorize USB")
	}
	if e:=(&Volume{USBPath:"/sys/bus/usb/devices/1-1",mounted:true}).ReleaseUSBConnection();e==nil {
		t.Fatal("must not detach a mounted volume")
	}
	if e:=(&Volume{Device:"/dev/sdb1"}).ReleaseUSBConnection();e==nil {
		t.Fatal("must not detach without the original iPod USB identity")
	}
	// A real-looking sysfs authorization file is NOT enough. The actual
	// iPod USB disk/partition ancestry must still be proven by Linux.
	root:=t.TempDir()
	if e:=os.WriteFile(root+"/idVendor",[]byte("05ac\n"),0600);e!=nil{t.Fatal(e)}
	if e:=os.WriteFile(root+"/authorized",[]byte("1\n"),0600);e!=nil{t.Fatal(e)}
	v:=&Volume{Device:"/dev/not-an-ipod",USBPath:root,mounted:false}
	if e:=v.ReleaseUSBConnection();e==nil{
		t.Fatal("must refuse to detach an Apple-looking USB device without matching physical partition")
	}
	b,e:=os.ReadFile(root+"/authorized")
	if e!=nil||string(b)!="1\n"{t.Fatalf("unsafe USB authorization mutation: %q %v",b,e)}
}
