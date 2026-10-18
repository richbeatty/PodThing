//go:build linux

package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const mountPoint="/run/podthing-ipod"

type Volume struct {
	Device string
	Disk string
	Root string
	FSType string
	USBPath string // exact enumerated iPod USB device used to mount this partition
	mounted bool
}
func diskForUSB(sysPath string) ([]string,error) {
	base,e:=filepath.EvalSymlinks(sysPath)
	if e!=nil{return nil,e}
	found:=[]string{}
	blocks,_:=filepath.Glob("/sys/class/block/*")
	for _,block:=range blocks {
		if _,e=os.Stat(filepath.Join(block,"partition"));e!=nil{continue}
		real,e:=filepath.EvalSymlinks(block)
		if e!=nil||!strings.HasPrefix(real,base+"/"){continue}
		found=append(found,filepath.Base(block))
	}
	sort.Strings(found)
	return found,nil
}
func diskName(part string) string {
	target,e:=filepath.EvalSymlinks(filepath.Join("/sys/class/block",part))
	if e!=nil{return ""}
	return filepath.Base(filepath.Dir(target))
}
func isMounted(path string) bool {
	b,e:=os.ReadFile("/proc/self/mountinfo")
	if e!=nil{return true} // fail closed
	for _,line:=range strings.Split(string(b),"\n") {
		parts:=strings.Fields(line)
		if len(parts)>4&&parts[4]==path{return true}
		for i,s:=range parts {
			if s=="-" && i+2<len(parts) && parts[i+2]==path{return true}
		}
	}
	return false
}
func setBlockReadOnly(path string) error {
	f,e:=os.OpenFile(path,os.O_RDONLY,0)
	if e!=nil{return e}
	defer f.Close()
	one:=int32(1)
	_,_,errno:=syscall.Syscall(syscall.SYS_IOCTL,f.Fd(),0x125d,uintptr(unsafe.Pointer(&one))) // BLKROSET
	if errno!=0{return fmt.Errorf("BLKROSET %s: %w",path,errno)}
	return nil
}
// InspectManagedMount permits recovery only for a kernel read-only
// filesystem located at the *exact* PodThing mountpoint and backed by a
// partition of the enumerated iPod. Never unmount an unrelated disk.
func InspectManagedMount(mountinfo []byte, partitions []string) (*Volume,error) {
	for _,line:=range strings.Split(string(mountinfo),"\n") {
		fields:=strings.Fields(line)
		if len(fields)<10||fields[4]!=mountPoint{continue}
		sep:=-1
		for i,v:=range fields{if v=="-" {sep=i;break}}
		if sep<0||sep+2>=len(fields){return nil,errors.New("invalid managed iPod mount metadata")}
		fs,dev:=fields[sep+1],fields[sep+2]
		if fs!="vfat"&&fs!="hfsplus"{return nil,errors.New("managed mount has unexpected filesystem")}
		if !strings.Contains(","+fields[5]+",",",ro,"){return nil,errors.New("managed iPod mount is not read-only")}
		for _,part:=range partitions{
			if dev!=filepath.Join("/dev",part){continue}
			return &Volume{Device:dev,Disk:filepath.Join("/dev",diskName(part)),Root:mountPoint,FSType:fs,mounted:true},nil
		}
		return nil,errors.New("managed mount does not belong to the connected iPod")
	}
	return nil,os.ErrNotExist
}
// RecoverMount verifies that a stale PodThing mount belongs to this exact
// enumerated iPod and is read-only. Safe eject does NOT require parsing the
// library database, which may be corrupt or unavailable after an app restart.
func RecoverMount(sysPath string) (*Volume,error) {
	parts,e:=diskForUSB(sysPath)
	if e!=nil{return nil,e}
	raw,e:=os.ReadFile("/proc/self/mountinfo")
	if e!=nil{return nil,e}
	vol,e:=InspectManagedMount(raw,parts)
	if e!=nil{return nil,e}
	vol.USBPath=sysPath
	return vol,nil
}
// RecoverUSB attaches to an existing, verified iPod mount after an app
// restart, then re-reads iTunesDB without modifying anything on the disk.
func RecoverUSB(sysPath string) (*Volume,*Library,error) {
	vol,e:=RecoverMount(sysPath)
	if e!=nil{return nil,nil,e}
	lib,e:=Read(mountPoint)
	if e!=nil{return nil,nil,fmt.Errorf("read recovered iPod library: %w",e)}
	return vol,lib,nil
}

// OpenUSB changes NO bytes on disk. Both the containing USB disk and its
// partition become kernel-enforced read-only before any filesystem mount.
// Never fsck, replay a journal, or fall back to a read-write mount.
func OpenUSB(sysPath string) (*Volume,*Library,error) {
	parts,e:=diskForUSB(sysPath)
	if e!=nil{return nil,nil,e}
	if len(parts)==0{return nil,nil,errors.New("no USB storage partitions under this iPod; reconnect it in disk mode")}
	if e=os.MkdirAll(mountPoint,0700);e!=nil{return nil,nil,e}
	if isMounted(mountPoint){return RecoverUSB(sysPath)}
	failures:=[]string{}
	for _,name:=range parts {
		part:=filepath.Join("/dev",name)
		disk:=filepath.Join("/dev",diskName(name))
		if disk=="/dev/" {continue}
		if isMounted(part) || isMounted(disk) {
			failures=append(failures,name+": device already mounted elsewhere")
			continue
		}
		if e=setBlockReadOnly(disk);e!=nil {failures=append(failures,e.Error());continue}
		if e=setBlockReadOnly(part);e!=nil {failures=append(failures,e.Error());continue}
		for _,fstype:=range []string{"vfat","hfsplus"} {
			flags:=uintptr(syscall.MS_RDONLY|syscall.MS_NODEV|syscall.MS_NOSUID|syscall.MS_NOEXEC|syscall.MS_NOATIME)
			if e=syscall.Mount(part,mountPoint,fstype,flags,"");e!=nil {continue}
			vol:=&Volume{Device:part,Disk:disk,Root:mountPoint,FSType:fstype,USBPath:sysPath,mounted:true}
			db,readErr:=Read(mountPoint)
			if readErr==nil {
				return vol,db,nil
			}
			if unmountErr:=syscall.Unmount(mountPoint,0);unmountErr!=nil {
				return nil,nil,fmt.Errorf("mounted iPod but cannot unmount after database error: %w (database: %v)",unmountErr,readErr)
			}
			failures=append(failures,name+"/"+fstype+": "+readErr.Error())
		}
	}
	return nil,nil,fmt.Errorf("could not open a read-only iPod music volume: %s",strings.Join(failures,"; "))
}
func (v *Volume) Eject() error {
	if v==nil||!v.mounted{return nil}
	// A successful unmount guarantees no open files in the host process.
	// This does not claim the physical iPod display will say OK to disconnect.
	if e:=syscall.Unmount(v.Root,0);e!=nil{return fmt.Errorf("iPod still in use; cannot safely disconnect: %w",e)}
	v.mounted=false
	return nil
}
// ReleaseUSBConnection logically disconnects the *verified iPod* from
// Linux after a successful, normal unmount. Linux's device authorization
// control affects only this one USB device, not the host controller or hub.
// It cannot cut VBUS power and does not guarantee what the iPod LCD shows.
// If authorization is unavailable or verification fails, the unmount is
// still safe, but caller must report USB detachment as unconfirmed.
func (v *Volume) ReleaseUSBConnection() error {
	if v==nil || v.USBPath=="" {return errors.New("iPod USB identity missing; storage is unmounted but USB detach was not attempted")}
	if v.Mounted() {return errors.New("refusing USB detach while iPod storage is mounted")}
	parts,e:=diskForUSB(v.USBPath)
	if os.IsNotExist(e) {return nil} // physically unplugged after unmount
	if e!=nil {return fmt.Errorf("verify iPod USB identity before detach: %w",e)}
	found:=false
	for _,part:=range parts {
		if filepath.Join("/dev",part)==v.Device {found=true;break}
	}
	if !found {return errors.New("iPod USB partition identity changed; refusing to detach another device")}
	if strings.TrimSpace(readUSBVendor(v.USBPath))!="05ac" {
		return errors.New("verified mount no longer belongs to an Apple USB device; USB detach refused")
	}
	target:=filepath.Join(v.USBPath,"authorized")
	if e:=os.WriteFile(target,[]byte("0"),0200);e!=nil {
		return fmt.Errorf("read-only iPod safely unmounted but USB logical disconnect failed: %w",e)
	}
	return nil
}

func readUSBVendor(path string) string {
	data,e:=os.ReadFile(filepath.Join(path,"idVendor"))
	if e!=nil{return ""}
	return string(data)
}

func (v *Volume) Mounted() bool {return v!=nil&&v.mounted}
func (v *Volume) Status() string {
	if !v.Mounted(){return "Ejected"}
	return v.FSType+" read-only on "+v.Device+" (host can safely unmount)"
}
var _=strconv.Itoa
