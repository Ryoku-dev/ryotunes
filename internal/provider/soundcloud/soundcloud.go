package soundcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/bus"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/netx"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	baseprovider "github.com/ryoku-dev/ryotunes/internal/provider"
)

const (
	apiBase             = "https://api-v2.soundcloud.com"
	homeURL             = "https://soundcloud.com/"
	userAgent           = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	soundCloudAuthKey   = "soundcloud_auth"
	fallbackClientID    = "3uJIGBRwdofKn6QKzONvDxUM1Vs4bTv9"
	expiryMarginSeconds = int64(60)
)

var (
	scriptRE        = regexp.MustCompile(`src="(https://a-v2\.sndcdn\.com/assets/[^"]+\.js)"`)
	clientIDRE      = regexp.MustCompile(`client_id\s*[:=]\s*"([A-Za-z0-9]{32})"`)
	validClientIDRE = regexp.MustCompile(`^[A-Za-z0-9]{32}$`)
)

// Options configures a SoundCloud provider.
type Options struct {
	DataDir    string
	KV         baseprovider.KV
	Bus        bus.Sink
	Log        *slog.Logger
	HTTPClient *http.Client
	Cookies    CookieSource
	APIBase    string
	HomeURL    string
}

// Provider implements the SoundCloud catalogue and account operations.
type Provider struct {
	dataDir string
	kv      baseprovider.KV
	bus     bus.Sink
	log     *slog.Logger
	http    *http.Client
	cookies CookieSource
	apiBase string
	homeURL string

	mu          sync.RWMutex
	clientID    string
	auth        *Auth
	signingIn   bool
	epoch       uint64
	refreshLock sync.Mutex
}

// New creates a SoundCloud provider and restores persisted authorization.
func New(o Options) (*Provider, error) {
	if o.KV == nil {
		o.KV = baseprovider.MapKV{}
	}
	if o.Bus == nil {
		o.Bus = bus.Discard
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.HTTPClient == nil {
		o.HTTPClient = netx.Client(60 * time.Second)
	}
	if o.Cookies == nil {
		o.Cookies = systemCookies{}
	}
	if o.APIBase == "" {
		o.APIBase = apiBase
	}
	if o.HomeURL == "" {
		o.HomeURL = homeURL
	}
	p := &Provider{
		dataDir: o.DataDir, kv: o.KV, bus: o.Bus, log: o.Log, http: o.HTTPClient,
		cookies: o.Cookies, apiBase: strings.TrimRight(o.APIBase, "/"), homeURL: o.HomeURL,
	}
	if raw, ok := o.KV.Get(soundCloudAuthKey); ok && raw != "" {
		var a Auth
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			return nil, fmt.Errorf("read SoundCloud authorization: %w", err)
		}
		if strings.TrimSpace(a.AccessToken) != "" {
			p.auth = &a
			if validClientIDRE.MatchString(a.ClientID) {
				p.clientID = a.ClientID
			}
		}
	}
	return p, nil
}

// ID returns the SoundCloud provider identifier.
func (p *Provider) ID() model.Provider { return model.ProviderSoundCloud }

// Capabilities reports SoundCloud features implemented by this provider.
func (p *Provider) Capabilities() baseprovider.Capabilities {
	return baseprovider.Capabilities{
		Home: true, Search: true, Library: true, Playlists: true,
		Rate: true, Downloads: true, Account: true, Waveform: true,
	}
}

// Home returns personal shelves followed by SoundCloud discover shelves.
func (p *Provider) Home(ctx context.Context, _ string) (model.Feed, error) {
	sections := make([]model.Shelf, 0)
	if p.signedIn() {
		if library, err := p.Library(ctx); err == nil {
			personalPlaylists := make([]model.BrowseItem, 0, len(library.Playlists)+len(library.Albums))
			personalPlaylists = append(personalPlaylists, library.Playlists...)
			personalPlaylists = append(personalPlaylists, library.Albums...)
			if len(personalPlaylists) != 0 {
				sections = append(sections, model.Shelf{Title: "Your playlists", Items: personalPlaylists})
			}
			if len(library.Liked) != 0 {
				items := make([]model.BrowseItem, 0, len(library.Liked))
				for _, item := range library.Liked {
					items = append(items, trackToCard(item))
				}
				sections = append(sections, model.Shelf{Title: "Liked tracks", Items: items})
			}
			if len(library.Artists) != 0 {
				sections = append(sections, model.Shelf{Title: "Following", Items: library.Artists})
			}
		} else if errors.Is(err, baseprovider.ErrSignInRequired) {
			return model.Feed{}, err
		}
	}
	var raw collection[struct {
		URN   string                      `json:"urn"`
		Title string                      `json:"title"`
		Items collection[json.RawMessage] `json:"items"`
	}]
	if err := p.get(ctx, "/mixed-selections", url.Values{"limit": {"20"}}, false, &raw); err != nil {
		return model.Feed{}, err
	}
	for _, selection := range raw.Collection {
		items := make([]model.BrowseItem, 0, len(selection.Items.Collection))
		for _, item := range selection.Items.Collection {
			var kind struct {
				Kind string `json:"kind"`
			}
			if json.Unmarshal(item, &kind) != nil {
				continue
			}
			switch kind.Kind {
			case "playlist":
				var v playlist
				if json.Unmarshal(item, &v) == nil {
					items = append(items, playlistCard(v))
				}
			case "system-playlist":
				var v systemPlaylist
				if json.Unmarshal(item, &v) == nil {
					items = append(items, systemCard(v))
				}
			}
		}
		if len(items) != 0 {
			sections = append(sections, model.Shelf{Title: selection.Title, Items: items})
		}
	}
	return model.Feed{Chips: []model.FeedChip{}, Sections: sections}, nil
}

// HomeMore returns no continuation because SoundCloud discover is not paged.
func (p *Provider) HomeMore(context.Context, string) (model.Feed, error) {
	return model.Feed{Chips: []model.FeedChip{}, Sections: []model.Shelf{}}, nil
}

// Search searches SoundCloud tracks.
func (p *Provider) Search(ctx context.Context, q string, kind model.SearchKind) (model.SearchPage, error) {
	if kind != model.SearchKindAll && kind != model.SearchKindSongs {
		return model.SearchPage{Items: []model.Track{}}, nil
	}
	return p.searchTracks(ctx, q, 0)
}

// SearchMore continues a SoundCloud track search.
func (p *Provider) SearchMore(ctx context.Context, token string) (model.SearchPage, error) {
	var t searchToken
	if err := decodeToken(token, &t); err != nil || t.Kind != "tracks" {
		return model.SearchPage{}, fmt.Errorf("invalid SoundCloud search continuation")
	}
	return p.searchTracks(ctx, t.Query, t.Offset)
}

func (p *Provider) searchTracks(ctx context.Context, q string, offset int) (model.SearchPage, error) {
	var page collection[track]
	values := url.Values{"q": {q}, "limit": {"20"}, "offset": {strconv.Itoa(offset)}}
	if err := p.get(ctx, "/search/tracks", values, false, &page); err != nil {
		return model.SearchPage{}, err
	}
	items := make([]model.Track, 0, len(page.Collection))
	for _, item := range page.Collection {
		items = append(items, trackModel(item))
	}
	var next *string
	if page.NextHref != nil && len(items) != 0 {
		v := encodeToken(searchToken{Kind: "tracks", Query: q, Offset: offset + len(items)})
		next = &v
	}
	return model.SearchPage{Items: items, Continuation: next}, nil
}

// SearchAll searches all SoundCloud catalogue kinds.
func (p *Provider) SearchAll(ctx context.Context, q string) (model.SearchAll, error) {
	tracks, users, playlists, err := p.mixedSearch(ctx, q)
	if err != nil {
		return model.SearchAll{}, err
	}
	out := model.SearchAll{Top: []model.BrowseItem{}, Songs: []model.BrowseItem{}, Albums: []model.BrowseItem{}, Artists: []model.BrowseItem{}, Playlists: []model.BrowseItem{}}
	for _, t := range tracks {
		out.Songs = append(out.Songs, trackCard(t))
	}
	for _, u := range users {
		out.Artists = append(out.Artists, userCard(u))
	}
	for _, pl := range playlists {
		if pl.IsAlbum {
			out.Albums = append(out.Albums, playlistCard(pl))
		} else {
			out.Playlists = append(out.Playlists, playlistCard(pl))
		}
	}
	return out, nil
}

// SearchAllMore returns no continuation because mixed SoundCloud search is one page.
func (p *Provider) SearchAllMore(context.Context, string) (model.SearchAll, error) {
	return model.SearchAll{Top: []model.BrowseItem{}, Songs: []model.BrowseItem{}, Albums: []model.BrowseItem{}, Artists: []model.BrowseItem{}, Playlists: []model.BrowseItem{}}, nil
}

// SearchCards searches one SoundCloud card category.
func (p *Provider) SearchCards(ctx context.Context, q string, category string) (model.CardsPage, error) {
	tracks, users, playlists, err := p.mixedSearch(ctx, q)
	if err != nil {
		return model.CardsPage{}, err
	}
	items := make([]model.BrowseItem, 0)
	switch category {
	case "songs":
		for _, t := range tracks {
			items = append(items, trackCard(t))
		}
	case "artists":
		for _, u := range users {
			items = append(items, userCard(u))
		}
	case "albums":
		for _, pl := range playlists {
			if pl.IsAlbum {
				items = append(items, playlistCard(pl))
			}
		}
	case "playlists":
		for _, pl := range playlists {
			if !pl.IsAlbum {
				items = append(items, playlistCard(pl))
			}
		}
	}
	return model.CardsPage{Items: items}, nil
}

// SearchCardsMore returns no continuation because SoundCloud card search is one page.
func (p *Provider) SearchCardsMore(context.Context, string) (model.CardsPage, error) {
	return model.CardsPage{Items: []model.BrowseItem{}}, nil
}

func (p *Provider) mixedSearch(ctx context.Context, q string) ([]track, []user, []playlist, error) {
	var page collection[json.RawMessage]
	if err := p.get(ctx, "/search", url.Values{"q": {q}, "limit": {"20"}}, false, &page); err != nil {
		return nil, nil, nil, err
	}
	var tracks []track
	var users []user
	var playlists []playlist
	for _, item := range page.Collection {
		var kind struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(item, &kind) != nil {
			continue
		}
		switch kind.Kind {
		case "track":
			var v track
			if json.Unmarshal(item, &v) == nil {
				tracks = append(tracks, v)
			}
		case "user":
			var v user
			if json.Unmarshal(item, &v) == nil {
				users = append(users, v)
			}
		case "playlist":
			var v playlist
			if json.Unmarshal(item, &v) == nil {
				playlists = append(playlists, v)
			}
		}
	}
	return tracks, users, playlists, nil
}

// Suggest is unsupported by the SoundCloud API v2 client.
func (p *Provider) Suggest(context.Context, string) ([]string, error) {
	return nil, baseprovider.ErrUnsupported
}

// Album returns a SoundCloud album, represented by a playlist.
func (p *Provider) Album(ctx context.Context, id string) (model.AlbumPage, error) {
	n, err := parseNumericID(id, playlistPrefix)
	if err != nil {
		return model.AlbumPage{}, err
	}
	pl, tracks, err := p.fetchPlaylist(ctx, n)
	if err != nil {
		return model.AlbumPage{}, err
	}
	return playlistPage(pl, tracks, p.owns(pl)), nil
}

// Artist returns a SoundCloud user page.
func (p *Provider) Artist(ctx context.Context, id string) (model.ArtistPage, error) {
	n, err := parseNumericID(id, userPrefix)
	if err != nil {
		return model.ArtistPage{}, fmt.Errorf("get_sc_user expects a SoundCloud user id")
	}
	var profile user
	if err := p.get(ctx, fmt.Sprintf("/users/%d", n), nil, false, &profile); err != nil {
		return model.ArtistPage{}, err
	}
	var tracks collection[track]
	if err := p.get(ctx, fmt.Sprintf("/users/%d/tracks", n), url.Values{"limit": {"20"}, "offset": {"0"}}, false, &tracks); err != nil {
		return model.ArtistPage{}, err
	}
	var albums collection[playlist]
	_ = p.get(ctx, fmt.Sprintf("/users/%d/albums", n), url.Values{"limit": {"50"}}, false, &albums)
	var lists collection[playlist]
	_ = p.get(ctx, fmt.Sprintf("/users/%d/playlists_without_albums", n), url.Values{"limit": {"50"}}, false, &lists)
	var likes collection[json.RawMessage]
	_ = p.get(ctx, fmt.Sprintf("/users/%d/likes", n), url.Values{"limit": {"50"}}, false, &likes)
	sections := make([]model.ArtistSection, 0, 3)
	if len(albums.Collection) != 0 {
		items := make([]model.BrowseItem, 0, len(albums.Collection))
		for _, v := range albums.Collection {
			items = append(items, playlistCard(v))
		}
		sections = append(sections, model.ArtistSection{Title: "Albums", Items: items})
	}
	if len(lists.Collection) != 0 {
		items := make([]model.BrowseItem, 0, len(lists.Collection))
		for _, v := range lists.Collection {
			items = append(items, playlistCard(v))
		}
		sections = append(sections, model.ArtistSection{Title: "Playlists", Items: items})
	}
	if len(likes.Collection) != 0 {
		items := make([]model.BrowseItem, 0, len(likes.Collection))
		for _, raw := range likes.Collection {
			var envelope struct {
				Track *track `json:"track"`
			}
			if json.Unmarshal(raw, &envelope) == nil && envelope.Track != nil {
				items = append(items, trackCard(*envelope.Track))
				continue
			}
			var value track
			if json.Unmarshal(raw, &value) == nil && value.ID != 0 {
				items = append(items, trackCard(value))
			}
		}
		if len(items) != 0 {
			sections = append(sections, model.ArtistSection{Title: "Likes", Items: items})
		}
	}
	top := make([]model.Track, 0, len(tracks.Collection))
	for _, v := range tracks.Collection {
		top = append(top, trackModel(v))
	}
	name := profile.Username
	followers := abbreviate(profile.FollowersCount)
	return model.ArtistPage{Name: &name, Thumbnail: artwork(profile.AvatarURL), Description: nonempty(profile.Description), Subscribers: &followers, ChannelID: id, TopSongs: top, Sections: sections}, nil
}

// Playlist returns a SoundCloud playlist or system playlist.
func (p *Provider) Playlist(ctx context.Context, id string, _ model.PlaylistSort, _ bool) (model.PlaylistPage, error) {
	if permalink, ok := strings.CutPrefix(id, systemPrefix); ok && permalink != "" {
		var sp systemPlaylist
		path := "/system-playlists/" + url.PathEscape("soundcloud:system-playlists:"+permalink)
		if err := p.get(ctx, path, nil, false, &sp); err != nil {
			return model.PlaylistPage{}, err
		}
		ids := make([]uint64, 0, len(sp.Tracks))
		for _, t := range sp.Tracks {
			ids = append(ids, t.ID)
		}
		tracks, err := p.hydrate(ctx, sp.Tracks, ids)
		if err != nil {
			return model.PlaylistPage{}, err
		}
		title := sp.Title
		subtitle := "SoundCloud"
		items := make([]model.Track, 0, len(tracks))
		for _, t := range tracks {
			items = append(items, trackModel(t))
		}
		art := artwork(sp.CalculatedArtworkURL)
		if art == nil {
			art = artwork(sp.ArtworkURL)
		}
		return model.PlaylistPage{Title: &title, Subtitle: &subtitle, Thumbnail: art, Description: nonempty(sp.Description), Items: items}, nil
	}
	n, err := parseNumericID(id, playlistPrefix)
	if err != nil {
		return model.PlaylistPage{}, err
	}
	pl, tracks, err := p.fetchPlaylist(ctx, n)
	if err != nil {
		return model.PlaylistPage{}, err
	}
	return playlistPage(pl, tracks, p.owns(pl)), nil
}

// PlaylistMore returns no continuation because SoundCloud playlists are hydrated in full.
func (p *Provider) PlaylistMore(context.Context, string) (model.PlaylistTracksPage, error) {
	return model.PlaylistTracksPage{Items: []model.Track{}}, nil
}

func (p *Provider) fetchPlaylist(ctx context.Context, id uint64) (playlist, []track, error) {
	var pl playlist
	if err := p.get(ctx, fmt.Sprintf("/playlists/%d", id), nil, false, &pl); err != nil {
		return playlist{}, nil, err
	}
	ids := make([]uint64, 0, len(pl.Tracks))
	for _, t := range pl.Tracks {
		ids = append(ids, t.ID)
	}
	tracks, err := p.hydrate(ctx, pl.Tracks, ids)
	return pl, tracks, err
}

func (p *Provider) hydrate(ctx context.Context, existing []track, ids []uint64) ([]track, error) {
	byID := make(map[uint64]track, len(ids))
	missing := make([]uint64, 0)
	for _, t := range existing {
		if t.User != nil {
			byID[t.ID] = t
		} else {
			missing = append(missing, t.ID)
		}
	}
	for chunk := range slices.Chunk(missing, 50) {
		parts := make([]string, 0, len(chunk))
		for _, id := range chunk {
			parts = append(parts, strconv.FormatUint(id, 10))
		}
		var rows []track
		if err := p.get(ctx, "/tracks", url.Values{"ids": {strings.Join(parts, ",")}}, false, &rows); err != nil {
			return nil, err
		}
		for _, t := range rows {
			byID[t.ID] = t
		}
	}
	out := make([]track, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// Library returns the signed-in user's likes, playlists, and followed artists.
func (p *Provider) Library(ctx context.Context) (model.Library, error) {
	me, err := p.me(ctx)
	if err != nil {
		return model.Library{}, err
	}
	var likes collection[json.RawMessage]
	if err := p.get(ctx, fmt.Sprintf("/users/%d/likes", me.ID), url.Values{"limit": {"50"}}, true, &likes); err != nil {
		return model.Library{}, err
	}
	out := model.Library{Liked: []model.Track{}, Playlists: []model.BrowseItem{}, Albums: []model.BrowseItem{}, Artists: []model.BrowseItem{}}
	for _, raw := range likes.Collection {
		var envelope struct {
			Track *track `json:"track"`
		}
		if json.Unmarshal(raw, &envelope) == nil && envelope.Track != nil {
			out.Liked = append(out.Liked, trackModel(*envelope.Track))
			continue
		}
		var t track
		if json.Unmarshal(raw, &t) == nil && t.ID != 0 {
			out.Liked = append(out.Liked, trackModel(t))
		}
	}
	var lists collection[playlist]
	if err := p.get(ctx, fmt.Sprintf("/users/%d/playlists", me.ID), url.Values{"limit": {"50"}}, true, &lists); err != nil {
		return model.Library{}, err
	}
	for _, pl := range lists.Collection {
		if pl.IsAlbum {
			out.Albums = append(out.Albums, playlistCard(pl))
		} else {
			out.Playlists = append(out.Playlists, playlistCard(pl))
		}
	}
	var follows collection[user]
	if err := p.get(ctx, fmt.Sprintf("/users/%d/followings", me.ID), url.Values{"limit": {"50"}}, true, &follows); err == nil {
		for _, u := range follows.Collection {
			out.Artists = append(out.Artists, userCard(u))
		}
	}
	return out, nil
}

// Permalink returns the canonical SoundCloud page URL for a track.
func (p *Provider) Permalink(ctx context.Context, trackID string) (string, error) {
	id, err := parseNumericID(trackID, trackPrefix)
	if err != nil {
		return "", err
	}
	var t track
	if err := p.get(ctx, fmt.Sprintf("/tracks/%d", id), nil, false, &t); err != nil {
		return "", err
	}
	return t.PermalinkURL, nil
}

// Resolve returns a fresh SoundCloud HLS URL for mpv.
func (p *Provider) Resolve(ctx context.Context, m model.Track, _ model.Quality) (model.Stream, error) {
	id, err := parseNumericID(m.VideoID, trackPrefix)
	if err != nil {
		return model.Stream{}, err
	}
	var t track
	if err := p.get(ctx, fmt.Sprintf("/tracks/%d", id), nil, false, &t); err != nil {
		return model.Stream{}, err
	}
	chosen := chooseTranscoding(t.Media.Transcodings)
	if chosen == nil {
		return model.Stream{}, unavailableError(fmt.Sprintf("track %d has no playable HLS transcoding", id))
	}
	q := url.Values{}
	if t.TrackAuthorization != nil {
		q.Set("track_authorization", *t.TrackAuthorization)
	}
	var resolved struct {
		URL string `json:"url"`
	}
	if err := p.getAbsolute(ctx, chosen.URL, q, false, &resolved); err != nil {
		return model.Stream{}, err
	}
	codec := "aac"
	if strings.Contains(chosen.Format.MimeType, "mpeg") {
		codec = "mp3"
	}
	return model.Stream{URL: resolved.URL, UserAgent: userAgent, Codec: codec}, nil
}

func chooseTranscoding(ts []transcoding) *transcoding {
	for _, fragment := range []string{"mp4", "mpeg", ""} {
		for i := range ts {
			if strings.EqualFold(ts[i].Format.Protocol, "hls") && (fragment == "" || strings.Contains(strings.ToLower(ts[i].Format.MimeType), fragment)) {
				return &ts[i]
			}
		}
	}
	return nil
}

// Next returns related SoundCloud tracks not already played.
func (p *Provider) Next(ctx context.Context, seed model.Track, played []string) ([]model.Track, error) {
	id, err := parseNumericID(seed.VideoID, trackPrefix)
	if err != nil {
		return nil, err
	}
	var page collection[track]
	if err := p.get(ctx, fmt.Sprintf("/tracks/%d/related", id), url.Values{"limit": {"20"}}, false, &page); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(played))
	for _, id := range played {
		seen[id] = struct{}{}
	}
	out := make([]model.Track, 0, len(page.Collection))
	for _, t := range page.Collection {
		m := trackModel(t)
		if _, ok := seen[m.VideoID]; !ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// Account returns the SoundCloud account in the common account shape.
func (p *Provider) Account(context.Context) (model.Account, error) {
	status := p.Status()
	return model.Account{SignedIn: status.SignedIn, Name: status.Name}, nil
}

// Rate likes or unlikes a SoundCloud track.
func (p *Provider) Rate(ctx context.Context, trackID string, rating model.Rating) error {
	id, err := parseNumericID(trackID, trackPrefix)
	if err != nil {
		return err
	}
	me, err := p.me(ctx)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/users/%d/track_likes/%d", me.ID, id)
	switch rating {
	case model.RatingLike:
		return p.write(ctx, http.MethodPut, path)
	case model.RatingIndifferent, model.RatingDislike:
		return p.write(ctx, http.MethodDelete, path)
	default:
		return fmt.Errorf("unsupported SoundCloud rating %q", rating)
	}
}

// Waveform fetches and normalizes a SoundCloud waveform to exactly 240 samples.
func (p *Provider) Waveform(ctx context.Context, id string) (model.Waveform, error) {
	n, err := parseNumericID(id, trackPrefix)
	if err != nil {
		return model.Waveform{}, fmt.Errorf("get_waveform expects a SoundCloud track id")
	}
	var t track
	if err := p.get(ctx, fmt.Sprintf("/tracks/%d", n), nil, false, &t); err != nil {
		return model.Waveform{}, err
	}
	if t.WaveformURL == nil {
		return model.Waveform{}, fmt.Errorf("track %d has no waveform", n)
	}
	var wave struct {
		Width, Height int
		Samples       []int `json:"samples"`
	}
	if err := p.getPublic(ctx, *t.WaveformURL, &wave); err != nil {
		return model.Waveform{}, err
	}
	return model.Waveform{Samples: downsample(wave.Samples, wave.Height, 240)}, nil
}

func downsample(samples []int, height, buckets int) []int {
	out := make([]int, buckets)
	if buckets == 0 || len(samples) == 0 {
		return out
	}
	denom := height
	if denom <= 0 {
		for _, sample := range samples {
			if sample > denom {
				denom = sample
			}
		}
	}
	if denom <= 0 {
		denom = 1
	}
	for i := range buckets {
		start := i * len(samples) / buckets
		end := max((i+1)*len(samples)/buckets, start+1)
		end = min(end, len(samples))
		peak := 0
		for _, sample := range samples[start:end] {
			if sample > peak {
				peak = sample
			}
		}
		out[i] = min(100, peak*100/denom)
	}
	return out
}

func trackToCard(t model.Track) model.BrowseItem {
	return model.BrowseItem{Kind: "song", ID: t.VideoID, Title: t.Title, Subtitle: stringPtr(t.Artists), Thumbnail: t.Thumbnail, Duration: t.Duration, ArtistRuns: t.ArtistRuns, PlayCount: t.PlayCount}
}

func (p *Provider) owns(pl playlist) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.auth == nil || pl.User == nil {
		return false
	}
	if p.auth.UserID != 0 {
		return pl.User.ID == p.auth.UserID
	}
	return p.auth.Username != nil && *p.auth.Username != "" && pl.User.Username == *p.auth.Username
}

func (p *Provider) signedIn() bool { p.mu.RLock(); defer p.mu.RUnlock(); return p.auth != nil }

func (p *Provider) me(ctx context.Context) (user, error) {
	if !p.signedIn() {
		return user{}, baseprovider.ErrSignInRequired
	}
	var me user
	if err := p.get(ctx, "/me", nil, true, &me); err != nil {
		return user{}, err
	}
	return me, nil
}

func (p *Provider) get(ctx context.Context, path string, values url.Values, personal bool, out any) error {
	return p.getAbsolute(ctx, p.apiBase+path, values, personal, out)
}

func (p *Provider) getAbsolute(ctx context.Context, endpoint string, values url.Values, personal bool, out any) error {
	if personal {
		if err := p.ensureFresh(ctx); err != nil {
			return err
		}
	}
	clientID, err := p.getClientID(ctx)
	if err != nil {
		return err
	}
	do := func(id string) (*http.Response, error) {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		for key, vals := range values {
			for _, value := range vals {
				q.Add(key, value)
			}
		}
		q.Set("client_id", id)
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		p.mu.RLock()
		auth := p.auth
		if auth != nil {
			req.Header.Set("Authorization", "OAuth "+auth.AccessToken)
		}
		p.mu.RUnlock()
		return p.http.Do(req)
	}
	resp, err := do(clientID)
	if err != nil {
		return err
	}
	if (resp.StatusCode == 401 || resp.StatusCode == 403) && !personal {
		resp.Body.Close()
		clientID, err = p.discoverClientID(ctx)
		if err != nil {
			return err
		}
		resp, err = do(clientID)
		if err != nil {
			return err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return fmt.Errorf("%w: SoundCloud resource", baseprovider.ErrNotFound)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		if personal {
			p.expireAuth("Your SoundCloud session expired. Sign in again to see your playlists and likes.")
			return baseprovider.ErrSignInRequired
		}
		return fmt.Errorf("SoundCloud answered %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("SoundCloud answered %d for %s", resp.StatusCode, endpoint)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *Provider) getPublic(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("SoundCloud answered %d for %s", resp.StatusCode, endpoint)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *Provider) write(ctx context.Context, method, path string) error {
	if err := p.ensureFresh(ctx); err != nil {
		return err
	}
	clientID, err := p.getClientID(ctx)
	if err != nil {
		return err
	}
	u, err := url.Parse(p.apiBase + path)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("client_id", clientID)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	p.mu.RLock()
	auth := p.auth
	if auth != nil {
		req.Header.Set("Authorization", "OAuth "+auth.AccessToken)
	}
	p.mu.RUnlock()
	if auth == nil {
		return baseprovider.ErrSignInRequired
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		p.expireAuth("Your SoundCloud session expired. Sign in again to see your playlists and likes.")
		return baseprovider.ErrSignInRequired
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("SoundCloud answered %d for %s", resp.StatusCode, path)
	}
	return nil
}

func (p *Provider) getClientID(ctx context.Context) (string, error) {
	p.mu.RLock()
	id := p.clientID
	p.mu.RUnlock()
	if id != "" {
		return id, nil
	}
	if p.dataDir != "" {
		raw, err := os.ReadFile(filepath.Join(p.dataDir, "soundcloud", "client_id"))
		trimmed := bytes.TrimSpace(raw)
		if err == nil && validClientIDRE.Match(trimmed) {
			id = string(trimmed)
			p.mu.Lock()
			p.clientID = id
			p.mu.Unlock()
			return id, nil
		}
	}
	return p.discoverClientID(ctx)
}

func (p *Provider) discoverClientID(ctx context.Context) (string, error) {
	var html strings.Builder
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.homeURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, err = io.Copy(&html, resp.Body)
	}
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	matches := scriptRE.FindAllStringSubmatch(html.String(), -1)
	for i := len(matches) - 1; i >= 0; i-- {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, matches[i][1], nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := p.http.Do(req)
		if err != nil {
			continue
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			continue
		}
		match := clientIDRE.FindSubmatch(raw)
		if len(match) != 2 {
			continue
		}
		id := string(match[1])
		p.mu.Lock()
		p.clientID = id
		p.mu.Unlock()
		if p.dataDir != "" {
			dir := filepath.Join(p.dataDir, "soundcloud")
			if os.MkdirAll(dir, 0700) == nil {
				_ = writeCacheFile(dir, "client_id", id)
			}
		}
		return id, nil
	}
	return "", fmt.Errorf("SoundCloud client_id could not be discovered")
}

func writeCacheFile(dir, name, value string) error {
	temp, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.WriteString(value); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, filepath.Join(dir, name))
}

func (p *Provider) expireAuth(message string) {
	p.mu.Lock()
	p.auth = nil
	p.epoch++
	p.mu.Unlock()
	_ = p.kv.Delete(soundCloudAuthKey)
	p.bus.Emit(protocol.EventSoundCloudAuth, model.SoundCloudAuthEvent{State: "expired", Message: &message})
}

type searchToken struct {
	Kind, Query string
	Offset      int
}

func encodeToken(v any) string { raw, _ := json.Marshal(v); return url.QueryEscape(string(raw)) }
func decodeToken(token string, out any) error {
	raw, err := url.QueryUnescape(token)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), out)
}

type unavailableError string

func (e unavailableError) Error() string      { return string(e) }
func (unavailableError) Is(target error) bool { return target == baseprovider.ErrUnavailable }

var (
	_ baseprovider.Provider = (*Provider)(nil)
	_ baseprovider.Rater    = (*Provider)(nil)
)
