package youtube

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSAPISIDHash(t *testing.T) {
	now := time.Unix(1700000000, 999999999)
	got := SAPISIDHash(now, "cookie-value")
	want := "SAPISIDHASH 1700000000_a512ef68af31da56e69b30f14e2d4a443b11b755"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseVisitorData(t *testing.T) {
	fixture := []byte(")]}'\n[[null,null,[7,\"other\",\"CgtFixtureVisitor\",[\"CgsNestedIgnored\"]]]]")
	got, err := parseVisitorData(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if got != "CgtFixtureVisitor" {
		t.Fatalf("got %q", got)
	}
	for _, invalid := range [][]byte{[]byte("not json"), []byte(`[[null,null,"CgtWrongShape"]]`), []byte(`[[null,null,["visitor"]]]`)} {
		if _, err := parseVisitorData(invalid); err == nil || err.Error() != "visitorData not found in sw.js_data" {
			t.Fatalf("invalid fixture error = %v", err)
		}
	}
}

func TestPlayerJSFunctionLocatorsAndExecution(t *testing.T) {
	data, err := os.ReadFile("testdata/player_excerpt.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	sigName := firstPatternName(js, sigNamePatterns)
	nName := firstPatternName(js, nNamePatterns)
	if sigName != "AB" || nName != "CD" {
		t.Fatalf("names = %q, %q", sigName, nName)
	}
	client := &Client{}
	client.cipher.js = js
	client.cipher.loadedAt = time.Now()
	client.cipher.sigSource = transformSource(js, sigName, "_sig")
	client.cipher.nSource = transformSource(js, nName, "_n")
	got, err := client.runCipher(context.Background(), true, "abc", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "cba" {
		t.Fatalf("signature got %q", got)
	}
	got, err = client.runCipher(context.Background(), false, "n", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "nx" {
		t.Fatalf("n transform got %q", got)
	}
}

func TestBrowseFixtureParsing(t *testing.T) {
	root := map[string]any{"contents": []any{
		map[string]any{"musicResponsiveListItemRenderer": map[string]any{
			"playlistItemData": map[string]any{"videoId": "video1", "playlistSetVideoId": "set1"},
			"flexColumns": []any{
				map[string]any{"musicResponsiveListItemFlexColumnRenderer": map[string]any{"text": map[string]any{"runs": []any{map[string]any{"text": "Song"}}}}},
				map[string]any{"musicResponsiveListItemFlexColumnRenderer": map[string]any{"text": map[string]any{"runs": []any{map[string]any{"text": "Artist", "navigationEndpoint": map[string]any{"browseEndpoint": map[string]any{"browseId": "UCartist"}}}, map[string]any{"text": "3:45"}}}}},
			},
			"thumbnail": map[string]any{"musicThumbnailRenderer": map[string]any{"thumbnail": map[string]any{"thumbnails": []any{map[string]any{"url": "https://img"}}}}},
		}},
		map[string]any{"continuationItemRenderer": map[string]any{"continuationEndpoint": map[string]any{"continuationCommand": map[string]any{"token": "next-token", "continuation": "next-token"}}}},
	}}
	tracks := parseTracks(root)
	if len(tracks) != 1 {
		t.Fatalf("tracks = %#v", tracks)
	}
	if tracks[0].VideoID != "video1" || tracks[0].Title != "Song" || tracks[0].Artists != "Artist" {
		t.Fatalf("track = %#v", tracks[0])
	}
	if tracks[0].Duration == nil || !strings.Contains(*tracks[0].Duration, "3:45") {
		t.Fatalf("duration = %#v", tracks[0].Duration)
	}
	if token := continuation(root); token == nil || *token != "next-token" {
		t.Fatalf("token = %#v", token)
	}
}

func TestSearchAllUsesItemNavigationAndCardShelf(t *testing.T) {
	flex := func(title string, subtitle map[string]any) []any {
		return []any{
			map[string]any{"musicResponsiveListItemFlexColumnRenderer": map[string]any{"text": map[string]any{"runs": []any{map[string]any{"text": title}}}}},
			map[string]any{"musicResponsiveListItemFlexColumnRenderer": map[string]any{"text": map[string]any{"runs": []any{subtitle}}}},
		}
	}
	root := map[string]any{"sectionListRenderer": map[string]any{"contents": []any{
		map[string]any{"musicCardShelfRenderer": map[string]any{
			"title":    map[string]any{"runs": []any{map[string]any{"text": "Radiohead", "navigationEndpoint": map[string]any{"browseEndpoint": map[string]any{"browseId": "UCtop"}}}}},
			"subtitle": map[string]any{"runs": []any{map[string]any{"text": "Artist"}}},
		}},
		map[string]any{"itemSectionRenderer": map[string]any{"contents": []any{
			map[string]any{"musicResponsiveListItemRenderer": map[string]any{
				"playlistItemData": map[string]any{"videoId": "song1"},
				"flexColumns":      flex("Song", map[string]any{"text": "Artist", "navigationEndpoint": map[string]any{"browseEndpoint": map[string]any{"browseId": "UCartist"}}}),
			}},
			map[string]any{"musicResponsiveListItemRenderer": map[string]any{
				"navigationEndpoint": map[string]any{"browseEndpoint": map[string]any{"browseId": "UCother"}},
				"flexColumns":        flex("Other Artist", map[string]any{"text": "Artist"}),
			}},
		}}},
	}}}
	result := searchAllFrom(root)
	if len(result.Top) != 1 || result.Top[0].Kind != "artist" {
		t.Fatalf("top = %#v", result.Top)
	}
	if len(result.Songs) != 1 || result.Songs[0].ID != "song1" {
		t.Fatalf("songs = %#v", result.Songs)
	}
	if len(result.Artists) != 1 || result.Artists[0].ID != "UCother" {
		t.Fatalf("artists = %#v", result.Artists)
	}
}
