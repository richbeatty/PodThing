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
// RecoverUSB attaches to an already-mounted, read-only volume belonging to
// this specific enumerated iPod. This is needed after an app restart: the
// kernel may still have the mount even though PodThing lost *Volume.
func RecoverUSB(sysPath string) (*Volume,*Library,error) {
	parts,e:=diskForUSB(sysPath)
	if e!=nil{return nil,nil,e}
	raw,e:=os.ReadFile("/proc/self/mountinfo")
	if e!=nil{return nil,nil,e}
	for _,line:=range strings.Split(string(raw),"\n") {
		fields:=strings.Fields(line)
		if len(fields)<10||fields[4]!=mountPoint{continue}
		sep:=-1
		for i,v:=range fields{if v=="-" {sep=i;break}}
		if sep<0||sep+2>=len(fields){return nil,nil,errors.New("invalid managed iPod mount metadata")}
		fs,dev:=fields[sep+1],fields[sep+2]
		if fs!="vfat"&&fs!="hfsplus"{return nil,nil,errors.New("managed mount has unexpected filesystem")}
		if !strings.Contains(","+fields[5]+",",",ro,"){return nil,nil,errors.New("managed iPod mount is not read-only")}
		for _,part:=range parts {
			if dev!=filepath.Join("/dev",part){continue}
			lib,e:=Read(mountPoint)
			if e!=nil{return nil,nil,fmt.Errorf("read recovered iPod library: %w",e)}
			return &Volume{Device:dev,Disk:filepath.Join("/dev",diskName(part)),Root:mountPoint,FSType:fs,mounted:true},lib,nil
		}
		return nil,nil,errors.New("managed mount does not belong to the connected iPod")
	}
	return nil,nil,os.ErrNotExist
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
			vol:=&Volume{Device:part,Disk:disk,Root:mountPoint,FSType:fstype,mounted:true}
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
func (v *Volume) Mounted() bool {return v!=nil&&v.mounted}
func (v *Volume) Status() string {
	if !v.Mounted(){return "Ejected"}
	return v.FSType+" read-only on "+v.Device+" (host can safely unmount)"
}
var _=strconv.Itoa
