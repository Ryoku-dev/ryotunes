package protocol

import (
	"reflect"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	// EventPosition reports playback position seconds.
	EventPosition = "position"
	// EventDuration reports playback duration seconds.
	EventDuration = "duration"
	// EventPlaybackState reports playing or paused.
	EventPlaybackState = "playback-state"
	// EventPlaybackError reports a terminal playback error.
	EventPlaybackError = "playback-error"
	// EventPlaybackNotice reports a nonterminal playback notice.
	EventPlaybackNotice = "playback-notice"
	// EventNowPlaying replaces now-playing metadata.
	EventNowPlaying = "now-playing"
	// EventQueueChanged replaces the complete queue.
	EventQueueChanged = "queue-changed"
	// EventQueueIndex patches queue pointer state.
	EventQueueIndex = "queue-index"
	// EventRating updates a current track rating.
	EventRating = "rating"
	// EventStopAfterCurrent updates the one-shot stop flag.
	EventStopAfterCurrent = "stop-after-current"
	// EventVolume updates volume percent.
	EventVolume = "volume"
	// EventAudioFx updates playback effects.
	EventAudioFx = "audio-fx"
	// EventShow asks the subscribed client to raise its window.
	EventShow = "show"
	// EventAuthChanged replaces the YouTube account snapshot.
	EventAuthChanged = "auth-changed"
	// EventSpotifyAuth reports Spotify authorization progress.
	EventSpotifyAuth = "spotify-auth"
	// EventSoundCloudAuth reports SoundCloud authorization progress.
	EventSoundCloudAuth = "soundcloud-auth"
	// EventTidalAuth reports TIDAL authorization progress.
	EventTidalAuth = "tidal-auth"
	// EventProviderChanged reports provider selection.
	EventProviderChanged = "provider-changed"
	// EventLibraryChanged invalidates a playlist or library card.
	EventLibraryChanged = "library-changed"
	// EventLocalChanged reports removed local entities.
	EventLocalChanged = "local-changed"
	// EventPersonalChanged replaces renderer-owned personalization.
	EventPersonalChanged = "personal-changed"
	// EventLyricsChanged invalidates displayed lyrics.
	EventLyricsChanged = "lyrics-changed"
	// EventCoverError reports a remote cover synchronization failure.
	EventCoverError = "cover-error"
	// EventLastfmState reports Last.fm connection state.
	EventLastfmState = "lastfm-state"
	// EventLTState replaces Listen Together state.
	EventLTState = "lt-state"
	// EventLTNotice reports a bare Listen Together notice string.
	EventLTNotice = "lt-notice"
	// EventDownloadsChanged replaces the download snapshot.
	EventDownloadsChanged = "downloads-changed"
)

// PlaybackState is the bare playback-state event payload.
type PlaybackState string

const (
	// PlaybackStatePlaying means audio is actively advancing.
	PlaybackStatePlaying PlaybackState = "playing"
	// PlaybackStatePaused means audio is not advancing.
	PlaybackStatePaused PlaybackState = "paused"
)

// PositionEvent is the position event payload.
type PositionEvent struct {
	Position float64 `json:"position"`
}

// DurationEvent is the duration event payload.
type DurationEvent struct {
	Duration float64 `json:"duration"`
}

// MessageEvent is a message-bearing error or notice payload.
type MessageEvent struct {
	Message string `json:"message"`
}

// RatingEvent is the rating event payload.
type RatingEvent struct {
	VideoID string       `json:"videoId"`
	Rating  model.Rating `json:"rating"`
}

// LibraryChangedEvent identifies the invalidated playlist.
type LibraryChangedEvent struct {
	ID string `json:"id"`
}

// EventDef describes one event payload type.
type EventDef struct {
	Name    string
	Payload reflect.Type
}

// Events is the complete version-one event registry.
var Events = []EventDef{
	eventDef[PositionEvent](EventPosition),
	eventDef[DurationEvent](EventDuration),
	eventDef[PlaybackState](EventPlaybackState),
	eventDef[MessageEvent](EventPlaybackError),
	eventDef[MessageEvent](EventPlaybackNotice),
	eventDef[*model.PlaybackTrack](EventNowPlaying),
	eventDef[model.QueueSnapshot](EventQueueChanged),
	eventDef[model.QueueIndex](EventQueueIndex),
	eventDef[RatingEvent](EventRating),
	eventDef[bool](EventStopAfterCurrent),
	eventDef[int](EventVolume),
	eventDef[model.AudioFx](EventAudioFx),
	eventDef[Null](EventShow),
	eventDef[model.Account](EventAuthChanged),
	eventDef[model.SpotifyAuthEvent](EventSpotifyAuth),
	eventDef[model.SoundCloudAuthEvent](EventSoundCloudAuth),
	eventDef[model.TidalAuthEvent](EventTidalAuth),
	eventDef[ProviderResult](EventProviderChanged),
	eventDef[LibraryChangedEvent](EventLibraryChanged),
	eventDef[model.LocalChanged](EventLocalChanged),
	eventDef[model.Personal](EventPersonalChanged),
	eventDef[Null](EventLyricsChanged),
	eventDef[MessageEvent](EventCoverError),
	eventDef[model.LastfmState](EventLastfmState),
	eventDef[model.LTState](EventLTState),
	eventDef[model.LTNotice](EventLTNotice),
	eventDef[model.DownloadSnapshot](EventDownloadsChanged),
}

func eventDef[P any](name string) EventDef {
	return EventDef{Name: name, Payload: reflect.TypeOf((*P)(nil)).Elem()}
}
