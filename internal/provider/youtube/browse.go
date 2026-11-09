package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/provider"
)

const (
	filterSong     = "EgWKAQIIAWoKEAkQBRAKEAMQBA%3D%3D"
	filterAlbum    = "EgWKAQIYAWoKEAkQChAFEAMQBA%3D%3D"
	filterArtist   = "EgWKAQIgAWoKEAkQChAFEAMQBA%3D%3D"
	filterPlaylist = "EgeKAQQoAEABagoQAxAEEAoQCRAF"
)

func (c *Client) browse(ctx context.Context, client, id, params string) (map[string]any, error) {
	body := map[string]any{}
	if id != "" {
		body["browseId"] = id
	}
	if params != "" {
		body["params"] = params
	}
	return c.do(ctx, client, "browse", body, true)
}

func (c *Client) browseMore(ctx context.Context, client, token string) (map[string]any, error) {
	encoded := url.QueryEscape(token)
	return c.do(ctx, client, "browse?ctoken="+encoded+"&continuation="+encoded+"&type=next", map[string]any{}, true)
}

func (c *Client) search(ctx context.Context, query, params string) (map[string]any, error) {
	body := map[string]any{"query": query}
	if params != "" {
		body["params"] = params
	}
	return c.do(ctx, "WEB_REMIX", "search", body, true)
}

func (c *Client) searchMore(ctx context.Context, token string) (map[string]any, error) {
	encoded := url.QueryEscape(token)
	return c.do(ctx, "WEB_REMIX", "search?ctoken="+encoded+"&continuation="+encoded+"&type=next", map[string]any{}, true)
}

// Home returns YouTube Music home shelves, optionally filtered by a chip params token.
func (c *Client) Home(ctx context.Context, params string) (model.Feed, error) {
	root, err := c.browse(ctx, "WEB_REMIX", "FEmusic_home", params)
	if err != nil {
		return model.Feed{}, err
	}
	return feedFrom(root), nil
}

// HomeMore returns the next home shelf continuation.
func (c *Client) HomeMore(ctx context.Context, token string) (model.Feed, error) {
	root, err := c.browseMore(ctx, "WEB_REMIX", token)
	if err != nil {
		return model.Feed{}, err
	}
	return feedFrom(root), nil
}

func feedFrom(root any) model.Feed {
	feed := model.Feed{Chips: parseChips(root), Continuation: continuation(root)}
	for _, shelf := range parseShelves(root) {
		items := shelf.Items[:0]
		for _, item := range shelf.Items {
			if !item.IsVideo {
				items = append(items, item)
			}
		}
		if len(items) != 0 {
			shelf.Items = items
			feed.Sections = append(feed.Sections, shelf)
		}
	}
	return feed
}

// Search searches tracks using YouTube's exact filter params.
func (c *Client) Search(ctx context.Context, query string, kind model.SearchKind) (model.SearchPage, error) {
	params := filterSong
	switch kind {
	case model.SearchKindSongs, model.SearchKindAll:
	case model.SearchKindAlbums:
		params = filterAlbum
	case model.SearchKindArtists:
		params = filterArtist
	case model.SearchKindPlaylists:
		params = filterPlaylist
	}
	root, err := c.search(ctx, query, params)
	if err != nil {
		return model.SearchPage{}, err
	}
	return trackPage(root), nil
}

// SearchMore continues a track search.
func (c *Client) SearchMore(ctx context.Context, token string) (model.SearchPage, error) {
	root, err := c.searchMore(ctx, token)
	if err != nil {
		return model.SearchPage{}, err
	}
	return trackPage(root), nil
}

func trackPage(root any) model.SearchPage {
	page := model.SearchPage{Continuation: continuation(root)}
	for _, track := range parseTracks(root) {
		if !track.IsVideo {
			page.Items = append(page.Items, track)
		}
	}
	return page
}

// SearchAll searches and returns categorized result cards.
func (c *Client) SearchAll(ctx context.Context, query string) (model.SearchAll, error) {
	root, err := c.search(ctx, query, "")
	if err != nil {
		return model.SearchAll{}, err
	}
	return searchAllFrom(root), nil
}

// SearchAllMore continues a categorized search.
func (c *Client) SearchAllMore(ctx context.Context, token string) (model.SearchAll, error) {
	root, err := c.searchMore(ctx, token)
	if err != nil {
		return model.SearchAll{}, err
	}
	return searchAllFrom(root), nil
}

func searchAllFrom(root any) model.SearchAll {
	result := model.SearchAll{Top: parseTopItems(root), Continuation: continuation(root)}
	for _, shelf := range parseShelves(root) {
		items := make([]model.BrowseItem, 0, len(shelf.Items))
		for _, item := range shelf.Items {
			if !item.IsVideo {
				items = append(items, item)
			}
		}
		switch searchShelfCategory(shelf) {
		case "top":
			result.Top = append(result.Top, items...)
		case "songs":
			result.Songs = append(result.Songs, items...)
		case "albums":
			result.Albums = append(result.Albums, items...)
		case "artists":
			result.Artists = append(result.Artists, items...)
		case "playlists":
			result.Playlists = append(result.Playlists, items...)
		}
	}
	needSongs, needAlbums := len(result.Songs) == 0, len(result.Albums) == 0
	needArtists, needPlaylists := len(result.Artists) == 0, len(result.Playlists) == 0
	for _, item := range parseSearchRows(root) {
		if item.IsVideo {
			continue
		}
		switch item.Kind {
		case "song":
			if needSongs {
				result.Songs = append(result.Songs, item)
			}
		case "album":
			if needAlbums {
				result.Albums = append(result.Albums, item)
			}
		case "artist":
			if needArtists {
				result.Artists = append(result.Artists, item)
			}
		case "playlist":
			if needPlaylists {
				result.Playlists = append(result.Playlists, item)
			}
		}
	}
	return result
}
func searchShelfCategory(shelf model.Shelf) string {
	title := strings.ToLower(shelf.Title)
	switch {
	case strings.Contains(title, "top result"):
		return "top"
	case strings.Contains(title, "song"):
		return "songs"
	case strings.Contains(title, "album"):
		return "albums"
	case strings.Contains(title, "artist"):
		return "artists"
	case strings.Contains(title, "playlist"):
		return "playlists"
	}
	if shelf.MoreParams == nil {
		return ""
	}
	params := *shelf.MoreParams
	decoded, _ := url.QueryUnescape(params)
	for category, encoded := range map[string]string{
		"songs":     filterSong,
		"albums":    filterAlbum,
		"artists":   filterArtist,
		"playlists": filterPlaylist,
	} {
		want, _ := url.QueryUnescape(encoded)
		if params == encoded || decoded == want {
			return category
		}
	}
	return ""
}

// SearchCards returns album, artist, or playlist cards.
func (c *Client) SearchCards(ctx context.Context, query, category string) (model.CardsPage, error) {
	params, err := searchCategoryParams(category)
	if err != nil {
		return model.CardsPage{}, err
	}
	root, err := c.search(ctx, query, params)
	if err != nil {
		return model.CardsPage{}, err
	}
	return cardsPage(root), nil
}

// SearchCardsMore continues a card search.
func (c *Client) SearchCardsMore(ctx context.Context, token string) (model.CardsPage, error) {
	root, err := c.searchMore(ctx, token)
	if err != nil {
		return model.CardsPage{}, err
	}
	return cardsPage(root), nil
}

func searchCategoryParams(category string) (string, error) {
	switch category {
	case "albums":
		return filterAlbum, nil
	case "artists":
		return filterArtist, nil
	case "playlists":
		return filterPlaylist, nil
	default:
		return "", fmt.Errorf("unknown search category: %s", category)
	}
}

func cardsPage(root any) model.CardsPage {
	page := model.CardsPage{Continuation: continuation(root)}
	for _, item := range parseBrowseItems(root) {
		if !item.IsVideo {
			page.Items = append(page.Items, item)
		}
	}
	return page
}

// Suggest returns YouTube Music query completions.
func (c *Client) Suggest(ctx context.Context, query string) ([]string, error) {
	root, err := c.do(ctx, "WEB_REMIX", "music/get_search_suggestions", map[string]any{"input": query}, true)
	if err != nil {
		return nil, err
	}
	return parseSuggestions(root), nil
}

// Album returns an album detail page.
func (c *Client) Album(ctx context.Context, id string) (model.AlbumPage, error) {
	root, err := c.browse(ctx, "WEB_REMIX", id, "")
	if err != nil {
		return model.AlbumPage{}, err
	}
	page := parsePlaylistPage(root)
	audioPlaylistID := ""
	walkMaps(root, func(m map[string]any) {
		if audioPlaylistID == "" {
			if candidate := firstString(m, "playlistId"); strings.HasPrefix(candidate, "OLAK5uy_") {
				audioPlaylistID = candidate
			}
		}
	})
	hasVideo := false
	for _, track := range page.Items {
		hasVideo = hasVideo || track.IsVideo
	}
	if hasVideo && audioPlaylistID != "" {
		if audioRoot, fetchErr := c.browse(ctx, "WEB_REMIX", "VL"+audioPlaylistID, ""); fetchErr == nil {
			audio := parsePlaylistPage(audioRoot)
			if len(audio.Items) == len(page.Items) {
				for i := range page.Items {
					if page.Items[i].IsVideo {
						page.Items[i].VideoID = audio.Items[i].VideoID
						if audio.Items[i].Duration != nil {
							page.Items[i].Duration = audio.Items[i].Duration
						}
						page.Items[i].IsVideo = false
					}
				}
			}
		}
	}
	return page, nil
}

// Artist returns an artist header, top tracks, and card sections.
func (c *Client) Artist(ctx context.Context, id string) (model.ArtistPage, error) {
	root, err := c.browse(ctx, "WEB_REMIX", id, "")
	if err != nil {
		return model.ArtistPage{}, err
	}
	page := parseArtistPage(root)
	if page.ChannelID == "" {
		page.ChannelID = id
	}
	filtered := page.TopSongs[:0]
	for _, track := range page.TopSongs {
		if !track.IsVideo {
			filtered = append(filtered, track)
		}
	}
	page.TopSongs = filtered
	return page, nil
}

// Playlist returns a playlist page in the requested server-side order.
func (c *Client) Playlist(ctx context.Context, id string, sortOrder model.PlaylistSort, desc bool) (model.PlaylistPage, error) {
	root, err := c.browse(ctx, "WEB_REMIX", id, playlistSortParams(sortOrder, desc))
	if err != nil {
		return model.PlaylistPage{}, err
	}
	return parsePlaylistPage(root), nil
}

// PlaylistMore continues a playlist track shelf.
func (c *Client) PlaylistMore(ctx context.Context, token string) (model.PlaylistTracksPage, error) {
	root, err := c.browseMore(ctx, "WEB_REMIX", token)
	if err != nil {
		return model.PlaylistTracksPage{}, err
	}
	return model.PlaylistTracksPage{Items: parseTracks(root), Continuation: continuation(root)}, nil
}

func playlistSortParams(sortOrder model.PlaylistSort, desc bool) string {
	switch sortOrder {
	case model.PlaylistSortNewest:
		if desc {
			return "2ggECAEQAw=="
		}
		return "2ggECAIQAw=="
	case model.PlaylistSortOldest:
		if desc {
			return "2ggECAIQAw=="
		}
		return "2ggECAEQAw=="
	case model.PlaylistSortTop:
		if desc {
			return "2ggECAEQBA=="
		}
		return "2ggECAIQBA=="
	case model.PlaylistSortTitle:
		if desc {
			return "2ggECAIQBQ=="
		}
		return "2ggECAEQBQ=="
	case model.PlaylistSortArtist:
		if desc {
			return "2ggECAIQBg=="
		}
		return "2ggECAEQBg=="
	case model.PlaylistSortAlbum:
		if desc {
			return "2ggECAIQBw=="
		}
		return "2ggECAEQBw=="
	default:
		return "2ggA"
	}
}

// BrowseGrid returns the flat card grid behind a shelf's More action.
func (c *Client) BrowseGrid(ctx context.Context, id, params string) (model.CardsPage, error) {
	root, err := c.browse(ctx, "WEB_REMIX", id, params)
	if err != nil {
		return model.CardsPage{}, err
	}
	return cardsPage(root), nil
}

// Library returns liked tracks and all three account card grids.
func (c *Client) Library(ctx context.Context) (model.Library, error) {
	c.mu.RLock()
	signedIn := c.cookie != ""
	c.mu.RUnlock()
	if !signedIn {
		return model.Library{}, provider.ErrSignInRequired
	}
	likedRoot, err := c.browse(ctx, "WEB_REMIX", "VLLM", "")
	if err != nil {
		return model.Library{}, err
	}
	result := model.Library{Liked: parseTracks(likedRoot)}
	for _, target := range []struct {
		id  string
		dst *[]model.BrowseItem
	}{{"FEmusic_liked_playlists", &result.Playlists}, {"FEmusic_liked_albums", &result.Albums}, {"FEmusic_library_corpus_track_artists", &result.Artists}} {
		items, err := c.libraryGrid(ctx, target.id)
		if err != nil {
			if len(*target.dst) == 0 {
				return model.Library{}, err
			}
			break
		}
		*target.dst = items
	}
	return result, nil
}

func (c *Client) libraryGrid(ctx context.Context, id string) ([]model.BrowseItem, error) {
	root, err := c.browse(ctx, "WEB_REMIX", id, "")
	if err != nil {
		return nil, err
	}
	items := parseBrowseItems(root)
	token := continuation(root)
	seen := make(map[string]bool)
	for range 20 {
		if token == nil || *token == "" || seen[*token] {
			break
		}
		seen[*token] = true
		page, err := c.browseMore(ctx, "WEB_REMIX", *token)
		if err != nil {
			break
		}
		more := parseBrowseItems(page)
		if len(more) == 0 {
			break
		}
		items = append(items, more...)
		token = continuation(page)
	}
	return items, nil
}

// Next returns audio-only autoplay rows after seed, excluding already played IDs.
func (c *Client) Next(ctx context.Context, seed model.Track, played []string) ([]model.Track, error) {
	root, err := c.next(ctx, seed.VideoID, "RDAMVM"+seed.VideoID)
	if err != nil {
		return nil, err
	}
	excluded := make(map[string]bool, len(played)+1)
	excluded[seed.VideoID] = true
	for _, id := range played {
		excluded[id] = true
	}
	var result []model.Track
	for _, track := range parseTracks(root) {
		if (!track.IsVideo || track.VideoID == seed.VideoID) && !excluded[track.VideoID] {
			result = append(result, track)
		}
	}
	return result, nil
}

func (c *Client) next(ctx context.Context, videoID, playlistID string) (map[string]any, error) {
	body := map[string]any{"isAudioOnly": true}
	if videoID != "" {
		body["videoId"] = videoID
	}
	if playlistID != "" {
		body["playlistId"] = playlistID
	}
	return c.do(ctx, "WEB_REMIX", "next", body, true)
}

// StartRadio starts a track, artist, or mood radio and returns its continuation.
func (c *Client) StartRadio(ctx context.Context, kind, id string) ([]model.Track, string, error) {
	videoID, playlistID := "", ""
	if kind == "track" || kind == "song" {
		videoID = id
	} else if kind == "artist" && strings.HasPrefix(id, "UC") {
		artist, err := c.Artist(ctx, id)
		if err != nil {
			return nil, "", err
		}
		if artist.RadioPlaylistID == nil {
			return nil, "", provider.ErrNotFound
		}
		playlistID = strings.TrimPrefix(*artist.RadioPlaylistID, "VL")
	} else {
		playlistID = strings.TrimPrefix(id, "VL")
	}
	root, err := c.next(ctx, videoID, playlistID)
	if err != nil {
		return nil, "", err
	}
	items := parseTracks(root)
	filtered := items[:0]
	for _, track := range items {
		if !track.IsVideo || track.VideoID == videoID {
			filtered = append(filtered, track)
		}
	}
	token := ""
	if value := continuation(root); value != nil {
		token = *value
	}
	return filtered, token, nil
}

// Lyrics resolves timed lyrics first and falls back to plain text.
func (c *Client) Lyrics(ctx context.Context, track model.Track) (*model.Lyrics, error) {
	nextRoot, err := c.next(ctx, track.VideoID, "")
	if err != nil {
		return nil, err
	}
	browseID := findString(nextRoot, "lyricsBrowseId")
	if browseID == "" {
		walkMaps(nextRoot, func(m map[string]any) {
			if browseID == "" {
				if id := firstString(m, "browseId"); strings.HasPrefix(id, "MPLY") {
					browseID = id
				}
			}
		})
	}
	if browseID == "" {
		return nil, provider.ErrNotFound
	}
	timed, timedErr := c.browse(ctx, "IOS_MUSIC", browseID, "")
	if timedErr == nil {
		if lines := parseTimedLyrics(timed); len(lines) != 0 {
			return &model.Lyrics{Source: "YouTube Music", Synced: true, Lines: lines}, nil
		}
	}
	plain, err := c.browse(ctx, "WEB_REMIX", browseID, "")
	if err != nil {
		if timedErr != nil {
			return nil, errors.Join(timedErr, err)
		}
		return nil, err
	}
	lines := parsePlainLyrics(plain)
	if len(lines) == 0 {
		return nil, provider.ErrNotFound
	}
	return &model.Lyrics{Source: "YouTube Music", Lines: lines}, nil
}
