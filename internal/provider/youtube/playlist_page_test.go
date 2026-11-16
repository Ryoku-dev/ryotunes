package youtube

import (
	"encoding/json"
	"errors"
	"testing"
)

func browseRoot(t *testing.T, raw string) any {
	t.Helper()
	var root any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		t.Fatal(err)
	}
	return root
}

// An answer with no header, no songs and no continuation (what YouTube sent for Liked music
// at a relaunch) is refused, so the client shows Retry instead of a blank playlist.
func TestPlaylistPageRefusesABlankAnswer(t *testing.T) {
	for name, raw := range map[string]string{
		"nothing":    `{}`,
		"bare shell": `{"responseContext":{"visitorData":"x"},"contents":{"singleColumnBrowseResultsRenderer":{"tabs":[{"tabRenderer":{"content":{"sectionListRenderer":{"contents":[]}}}}]}}}`,
	} {
		page, err := playlistPageOf(browseRoot(t, raw))
		if !errors.Is(err, errEmptyPlaylistPage) {
			t.Errorf("%s: err = %v, page %+v; want the empty-page error", name, err, page)
		}
	}
	if errEmptyPlaylistPage.Error() != "YouTube Music sent an empty page. Try again." {
		t.Fatalf("wording = %q", errEmptyPlaylistPage)
	}
}

// A real playlist with no songs keeps its header and still loads, as does Liked music,
// whose header is its cover alone.
func TestPlaylistPageKeepsARealEmptyPlaylist(t *testing.T) {
	titled := `{"header":{"musicEditablePlaylistDetailHeaderRenderer":{"header":{"musicDetailHeaderRenderer":{"title":{"runs":[{"text":"Road trip"}]}}}}},
		"contents":{"singleColumnBrowseResultsRenderer":{"tabs":[{"tabRenderer":{"content":{"sectionListRenderer":{"contents":[]}}}}]}}}`
	page, err := playlistPageOf(browseRoot(t, titled))
	if err != nil || page.Title == nil || len(page.Items) != 0 {
		t.Fatalf("empty titled playlist: err %v, title %v, %d songs", err, page.Title, len(page.Items))
	}

	coverOnly := `{"header":{"musicImmersiveHeaderRenderer":{"thumbnail":{"musicThumbnailRenderer":{"thumbnail":{"thumbnails":[{"url":"https://img.test/liked-songs.png","width":544,"height":544}]}}}}}}`
	page, err = playlistPageOf(browseRoot(t, coverOnly))
	if err != nil || page.Thumbnail == nil || *page.Thumbnail != "https://img.test/liked-songs.png" {
		t.Fatalf("cover-only header: err %v, thumbnail %v", err, page.Thumbnail)
	}
}
