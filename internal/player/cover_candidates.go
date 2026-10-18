package player

import "github.com/richbeatty/PodThing/internal/storage"

// coverCandidates checks the playing/representative track first, then
// several distinct tracks from exactly the same artist and album. It must
// not stop after one missing embedded image.
func coverCandidates(tracks []storage.Track, representative storage.Track, limit int) []storage.Track {
	if limit<=0 {return nil}
	result:=[]storage.Track{representative}
	seen:=map[uint32]bool{representative.ID:true}
	for _,t:=range tracks {
		if len(result)>=limit {break}
		if seen[t.ID] || t.Artist!=representative.Artist || t.Album!=representative.Album {continue}
		seen[t.ID]=true
		result=append(result,t)
	}
	return result
}
