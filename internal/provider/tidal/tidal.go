package tidal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
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

const tidalAPI = "https://api.tidal.com/v1"

// Options configures a TIDAL provider.
type Options struct {
	KV         baseprovider.KV
	Bus        bus.Sink
	Log        *slog.Logger
	HTTPClient *http.Client
	APIBase    string
	UserAgent  string
}

// Provider implements TIDAL browsing, favorites, OAuth, and playback.
type Provider struct {
	kv        baseprovider.KV
	bus       bus.Sink
	log       *slog.Logger
	http      *http.Client
	apiBase   string
	userAgent string

	mu          sync.RWMutex
	credentials Credentials
	auth        *Auth
	signingIn   bool
	epoch       uint64
	refreshLock sync.Mutex
}

// New creates a TIDAL provider and restores credentials and authorization.
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
	if o.APIBase == "" {
		o.APIBase = tidalAPI
	}
	if o.UserAgent == "" {
		o.UserAgent = "Ryotunes"
	}
	p := &Provider{kv: o.KV, bus: o.Bus, log: o.Log, http: o.HTTPClient, apiBase: strings.TrimRight(o.APIBase, "/"), userAgent: o.UserAgent, credentials: defaultCredentials()}
	if raw, ok := o.KV.Get(tidalCredentialsKey); ok && raw != "" {
		var stored storedCredentials
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			return nil, fmt.Errorf("read TIDAL credentials: %w", err)
		}
		p.credentials = credentialsFromStored(stored)
	}
	if raw, ok := o.KV.Get(tidalAuthKey); ok && raw != "" {
		var auth Auth
		if err := json.Unmarshal([]byte(raw), &auth); err != nil {
			return nil, fmt.Errorf("read TIDAL authorization: %w", err)
		}
		if auth.AccessToken != "" {
			p.auth = &auth
		}
	}
	return p, nil
}

// ID returns the TIDAL provider identifier.
func (p *Provider) ID() model.Provider { return model.ProviderTidal }

// Capabilities reports TIDAL features implemented by this provider.
func (p *Provider) Capabilities() baseprovider.Capabilities {
	return baseprovider.Capabilities{Home: true, Search: true, Library: true, Playlists: true, Downloads: true, Account: true}
}

// Home returns personal TIDAL shelves and mixes from the home page endpoint.
func (p *Provider) Home(ctx context.Context, _ string) (model.Feed, error) {
	if !p.signedIn() {
		return model.Feed{Chips: []model.FeedChip{}, Sections: []model.Shelf{}}, nil
	}
	library, err := p.Library(ctx)
	if err != nil {
		return model.Feed{}, err
	}
	sections := make([]model.Shelf, 0, 4)
	if len(library.Playlists) != 0 {
		sections = append(sections, model.Shelf{Title: "Your playlists", Items: library.Playlists})
	}
	if len(library.Liked) != 0 {
		items := make([]model.BrowseItem, 0, len(library.Liked))
		for _, t := range library.Liked {
			items = append(items, model.BrowseItem{Kind: "song", ID: t.VideoID, Title: t.Title, Subtitle: pointerIfText(t.Artists), Thumbnail: t.Thumbnail, Duration: t.Duration, ArtistRuns: t.ArtistRuns, Explicit: t.Explicit})
		}
		sections = append(sections, model.Shelf{Title: "Your tracks", Items: items})
	}
	if len(library.Albums) != 0 {
		sections = append(sections, model.Shelf{Title: "Your albums", Items: library.Albums})
	}
	mixes, err := p.homeMixes(ctx)
	if err != nil {
		return model.Feed{}, err
	}
	if len(mixes) != 0 {
		items := make([]model.BrowseItem, 0, len(mixes))
		for _, m := range mixes {
			items = append(items, mixCard(m))
		}
		sections = append(sections, model.Shelf{Title: "Mixes for you", Items: items})
	}
	return model.Feed{Chips: []model.FeedChip{}, Sections: sections}, nil
}

// HomeMore returns no continuation because TIDAL home is returned as complete shelves.
func (p *Provider) HomeMore(context.Context, string) (model.Feed, error) {
	return model.Feed{Chips: []model.FeedChip{}, Sections: []model.Shelf{}}, nil
}

func (p *Provider) homeMixes(ctx context.Context) ([]mix, error) {
	var raw any
	if err := p.api(ctx, "/pages/home", url.Values{"deviceType": {"BROWSER"}}, &raw); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []mix
	var walk func(any)
	walk = func(v any) {
		switch node := v.(type) {
		case []any:
			for _, child := range node {
				walk(child)
			}
		case map[string]any:
			id, _ := node["id"].(string)
			mixID, _ := node["mixId"].(string)
			if mixID != "" {
				id = mixID
			}
			title, _ := node["title"].(string)
			typ, _ := node["type"].(string)
			if id != "" && title != "" && (mixID != "" || strings.Contains(strings.ToLower(typ), "mix") || strings.Contains(strings.ToLower(id), "mix")) && !seen[id] {
				seen[id] = true
				m := mix{ID: id, Title: title}
				if s, ok := node["subTitle"].(string); ok {
					m.SubTitle = &s
				}
				if image, ok := node["image"].(string); ok {
					m.Image = &image
				}
				out = append(out, m)
			}
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(raw)
	return out, nil
}

// Search searches TIDAL tracks.
func (p *Provider) Search(ctx context.Context, q string, kind model.SearchKind) (model.SearchPage, error) {
	if kind != model.SearchKindAll && kind != model.SearchKindSongs {
		return model.SearchPage{Items: []model.Track{}}, nil
	}
	return p.searchTracks(ctx, q, 0)
}

// SearchMore continues a TIDAL track search.
func (p *Provider) SearchMore(ctx context.Context, token string) (model.SearchPage, error) {
	t, err := decodePageToken(token)
	if err != nil || t.Kind != "tracks" {
		return model.SearchPage{}, fmt.Errorf("invalid TIDAL continuation")
	}
	return p.searchTracks(ctx, t.Query, t.Offset)
}

func (p *Provider) searchTracks(ctx context.Context, q string, offset int) (model.SearchPage, error) {
	var result searchResults
	values := url.Values{"query": {q}, "limit": {"20"}, "offset": {strconv.Itoa(offset)}}
	if err := p.api(ctx, "/search", values, &result); err != nil {
		return model.SearchPage{}, err
	}
	items := make([]model.Track, 0, len(result.Tracks.Items))
	for _, t := range result.Tracks.Items {
		items = append(items, trackModel(t))
	}
	var continuation *string
	if result.Tracks.Total != nil && offset+len(items) < int(*result.Tracks.Total) {
		next := encodePageToken(pageToken{Kind: "tracks", Query: q, Offset: offset + len(items)})
		continuation = &next
	}
	return model.SearchPage{Items: items, Continuation: continuation}, nil
}

// SearchAll searches all TIDAL catalogue kinds.
func (p *Provider) SearchAll(ctx context.Context, q string) (model.SearchAll, error) {
	result, err := p.searchAll(ctx, q, 0)
	if err != nil {
		return model.SearchAll{}, err
	}
	out := model.SearchAll{Top: []model.BrowseItem{}, Songs: []model.BrowseItem{}, Albums: []model.BrowseItem{}, Artists: []model.BrowseItem{}, Playlists: []model.BrowseItem{}}
	for _, t := range result.Tracks.Items {
		out.Songs = append(out.Songs, trackCard(t))
	}
	for _, a := range result.Albums.Items {
		out.Albums = append(out.Albums, albumCard(a))
	}
	for _, a := range result.Artists.Items {
		out.Artists = append(out.Artists, artistCard(a))
	}
	for _, list := range result.Playlists.Items {
		out.Playlists = append(out.Playlists, playlistCard(list))
	}
	return out, nil
}

// SearchAllMore returns no continuation because TIDAL mixed search is one page.
func (p *Provider) SearchAllMore(context.Context, string) (model.SearchAll, error) {
	return model.SearchAll{Top: []model.BrowseItem{}, Songs: []model.BrowseItem{}, Albums: []model.BrowseItem{}, Artists: []model.BrowseItem{}, Playlists: []model.BrowseItem{}}, nil
}

// SearchCards searches one TIDAL card category.
func (p *Provider) SearchCards(ctx context.Context, q string, category string) (model.CardsPage, error) {
	result, err := p.searchAll(ctx, q, 0)
	if err != nil {
		return model.CardsPage{}, err
	}
	items := make([]model.BrowseItem, 0)
	switch category {
	case "songs":
		for _, v := range result.Tracks.Items {
			items = append(items, trackCard(v))
		}
	case "albums":
		for _, v := range result.Albums.Items {
			items = append(items, albumCard(v))
		}
	case "artists":
		for _, v := range result.Artists.Items {
			items = append(items, artistCard(v))
		}
	case "playlists":
		for _, v := range result.Playlists.Items {
			items = append(items, playlistCard(v))
		}
	}
	return model.CardsPage{Items: items}, nil
}

// SearchCardsMore returns no continuation because TIDAL card search is one page.
func (p *Provider) SearchCardsMore(context.Context, string) (model.CardsPage, error) {
	return model.CardsPage{Items: []model.BrowseItem{}}, nil
}

func (p *Provider) searchAll(ctx context.Context, q string, offset int) (searchResults, error) {
	var result searchResults
	err := p.api(ctx, "/search", url.Values{"query": {q}, "limit": {"20"}, "offset": {strconv.Itoa(offset)}}, &result)
	return result, err
}

// Suggest is unsupported by the TIDAL v1 catalogue API.
func (p *Provider) Suggest(context.Context, string) ([]string, error) {
	return nil, baseprovider.ErrUnsupported
}

// Album returns a TIDAL album and all of its tracks.
func (p *Provider) Album(ctx context.Context, id string) (model.AlbumPage, error) {
	n, err := parseNumericID(id, albumPrefix)
	if err != nil {
		return model.AlbumPage{}, err
	}
	var a album
	if err := p.api(ctx, fmt.Sprintf("/albums/%d", n), nil, &a); err != nil {
		return model.AlbumPage{}, err
	}
	tracks, err := p.allTracks(ctx, fmt.Sprintf("/albums/%d/tracks", n))
	if err != nil {
		return model.AlbumPage{}, err
	}
	items := make([]model.Track, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, trackModel(t))
	}
	title := a.Title
	subtitle := "Album"
	if a.ReleaseDate != nil && len(*a.ReleaseDate) >= 4 {
		subtitle += " · " + (*a.ReleaseDate)[:4]
	}
	artistName := artistLine(a.Artists)
	if artistName != "" {
		subtitle = artistName + " · " + subtitle
	}
	return model.AlbumPage{Title: &title, Subtitle: &subtitle, Thumbnail: coverURL(a.Cover, 1280), Items: items}, nil
}

// Artist returns a TIDAL artist page with top tracks and albums.
func (p *Provider) Artist(ctx context.Context, id string) (model.ArtistPage, error) {
	n, err := parseNumericID(id, artistPrefix)
	if err != nil {
		return model.ArtistPage{}, err
	}
	var a artist
	if err := p.api(ctx, fmt.Sprintf("/artists/%d", n), nil, &a); err != nil {
		return model.ArtistPage{}, err
	}
	var top page[track]
	if err := p.api(ctx, fmt.Sprintf("/artists/%d/toptracks", n), url.Values{"limit": {"20"}}, &top); err != nil {
		return model.ArtistPage{}, err
	}
	var albums page[album]
	if err := p.api(ctx, fmt.Sprintf("/artists/%d/albums", n), url.Values{"limit": {"50"}}, &albums); err != nil {
		return model.ArtistPage{}, err
	}
	songs := make([]model.Track, 0, len(top.Items))
	for _, t := range top.Items {
		songs = append(songs, trackModel(t))
	}
	sections := []model.ArtistSection{}
	if len(albums.Items) != 0 {
		cards := make([]model.BrowseItem, 0, len(albums.Items))
		for _, item := range albums.Items {
			cards = append(cards, albumCard(item))
		}
		sections = append(sections, model.ArtistSection{Title: "Albums", Items: cards})
	}
	name := a.Name
	return model.ArtistPage{Name: &name, Thumbnail: coverURL(a.Picture, 1280), ChannelID: id, TopSongs: songs, Sections: sections}, nil
}

// Playlist returns a TIDAL playlist or mix.
func (p *Provider) Playlist(ctx context.Context, id string, _ model.PlaylistSort, _ bool) (model.PlaylistPage, error) {
	if mixID, ok := strings.CutPrefix(id, mixPrefix); ok && mixID != "" {
		return p.Mix(ctx, mixID)
	}
	uuid, err := parseStringID(id, playlistPrefix)
	if err != nil {
		return model.PlaylistPage{}, err
	}
	var list playlist
	if err := p.api(ctx, "/playlists/"+url.PathEscape(uuid), nil, &list); err != nil {
		return model.PlaylistPage{}, err
	}
	tracks, err := p.allTracks(ctx, "/playlists/"+url.PathEscape(uuid)+"/tracks")
	if err != nil {
		return model.PlaylistPage{}, err
	}
	items := make([]model.Track, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, trackModel(t))
	}
	title := list.Title
	var subtitle *string
	if list.TrackCount != nil {
		v := countTracks(uint64(*list.TrackCount))
		subtitle = &v
	}
	cover := list.SquareImage
	if cover == nil {
		cover = list.Image
	}
	return model.PlaylistPage{Title: &title, Subtitle: subtitle, Thumbnail: coverURL(cover, 1280), Description: list.Description, Items: items}, nil
}

// Mix returns a TIDAL mix and all of its tracks.
func (p *Provider) Mix(ctx context.Context, id string) (model.PlaylistPage, error) {
	var metadata mix
	if err := p.api(ctx, "/mixes/"+url.PathEscape(id), nil, &metadata); err != nil {
		return model.PlaylistPage{}, err
	}
	tracks, err := p.allTracks(ctx, "/mixes/"+url.PathEscape(id)+"/items")
	if err != nil {
		return model.PlaylistPage{}, err
	}
	items := make([]model.Track, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, trackModel(t))
	}
	title := metadata.Title
	return model.PlaylistPage{Title: &title, Subtitle: metadata.SubTitle, Thumbnail: coverURL(metadata.Image, 1280), Description: metadata.Description, Items: items}, nil
}

// PlaylistMore returns no continuation because TIDAL playlists are fetched in full.
func (p *Provider) PlaylistMore(context.Context, string) (model.PlaylistTracksPage, error) {
	return model.PlaylistTracksPage{Items: []model.Track{}}, nil
}

func (p *Provider) allTracks(ctx context.Context, path string) ([]track, error) {
	const batch = 100
	var out []track
	for offset := 0; ; offset += batch {
		var current page[track]
		if err := p.api(ctx, path, url.Values{"limit": {strconv.Itoa(batch)}, "offset": {strconv.Itoa(offset)}}, &current); err != nil {
			return nil, err
		}
		out = append(out, current.Items...)
		if len(current.Items) < batch || (current.Total != nil && len(out) >= int(*current.Total)) {
			break
		}
	}
	return out, nil
}

// Library returns TIDAL favorites and user playlists.
func (p *Provider) Library(ctx context.Context) (model.Library, error) {
	auth, err := p.ensureFresh(ctx)
	if err != nil {
		return model.Library{}, err
	}
	out := model.Library{Liked: []model.Track{}, Playlists: []model.BrowseItem{}, Albums: []model.BrowseItem{}, Artists: []model.BrowseItem{}}
	trackFavorites, err := collectPages(func(offset int) (page[favorite[track]], error) {
		var current page[favorite[track]]
		err := p.api(ctx, fmt.Sprintf("/users/%d/favorites/tracks", auth.UserID), url.Values{"limit": {"100"}, "offset": {strconv.Itoa(offset)}}, &current)
		return current, err
	})
	if err != nil {
		return out, err
	}
	for _, item := range trackFavorites {
		out.Liked = append(out.Liked, trackModel(item.Item))
	}
	albumFavorites, err := collectPages(func(offset int) (page[favorite[album]], error) {
		var current page[favorite[album]]
		err := p.api(ctx, fmt.Sprintf("/users/%d/favorites/albums", auth.UserID), url.Values{"limit": {"100"}, "offset": {strconv.Itoa(offset)}}, &current)
		return current, err
	})
	if err != nil {
		return out, err
	}
	for _, item := range albumFavorites {
		out.Albums = append(out.Albums, albumCard(item.Item))
	}
	playlists, err := collectPages(func(offset int) (page[playlist], error) {
		var current page[playlist]
		err := p.api(ctx, fmt.Sprintf("/users/%d/playlists", auth.UserID), url.Values{"limit": {"100"}, "offset": {strconv.Itoa(offset)}}, &current)
		return current, err
	})
	if err != nil {
		return out, err
	}
	for _, item := range playlists {
		out.Playlists = append(out.Playlists, playlistCard(item))
	}
	return out, nil
}

func collectPages[T any](fetch func(offset int) (page[T], error)) ([]T, error) {
	var out []T
	for {
		current, err := fetch(len(out))
		if err != nil {
			return nil, err
		}
		out = append(out, current.Items...)
		if len(current.Items) < 100 || (current.Total != nil && len(out) >= int(*current.Total)) {
			return out, nil
		}
	}
}

// Resolve returns a playable BTS URL or inline DASH data URL.
func (p *Provider) Resolve(ctx context.Context, item model.Track, requested model.Quality) (model.Stream, error) {
	id, err := parseNumericID(item.VideoID, trackPrefix)
	if err != nil {
		return model.Stream{}, err
	}
	auth, err := p.ensureFresh(ctx)
	if err != nil {
		return model.Stream{}, err
	}
	quality := qualityFor(requested, ceilingForEntitlement(auth.Entitlement))
	values := url.Values{"audioquality": {quality.String()}, "playbackmode": {"STREAM"}, "assetpresentation": {"FULL"}, "immersiveaudio": {"false"}}
	var info playbackInfo
	if err := p.api(ctx, fmt.Sprintf("/tracks/%d/playbackinfopostpaywall", id), values, &info); err != nil {
		return model.Stream{}, err
	}
	return streamFromPlaybackInfo(info)
}

// Next returns TIDAL track radio results not already played.
func (p *Provider) Next(ctx context.Context, seed model.Track, played []string) ([]model.Track, error) {
	id, err := parseNumericID(seed.VideoID, trackPrefix)
	if err != nil {
		return nil, err
	}
	var result page[track]
	if err := p.api(ctx, fmt.Sprintf("/tracks/%d/radio", id), url.Values{"limit": {"20"}}, &result); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, id := range played {
		seen[id] = true
	}
	out := make([]model.Track, 0, len(result.Items))
	for _, t := range result.Items {
		mapped := trackModel(t)
		if !seen[mapped.VideoID] {
			out = append(out, mapped)
		}
	}
	return out, nil
}

// Account returns the TIDAL account in the common account shape.
func (p *Provider) Account(context.Context) (model.Account, error) {
	status := p.Status()
	return model.Account{SignedIn: status.SignedIn, Name: status.Name}, nil
}

func (p *Provider) signedIn() bool { p.mu.RLock(); defer p.mu.RUnlock(); return p.auth != nil }

func (p *Provider) api(ctx context.Context, path string, query url.Values, out any) error {
	auth, err := p.ensureFresh(ctx)
	if err != nil {
		return err
	}
	u, err := url.Parse(p.apiBase + path)
	if err != nil {
		return err
	}
	q := u.Query()
	for key, values := range query {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	q.Set("countryCode", auth.CountryCode)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("User-Agent", p.userAgent)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		p.expireAuth()
		return fmt.Errorf("%w: TIDAL authorization expired", baseprovider.ErrSignInRequired)
	}
	if resp.StatusCode == 404 {
		return fmt.Errorf("%w: TIDAL resource", baseprovider.ErrNotFound)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("TIDAL answered %d for %s", resp.StatusCode, path)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *Provider) expireAuth() {
	p.mu.Lock()
	p.auth = nil
	p.epoch++
	p.mu.Unlock()
	_ = p.kv.Delete(tidalAuthKey)
	p.bus.Emit(protocol.EventTidalAuth, model.TidalAuthEvent{State: "expired"})
}

var (
	_ baseprovider.Provider = (*Provider)(nil)
)
