package downloads

import (
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestSavedCollectionTracksKeepDaemonOrderWhenFinishesDiffer(t *testing.T) {
	collection := "Album"
	finishedFirst, finishedSecond, finishedThird := int64(10), int64(30), int64(20)
	all := []model.DownloadEntry{
		{ID: "one", VideoID: "first", Status: model.DownloadStatusCompleted, Collection: &collection, FinishedAt: &finishedFirst},
		{ID: "two", VideoID: "second", Status: model.DownloadStatusCompleted, Collection: &collection, FinishedAt: &finishedSecond},
		{ID: "three", VideoID: "third", Status: model.DownloadStatusCompleted, Collection: &collection, FinishedAt: &finishedThird},
	}
	_, history, _, _ := projectDownloads(all)
	if len(history) != 1 || history[0].collection == nil {
		t.Fatalf("history = %#v", history)
	}
	tracks := savedJobTracks(history[0].collection.jobs, all)
	if len(tracks) != 3 {
		t.Fatalf("tracks = %#v", tracks)
	}
	if got := []string{tracks[0].VideoID, tracks[1].VideoID, tracks[2].VideoID}; got[0] != "first" || got[1] != "second" || got[2] != "third" {
		t.Fatalf("tracks = %#v", tracks)
	}
}

func TestPartlyFailedCollectionKeepsRetryAsPrimaryAction(t *testing.T) {
	_, claim, _, _, _, action, _ := collectionPresentation(downloadStats{done: 2, failed: 1}, 3)
	if claim != "Failed" || action != "Retry" {
		t.Fatalf("presentation = claim %q action %q", claim, action)
	}
}

func TestSavedContextStartsAtClickedDuplicate(t *testing.T) {
	jobs := []model.DownloadEntry{
		{ID: "older", VideoID: "same", Status: model.DownloadStatusCompleted},
		{ID: "newer", VideoID: "same", Status: model.DownloadStatusCompleted},
	}
	items := []downloadItem{{job: &jobs[0]}, {job: &jobs[1]}}
	tracks, start := savedDownloadContext(items, "newer")
	if len(tracks) != 2 || start != 1 {
		t.Fatalf("context = %#v start = %d", tracks, start)
	}
}
