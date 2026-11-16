package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/store"
)

// The derived playlists' library cards carry a cover from their own songs: the first
// song's cover, or the four-cover mosaic once four distinct covers exist.
func TestSmartPlaylistCardsCarryCovers(t *testing.T) {
	root := t.TempDir()
	st, err := store.Open(store.Dirs{
		Data: filepath.Join(root, "data"), Cache: filepath.Join(root, "cache"),
		Config: filepath.Join(root, "config"), State: filepath.Join(root, "state"), Runtime: filepath.Join(root, "run"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := &App{Store: st}
	play := func(id, cover string) {
		t.Helper()
		track := model.Track{VideoID: id, Title: id}
		if cover != "" {
			track.Thumbnail = &cover
		}
		if err := st.RecordPlay(track); err != nil {
			t.Fatal(err)
		}
	}

	if cards, err := smartPlaylistCards(context.Background(), app); err != nil || len(cards) != 0 {
		t.Fatalf("no plays: %d cards, %v", len(cards), err)
	}

	play("one", "https://img.test/one.jpg")
	play("two", "")
	cards, err := smartPlaylistCards(context.Background(), app)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) == 0 {
		t.Fatal("recent plays made no derived playlist")
	}
	for _, card := range cards {
		if card.Thumbnail == nil || *card.Thumbnail != "https://img.test/one.jpg" {
			t.Fatalf("%s cover = %v, want its one song cover", card.ID, card.Thumbnail)
		}
	}

	for _, id := range []string{"three", "four", "five"} {
		play(id, "https://img.test/"+id+".jpg")
	}
	cards, err = smartPlaylistCards(context.Background(), app)
	if err != nil {
		t.Fatal(err)
	}
	for _, card := range cards {
		if card.Thumbnail == nil || !strings.HasPrefix(*card.Thumbnail, model.CollageThumbnailPrefix) {
			t.Fatalf("%s cover = %v, want the four-cover mosaic", card.ID, card.Thumbnail)
		}
	}
	page, err := smartPlaylist(context.Background(), app, model.RecentlyPlayedPlaylistID)
	if err != nil || page.Cover == nil || !strings.HasPrefix(*page.Cover, model.CollageThumbnailPrefix) {
		t.Fatalf("the playlist page's cover = %v (%v)", page.Cover, err)
	}
}
