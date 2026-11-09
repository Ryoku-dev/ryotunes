package widgets

import (
	"strings"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

func textValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func browseKey(item model.BrowseItem) string {
	if item.ID != "" {
		return strings.ToLower(item.Kind) + "\x00" + item.ID
	}
	return strings.ToLower(item.Kind) + "\x00" + item.Title
}

func browseKind(item model.BrowseItem) string {
	kind := strings.ToLower(strings.TrimSpace(item.Kind))
	switch kind {
	case "song", "track", "video":
		return "Song"
	case "album", "single", "ep":
		return "Album"
	case "artist", "channel", "user":
		return "Artist"
	case "playlist", "mix":
		return "Playlist"
	case "station", "radio":
		return "Station"
	default:
		if kind == "" {
			return ""
		}
		return strings.ToUpper(kind[:1]) + kind[1:]
	}
}

func browseSubtitle(item model.BrowseItem) string {
	if subtitle := textValue(item.Subtitle); subtitle != "" {
		return subtitle
	}
	return browseKind(item)
}

func browseIsArtist(item model.BrowseItem) bool {
	switch strings.ToLower(strings.TrimSpace(item.Kind)) {
	case "artist", "channel", "user":
		return true
	default:
		return false
	}
}

func browseIsTrack(item model.BrowseItem) bool {
	switch strings.ToLower(strings.TrimSpace(item.Kind)) {
	case "song", "track", "video", "station", "radio":
		return true
	default:
		return false
	}
}

// IsLikedCollection reports provider and local aliases for the built-in liked-songs collection.
func IsLikedCollection(item model.BrowseItem, provider model.Provider) bool {
	id := strings.TrimSpace(item.ID)
	switch {
	case id == model.LocalLikedPlaylistID:
		return true
	case strings.EqualFold(id, "LOCAL:liked"), strings.EqualFold(id, "RYOTUNES_LIKED"):
		return true
	}
	switch provider {
	case model.ProviderYouTube:
		return strings.EqualFold(id, "VLLM") || strings.EqualFold(id, "LM")
	case model.ProviderSpotify:
		return strings.EqualFold(id, "spotify:playlist:liked") || strings.EqualFold(id, "liked")
	default:
		return false
	}
}
