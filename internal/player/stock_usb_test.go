package player

import (
    "strings"
    "testing"

    "github.com/richbeatty/PodThing/internal/hardware"
)

func TestUSBAccessoryDiscoveryNoDiskModeGuess(t *testing.T) {
    root := []hardware.Device{
        {Name:"usb1",Vendor:"1d6b",ProductID:"0002"},
        {Name:"usb2",Vendor:"1d6b",ProductID:"0003"},
    }
    if e:=accessoryDiscoveryError(root,"device");e==nil||!strings.Contains(e.Error(),"Enable USB host"){
        t.Fatalf("wrong USB role should be clear: %v",e)
    }
    if e:=accessoryDiscoveryError(root,"host");e==nil||
       !strings.Contains(e.Error(),"only its internal root hubs")||
       strings.Contains(e.Error(),"disk mode"){
        t.Fatalf("USB-only host report must not suggest iPod disk mode: %v",e)
    }
    withHub:=append(append([]hardware.Device{},root...),hardware.Device{Name:"1-1",Product:"USB Hub",Vendor:"1234",ProductID:"abcd"})
    if e:=accessoryDiscoveryError(withHub,"host");e==nil||!strings.Contains(e.Error(),"but no Apple iPod"){
        t.Fatalf("downstream hub without iPod: %v",e)
    }
    unknownApple:=append(append([]hardware.Device{},root...),hardware.Device{Name:"1-1",Product:"Apple Device",Vendor:"05ac",ProductID:"ffff"})
    if e:=accessoryDiscoveryError(unknownApple,"host");e==nil||!strings.Contains(e.Error(),"not recognized"){
        t.Fatalf("unknown Apple product ID: %v",e)
    }
    ipod:=append(append([]hardware.Device{},root...),hardware.Device{Name:"1-1",Product:"iPod",Vendor:"05ac",ProductID:"1261",IPod:true})
    if e:=accessoryDiscoveryError(ipod,"host");e!=nil{
        t.Fatalf("enumerated iPod can begin non-storage session: %v",e)
    }
    if e:=accessoryDiscoveryError(append(ipod,ipod[2]),"host");e==nil||!strings.Contains(e.Error(),"2 iPods"){
        t.Fatalf("multiple iPods should be rejected: %v",e)
    }
}

func TestDefaultUIConnectIsUSBAccessoryNotStorage(t *testing.T) {
    a,e:=New(true,t.TempDir())
    if e!=nil{t.Fatal(e)}
    defer a.Close()
    a.mu.Lock()
    a.s.Demo=false
    a.s.Connected=false
    a.s.Screen="library"
    a.s.HostRole="host"
    a.mu.Unlock()
    v:=a.View()
    foundMain:=false
    for _,h:=range v.Hits {
        if h.X==240&&h.Y==267 {
            foundMain=true
            if h.Action.Name!="connect" { t.Fatalf("main CTA launched %q; expected USB accessory",h.Action.Name) }
        }
    }
    if !foundMain { t.Fatal("missing main USB accessory connect action") }
    a.mu.Lock()
    a.s.Screen="settings-usb"
    a.mu.Unlock()
    v=a.View()
    if len(v.Rows)==0||v.Rows[0].Action.Name!="connect" {
        t.Fatalf("USB accessory must appear first in connection menu: %+v",v.Rows)
    }
    gotFallback:=false
    for _,row:=range v.Rows {
        if row.Action.Name=="storage-open" {
            gotFallback=true
            if !strings.Contains(strings.ToLower(row.Detail),"experimental") {
                t.Fatal("storage fallback must be marked experimental")
            }
        }
        if strings.HasPrefix(row.Action.Name,"serial-") {
            t.Fatalf("serial adapter controls must not appear: %+v",row)
        }
    }
    if !gotFallback {t.Fatal("existing storage fallback missing")}
}
