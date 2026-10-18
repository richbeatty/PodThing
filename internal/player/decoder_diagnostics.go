package player

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// decoderVersion probes the exact absolute decoder binary used for real iPod
// playback. Include dynamic-loader stderr in diagnostics for exit status 127.
func decoderVersion() string {
	ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second)
	defer cancel()
	b,e:=exec.CommandContext(ctx,"/usr/bin/ffmpeg","-hide_banner","-version").CombinedOutput()
	out:=strings.TrimSpace(string(b))
	if len(out)>500 {out=out[:500]}
	line:=out
	if at:=strings.IndexByte(out,'\n');at>=0 {line=out[:at]}
	if e!=nil {
		if ctx.Err()!=nil{return "unavailable: FFmpeg runtime probe timed out"}
		if line=="" {line="no loader stderr received"}
		return fmt.Sprintf("unavailable: %v: %s",e,line)
	}
	if line=="" {return "FFmpeg started but printed no version"}
	return line
}
