package storage

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDecoderExit127ShowsMissingRuntimeReason(t *testing.T) {
	err:=exec.Command("sh","-c","exit 127").Run()
	if err==nil {t.Fatal("expected exit status 127")}
	msg:=decoderFailure(err,"/usr/bin/ffmpeg: error while loading shared libraries: libexample.so.0: cannot open shared object file").Error()
	for _,want:=range []string{"exit 127","libexample.so.0","FFmpeg runtime"} {
		if !strings.Contains(msg,want) {t.Fatalf("missing %q from %q",want,msg)}
	}
}

func TestDecoderErrorsPreserveDetailsAndBoundOutput(t *testing.T) {
	msg:=decoderFailure(errors.New("mock failure"),"ALSA output does not exist").Error()
	if !strings.Contains(msg,"ALSA output does not exist"){t.Fatal(msg)}
	long:=decoderFailure(errors.New("mock failure"),strings.Repeat("a",10000)).Error()
	if len(long)>1700{t.Fatalf("unbounded FFmpeg error: %d bytes",len(long))}
}
