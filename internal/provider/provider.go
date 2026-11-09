// Package provider is the contract between the daemon and a music source. A
// provider turns ids into model values and streams; it never touches the
// player, the store or the event bus. The daemon asserts the optional
// interfaces and reads Capabilities to decide which controls exist.
package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Typed failures the daemon and the queue act on. Wrap them with %w so
// callers can errors.Is them; the handler picks the user-visible sentence.
var (
	// ErrSignInRequired means the account must sign in before this works;
	// the queue halts on it instead of skipping.
	ErrSignInRequired = errors.New("sign in required")
	// ErrUnavailable means the item exists but cannot be played or fetched
	// (region, DRM, removed); the queue skips it.
	ErrUnavailable = errors.New("unavailable")
	// ErrBotGated is YouTube refusing anonymous playback for a missing or
	// stale visitorData; the daemon heals the identity once and retries.
	ErrBotGated = errors.New("bot gated")
	// ErrNotFound means the id does not exist at the provider.
	ErrNotFound = errors.New("not found")
	// ErrUnsupported means this provider has no such feature.
	ErrUnsupported = errors.New("unsupported by provider")
	// ErrNotPremium means a Spotify account without Premium.
	ErrNotPremium = errors.New("the account has no Spotify Premium")
)

// Capabilities tells the client which controls a source supports.
type Capabilities struct {
	Home, Search, Suggest       bool
	Library, Playlists, Editing bool
	Rate, SaveAlbum             bool
	Radio, Lyrics, Downloads    bool
	Account                     bool
	Waveform                    bool
}

// Provider is one music source.
type Provider interface {
	ID() model.Provider
	Capabilities() Capabilities
	Home(ctx context.Context, params string) (model.Feed, error)
	HomeMore(ctx context.Context, token string) (model.Feed, error)
	Search(ctx context.Context, q string, kind model.SearchKind) (model.SearchPage, error)
	SearchMore(ctx context.Context, token string) (model.SearchPage, error)
	SearchAll(ctx context.Context, q string) (model.SearchAll, error)
	SearchAllMore(ctx context.Context, token string) (model.SearchAll, error)
	SearchCards(ctx context.Context, q string, category string) (model.CardsPage, error)
	SearchCardsMore(ctx context.Context, token string) (model.CardsPage, error)
	Suggest(ctx context.Context, q string) ([]string, error)
	Album(ctx context.Context, id string) (model.AlbumPage, error)
	Artist(ctx context.Context, id string) (model.ArtistPage, error)
	Playlist(ctx context.Context, id string, sort model.PlaylistSort, desc bool) (model.PlaylistPage, error)
	PlaylistMore(ctx context.Context, token string) (model.PlaylistTracksPage, error)
	Library(ctx context.Context) (model.Library, error)
	// Resolve returns what mpv should play for the track.
	Resolve(ctx context.Context, track model.Track, q model.Quality) (model.Stream, error)
	// Next returns continuation tracks after seed, excluding ids in played.
	Next(ctx context.Context, seed model.Track, played []string) ([]model.Track, error)
	Account(ctx context.Context) (model.Account, error)
}

// Rater likes, dislikes or clears a rating.
type Rater interface {
	Rate(ctx context.Context, trackID string, rating model.Rating) error
}

// AlbumSaver saves or removes an album from the library.
type AlbumSaver interface {
	SetAlbumSaved(ctx context.Context, albumID string, saved bool) error
}

// ArtistSubscriber follows or unfollows an artist channel.
type ArtistSubscriber interface {
	SetArtistSubscribed(ctx context.Context, channelID string, subscribed bool) error
}

// PlaylistEditor edits account playlists.
type PlaylistEditor interface {
	CreatePlaylist(ctx context.Context, title string) (string, error)
	EditPlaylist(ctx context.Context, id string, name, description *string, public *bool) error
	DeletePlaylist(ctx context.Context, id string) error
	AddToPlaylist(ctx context.Context, playlistID, trackID string) error
	RemoveFromPlaylist(ctx context.Context, playlistID, trackID, setVideoID string) error
}

// Lyricist fetches provider lyrics for one of its tracks.
type Lyricist interface {
	Lyrics(ctx context.Context, track model.Track) (*model.Lyrics, error)
}

// Radio starts a station from a seed.
type Radio interface {
	StartRadio(ctx context.Context, kind, id string) ([]model.Track, string, error)
}

// Grid serves the "more" browse grids behind shelf titles.
type Grid interface {
	BrowseGrid(ctx context.Context, id, params string) (model.CardsPage, error)
}

// Registry holds the configured providers by id.
type Registry struct {
	byID map[model.Provider]Provider
}

// NewRegistry registers providers in order; a duplicate id panics because it
// is a wiring bug, not a runtime condition.
func NewRegistry(ps ...Provider) *Registry {
	r := &Registry{byID: make(map[model.Provider]Provider, len(ps))}
	for _, p := range ps {
		if _, dup := r.byID[p.ID()]; dup {
			panic(fmt.Sprintf("provider %q registered twice", p.ID()))
		}
		r.byID[p.ID()] = p
	}
	return r
}

// Get returns the provider for id.
func (r *Registry) Get(id model.Provider) (Provider, bool) {
	p, ok := r.byID[id]
	return p, ok
}

// ForTrack returns the provider that owns the track id.
func (r *Registry) ForTrack(trackID string) (Provider, bool) {
	return r.Get(model.ProviderOf(trackID))
}

// All returns every provider.
func (r *Registry) All() []Provider {
	out := make([]Provider, 0, len(r.byID))
	for _, p := range r.byID {
		out = append(out, p)
	}
	return out
}

// KV is the small persistence a provider may need (visitorData, tokens,
// cookies). The store implements it over the settings table; tests use a map.
type KV interface {
	Get(key string) (string, bool)
	Set(key, value string) error
	Delete(key string) error
}

// MapKV is an in-memory KV for tests.
type MapKV map[string]string

// Get implements KV.
func (m MapKV) Get(key string) (string, bool) { v, ok := m[key]; return v, ok }

// Set implements KV.
func (m MapKV) Set(key, value string) error { m[key] = value; return nil }

// Delete implements KV.
func (m MapKV) Delete(key string) error { delete(m, key); return nil }
