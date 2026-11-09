package mpris

import (
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestMetadataForMapsMPRISFields(t *testing.T) {
	thumbnail := "file:///tmp/cover.jpg"
	album := "The Album"
	playback := model.PlaybackSnapshot{
		Now: &model.PlaybackTrack{
			VideoID:    "abcdefghijk",
			Title:      "The Song",
			Artists:    "Fallback Artist",
			ArtistRuns: []model.Artist{{Text: "Artist One"}, {Text: "Artist Two"}},
			Thumbnail:  &thumbnail,
		},
		Duration: 123.456,
	}
	queue := model.QueueSnapshot{
		Items:        []model.QueueItem{{VideoID: "abcdefghijk", Album: &album}},
		CurrentIndex: 0,
	}

	got, trackID := metadataFor(playback, queue)
	if trackID == noTrackPath() {
		t.Fatal("track id must identify the current track")
	}
	if value := got["xesam:title"].Value(); value != "The Song" {
		t.Fatalf("title = %#v", value)
	}
	artists, ok := got["xesam:artist"].Value().([]string)
	if !ok || len(artists) != 2 || artists[0] != "Artist One" || artists[1] != "Artist Two" {
		t.Fatalf("artists = %#v", got["xesam:artist"].Value())
	}
	if value := got["xesam:album"].Value(); value != album {
		t.Fatalf("album = %#v", value)
	}
	if value := got["mpris:artUrl"].Value(); value != thumbnail {
		t.Fatalf("art URL = %#v", value)
	}
	if value := got["mpris:length"].Value(); value != int64(123456000) {
		t.Fatalf("length = %#v", value)
	}
	if value := got["mpris:trackid"].Value(); value != trackID {
		t.Fatalf("track id metadata = %#v, want %#v", value, trackID)
	}
}

func TestMetadataForNoTrackIsEmpty(t *testing.T) {
	metadata, trackID := metadataFor(model.PlaybackSnapshot{}, model.QueueSnapshot{})
	if len(metadata) != 0 {
		t.Fatalf("metadata = %#v", metadata)
	}
	if trackID != noTrackPath() {
		t.Fatalf("track id = %q", trackID)
	}
}

func TestMetadataForConvertsLocalArtworkToFileURL(t *testing.T) {
	thumbnail := "/home/user/Music/cover art.jpg"
	metadata, _ := metadataFor(model.PlaybackSnapshot{
		Now: &model.PlaybackTrack{VideoID: "LOCAL:/home/user/Music/song.flac", Title: "Song", Thumbnail: &thumbnail},
	}, model.QueueSnapshot{})
	if value := metadata["mpris:artUrl"].Value(); value != "file:///home/user/Music/cover%20art.jpg" {
		t.Fatalf("art URL = %#v", value)
	}
}
