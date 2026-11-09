package act

import (
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func stringPointer(value string) *string { return &value }

func TestTrackFromCardSubtitleFacts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		subtitle   string
		duration   *string
		artistRuns []model.Artist
		artists    string
		wantTime   string
	}{
		{name: "youtube song", subtitle: "Linkin Park • Song • 2:39", artists: "Linkin Park", wantTime: "2:39"},
		{name: "facts only", subtitle: "Song • 2:39", artists: "", wantTime: "2:39"},
		{name: "multiple artists", subtitle: "Alice • Bob • Video • 2024 • 1.2M plays • 1:02:03", artists: "Alice, Bob", wantTime: "1:02:03"},
		{name: "view and song counts", subtitle: "Low Arc • 3.4K views • 12 songs • EP • 2023", artists: "Low Arc"},
		{name: "artist runs win", subtitle: "Wrong subtitle • Song • 4:00", artistRuns: []model.Artist{{Text: "Kite Assembly"}, {Text: " Mica "}}, artists: "Kite Assembly, Mica", wantTime: "4:00"},
		{name: "card duration wins", subtitle: "North Register • Song • 2:39", duration: stringPointer("3:15"), artists: "North Register", wantTime: "3:15"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			card := model.BrowseItem{ID: "video", Title: "Track", Subtitle: &test.subtitle, Duration: test.duration, ArtistRuns: test.artistRuns}
			track := TrackFromCard(card)
			if track.Artists != test.artists {
				t.Errorf("Artists = %q, want %q", track.Artists, test.artists)
			}
			gotDuration := ""
			if track.Duration != nil {
				gotDuration = *track.Duration
			}
			if gotDuration != test.wantTime {
				t.Errorf("Duration = %q, want %q", gotDuration, test.wantTime)
			}
		})
	}
}

func TestTrackFromCardPreservesIdentity(t *testing.T) {
	t.Parallel()
	thumbnail := "cover"
	card := model.BrowseItem{
		ID: "spotify:track:1", Title: "Measured light", Thumbnail: &thumbnail,
		PlayCount: stringPointer("1.2M"), IsVideo: true, IsUpload: true, Explicit: true,
	}
	track := TrackFromCard(card)
	if track.VideoID != card.ID || track.Title != card.Title || track.Thumbnail != card.Thumbnail || track.PlayCount != card.PlayCount || !track.IsVideo || !track.IsUpload || !track.Explicit {
		t.Fatalf("identity fields were not preserved: %#v", track)
	}
}
