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

func TestPositionAtUsesPlaybackSpeedAndSeekPreview(t *testing.T) {
	base := time.Unix(2000, 0)
	for _, test := range []struct {
		speed float64
		want  float64
	}{
		{speed: .5, want: 5},
		{speed: 1, want: 6},
		{speed: 1.5, want: 7},
	} {
		store := New(protocol.Snapshot{
			Playback: model.PlaybackSnapshot{Duration: 30, Paused: false},
			AudioFx:  model.AudioFx{Speed: test.speed},
		})
		if err := store.ApplyAt(protocol.EventPosition, json.RawMessage(`{"position":4}`), base); err != nil {
			t.Fatal(err)
		}
		if got := store.PositionAt(base.Add(2 * time.Second)); math.Abs(got-test.want) > .001 {
			t.Errorf("speed %.1f position = %v, want %v", test.speed, got, test.want)
		}
		preview := 19.0
		store.SetSeekDrag(&preview)
		if got := store.PositionAt(base.Add(20 * time.Second)); got != preview {
			t.Errorf("speed %.1f seek preview = %v, want %v", test.speed, got, preview)
		}
	}
}

func TestPositionConfirmationSuppressesInvalidationButJumpDoesNot(t *testing.T) {
	base := time.Unix(3000, 0)
	store := New(protocol.Snapshot{
		Playback: model.PlaybackSnapshot{Duration: 100},
		AudioFx:  model.AudioFx{Speed: 1},
	})
	if err := store.ApplyAt(protocol.EventPosition, json.RawMessage(`{"position":20}`), base); err != nil {
		t.Fatal(err)
	}

	invalidations := 0
	unsubscribe := store.Subscribe(func() { invalidations++ })
	defer unsubscribe()

	if err := store.ApplyAt(protocol.EventPosition, json.RawMessage(`{"position":20.35}`), base.Add(250*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if invalidations != 0 {
		t.Fatalf("confirming sample invalidated %d times, want 0", invalidations)
	}
	if got := store.Snapshot().Playback.Position; got != 20.35 {
		t.Fatalf("confirming sample was not stored: %v", got)
	}

	if err := store.ApplyAt(protocol.EventPosition, json.RawMessage(`{"position":25}`), base.Add(500*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if invalidations != 1 {
		t.Fatalf("position jump invalidated %d times, want 1", invalidations)
	}
}

func TestSnapshotCallerMutationCannotCorruptStore(t *testing.T) {
	artistID := "artist"
	store := New(protocol.Snapshot{
		Queue: model.QueueSnapshot{Items: []model.Track{{
			VideoID: "track", Title: "Original",
			ArtistRuns: []model.Artist{{Text: "Artist", ID: &artistID}},
		}}},
		Settings: model.SettingsSnapshot{"quality": "AUTO"},
	})
	store.SetAuthFlow(model.ProviderSpotify, "https://example.invalid/auth", "")

	damaged := store.Snapshot()
	store.SetSetting("revision", "next")
	damaged.Settings["quality"] = "BROKEN"
	damaged.Queue.Items[0].Title = "Changed"
	damaged.Queue.Items[0].ArtistRuns[0].Text = "Changed"
	damaged.AuthFlows[model.ProviderSpotify] = AuthFlow{}

	next := store.Snapshot()
	if next.Settings["quality"] != "AUTO" {
		t.Fatalf("settings changed through snapshot: %q", next.Settings["quality"])
	}
	if next.Queue.Items[0].Title != "Original" || next.Queue.Items[0].ArtistRuns[0].Text != "Artist" {
		t.Fatalf("queue changed through snapshot: %#v", next.Queue.Items[0])
	}
	if flow := next.AuthFlows[model.ProviderSpotify]; flow.URL != "https://example.invalid/auth" {
		t.Fatalf("auth flow changed through snapshot: %#v", flow)
	}
}

func TestAuthFlowURLClaimedOncePerProvider(t *testing.T) {
	store := NewEmpty()
	url := "https://example.invalid/authorize"
	store.SetAuthFlow(model.ProviderSpotify, url, "")
	if !store.ClaimAuthURL(model.ProviderSpotify, url) {
		t.Fatal("first URL claim was rejected")
	}
	if store.ClaimAuthURL(model.ProviderSpotify, url) {
		t.Fatal("repeated URL claim was accepted")
	}
	store.CancelAuthFlow(model.ProviderSpotify)
	if store.ClaimAuthURL(model.ProviderSpotify, url) {
		t.Fatal("cancelled flow reopened a repeated URL")
	}
	next := url + "?next=1"
	if !store.ClaimAuthURL(model.ProviderSpotify, next) {
		t.Fatal("new URL claim was rejected")
	}
}

func TestClientPreferencesEffectsDefaultOnAndRoundTrip(t *testing.T) {
	var missing ClientPreferences
	if err := json.Unmarshal([]byte(`{"zoomPercent":100}`), &missing); err != nil {
		t.Fatal(err)
	}
	if !missing.VisualizerOn() || !missing.WaveOn() {
		t.Fatal("missing effect preferences must default on")
	}
	off := false
	input := ClientPreferences{Visualizer: &off, WaveProgress: &off}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output ClientPreferences
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	if output.VisualizerOn() || output.WaveOn() {
		t.Fatalf("explicit off did not round trip: %s", raw)
	}
}

func TestPositionEventUpdatesRawPositionDuringSeekHold(t *testing.T) {
	base := time.Unix(1000, 0)
	store := New(protocol.Snapshot{Playback: model.PlaybackSnapshot{Duration: 20, Paused: true}})
	held := 12.0
	store.SetSeekDrag(&held)

	if err := store.ApplyAt(protocol.EventPosition, json.RawMessage(`{"position":9}`), base); err != nil {
		t.Fatal(err)
	}
	if got := store.PositionAt(base); got != held {
		t.Fatalf("shown position = %v, want held position %v", got, held)
	}
	if got := store.Snapshot().Playback.Position; got != 9 {
		t.Fatalf("raw position = %v, want daemon position 9", got)
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
		protocol.EventEqualizer: `{"enabled":true,"preamp":-3,"bands":[]}`,
		protocol.EventShow:      `null`, protocol.EventAuthChanged: `{"signedIn":false}`,
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
