package state

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func TestPositionInterpolationAndPause(t *testing.T) {
	base := time.Unix(1000, 0)
	store := New(protocol.Snapshot{Playback: model.PlaybackSnapshot{Duration: 20, Paused: false}})
	if err := store.ApplyAt(protocol.EventPosition, json.RawMessage(`{"position":4}`), base); err != nil {
		t.Fatal(err)
	}
	if got := store.PositionAt(base.Add(1500 * time.Millisecond)); math.Abs(got-5.5) > 0.001 {
		t.Fatalf("interpolated position = %v, want 5.5", got)
	}
	if err := store.ApplyAt(protocol.EventPlaybackState, json.RawMessage(`"paused"`), base.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := store.PositionAt(base.Add(9 * time.Second)); math.Abs(got-6) > 0.001 {
		t.Fatalf("paused position = %v, want 6", got)
	}
	if err := store.ApplyAt(protocol.EventPosition, json.RawMessage(`{"position":99}`), base.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := store.PositionAt(base.Add(10 * time.Second)); got != 20 {
		t.Fatalf("clamped position = %v, want 20", got)
	}
	if err := store.ApplyAt(protocol.EventPosition, json.RawMessage(`{"position":"not-a-number"}`), base.Add(11*time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := store.PositionAt(base.Add(11 * time.Second)); got != 0 {
		t.Fatalf("malformed position = %v, want 0", got)
	}
}

func TestSpotifyRestoreEventWinsOpeningSnapshot(t *testing.T) {
	store := NewEmpty()
	if err := store.Apply(protocol.EventSpotifyAuth, json.RawMessage(`{"state":"restored"}`)); err != nil {
		t.Fatal(err)
	}
	store.ApplySnapshot(protocol.Snapshot{Spotify: model.SpotifyStatus{SignedIn: false, Stored: true}})
	got := store.Snapshot().Spotify
	if !got.SignedIn || !got.Stored || got.Premium == nil || !*got.Premium {
		t.Fatalf("restore was overwritten by opening snapshot: %#v", got)
	}

	store.ApplySnapshot(protocol.Snapshot{Spotify: model.SpotifyStatus{SignedIn: false, Stored: false}})
	got = store.Snapshot().Spotify
	if got.SignedIn || got.Stored {
		t.Fatalf("later reconnect snapshot did not become authoritative: %#v", got)
	}
}

func TestEveryProtocolEventHasReducer(t *testing.T) {
	payloads := map[string]string{
		protocol.EventPosition: `{"position":0}`, protocol.EventDuration: `{"duration":0}`,
		protocol.EventPlaybackState: `"paused"`, protocol.EventPlaybackError: `{"message":"x"}`,
		protocol.EventPlaybackNotice: `{"message":"x"}`, protocol.EventNowPlaying: `null`,
		protocol.EventQueueChanged: `{}`, protocol.EventQueueIndex: `{}`, protocol.EventRating: `{}`,
		protocol.EventStopAfterCurrent: `false`, protocol.EventVolume: `0`, protocol.EventAudioFx: `{}`,
		protocol.EventShow: `null`, protocol.EventAuthChanged: `{"signedIn":false}`,
		protocol.EventSpotifyAuth: `{}`, protocol.EventSoundCloudAuth: `{}`, protocol.EventTidalAuth: `{}`,
		protocol.EventProviderChanged: `{"provider":"youtube"}`, protocol.EventLibraryChanged: `{"id":"x"}`,
		protocol.EventLocalChanged: `{"removed":[]}`, protocol.EventPersonalChanged: `{}`,
		protocol.EventLyricsChanged: `null`, protocol.EventCoverError: `{"message":"x"}`,
		protocol.EventLastfmState: `{}`, protocol.EventLTState: `{}`, protocol.EventLTNotice: `"x"`,
		protocol.EventDownloadsChanged: `{}`,
	}
	store := NewEmpty()
	for _, event := range protocol.Events {
		payload, exists := payloads[event.Name]
		if !exists {
			t.Fatalf("test payload missing for %q", event.Name)
		}
		if err := store.Apply(event.Name, json.RawMessage(payload)); err != nil {
			t.Fatalf("event %q is not reduced: %v", event.Name, err)
		}
	}
}
