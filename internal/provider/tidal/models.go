package tidal

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	trackPrefix    = "tidal:track:"
	artistPrefix   = "tidal:artist:"
	albumPrefix    = "tidal:album:"
	playlistPrefix = "tidal:playlist:"
	mixPrefix      = "tidal:mix:"
)

type artist struct {
	ID      uint64  `json:"id"`
	Name    string  `json:"name"`
	Picture *string `json:"picture"`
}

type album struct {
	ID          uint64   `json:"id"`
	Title       string   `json:"title"`
	Cover       *string  `json:"cover"`
	TrackCount  *uint32  `json:"numberOfTracks"`
	ReleaseDate *string  `json:"releaseDate"`
	Artists     []artist `json:"artists"`
}

type track struct {
	ID           uint64   `json:"id"`
	Title        string   `json:"title"`
	Duration     *uint32  `json:"duration"`
	TrackNumber  *uint32  `json:"trackNumber"`
	Artists      []artist `json:"artists"`
	Album        *album   `json:"album"`
	AudioQuality *string  `json:"audioQuality"`
	StreamReady  *bool    `json:"streamReady"`
	Explicit     bool     `json:"explicit"`
}

type playlist struct {
	UUID        string  `json:"uuid"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	TrackCount  *uint32 `json:"numberOfTracks"`
	Image       *string `json:"image"`
	SquareImage *string `json:"squareImage"`
}

type page[T any] struct {
	Items  []T     `json:"items"`
	Total  *uint32 `json:"totalNumberOfItems"`
	Offset *uint32 `json:"offset"`
	Limit  *uint32 `json:"limit"`
}

type favorite[T any] struct {
	Created *string `json:"created"`
	Item    T       `json:"item"`
}

type searchResults struct {
	Tracks    page[track]    `json:"tracks"`
	Albums    page[album]    `json:"albums"`
	Artists   page[artist]   `json:"artists"`
	Playlists page[playlist] `json:"playlists"`
}

type mix struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	SubTitle    *string `json:"subTitle"`
	Description *string `json:"description"`
	Image       *string `json:"image"`
}

func parseNumericID(id, prefix string) (uint64, error) {
	raw, ok := strings.CutPrefix(id, prefix)
	if !ok || raw == "" {
		return 0, fmt.Errorf("invalid TIDAL id")
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid TIDAL id")
	}
	return n, nil
}

func parseStringID(id, prefix string) (string, error) {
	raw, ok := strings.CutPrefix(id, prefix)
	if !ok || strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("invalid TIDAL id")
	}
	return raw, nil
}

func coverURL(id *string, size int) *string {
	if id == nil || strings.TrimSpace(*id) == "" {
		return nil
	}
	value := fmt.Sprintf("https://resources.tidal.com/images/%s/%dx%d.jpg", strings.ReplaceAll(*id, "-", "/"), size, size)
	return &value
}

func artistLine(artists []artist) string {
	names := make([]string, 0, len(artists))
	for _, a := range artists {
		if strings.TrimSpace(a.Name) != "" {
			names = append(names, a.Name)
		}
	}
	return strings.Join(names, ", ")
}

func artistRuns(artists []artist) []model.Artist {
	out := make([]model.Artist, 0, len(artists))
	for _, a := range artists {
		id := fmt.Sprintf("%s%d", artistPrefix, a.ID)
		out = append(out, model.Artist{Text: a.Name, ID: &id})
	}
	return out
}

func trackModel(t track) model.Track {
	var duration *string
	if t.Duration != nil {
		v := formatDuration(*t.Duration)
		duration = &v
	}
	var artistID *string
	if len(t.Artists) != 0 {
		v := fmt.Sprintf("%s%d", artistPrefix, t.Artists[0].ID)
		artistID = &v
	}
	var albumTitle, albumID, thumbnail *string
	if t.Album != nil {
		albumTitle = pointerIfText(t.Album.Title)
		v := fmt.Sprintf("%s%d", albumPrefix, t.Album.ID)
		albumID = &v
		thumbnail = coverURL(t.Album.Cover, 320)
	}
	return model.Track{
		VideoID: fmt.Sprintf("%s%d", trackPrefix, t.ID), Title: t.Title,
		Artists: artistLine(t.Artists), ArtistID: artistID, ArtistRuns: artistRuns(t.Artists),
		Album: albumTitle, AlbumID: albumID, Duration: duration, Thumbnail: thumbnail,
		Explicit: t.Explicit,
	}
}

func trackCard(t track) model.BrowseItem {
	m := trackModel(t)
	return model.BrowseItem{
		Kind: "song", ID: m.VideoID, Title: m.Title, Subtitle: pointerIfText(m.Artists),
		Thumbnail: m.Thumbnail, Duration: m.Duration, ArtistRuns: m.ArtistRuns, Explicit: m.Explicit,
	}
}

func artistCard(a artist) model.BrowseItem {
	return model.BrowseItem{Kind: "artist", ID: fmt.Sprintf("%s%d", artistPrefix, a.ID), Title: a.Name, Thumbnail: coverURL(a.Picture, 320)}
}

func albumCard(a album) model.BrowseItem {
	subtitle := "Album"
	if a.ReleaseDate != nil && len(*a.ReleaseDate) >= 4 {
		subtitle += " · " + (*a.ReleaseDate)[:4]
	}
	return model.BrowseItem{
		Kind: "album", ID: fmt.Sprintf("%s%d", albumPrefix, a.ID), Title: a.Title,
		Subtitle: &subtitle, Thumbnail: coverURL(a.Cover, 320), ArtistRuns: artistRuns(a.Artists),
	}
}

func playlistCard(p playlist) model.BrowseItem {
	var subtitle *string
	if p.TrackCount != nil {
		v := countTracks(uint64(*p.TrackCount))
		subtitle = &v
	}
	cover := p.SquareImage
	if cover == nil {
		cover = p.Image
	}
	return model.BrowseItem{Kind: "playlist", ID: playlistPrefix + p.UUID, Title: p.Title, Subtitle: subtitle, Thumbnail: coverURL(cover, 320)}
}

func mixCard(m mix) model.BrowseItem {
	return model.BrowseItem{Kind: "playlist", ID: mixPrefix + m.ID, Title: m.Title, Subtitle: m.SubTitle, Thumbnail: coverURL(m.Image, 320)}
}

func formatDuration(seconds uint32) string {
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func countTracks(n uint64) string {
	if n == 1 {
		return "1 track"
	}
	return fmt.Sprintf("%d tracks", n)
}

func pointerIfText(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}

func encodePageToken(v pageToken) string {
	return url.QueryEscape(v.Kind + "\n" + v.Query + "\n" + strconv.Itoa(v.Offset))
}

func decodePageToken(raw string) (pageToken, error) {
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return pageToken{}, err
	}
	parts := strings.Split(decoded, "\n")
	if len(parts) != 3 {
		return pageToken{}, fmt.Errorf("invalid TIDAL continuation")
	}
	offset, err := strconv.Atoi(parts[2])
	if err != nil {
		return pageToken{}, fmt.Errorf("invalid TIDAL continuation")
	}
	return pageToken{Kind: parts[0], Query: parts[1], Offset: offset}, nil
}

type pageToken struct {
	Kind, Query string
	Offset      int
}
