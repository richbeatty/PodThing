package player

import (
 "testing"

 "github.com/richbeatty/PodThing/internal/storage"
)
func TestCoverCandidatesKeepAlbumAndRetrySeveralTracks(t *testing.T) {
 tracks:=[]storage.Track{
  {ID:1,Artist:"A",Album:"One",Title:"one"},
  {ID:2,Artist:"B",Album:"One",Title:"wrong artist"},
  {ID:3,Artist:"A",Album:"Two",Title:"wrong album"},
  {ID:4,Artist:"A",Album:"One",Title:"alternate no cover"},
  {ID:5,Artist:"A",Album:"One",Title:"alternate with cover"},
  {ID:6,Artist:"A",Album:"One",Title:"more"},
  {ID:7,Artist:"A",Album:"One",Title:"more"},
 }
 c:=coverCandidates(tracks,tracks[0],5)
 if len(c)!=5{t.Fatalf("want primary + four, got %v",c)}
 ids:=[]uint32{1,4,5,6,7}
 for i,want:=range ids {if c[i].ID!=want {t.Fatalf("candidate %d = %d, want %d",i,c[i].ID,want)}}
 if len(coverCandidates(tracks,tracks[0],0))!=0{t.Fatal("zero limit accepted")}
 if len(coverCandidates(tracks,tracks[0],1))!=1{t.Fatal("single candidate wrong")}
}
