package spotify

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const madeForYouPage = "0JQ5DAt0tbjZptfcdMSKl3"

type pfImage struct {
	Height int    `json:"height"`
	URL    string `json:"url"`
}
type pfArtwork struct {
	Sources []pfImage `json:"sources"`
}
type pfArtistRef struct {
	URI     string `json:"uri"`
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
}
type pfArtists struct {
	Items []pfArtistRef `json:"items"`
}
type pfTrack struct {
	URI         string    `json:"uri"`
	Name        string    `json:"name"`
	Artists     pfArtists `json:"artists"`
	Playability struct {
		Playable bool `json:"playable"`
	} `json:"playability"`
	ContentRating struct {
		Label string `json:"label"`
	} `json:"contentRating"`
	DiscNumber  int `json:"discNumber"`
	TrackNumber int `json:"trackNumber"`
	Duration    struct {
		Milliseconds uint64 `json:"totalMilliseconds"`
	} `json:"duration"`
	Playcount string `json:"playcount"`
}
type pfAlbum struct {
	URI   string `json:"uri"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"`
	Type  string `json:"type"`
	Date  struct {
		ISO       string `json:"isoString"`
		Precision string `json:"precision"`
		Year      int    `json:"year"`
		Month     *int   `json:"month"`
		Day       *int   `json:"day"`
	} `json:"date"`
	Artists   pfArtists `json:"artists"`
	Cover     pfArtwork `json:"coverArt"`
	Copyright struct {
		Items []struct {
			Text string `json:"text"`
		} `json:"items"`
	} `json:"copyright"`
	Tracks struct {
		Items []struct {
			Track pfTrack `json:"track"`
		} `json:"items"`
		Total int `json:"totalCount"`
	} `json:"tracksV2"`
}

type pfBrowseEntity struct {
	Type   string `json:"__typename"`
	URI    string `json:"uri"`
	Name   string `json:"name"`
	Images struct {
		Items []pfArtwork `json:"items"`
	} `json:"images"`
	Cover   pfArtwork `json:"coverArt"`
	Artists pfArtists `json:"artists"`
	Owner   struct {
		Data *struct {
			Name     string `json:"name"`
			Username string `json:"username"`
		} `json:"data"`
	} `json:"ownerV2"`
	Data *struct {
		Card *struct {
			Title *struct {
				Label string `json:"transformedLabel"`
			} `json:"title"`
			Artwork pfArtwork `json:"artwork"`
		} `json:"cardRepresentation"`
	} `json:"data"`
}
type pfBrowseItem struct {
	URI     string `json:"uri"`
	Content *struct {
		Data *pfBrowseEntity `json:"data"`
	} `json:"content"`
}
type pfBrowseSection struct {
	Data struct {
		Title *struct {
			Label string `json:"transformedLabel"`
		} `json:"title"`
	} `json:"data"`
	Items struct {
		Items []pfBrowseItem `json:"items"`
	} `json:"sectionItems"`
}
type pfBrowseContainer struct {
	Header *struct {
		Title *struct {
			Label string `json:"transformedLabel"`
		} `json:"title"`
	} `json:"header"`
	Sections struct {
		Items []pfBrowseSection `json:"items"`
	} `json:"sections"`
}
type pfSearchHit struct {
	Data *pfBrowseEntity `json:"data"`
}
type pfSearchData struct {
	Search *struct {
		Albums struct {
			Items []pfSearchHit `json:"items"`
		} `json:"albumsV2"`
		Playlists struct {
			Items []pfSearchHit `json:"items"`
		} `json:"playlists"`
	} `json:"searchV2"`
}

// Genres returns Spotify's top-level browse cards.
func (p *Spotify) Genres(ctx context.Context) ([]model.BrowseItem, error) {
	sess, err := p.session(ctx)
	if err != nil {
		return nil, err
	}
	var data struct {
		Browse *pfBrowseContainer `json:"browseStart"`
	}
	variables := map[string]any{
		"pagePagination":           map[string]int{"offset": 0, "limit": 10},
		"sectionPagination":        map[string]int{"offset": 0, "limit": 99},
		"browseEndUserIntegration": "INTEGRATION_WEB_PLAYER",
	}
	if err := p.pathfinderQuery(ctx, sess, "browseAll", variables, &data); err != nil {
		return nil, mapProviderError(err)
	}
	if data.Browse == nil {
		return nil, fmt.Errorf("browseAll Pathfinder response has no browse")
	}
	var cards []model.BrowseItem
	for _, section := range data.Browse.Sections.Items {
		for _, item := range section.Items.Items {
			if card, ok := browseCard(item); ok && card.Kind == "browse" {
				cards = append(cards, card)
			}
		}
	}
	return cards, nil
}

// Home implements provider.Provider.
func (p *Spotify) Home(ctx context.Context, params string) (model.Feed, error) {
	id := madeForYouPage
	if strings.TrimSpace(params) != "" {
		id = strings.TrimSpace(params)
	}
	return p.browsePage(ctx, id)
}

// HomeMore implements provider.Provider; Spotify browse pages are fetched in full.
func (p *Spotify) HomeMore(context.Context, string) (model.Feed, error) { return model.Feed{}, nil }

// BrowseGrid implements provider.Grid.
func (p *Spotify) BrowseGrid(ctx context.Context, id, _ string) (model.CardsPage, error) {
	feed, err := p.browsePage(ctx, id)
	if err != nil {
		return model.CardsPage{}, err
	}
	var items []model.BrowseItem
	for _, shelf := range feed.Sections {
		items = append(items, shelf.Items...)
	}
	return model.CardsPage{Items: items}, nil
}

func (p *Spotify) browsePage(ctx context.Context, id string) (model.Feed, error) {
	sess, err := p.session(ctx)
	if err != nil {
		return model.Feed{}, err
	}
	var data struct {
		Browse *pfBrowseContainer `json:"browse"`
	}
	variables := map[string]any{"uri": "spotify:page:" + id, "pagePagination": map[string]int{"offset": 0, "limit": 20}, "sectionPagination": map[string]int{"offset": 0, "limit": 10}, "browseEndUserIntegration": "INTEGRATION_WEB_PLAYER", "includeEpisodeContentRatingsV2": false}
	if err := p.pathfinderQuery(ctx, sess, "browsePage", variables, &data); err != nil {
		return model.Feed{}, mapProviderError(err)
	}
	if data.Browse == nil {
		return model.Feed{}, fmt.Errorf("browsePage Pathfinder response has no browse")
	}
	feed := model.Feed{}
	for _, section := range data.Browse.Sections.Items {
		shelf := model.Shelf{}
		if section.Data.Title != nil {
			shelf.Title = section.Data.Title.Label
		}
		for _, item := range section.Items.Items {
			if card, ok := browseCard(item); ok {
				shelf.Items = append(shelf.Items, card)
			}
		}
		if len(shelf.Items) > 0 {
			feed.Sections = append(feed.Sections, shelf)
		}
	}
	return feed, nil
}

func browseCard(item pfBrowseItem) (model.BrowseItem, bool) {
	if item.Content == nil || item.Content.Data == nil {
		return model.BrowseItem{}, false
	}
	entity := item.Content.Data
	switch entity.Type {
	case "Playlist":
		id := strings.TrimPrefix(item.URI, "spotify:playlist:")
		if id == item.URI || id == "" {
			id = strings.TrimPrefix(entity.URI, "spotify:playlist:")
		}
		card := model.BrowseItem{Kind: "playlist", ID: "spotify:playlist:" + id, Title: entity.Name}
		if entity.Owner.Data != nil && entity.Owner.Data.Name != "" {
			card.Subtitle = new(entity.Owner.Data.Name)
		}
		if len(entity.Images.Items) > 0 {
			card.Thumbnail = imageFrom(entity.Images.Items[0].Sources, false)
		}
		return card, id != ""
	case "Album":
		id := strings.TrimPrefix(item.URI, "spotify:album:")
		if id == item.URI || id == "" {
			id = strings.TrimPrefix(entity.URI, "spotify:album:")
		}
		card := model.BrowseItem{Kind: "album", ID: "spotify:album:" + id, Title: entity.Name, ArtistRuns: mapArtists(entity.Artists)}
		if names := artistNames(card.ArtistRuns); names != "" {
			card.Subtitle = new(names)
		}
		card.Thumbnail = imageFrom(entity.Cover.Sources, false)
		return card, id != ""
	case "BrowseSectionContainer":
		id := strings.TrimPrefix(item.URI, "spotify:page:")
		if entity.Data == nil || entity.Data.Card == nil || id == item.URI || id == "" {
			return model.BrowseItem{}, false
		}
		card := model.BrowseItem{Kind: "browse", ID: id}
		if entity.Data.Card.Title != nil {
			card.Title = entity.Data.Card.Title.Label
		}
		card.Thumbnail = imageFrom(entity.Data.Card.Artwork.Sources, false)
		return card, card.Title != ""
	default:
		return model.BrowseItem{}, false
	}
}

// Search implements provider.Provider.
func (p *Spotify) Search(ctx context.Context, query string, kind model.SearchKind) (model.SearchPage, error) {
	if kind != model.SearchKindAll && kind != model.SearchKindSongs {
		return model.SearchPage{}, nil
	}
	tracks, err := p.searchTracks(ctx, query)
	return model.SearchPage{Items: tracks}, err
}

// SearchMore implements provider.Provider; Spotify search has no exposed continuation.
func (p *Spotify) SearchMore(context.Context, string) (model.SearchPage, error) {
	return model.SearchPage{}, nil
}

// SearchAll implements provider.Provider.
func (p *Spotify) SearchAll(ctx context.Context, query string) (model.SearchAll, error) {
	tracks, err := p.searchTracks(ctx, query)
	if err != nil {
		return model.SearchAll{}, err
	}
	albums, albumErr := p.searchPathfinderCards(ctx, query, "searchAlbums")
	playlists, playlistErr := p.searchPathfinderCards(ctx, query, "searchPlaylists")
	if albumErr != nil {
		p.log.Warn("Spotify album search failed", "error", albumErr)
	}
	if playlistErr != nil {
		p.log.Warn("Spotify playlist search failed", "error", playlistErr)
	}
	result := model.SearchAll{Albums: albums, Playlists: playlists}
	seen := map[string]bool{}
	for _, track := range tracks {
		card := trackCard(track)
		result.Songs = append(result.Songs, card)
		for _, artist := range track.ArtistRuns {
			if artist.ID != nil && !seen[*artist.ID] {
				seen[*artist.ID] = true
				result.Artists = append(result.Artists, model.BrowseItem{Kind: "artist", ID: *artist.ID, Title: artist.Text})
			}
		}
	}
	result.Top = append(result.Top, result.Songs...)
	if len(result.Top) > 5 {
		result.Top = result.Top[:5]
	}
	return result, nil
}

// SearchAllMore implements provider.Provider.
func (p *Spotify) SearchAllMore(context.Context, string) (model.SearchAll, error) {
	return model.SearchAll{}, nil
}

// SearchCards implements provider.Provider.
func (p *Spotify) SearchCards(ctx context.Context, query, category string) (model.CardsPage, error) {
	switch strings.ToLower(category) {
	case "album", "albums":
		items, err := p.searchPathfinderCards(ctx, query, "searchAlbums")
		return model.CardsPage{Items: items}, err
	case "playlist", "playlists":
		items, err := p.searchPathfinderCards(ctx, query, "searchPlaylists")
		return model.CardsPage{Items: items}, err
	case "artist", "artists":
		all, err := p.SearchAll(ctx, query)
		return model.CardsPage{Items: all.Artists}, err
	default:
		return model.CardsPage{}, nil
	}
}

// SearchCardsMore implements provider.Provider.
func (p *Spotify) SearchCardsMore(context.Context, string) (model.CardsPage, error) {
	return model.CardsPage{}, nil
}

// Suggest implements provider.Provider; Spotify does not expose suggestions to this client.
func (p *Spotify) Suggest(context.Context, string) ([]string, error) { return []string{}, nil }

func (p *Spotify) searchTracks(ctx context.Context, query string) ([]model.Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []model.Track{}, nil
	}
	sess, err := p.session(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := sess.Spclient().ContextResolve(ctx, "spotify:search:"+url.QueryEscape(query))
	if err != nil {
		return nil, mapProviderError(err)
	}
	var uris []string
	for _, page := range resolved.GetPages() {
		for _, track := range page.GetTracks() {
			if strings.HasPrefix(track.GetUri(), "spotify:track:") {
				uris = append(uris, track.GetUri())
			}
		}
	}
	known, err := p.trackMetadata(ctx, sess, uris)
	if err != nil {
		return nil, mapProviderError(err)
	}
	items := make([]model.Track, 0, len(uris))
	for _, uri := range uris {
		if track, ok := known[uri]; ok {
			items = append(items, track)
		}
	}
	return items, nil
}

// PlayCount fetches Spotify's reported play count for a track.
func (p *Spotify) PlayCount(ctx context.Context, id string) (*string, error) {
	bare, err := bareID(id, "track")
	if err != nil {
		return nil, err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return nil, err
	}
	var data struct {
		Track *struct {
			Playcount string `json:"playcount"`
		} `json:"trackUnion"`
	}
	if err := p.pathfinderQuery(ctx, sess, "getTrack", map[string]any{"uri": "spotify:track:" + bare}, &data); err != nil {
		return nil, mapProviderError(err)
	}
	if data.Track == nil {
		return nil, fmt.Errorf("track play count response has no track")
	}
	count, err := strconv.ParseUint(data.Track.Playcount, 10, 64)
	if err != nil && data.Track.Playcount != "" {
		return nil, fmt.Errorf("invalid track play count")
	}
	return countLabel(count), nil
}

func (p *Spotify) searchPathfinderCards(ctx context.Context, query, operation string) ([]model.BrowseItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []model.BrowseItem{}, nil
	}
	sess, err := p.session(ctx)
	if err != nil {
		return nil, err
	}
	var data pfSearchData
	if err := p.pathfinderQuery(ctx, sess, operation, map[string]any{"searchTerm": query, "offset": 0, "limit": 20}, &data); err != nil {
		return nil, mapProviderError(err)
	}
	if data.Search == nil {
		return nil, fmt.Errorf("%s Pathfinder response has no search", operation)
	}
	rows := data.Search.Albums.Items
	if operation == "searchPlaylists" {
		rows = data.Search.Playlists.Items
	}
	out := make([]model.BrowseItem, 0, len(rows))
	for _, row := range rows {
		entity := row.Data
		if entity == nil || entity.URI == "" || entity.Name == "" {
			continue
		}
		card := model.BrowseItem{Kind: "album", ID: entity.URI, Title: entity.Name}
		if operation == "searchPlaylists" {
			card.Kind = "playlist"
			if entity.Owner.Data != nil {
				card.Subtitle = new(entity.Owner.Data.Name)
			}
			if len(entity.Images.Items) > 0 {
				card.Thumbnail = imageFrom(entity.Images.Items[0].Sources, false)
			}
		} else {
			card.ArtistRuns = mapArtists(entity.Artists)
			if names := artistNames(card.ArtistRuns); names != "" {
				card.Subtitle = new(names)
			}
			card.Thumbnail = imageFrom(entity.Cover.Sources, false)
		}
		out = append(out, card)
	}
	return out, nil
}

// Album implements provider.Provider.
func (p *Spotify) Album(ctx context.Context, id string) (model.AlbumPage, error) {
	bare, err := bareID(id, "album")
	if err != nil {
		return model.AlbumPage{}, err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return model.AlbumPage{}, err
	}
	var page model.AlbumPage
	offset := 0
	for {
		var data struct {
			Album *pfAlbum `json:"albumUnion"`
		}
		vars := map[string]any{"uri": "spotify:album:" + bare, "locale": "", "offset": offset, "limit": 50}
		if err := p.pathfinderQuery(ctx, sess, "getAlbum", vars, &data); err != nil {
			return page, mapProviderError(err)
		}
		if data.Album == nil {
			return page, fmt.Errorf("album Pathfinder response has no album")
		}
		album := data.Album
		if page.Title == nil {
			page.Title = new(album.Name)
			page.Subtitle = new(artistNames(mapArtists(album.Artists)))
			page.Thumbnail = imageFrom(album.Cover.Sources, true)
			page.Cover = page.Thumbnail
		}
		for _, item := range album.Tracks.Items {
			page.Items = append(page.Items, mapPFTrack(item.Track, album))
		}
		loaded := len(album.Tracks.Items)
		offset += loaded
		if loaded == 0 || offset >= album.Tracks.Total {
			break
		}
	}
	return page, nil
}

func mapPFTrack(raw pfTrack, album *pfAlbum) model.Track {
	artists := mapArtists(raw.Artists)
	track := model.Track{VideoID: raw.URI, Title: raw.Name, Artists: artistNames(artists), ArtistRuns: artists, Album: new(album.Name), AlbumID: new("spotify:album:" + strings.TrimPrefix(album.URI, "spotify:album:")), Thumbnail: imageFrom(album.Cover.Sources, false), Explicit: raw.ContentRating.Label == "EXPLICIT"}
	if len(artists) > 0 {
		track.ArtistID = artists[0].ID
	}
	if raw.Duration.Milliseconds > 0 {
		track.Duration = new(durationLabel(time.Duration(raw.Duration.Milliseconds) * time.Millisecond))
	}
	if count, err := strconv.ParseUint(raw.Playcount, 10, 64); err == nil {
		track.PlayCount = countLabel(count)
	}
	return track
}

// Artist implements provider.Provider.
func (p *Spotify) Artist(ctx context.Context, id string) (model.ArtistPage, error) {
	bare, err := bareID(id, "artist")
	if err != nil {
		return model.ArtistPage{}, err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return model.ArtistPage{}, err
	}
	var data struct {
		Artist *struct {
			Profile struct {
				Name      string `json:"name"`
				Biography *struct {
					Text *string `json:"text"`
				} `json:"biography"`
			} `json:"profile"`
			Visuals struct {
				Avatar pfArtwork `json:"avatarImage"`
			} `json:"visuals"`
			Discography struct {
				Top *struct {
					Items []struct {
						Track pfTrack `json:"track"`
					} `json:"items"`
				} `json:"topTracks"`
				Albums       pfReleaseGroups `json:"albums"`
				Singles      pfReleaseGroups `json:"singles"`
				Compilations pfReleaseGroups `json:"compilations"`
			} `json:"discography"`
			Stats *struct {
				Monthly uint64 `json:"monthlyListeners"`
			} `json:"stats"`
		} `json:"artistUnion"`
	}
	vars := map[string]any{"uri": "spotify:artist:" + bare, "locale": "", "preReleaseV2": true}
	if err := p.pathfinderQuery(ctx, sess, "queryArtistOverview", vars, &data); err != nil {
		return model.ArtistPage{}, mapProviderError(err)
	}
	if data.Artist == nil {
		return model.ArtistPage{}, fmt.Errorf("artist Pathfinder response has no artist")
	}
	a := data.Artist
	page := model.ArtistPage{Name: new(a.Profile.Name), Thumbnail: imageFrom(a.Visuals.Avatar.Sources, true), ChannelID: "spotify:artist:" + bare}
	if a.Profile.Biography != nil {
		page.Description = a.Profile.Biography.Text
	}
	if a.Stats != nil {
		page.MonthlyListeners = countLabel(a.Stats.Monthly)
	}
	var uris []string
	plays := map[string]*string{}
	if a.Discography.Top != nil {
		for _, item := range a.Discography.Top.Items {
			uris = append(uris, item.Track.URI)
			if n, e := strconv.ParseUint(item.Track.Playcount, 10, 64); e == nil {
				plays[item.Track.URI] = countLabel(n)
			}
		}
	}
	known, err := p.trackMetadata(ctx, sess, uris)
	if err != nil {
		return page, mapProviderError(err)
	}
	for _, uri := range uris {
		if track, ok := known[uri]; ok {
			track.PlayCount = plays[uri]
			page.TopSongs = append(page.TopSongs, track)
		}
	}
	albums := append(append(a.Discography.Albums.Items, a.Discography.Singles.Items...), a.Discography.Compilations.Items...)
	if len(albums) > 0 {
		section := model.ArtistSection{Title: "Releases"}
		seen := map[string]bool{}
		for _, group := range albums {
			for _, release := range group.Releases.Items {
				if seen[release.ID] {
					continue
				}
				seen[release.ID] = true
				card := model.BrowseItem{Kind: "album", ID: "spotify:album:" + release.ID, Title: release.Name, Thumbnail: imageFrom(release.Cover.Sources, false)}
				section.Items = append(section.Items, card)
			}
		}
		page.Sections = []model.ArtistSection{section}
	}
	return page, nil
}

type pfReleaseGroups struct {
	Items []struct {
		Releases struct {
			Items []struct {
				ID    string    `json:"id"`
				Name  string    `json:"name"`
				Cover pfArtwork `json:"coverArt"`
			} `json:"items"`
		} `json:"releases"`
	} `json:"items"`
}

func mapArtists(raw pfArtists) []model.Artist {
	out := make([]model.Artist, 0, len(raw.Items))
	for _, a := range raw.Items {
		if a.Profile.Name == "" {
			continue
		}
		var id *string
		if strings.HasPrefix(a.URI, "spotify:artist:") {
			id = new(a.URI)
		}
		out = append(out, model.Artist{Text: a.Profile.Name, ID: id})
	}
	return out
}
func artistNames(artists []model.Artist) string {
	names := make([]string, 0, len(artists))
	for _, a := range artists {
		names = append(names, a.Text)
	}
	return strings.Join(names, ", ")
}
func imageFrom(images []pfImage, large bool) *string {
	if len(images) == 0 {
		return nil
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Height < images[j].Height })
	picked := images[0]
	if large {
		for _, image := range images {
			picked = image
			if image.Height >= 300 {
				break
			}
		}
	}
	if picked.URL == "" {
		return nil
	}
	return new(picked.URL)
}
func trackCard(track model.Track) model.BrowseItem {
	return model.BrowseItem{Kind: "track", ID: track.VideoID, Title: track.Title, Subtitle: new(track.Artists), Thumbnail: track.Thumbnail, Duration: track.Duration, ArtistRuns: track.ArtistRuns, PlayCount: track.PlayCount, Explicit: track.Explicit}
}
