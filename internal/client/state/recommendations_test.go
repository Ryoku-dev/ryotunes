package state

import (
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestPickedForYouFixture(t *testing.T) {
	now := time.UnixMilli(1000 * 24 * 60 * 60 * 1000)
	feed := model.Feed{Sections: []model.Shelf{{Title: "Fixture", Items: []model.BrowseItem{
		recommendationSong("A", "UCx"),
		recommendationSong("B", "UCx"),
		recommendationSong("C", "UCy"),
		recommendationSong("spotify:track:foreign", "spotify:artist:x"),
		recommendationSong("A", "UCx"),
	}}}}
	personal := EmptyPersonal()
	personal.Recent["A"] = RecentItem{BrowseItem: model.BrowseItem{ID: "A", Kind: "song", Title: "A"}, At: now.Add(-24 * time.Hour).UnixMilli()}

	got := PickedForYouAt(feed, personal, now)
	if len(got) != 3 {
		t.Fatalf("result length = %d, want 3", len(got))
	}
	if got[0].ID != "B" || got[1].ID != "C" || got[2].ID != "A" {
		t.Fatalf("fatigue/diversity order = %q, %q, %q", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestPickedForYouCapsOutput(t *testing.T) {
	items := make([]model.BrowseItem, 20)
	for i := range items {
		items[i] = recommendationSong(string(rune('a'+i)), "UC"+string(rune('a'+i)))
	}
	got := PickedForYouAt(model.Feed{Sections: []model.Shelf{{Items: items}}}, EmptyPersonal(), time.Unix(0, 0))
	if len(got) != recommendationOutMax {
		t.Fatalf("result length = %d, want %d", len(got), recommendationOutMax)
	}
}

func recommendationSong(id, artistID string) model.BrowseItem {
	thumbnail := "art"
	return model.BrowseItem{
		Kind: "song", ID: id, Title: id, Subtitle: new(artistID), Thumbnail: &thumbnail,
		ArtistRuns: []model.Artist{{Text: artistID, ID: new(artistID)}},
	}
}
