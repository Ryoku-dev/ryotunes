package local

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func trackToModel(track LocalTrack) model.Track {
	var duration *string
	if track.DurationSecs > 0 {
		formatted := formatDuration(track.DurationSecs)
		duration = &formatted
	}
	var cover *string
	if track.Cover != "" {
		value := track.Cover
		cover = &value
	}
	album := track.Album
	return model.Track{
		VideoID:   SongID(track.Path),
		Title:     track.Title,
		Artists:   track.Artist,
		Album:     &album,
		Duration:  duration,
		Thumbnail: cover,
	}
}

func songsOf(tracks []LocalTrack) []model.Track {
	out := make([]model.Track, len(tracks))
	for i, track := range tracks {
		out[i] = trackToModel(track)
	}
	return out
}

func albumsOf(tracks []LocalTrack) []model.BrowseItem {
	groups := make(map[string][]LocalTrack)
	for _, track := range tracks {
		groups[track.AlbumKey] = append(groups[track.AlbumKey], track)
	}
	albums := make([]model.BrowseItem, 0, len(groups))
	for _, group := range groups {
		face := group[0]
		for _, track := range group {
			if track.Cover != "" {
				face = track
				break
			}
		}
		subtitle := fmt.Sprintf("%s • %d song%s", albumArtist(group), len(group), plural(len(group)))
		var thumbnail *string
		if face.Cover != "" {
			cover := face.Cover
			thumbnail = &cover
		}
		albums = append(albums, model.BrowseItem{
			Kind:      "album",
			ID:        AlbumID(face.AlbumKey),
			Title:     face.Album,
			Subtitle:  &subtitle,
			Thumbnail: thumbnail,
		})
	}
	sort.Slice(albums, func(i, j int) bool {
		return strings.ToLower(albums[i].Title) < strings.ToLower(albums[j].Title)
	})
	return albums
}

func artistsOf(tracks []LocalTrack) []model.BrowseItem {
	groups := make(map[string][]LocalTrack)
	for _, track := range tracks {
		name := primaryArtist(track.Artist)
		groups[name] = append(groups[name], track)
	}
	artists := make([]model.BrowseItem, 0, len(groups))
	for name, group := range groups {
		subtitle := fmt.Sprintf("%d song%s", len(group), plural(len(group)))
		var thumbnail *string
		for _, track := range group {
			if track.Cover != "" {
				cover := track.Cover
				thumbnail = &cover
				break
			}
		}
		artists = append(artists, model.BrowseItem{
			Kind:      "artist",
			ID:        ArtistID(name),
			Title:     name,
			Subtitle:  &subtitle,
			Thumbnail: thumbnail,
		})
	}
	sort.Slice(artists, func(i, j int) bool {
		return strings.ToLower(artists[i].Title) < strings.ToLower(artists[j].Title)
	})
	return artists
}

func albumPage(tracks []LocalTrack) model.AlbumPage {
	sort.SliceStable(tracks, func(i, j int) bool {
		if tracks[i].TrackNo != tracks[j].TrackNo {
			if tracks[i].TrackNo == 0 {
				return false
			}
			if tracks[j].TrackNo == 0 {
				return true
			}
			return tracks[i].TrackNo < tracks[j].TrackNo
		}
		return strings.ToLower(tracks[i].Path) < strings.ToLower(tracks[j].Path)
	})
	title := tracks[0].Album
	subtitle := fmt.Sprintf("%s · On this device", albumArtist(tracks))
	var thumbnail *string
	for _, track := range tracks {
		if track.Cover != "" {
			cover := track.Cover
			thumbnail = &cover
			break
		}
	}
	return model.AlbumPage{
		Title:     &title,
		Subtitle:  &subtitle,
		Thumbnail: thumbnail,
		Items:     songsOf(tracks),
	}
}

func albumArtist(tracks []LocalTrack) string {
	if len(tracks) == 0 {
		return UnknownArtist
	}
	artist := tracks[0].Artist
	for _, track := range tracks[1:] {
		if track.Artist != artist {
			return "Various artists"
		}
	}
	return artist
}

func primaryArtist(artist string) string {
	cut := len(artist)
	if i := strings.IndexByte(artist, ';'); i >= 0 {
		cut = i
	}
	markers := [...]string{" feat.", " feat ", " ft.", " ft ", " featuring "}
	for index := range artist {
		for _, marker := range markers {
			end := index + len(marker)
			if index < cut && end <= len(artist) && strings.EqualFold(artist[index:end], marker) {
				cut = index
			}
		}
	}
	return strings.TrimSpace(artist[:cut])
}

func playlistCard(row model.DevicePlaylist, items []model.Track) model.BrowseItem {
	subtitle := fmt.Sprintf("%d track%s · On this device", row.TrackCount, plural(int(row.TrackCount)))
	return model.BrowseItem{
		Kind:      "playlist",
		ID:        row.ID,
		Title:     row.Title,
		Subtitle:  &subtitle,
		Thumbnail: playlistArtwork(items),
	}
}

func playlistArtwork(items []model.Track) *string {
	covers := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, item := range items {
		if item.Thumbnail == nil {
			continue
		}
		cover := strings.TrimSpace(*item.Thumbnail)
		if cover == "" {
			continue
		}
		if _, duplicate := seen[cover]; duplicate {
			continue
		}
		seen[cover] = struct{}{}
		covers = append(covers, cover)
		if len(covers) == 4 {
			break
		}
	}
	if len(covers) == 0 {
		return nil
	}
	if len(covers) < 4 {
		return &covers[0]
	}
	encoded, err := json.Marshal(covers)
	if err != nil {
		return &covers[0]
	}
	value := model.CollageThumbnailPrefix + string(encoded)
	return &value
}

func formatDuration(seconds int64) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	remaining := seconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, remaining)
	}
	return fmt.Sprintf("%d:%02d", minutes, remaining)
}
