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
