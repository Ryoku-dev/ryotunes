package model

import "strings"

// Provider identifies a music catalogue or local source.
type Provider string

const (
	// ProviderYouTube identifies YouTube Music.
	ProviderYouTube Provider = "youtube"
	// ProviderSpotify identifies Spotify.
	ProviderSpotify Provider = "spotify"
	// ProviderSoundCloud identifies SoundCloud.
	ProviderSoundCloud Provider = "soundcloud"
	// ProviderTidal identifies TIDAL.
	ProviderTidal Provider = "tidal"
	// ProviderLocal identifies local files and their library entities.
	ProviderLocal Provider = "local"
	// ProviderRadio identifies live internet radio.
	ProviderRadio Provider = "radio"
)

const (
	// CollageThumbnailPrefix marks a four-image device-playlist collage.
	CollageThumbnailPrefix = "ryotunes-collage:"
	// LocalPlaylistPrefix prefixes device playlist IDs.
	LocalPlaylistPrefix = "RYOTUNES_LOCAL_PLAYLIST:"
	// LocalLikedPlaylistID is the fixed device liked-songs playlist.
	LocalLikedPlaylistID = "RYOTUNES_LOCAL_PLAYLIST:liked"
	// OnRepeatPlaylistID identifies the On Repeat smart playlist.
	OnRepeatPlaylistID = "RYOTUNES_ON_REPEAT"
	// RecentlyPlayedPlaylistID identifies the Recently Played smart playlist.
	RecentlyPlayedPlaylistID = "RYOTUNES_RECENTLY_PLAYED"
	// RediscoverPlaylistID identifies the Rediscover smart playlist.
	RediscoverPlaylistID = "RYOTUNES_REDISCOVER"
)

// ProviderOf derives a provider from a wire ID and returns an empty provider for Ryotunes-owned collections or an empty ID.
func ProviderOf(id string) Provider {
	if id == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(id, "spotify:"):
		return ProviderSpotify
	case strings.HasPrefix(id, "sc:"):
		return ProviderSoundCloud
	case strings.HasPrefix(id, "tidal:"):
		return ProviderTidal
	case strings.HasPrefix(id, "LOCAL"):
		return ProviderLocal
	case strings.HasPrefix(id, "RYOTUNES_RADIO:"):
		return ProviderRadio
	case IsRyotunesOwn(id):
		return ""
	default:
		return ProviderYouTube
	}
}

// IsRyotunesOwn reports whether an ID names a Ryotunes-created collection rather than a provider entity.
func IsRyotunesOwn(id string) bool {
	return strings.HasPrefix(id, "RYOTUNES_")
}
