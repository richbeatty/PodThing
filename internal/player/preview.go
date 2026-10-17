package player

import "github.com/richbeatty/PodThing/internal/iap"

// PreviewModern is only used by the --demo --preview-modern screenshot
// builder. It does not mount or communicate with a physical iPod.
func (a *App) PreviewModern() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.s.Demo=true
	a.s.StorageMode=true
	a.s.SafeToDisconnect=false
	a.s.Screen="playing"
	a.s.Settings.AudioMode="simulated"
	a.s.Now=iap.NowPlaying{
		Title:"A Song for the Road",
		Artist:"Example Artist",
		Album:"Sample Library",
		Position:54000,
		Duration:198000,
		Playing:true,
	}
	a.s.Artwork=nil
	a.s.HasArtwork=false
	a.s.Error=""
}
