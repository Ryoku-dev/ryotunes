package soundcloud

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	trackPrefix    = "sc:track:"
	userPrefix     = "sc:user:"
	playlistPrefix = "sc:playlist:"
	systemPrefix   = "sc:system:"
)

type collection[T any] struct {
	Collection []T     `json:"collection"`
	NextHref   *string `json:"next_href"`
}

type userRef struct {
	ID        uint64  `json:"id"`
	Username  string  `json:"username"`
	Permalink string  `json:"permalink"`
	AvatarURL *string `json:"avatar_url"`
}

type visuals struct {
	Visuals []struct {
		VisualURL *string `json:"visual_url"`
	} `json:"visuals"`
}

type user struct {
	ID             uint64   `json:"id"`
	Username       string   `json:"username"`
	FullName       *string  `json:"full_name"`
	Permalink      string   `json:"permalink"`
	AvatarURL      *string  `json:"avatar_url"`
	FollowersCount uint64   `json:"followers_count"`
	TrackCount     uint64   `json:"track_count"`
	City           *string  `json:"city"`
	Country        *string  `json:"country"`
	CountryCode    *string  `json:"country_code"`
	Description    *string  `json:"description"`
	Verified       bool     `json:"verified"`
	Visuals        *visuals `json:"visuals"`
}

type transcoding struct {
	URL     string  `json:"url"`
	Preset  *string `json:"preset"`
	Quality *string `json:"quality"`
	Format  struct {
		Protocol string `json:"protocol"`
		MimeType string `json:"mime_type"`
	} `json:"format"`
}

type track struct {
	ID                 uint64   `json:"id"`
	Kind               string   `json:"kind"`
	Title              string   `json:"title"`
	User               *userRef `json:"user"`
	Duration           uint64   `json:"duration"`
	ArtworkURL         *string  `json:"artwork_url"`
	Genre              *string  `json:"genre"`
	PlaybackCount      *uint64  `json:"playback_count"`
	LikesCount         *uint64  `json:"likes_count"`
	CreatedAt          string   `json:"created_at"`
	PermalinkURL       string   `json:"permalink_url"`
	WaveformURL        *string  `json:"waveform_url"`
	TrackAuthorization *string  `json:"track_authorization"`
	Media              struct {
		Transcodings []transcoding `json:"transcodings"`
	} `json:"media"`
	Description *string `json:"description"`
}

type playlist struct {
	ID          uint64   `json:"id"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	User        *userRef `json:"user"`
	ArtworkURL  *string  `json:"artwork_url"`
	IsAlbum     bool     `json:"is_album"`
	SetType     *string  `json:"set_type"`
	TrackCount  uint64   `json:"track_count"`
	ReleaseDate *string  `json:"release_date"`
	Duration    uint64   `json:"duration"`
	Description *string  `json:"description"`
	Tracks      []track  `json:"tracks"`
}

type systemPlaylist struct {
	URN                  string  `json:"urn"`
	Kind                 string  `json:"kind"`
	Permalink            string  `json:"permalink"`
	Title                string  `json:"title"`
	ShortTitle           string  `json:"short_title"`
	Description          *string `json:"description"`
	CalculatedArtworkURL *string `json:"calculated_artwork_url"`
	ArtworkURL           *string `json:"artwork_url"`
	TrackCount           *uint64 `json:"track_count"`
	Tracks               []track `json:"tracks"`
}

func parseNumericID(id, prefix string) (uint64, error) {
	raw, ok := strings.CutPrefix(id, prefix)
	if !ok || raw == "" {
		return 0, fmt.Errorf("invalid %s id", strings.TrimSuffix(prefix, ":"))
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s id", strings.TrimSuffix(prefix, ":"))
	}
	return n, nil
}

func artwork(v *string) *string {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil
	}
	u := strings.Replace(*v, "-large.", "-t500x500.", 1)
	return &u
}

func trackModel(t track) model.Track {
	u := t.User
	artist := ""
	var artistID *string
	var runs []model.Artist
	var thumb *string
	if u != nil {
		artist = u.Username
		id := fmt.Sprintf("%s%d", userPrefix, u.ID)
		artistID = &id
		runs = []model.Artist{{Text: artist, ID: &id}}
		thumb = artwork(u.AvatarURL)
	}
	if art := artwork(t.ArtworkURL); art != nil {
		thumb = art
	}
	duration := formatDurationMS(t.Duration)
	out := model.Track{
		VideoID:    fmt.Sprintf("%s%d", trackPrefix, t.ID),
		Title:      t.Title,
		Artists:    artist,
		ArtistID:   artistID,
		ArtistRuns: runs,
		Duration:   &duration,
		Thumbnail:  thumb,
		Genre:      nonempty(t.Genre),
	}
	if t.PlaybackCount != nil {
		v := abbreviate(*t.PlaybackCount)
		out.PlayCount = &v
		out.Plays = &v
	}
	if t.LikesCount != nil {
		v := abbreviate(*t.LikesCount)
		out.Likes = &v
	}
	return out
}

func trackCard(t track) model.BrowseItem {
	m := trackModel(t)
	return model.BrowseItem{
		Kind: "song", ID: m.VideoID, Title: m.Title, Subtitle: stringPtr(m.Artists),
		Thumbnail: m.Thumbnail, Duration: m.Duration, ArtistRuns: m.ArtistRuns,
		PlayCount: m.PlayCount,
	}
}

func userCard(u user) model.BrowseItem {
	return model.BrowseItem{
		Kind: "artist", ID: fmt.Sprintf("%s%d", userPrefix, u.ID), Title: u.Username,
		Thumbnail: artwork(u.AvatarURL),
	}
}

func playlistCard(p playlist) model.BrowseItem {
	kind := "playlist"
	subtitle := playlistSubtitle(p)
	if p.IsAlbum {
		kind = "album"
		subtitle = "Album"
		if p.ReleaseDate != nil && len(*p.ReleaseDate) >= 4 {
			subtitle += " · " + (*p.ReleaseDate)[:4]
		}
	}
	return model.BrowseItem{
		Kind: kind, ID: fmt.Sprintf("%s%d", playlistPrefix, p.ID), Title: p.Title,
		Subtitle: &subtitle, Thumbnail: artwork(p.ArtworkURL),
	}
}

func systemCard(p systemPlaylist) model.BrowseItem {
	title := p.ShortTitle
	if title == "" {
		title = p.Title
	}
	count := uint64(len(p.Tracks))
	if p.TrackCount != nil {
		count = *p.TrackCount
	}
	subtitle := fmt.Sprintf("Trending · %s", trackCount(count))
	art := artwork(p.CalculatedArtworkURL)
	if art == nil {
		art = artwork(p.ArtworkURL)
	}
	return model.BrowseItem{Kind: "playlist", ID: systemPrefix + p.Permalink, Title: title, Subtitle: &subtitle, Thumbnail: art}
}

func playlistPage(p playlist, tracks []track, owned bool) model.PlaylistPage {
	title := p.Title
	subtitle := playlistSubtitle(p)
	items := make([]model.Track, 0, len(tracks))
	for _, t := range tracks {
		items = append(items, trackModel(t))
	}
	return model.PlaylistPage{
		Title: &title, Subtitle: &subtitle, Thumbnail: artwork(p.ArtworkURL),
		Description: nonempty(p.Description), Items: items, Owned: owned,
	}
}

func formatDurationMS(ms uint64) string {
	seconds := ms / 1000
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func abbreviate(n uint64) string {
	for _, unit := range []struct {
		value  uint64
		suffix string
	}{{1_000_000_000, "B"}, {1_000_000, "M"}, {1_000, "K"}} {
		if n >= unit.value {
			whole := float64(n) / float64(unit.value)
			if whole < 10 {
				text := strings.TrimSuffix(fmt.Sprintf("%.1f", whole), ".0")
				return text + unit.suffix
			}
			return fmt.Sprintf("%.0f%s", whole, unit.suffix)
		}
	}
	return strconv.FormatUint(n, 10)
}

func playlistSubtitle(p playlist) string {
	owner := ""
	if p.User != nil {
		owner = p.User.Username
	}
	count := trackCount(p.TrackCount)
	if owner == "" {
		return count
	}
	return owner + " · " + count
}

func trackCount(n uint64) string {
	if n == 1 {
		return "1 track"
	}
	return fmt.Sprintf("%d tracks", n)
}

func nonempty(v *string) *string {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil
	}
	return v
}

func stringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
