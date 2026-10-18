package player

import (
	"strings"
	"testing"
)

func TestSerialDockControlsAreExplicitAndNeverMountInDemo(t *testing.T) {
	a,e:=New(true,t.TempDir())
	if e!=nil{t.Fatal(e)}
	defer a.Close()
	a.mu.Lock()
	a.s.Screen="settings-usb"
	a.mu.Unlock()
	v:=a.View()
	needed:=map[string]bool{
		"serial-open":false,"serial-play":false,"serial-next":false,
		"serial-previous":false,"serial-browse":false,"serial-close":false,
	}
	for _,row:=range v.Rows {
		if _,ok:=needed[row.Action.Name];ok{needed[row.Action.Name]=true}
	}
	for name,found:=range needed {if !found{t.Errorf("missing serial control %s",name)}}
	if err:=a.Action(Action{Name:"serial-open"});err==nil||!strings.Contains(err.Error(),"real 30-pin UART adapter"){
		t.Fatalf("demo must reject serial hardware access, got %v",err)
	}
	if err:=a.Action(Action{Name:"serial-play"});err==nil||!strings.Contains(err.Error(),"Serial dock test"){
		t.Fatalf("serial buttons require explicit open, got %v",err)
	}
	if s:=a.Snapshot();s.SerialPort!=""||s.StorageMode{
		t.Fatalf("serial test wrote a fake connection or mount: %+v",s)
	}
}
