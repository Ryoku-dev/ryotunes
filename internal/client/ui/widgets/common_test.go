package widgets

import (
	"testing"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestIsLikedCollectionAliases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id       string
		provider model.Provider
		want     bool
	}{
		{id: "VLLM", provider: model.ProviderYouTube, want: true},
		{id: "LM", provider: model.ProviderYouTube, want: true},
		{id: "spotify:playlist:liked", provider: model.ProviderSpotify, want: true},
		{id: "liked", provider: model.ProviderSpotify, want: true},
		{id: model.LocalLikedPlaylistID, provider: model.ProviderLocal, want: true},
		{id: "LOCAL:liked", provider: model.ProviderLocal, want: true},
		{id: "RYOTUNES_LIKED", provider: model.ProviderYouTube, want: true},
		{id: "liked", provider: model.ProviderYouTube, want: false},
		{id: "my-liked-mix", provider: model.ProviderSpotify, want: false},
	}
	for _, test := range cases {
		item := model.BrowseItem{Kind: "playlist", ID: test.id}
		if got := IsLikedCollection(item, test.provider); got != test.want {
			t.Errorf("IsLikedCollection(%q, %q) = %t, want %t", test.id, test.provider, got, test.want)
		}
	}
}
