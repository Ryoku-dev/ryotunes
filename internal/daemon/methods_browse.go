package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
	"github.com/ryoku-dev/ryotunes/internal/provider"
	"github.com/ryoku-dev/ryotunes/internal/provider/radio"
)

func init() {
	register(protocol.MethodGetHome, typed(getHome))
	register(protocol.MethodGetHomeMore, typed(getHomeMore))
	register(protocol.MethodSearch, typed(search))
	register(protocol.MethodSearchPage, typed(searchPage))
	register(protocol.MethodSearchPageMore, typed(searchPageMore))
	register(protocol.MethodSearchAll, typed(searchAll))
	register(protocol.MethodSearchAllMore, typed(searchAllMore))
	register(protocol.MethodSearchCards, typed(searchCards))
	register(protocol.MethodSearchCardsPage, typed(searchCardsPage))
	register(protocol.MethodSearchCardsMore, typed(searchCardsMore))
	register(protocol.MethodGetAlbum, typed(getAlbum))
	register(protocol.MethodGetArtist, typed(getArtist))
	register(protocol.MethodGetSCUser, typed(getSCUser))
	register(protocol.MethodGetBrowseGrid, typed(getBrowseGrid))
	register(protocol.MethodGetPlaylist, typed(getPlaylist))
	register(protocol.MethodGetPlaylistMore, typed(getPlaylistMore))
	register(protocol.MethodGetLyrics, typed(getLyrics))
}

func selectedProvider(a *App) provider.Provider {
	a.mu.Lock()
	selected := model.Provider(a.provider)
	a.mu.Unlock()
	if p, ok := a.Providers.Get(selected); ok {
		return p
	}
	p, _ := a.Providers.Get(model.ProviderYouTube)
	return p
}

func getHome(ctx context.Context, a *App, p protocol.HomeParams) (model.Feed, error) {
	selected := selectedProvider(a)
	if selected.ID() == model.ProviderTidal {
		feed, err := selected.Home(ctx, "")
		return feed, providerError(selected.ID(), err)
	}
	params := ""
	if p.Params != nil {
		params = *p.Params
	}
	feed, err := a.YouTube.Home(ctx, params)
	if err != nil || p.Params != nil {
		return feed, err
	}
	if a.Spotify.Status().SignedIn {
		if extra, extraErr := a.Spotify.Home(ctx, ""); extraErr == nil {
			feed.Sections = append(feed.Sections, extra.Sections...)
		}
	}
	if a.SoundCloud.Status().SignedIn {
		if extra, extraErr := a.SoundCloud.Home(ctx, ""); extraErr == nil {
			feed.Sections = append(feed.Sections, extra.Sections...)
		}
	}
	if a.Tidal.Status().SignedIn {
		if extra, extraErr := a.Tidal.Home(ctx, ""); extraErr == nil {
			feed.Sections = append(feed.Sections, extra.Sections...)
		}
	}
	return feed, nil
}
func getHomeMore(ctx context.Context, a *App, p protocol.TokenParams) (model.Feed, error) {
	return a.YouTube.HomeMore(ctx, p.Token)
}
func search(ctx context.Context, a *App, p protocol.QueryParams) ([]model.Track, error) {
	pr := selectedProvider(a)
	page, err := pr.Search(ctx, p.Query, model.SearchKindSongs)
	return page.Items, providerError(pr.ID(), err)
}
func searchPage(ctx context.Context, a *App, p protocol.QueryParams) (model.SearchPage, error) {
	pr := selectedProvider(a)
	page, err := pr.Search(ctx, p.Query, model.SearchKindSongs)
	return page, providerError(pr.ID(), err)
}
func searchPageMore(ctx context.Context, a *App, p protocol.TokenParams) (model.SearchPage, error) {
	pr := selectedProvider(a)
	if pr.ID() != model.ProviderYouTube {
		return model.SearchPage{Items: []model.Track{}}, nil
	}
	return pr.SearchMore(ctx, p.Token)
}
func searchAll(ctx context.Context, a *App, p protocol.QueryParams) (model.SearchAll, error) {
	pr := selectedProvider(a)
	result, err := pr.SearchAll(ctx, p.Query)
	return result, providerError(pr.ID(), err)
}
func searchAllMore(ctx context.Context, a *App, p protocol.TokenParams) (model.SearchAll, error) {
	pr := selectedProvider(a)
	if pr.ID() != model.ProviderYouTube {
		return model.SearchAll{Top: []model.BrowseItem{}, Songs: []model.BrowseItem{}, Albums: []model.BrowseItem{}, Artists: []model.BrowseItem{}, Playlists: []model.BrowseItem{}}, nil
	}
	return pr.SearchAllMore(ctx, p.Token)
}
func searchCards(ctx context.Context, a *App, p protocol.SearchCardsParams) ([]model.BrowseItem, error) {
	pr := selectedProvider(a)
	page, err := pr.SearchCards(ctx, p.Query, string(p.Category))
	return page.Items, providerError(pr.ID(), err)
}
func searchCardsPage(ctx context.Context, a *App, p protocol.SearchCardsParams) (model.CardsPage, error) {
	pr := selectedProvider(a)
	page, err := pr.SearchCards(ctx, p.Query, string(p.Category))
	return page, providerError(pr.ID(), err)
}
func searchCardsMore(ctx context.Context, a *App, p protocol.TokenParams) (model.CardsPage, error) {
	pr := selectedProvider(a)
	if pr.ID() != model.ProviderYouTube {
		return model.CardsPage{Items: []model.BrowseItem{}}, nil
	}
	return pr.SearchCardsMore(ctx, p.Token)
}
func getAlbum(ctx context.Context, a *App, p protocol.IDParams) (model.AlbumPage, error) {
	pr := providerForID(a, p.ID)
	page, err := pr.Album(ctx, p.ID)
	return page, providerError(pr.ID(), err)
}
func getArtist(ctx context.Context, a *App, p protocol.IDParams) (model.ArtistPage, error) {
	pr := providerForID(a, p.ID)
	page, err := pr.Artist(ctx, p.ID)
	return page, providerError(pr.ID(), err)
}
func getSCUser(ctx context.Context, a *App, p protocol.IDParams) (model.ArtistPage, error) {
	if model.ProviderOf(p.ID) != model.ProviderSoundCloud {
		return model.ArtistPage{}, fmt.Errorf("get_sc_user expects a SoundCloud user id")
	}
	page, err := a.SoundCloud.Artist(ctx, p.ID)
	return page, providerError(model.ProviderSoundCloud, err)
}
func getBrowseGrid(ctx context.Context, a *App, p protocol.BrowseGridParams) ([]model.BrowseItem, error) {
	pr := providerForID(a, p.ID)
	grid, ok := pr.(provider.Grid)
	if !ok {
		return nil, provider.ErrUnsupported
	}
	params := ""
	if p.Params != nil {
		params = *p.Params
	}
	page, err := grid.BrowseGrid(ctx, p.ID, params)
	return page.Items, providerError(pr.ID(), err)
}
func getPlaylist(ctx context.Context, a *App, p protocol.PlaylistParams) (model.PlaylistPage, error) {
	if isSmartPlaylist(p.ID) {
		return smartPlaylist(ctx, a, p.ID)
	}
	pr := providerForID(a, p.ID)
	sort, desc := model.PlaylistSortDefault, false
	if p.Sort != nil {
		sort = *p.Sort
	}
	if p.Desc != nil {
		desc = *p.Desc
	}
	page, err := pr.Playlist(ctx, p.ID, sort, desc)
	if err != nil && strings.HasPrefix(p.ID, model.LocalPlaylistPrefix) && errors.Is(err, provider.ErrNotFound) {
		return model.PlaylistPage{}, fmt.Errorf("That device playlist no longer exists.")
	}
	if err == nil && (pr.ID() == model.ProviderYouTube || pr.ID() == model.ProviderLocal) {
		if cover, coverErr := a.Store.PlaylistArtwork(p.ID); coverErr == nil && cover != "" {
			page.Cover = &cover
		}
	}
	return page, providerError(pr.ID(), err)
}
func smartPlaylist(ctx context.Context, a *App, id string) (model.PlaylistPage, error) {
	var title, subtitle string
	var items []model.Track
	switch id {
	case model.OnRepeatPlaylistID:
		title = "On Repeat"
		rows, err := a.Store.TopPlays(now().AddDate(0, 0, -30), 100)
		if err != nil {
			return model.PlaylistPage{}, err
		}
		for _, row := range rows {
			items = append(items, row.Track)
		}
		subtitle = fmt.Sprintf("%d songs you've played most this month", len(items))
	case model.RecentlyPlayedPlaylistID:
		title = "Recently Played"
		var err error
		items, err = a.Store.RecentUniquePlays(now().AddDate(0, 0, -7), 100)
		if err != nil {
			return model.PlaylistPage{}, err
		}
		subtitle = fmt.Sprintf("%d songs you've returned to this week", len(items))
	case model.RediscoverPlaylistID:
		title = "Rediscover"
		var err error
		items, err = a.Store.RediscoverPlays(now().AddDate(0, 0, -14), now().AddDate(0, 0, -7), 100)
		if err != nil {
			return model.PlaylistPage{}, err
		}
		subtitle = fmt.Sprintf("%d songs ready for another listen", len(items))
	default:
		return model.PlaylistPage{}, provider.ErrNotFound
	}
	if items == nil {
		items = []model.Track{}
	}
	return model.PlaylistPage{Title: &title, Subtitle: &subtitle, Items: items}, nil
}
func getPlaylistMore(ctx context.Context, a *App, p protocol.TokenParams) (model.PlaylistTracksPage, error) {
	return a.YouTube.PlaylistMore(ctx, p.Token)
}
func getLyrics(ctx context.Context, a *App, p protocol.LyricsParams) (*model.Lyrics, error) {
	if _, ok := radio.UUIDFromID(p.VideoID); ok {
		return nil, nil
	}
	if model.ProviderOf(p.VideoID) == model.ProviderSpotify && !a.Spotify.Status().SignedIn {
		return nil, nil
	}
	value, err := a.Lyrics.Get(ctx, p)
	return value, providerError(model.ProviderOf(p.VideoID), err)
}
