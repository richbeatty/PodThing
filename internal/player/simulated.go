package player

import "time"

// SimulatedPlayback advances the UI clock without opening a music file,
// starting ffmpeg, or claiming audio was sent to an output.
type SimulatedPlayback struct {
	position uint32
	duration uint32
	running bool
	started time.Time
}
func (p *SimulatedPlayback) Reset() {
	*p=SimulatedPlayback{}
}
func (p *SimulatedPlayback) Start(duration uint32) {
	if duration==0 {duration=240000}
	p.position=0
	p.duration=duration
	p.running=true
	p.started=time.Now()
}
func (p *SimulatedPlayback) State() (uint32,bool) {
	if p.duration==0{return 0,false}
	position:=uint64(p.position)
	if p.running {position+=uint64(time.Since(p.started)/time.Millisecond)}
	if position>uint64(p.duration){position=uint64(p.duration)}
	return uint32(position),p.running
}
func (p *SimulatedPlayback) Toggle() bool {
	if p.duration==0 {return false}
	if p.running {
		p.position,_=p.State()
		p.running=false
	}else{
		p.running=true
		p.started=time.Now()
	}
	return p.running
}
func (p *SimulatedPlayback) Seek(position uint32) {
	if p.duration==0{return}
	if position>p.duration{position=p.duration}
	p.position=position
	p.started=time.Now()
}
func (a *App) simulatedTick() {
	if a.library==nil{return}
	pos,playing:=a.sim.State()
	a.mu.Lock()
	a.s.Now.Position=pos
	a.s.Now.Playing=playing
	a.mu.Unlock()
	if playing && pos>=a.sim.duration {
		current:=a.Snapshot()
		if current.Now.Repeat==1 {
			if err:=a.playStorage(uint32(a.activeTrack));err!=nil {a.Log("Simulated repeat: "+err.Error())}
		}else if err:=a.storageControl("next");err!=nil {
			a.sim.Seek(a.sim.duration)
			a.sim.running=false
			a.mu.Lock()
			a.s.Now.Playing=false
			a.s.Status="End of queue (simulated)"
			a.mu.Unlock()
		}
	}
}
