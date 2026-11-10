package state

import (
	"encoding/json"
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

func TestSoundCloudQuietReadoptionShowsNoToast(t *testing.T) {
	store := NewEmpty()
	if err := store.Apply(protocol.EventSoundCloudAuth, json.RawMessage(`{"state":"restored","name":"Sam"}`)); err != nil {
		t.Fatal(err)
	}
	got := store.Snapshot()
	if !got.SoundCloud.SignedIn || got.SoundCloud.Name == nil || *got.SoundCloud.Name != "Sam" {
		t.Fatalf("restored state = %#v", got.SoundCloud)
	}
	if got.Toast != nil {
		t.Fatalf("quiet re-adoption toasted %q", got.Toast.Message)
	}
}

func TestSoundCloudStaleTokenSignsOutWithoutToastOrEndingAFlow(t *testing.T) {
	name := "Sam"
	store := New(protocol.Snapshot{SoundCloud: model.SoundCloudStatus{SignedIn: true, Name: &name}})
	store.BeginAuthFlow(model.ProviderSoundCloud)
	if err := store.Apply(protocol.EventSoundCloudAuth, json.RawMessage(`{"state":"stale"}`)); err != nil {
		t.Fatal(err)
	}
	got := store.Snapshot()
	if got.SoundCloud.SignedIn || got.SoundCloud.Name != nil {
		t.Fatalf("stale state = %#v", got.SoundCloud)
	}
	if got.Toast != nil {
		t.Fatalf("stale token toasted %q", got.Toast.Message)
	}
	if !got.AuthFlows[model.ProviderSoundCloud].Waiting {
		t.Fatal("a stale token ended a running sign-in flow")
	}
}

func TestSoundCloudSessionExpiredShowsDaemonReason(t *testing.T) {
	store := New(protocol.Snapshot{SoundCloud: model.SoundCloudStatus{SignedIn: true}})
	reason := "Your SoundCloud session expired. Sign in again to see your playlists and likes."
	payload, err := json.Marshal(model.SoundCloudAuthEvent{State: "session_expired", Message: &reason})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Apply(protocol.EventSoundCloudAuth, payload); err != nil {
		t.Fatal(err)
	}
	got := store.Snapshot()
	if got.SoundCloud.SignedIn || got.SoundCloud.Error != reason {
		t.Fatalf("expired state = %#v", got.SoundCloud)
	}
	if got.Toast == nil || got.Toast.Message != reason {
		t.Fatalf("expired toast = %#v, want the daemon's reason", got.Toast)
	}
}
