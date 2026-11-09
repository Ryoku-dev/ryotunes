package spotify

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	playlist4pb "github.com/devgianlu/go-librespot/proto/spotify/playlist4"
	"github.com/devgianlu/go-librespot/spclient"
	"github.com/ryoku-dev/ryotunes/internal/model"
	providerapi "github.com/ryoku-dev/ryotunes/internal/provider"
	"google.golang.org/protobuf/proto"
)

// Playlist implements provider.Provider.
func (p *Spotify) Playlist(ctx context.Context, id string, ordering model.PlaylistSort, desc bool) (model.PlaylistPage, error) {
	if id == "spotify:playlist:liked" || id == "liked" {
		library, err := p.Library(ctx)
		if err != nil {
			return model.PlaylistPage{}, err
		}
		title := "Liked Songs"
		return model.PlaylistPage{Title: &title, Items: library.Liked, Owned: true}, nil
	}
	bare, err := bareID(id, "playlist")
	if err != nil {
		return model.PlaylistPage{}, err
	}
	sess, err := p.session(ctx)
	if err != nil {
		return model.PlaylistPage{}, err
	}
	content, err := p.playlistSnapshot(ctx, sess.Spclient(), bare)
	if err != nil {
		return model.PlaylistPage{}, mapProviderError(err)
	}
	attrs := content.GetAttributes()
	page := model.PlaylistPage{Title: new(attrs.GetName()), Description: new(attrs.GetDescription()), Owned: content.GetOwnerUsername() == sess.Username(), Collaborative: attrs.GetCollaborative()}
	page.Thumbnail = playlistCover(attrs)
	page.Cover = page.Thumbnail
	items := content.GetContents().GetItems()
	uris := make([]string, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(item.GetUri(), "spotify:track:") {
			uris = append(uris, item.GetUri())
		}
	}
	known, err := p.trackMetadata(ctx, sess, uris)
	if err != nil {
		return page, mapProviderError(err)
	}
	for _, item := range items {
		track, ok := known[item.GetUri()]
		if !ok {
			continue
		}
		attrs := item.GetAttributes()
		if id := attrs.GetItemId(); len(id) > 0 {
			track.SetVideoID = new(hex.EncodeToString(id))
		}
		if by := attrs.GetAddedBy(); by != "" {
			track.AddedBy = new(by)
		}
		page.Items = append(page.Items, track)
	}
	sortPlaylist(page.Items, ordering, desc)
	return page, nil
}

// PlaylistMore implements provider.Provider; playlist snapshots are fetched in full.
func (p *Spotify) PlaylistMore(context.Context, string) (model.PlaylistTracksPage, error) {
	return model.PlaylistTracksPage{}, nil
}

func (p *Spotify) playlistSnapshot(ctx context.Context, sp spclientRequester, id string) (*playlist4pb.SelectedListContent, error) {
	resp, err := sp.Request(ctx, http.MethodGet, "/playlist/v2/playlist/"+id, url.Values{"from": {"0"}, "length": {"10000"}}, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("playlist status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var content playlist4pb.SelectedListContent
	if err := proto.Unmarshal(body, &content); err != nil {
		return nil, err
	}
	return &content, nil
}

type spclientRequester interface {
	Request(context.Context, string, string, url.Values, http.Header, []byte) (*http.Response, error)
}

func playlistCover(attrs *playlist4pb.ListAttributes) *string {
	if attrs == nil {
		return nil
	}
	for _, size := range attrs.GetPictureSize() {
		if u := size.GetUrl(); strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
			return new(u)
		}
	}
	if len(attrs.GetPicture()) > 0 {
		return new("https://i.scdn.co/image/" + hex.EncodeToString(attrs.GetPicture()))
	}
	return nil
}

func sortPlaylist(items []model.Track, ordering model.PlaylistSort, desc bool) {
	less := func(i, j int) bool { return false }
	switch ordering {
	case model.PlaylistSortTitle:
		less = func(i, j int) bool { return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title) }
	case model.PlaylistSortArtist:
		less = func(i, j int) bool { return strings.ToLower(items[i].Artists) < strings.ToLower(items[j].Artists) }
	case model.PlaylistSortAlbum:
		less = func(i, j int) bool {
			a, b := "", ""
			if items[i].Album != nil {
				a = *items[i].Album
			}
			if items[j].Album != nil {
				b = *items[j].Album
			}
			return strings.ToLower(a) < strings.ToLower(b)
		}
	default:
		if desc {
			reverseTracks(items)
		}
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		if desc {
			return less(j, i)
		}
		return less(i, j)
	})
}
func reverseTracks(items []model.Track) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

// Library implements provider.Provider.
func (p *Spotify) Library(ctx context.Context) (model.Library, error) {
	sess, err := p.session(ctx)
	if err != nil {
		return model.Library{}, err
	}
	library := model.Library{}
	tracks, err := p.webSavedTracks(ctx, sess)
	if err != nil {
		return library, mapProviderError(err)
	}
	library.Liked = tracks
	root, err := sess.Spclient().Rootlist(ctx, sess.Username(), 0, 10000)
	if err == nil {
		library.Playlists = mapRootlist(root)
	} else {
		p.log.Warn("Spotify rootlist failed", "error", err)
	}
	albums, err := p.webSavedAlbums(ctx, sess)
	if err == nil {
		library.Albums = albums
	} else {
		p.log.Warn("Spotify saved albums failed", "error", err)
	}
	artists, err := p.webSavedArtists(ctx, sess)
	if err == nil {
		library.Artists = artists
	} else {
		p.log.Warn("Spotify saved artists failed", "error", err)
	}
	return library, nil
}

func mapRootlist(root *playlist4pb.SelectedListContent) []model.BrowseItem {
	if root == nil {
		return nil
	}
	contents := root.GetContents()
	items := contents.GetItems()
	meta := contents.GetMetaItems()
	out := make([]model.BrowseItem, 0, len(items))
	for index, item := range items {
		uri := item.GetUri()
		if !strings.HasPrefix(uri, "spotify:playlist:") {
			continue
		}
		var detail *playlist4pb.MetaItem
		if index < len(meta) {
			detail = meta[index]
		}
		title := "Unknown"
		var thumb *string
		var subtitle *string
		if detail != nil {
			if n := detail.GetAttributes().GetName(); n != "" {
				title = n
			}
			thumb = playlistCover(detail.GetAttributes())
			if owner := detail.GetOwnerUsername(); owner != "" {
				subtitle = new(owner)
			}
		}
		out = append(out, model.BrowseItem{Kind: "playlist", ID: uri, Title: title, Subtitle: subtitle, Thumbnail: thumb})
	}
	return out
}

type webArtist struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}
type webAlbum struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Artists []webArtist `json:"artists"`
	Images  []struct {
		URL string `json:"url"`
	} `json:"images"`
}
type webTrack struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Artists  []webArtist `json:"artists"`
	Album    webAlbum    `json:"album"`
	Duration int64       `json:"duration_ms"`
	Explicit bool        `json:"explicit"`
}

func (p *Spotify) webSavedTracks(ctx context.Context, sess spotifySession) ([]model.Track, error) {
	endpoint := "https://api.spotify.com/v1/me/tracks?limit=50"
	var out []model.Track
	for endpoint != "" {
		var page struct {
			Items []struct {
				Track webTrack `json:"track"`
			} `json:"items"`
			Next string `json:"next"`
		}
		if err := p.webJSON(ctx, sess, endpoint, &page); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			out = append(out, mapWebTrack(item.Track))
		}
		endpoint = page.Next
	}
	return out, nil
}
func (p *Spotify) webSavedAlbums(ctx context.Context, sess spotifySession) ([]model.BrowseItem, error) {
	endpoint := "https://api.spotify.com/v1/me/albums?limit=50"
	var out []model.BrowseItem
	for endpoint != "" {
		var page struct {
			Items []struct {
				Album webAlbum `json:"album"`
			} `json:"items"`
			Next string `json:"next"`
		}
		if err := p.webJSON(ctx, sess, endpoint, &page); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			card := model.BrowseItem{Kind: "album", ID: "spotify:album:" + item.Album.ID, Title: item.Album.Name}
			if len(item.Album.Artists) > 0 {
				card.Subtitle = new(item.Album.Artists[0].Name)
			}
			if len(item.Album.Images) > 0 {
				card.Thumbnail = new(item.Album.Images[len(item.Album.Images)-1].URL)
			}
			out = append(out, card)
		}
		endpoint = page.Next
	}
	return out, nil
}
func (p *Spotify) webSavedArtists(ctx context.Context, sess spotifySession) ([]model.BrowseItem, error) {
	endpoint := "https://api.spotify.com/v1/me/following?type=artist&limit=50"
	var out []model.BrowseItem
	for endpoint != "" {
		var page struct {
			Artists struct {
				Items []webArtist `json:"items"`
				Next  string      `json:"next"`
			} `json:"artists"`
		}
		if err := p.webJSON(ctx, sess, endpoint, &page); err != nil {
			return nil, err
		}
		for _, artist := range page.Artists.Items {
			card := model.BrowseItem{Kind: "artist", ID: "spotify:artist:" + artist.ID, Title: artist.Name}
			if len(artist.Images) > 0 {
				card.Thumbnail = new(artist.Images[len(artist.Images)-1].URL)
			}
			out = append(out, card)
		}
		endpoint = page.Artists.Next
	}
	return out, nil
}

type spotifySession interface{ Spclient() *spclient.Spclient }

func (p *Spotify) webJSON(ctx context.Context, sess spotifySession, endpoint string, target any) error {
	token, err := sess.Spclient().GetAccessToken(ctx, false)
	if err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return providerapi.ErrSignInRequired
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Spotify Web API status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(target)
}
func mapWebTrack(raw webTrack) model.Track {
	artists := make([]model.Artist, 0, len(raw.Artists))
	for _, a := range raw.Artists {
		artists = append(artists, model.Artist{Text: a.Name, ID: new("spotify:artist:" + a.ID)})
	}
	track := model.Track{VideoID: "spotify:track:" + raw.ID, Title: raw.Name, Artists: artistNames(artists), ArtistRuns: artists, Explicit: raw.Explicit}
	if len(artists) > 0 {
		track.ArtistID = artists[0].ID
	}
	if raw.Album.ID != "" {
		track.AlbumID = new("spotify:album:" + raw.Album.ID)
		track.Album = new(raw.Album.Name)
	}
	if len(raw.Album.Images) > 0 {
		track.Thumbnail = new(raw.Album.Images[len(raw.Album.Images)-1].URL)
	}
	if raw.Duration > 0 {
		track.Duration = new(durationLabel(time.Duration(raw.Duration) * time.Millisecond))
	}
	return track
}

// Account implements provider.Provider.
func (p *Spotify) Account(ctx context.Context) (model.Account, error) {
	p.mu.RLock()
	signedIn := p.sess != nil
	p.mu.RUnlock()
	if !signedIn {
		return model.Account{SignedIn: false}, nil
	}
	if _, err := p.session(ctx); err != nil {
		return model.Account{}, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return model.Account{SignedIn: true, Name: new(p.name)}, nil
}

// Next implements provider.Provider with Spotify track radio.
func (p *Spotify) Next(ctx context.Context, seed model.Track, played []string) ([]model.Track, error) {
	tracks, _, err := p.StartRadio(ctx, "track", seed.VideoID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(played)+1)
	seen[seed.VideoID] = true
	for _, id := range played {
		seen[id] = true
	}
	out := tracks[:0]
	for _, track := range tracks {
		if !seen[track.VideoID] {
			seen[track.VideoID] = true
			out = append(out, track)
		}
	}
	return out, nil
}
