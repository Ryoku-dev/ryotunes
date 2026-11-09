package protocol

import (
	"encoding/json"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

// Null marks a method whose params or result are JSON null.
type Null struct{}

// Hello is the hello handshake result.
type Hello struct {
	Protocol int    `json:"protocol"`
	Daemon   string `json:"daemon"`
	// Clients counts live subscribed connections, so a launcher can raise an
	// open window instead of starting a second client.
	Clients int `json:"clients"`
}

// SubscribeParams chooses event names; version one currently delivers all events after subscribing.
type SubscribeParams struct {
	Events []string `json:"events"`
}

// Snapshot is the complete subscribe result used to restore a client.
type Snapshot struct {
	Playback   model.PlaybackSnapshot `json:"playback"`
	Queue      model.QueueSnapshot    `json:"queue"`
	Settings   model.SettingsSnapshot `json:"settings"`
	Auth       model.Account          `json:"auth"`
	AudioFx    model.AudioFx          `json:"audioFx"`
	Equalizer  model.Equalizer        `json:"equalizer"`
	Provider   model.Provider         `json:"provider"`
	Spotify    model.SpotifyStatus    `json:"spotify"`
	SoundCloud model.SoundCloudStatus `json:"soundcloud"`
	Tidal      model.TidalStatus      `json:"tidal"`
	Downloads  model.DownloadSnapshot `json:"downloads"`
	Personal   model.Personal         `json:"personal"`
	LT         model.LTState          `json:"lt"`
}

// QueryParams carries a search query.
type QueryParams struct {
	Query string `json:"query"`
}

// TokenParams carries an opaque continuation token.
type TokenParams struct {
	Token string `json:"token"`
}

// SearchCardsParams carries a card search query and category.
type SearchCardsParams struct {
	Query    string           `json:"query"`
	Category model.SearchKind `json:"category"`
}

// PlayParams starts one track.
type PlayParams struct {
	Item model.Track `json:"item"`
}

// PrefetchStreamParams resolves a stream without starting playback.
type PrefetchStreamParams struct {
	VideoID  string `json:"videoId"`
	IsUpload *bool  `json:"isUpload,omitempty"`
}

// IndexParams carries one queue index.
type IndexParams struct {
	Index int `json:"index"`
}

// AddToQueueParams appends tracks to the manual tail.
type AddToQueueParams struct {
	Items        []model.Track       `json:"items"`
	From         *model.QueueContext `json:"from"`
	Continuation *string             `json:"continuation"`
}

// MoveInQueueParams moves a queue row.
type MoveInQueueParams struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// PlayNextParams inserts tracks into the play-next block.
type PlayNextParams struct {
	Items []model.Track       `json:"items"`
	From  *model.QueueContext `json:"from"`
}

// SetRepeatParams changes repeat mode.
type SetRepeatParams struct {
	Mode model.RepeatMode `json:"mode"`
}

// EqualizerPresetParams names a saved equalizer preset.
type EqualizerPresetParams struct {
	Name string `json:"name"`
}

// ImportEqualizerParams carries Equalizer APO or AutoEQ ParametricEQ text.
// A non-empty Name also saves the imported curve as a preset.
type ImportEqualizerParams struct {
	Text string `json:"text"`
	Name string `json:"name,omitempty"`
}

// ImportEqualizerResult is the applied equalizer and what the import dropped
// or changed, one plain sentence per warning.
type ImportEqualizerResult struct {
	Equalizer model.Equalizer `json:"equalizer"`
	Warnings  []string        `json:"warnings,omitempty"`
}

// SetStopAfterCurrentParams changes the one-shot queue stop.
type SetStopAfterCurrentParams struct {
	Enabled bool `json:"enabled"`
}

// SeekParams seeks in seconds.
type SeekParams struct {
	Position float64 `json:"position"`
}

// SetVolumeParams changes volume percent.
type SetVolumeParams struct {
	Volume int `json:"volume"`
}

// SetSettingParams writes one renderer-visible string setting.
type SetSettingParams struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PersonalResult wraps the opaque personal object returned by get_personal.
type PersonalResult struct {
	Personal model.Personal `json:"personal"`
}

// SetPersonalParams replaces the opaque personal object.
type SetPersonalParams = PersonalResult

// ClearCachesParams chooses whether visitor identity is rotated.
type ClearCachesParams struct {
	Rotate *bool `json:"rotate,omitempty"`
}

// ClearCachesResult reports whether visitor identity refresh succeeded.
type ClearCachesResult struct {
	VisitorDataRefreshed bool `json:"visitorDataRefreshed"`
}

// SwitchAccountParams selects an opaque account identity.
type SwitchAccountParams struct {
	SelectionKey string `json:"selectionKey"`
}

// ProviderParams selects a provider.
type ProviderParams struct {
	Provider model.Provider `json:"provider"`
}

// ProviderResult reports the selected provider.
type ProviderResult = ProviderParams

// StartedResult reports whether an asynchronous authorization flow started.
type StartedResult struct {
	Started bool `json:"started"`
}

// AuthFlowResult carries a browser URL for a client-owned sign-in handoff.
type AuthFlowResult struct {
	URL string `json:"url"`
}

// SignedOutResult is the provider sign-out result.
type SignedOutResult struct {
	SignedIn bool `json:"signedIn"`
}

// TidalCredentialsParams updates optional TIDAL device and PKCE credentials.
type TidalCredentialsParams struct {
	DeviceID     *string `json:"deviceId,omitempty"`
	DeviceSecret *string `json:"deviceSecret,omitempty"`
	PKCEID       *string `json:"pkceId,omitempty"`
	PKCESecret   *string `json:"pkceSecret,omitempty"`
	PKCERedirect *string `json:"pkceRedirect,omitempty"`
}

// HomeParams optionally filters the YouTube home feed.
type HomeParams struct {
	Params *string `json:"params,omitempty"`
}

// PlaylistParams selects and optionally sorts a playlist.
type PlaylistParams struct {
	ID   string              `json:"id"`
	Sort *model.PlaylistSort `json:"sort,omitempty"`
	Desc *bool               `json:"desc,omitempty"`
}

// IDParams carries a provider entity ID.
type IDParams struct {
	ID string `json:"id"`
}

// BrowseGridParams selects a browse grid and optional provider params.
type BrowseGridParams struct {
	ID     string  `json:"id"`
	Params *string `json:"params,omitempty"`
}

// PathParams carries a filesystem path.
type PathParams struct {
	Path string `json:"path"`
}

// PlayPlaylistParams replaces the queue with a collection.
type PlayPlaylistParams struct {
	Items        []model.Track `json:"items"`
	Start        *int          `json:"start,omitempty"`
	SourceID     *string       `json:"sourceId,omitempty"`
	SourceName   *string       `json:"sourceName,omitempty"`
	Shuffle      *bool         `json:"shuffle,omitempty"`
	Continuation *string       `json:"continuation,omitempty"`
}

// StartRadioParams starts provider radio from one entity.
type StartRadioParams struct {
	Kind string  `json:"kind"`
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`
}

// RadioStationsParams filters and pages radio stations.
type RadioStationsParams struct {
	Query  *string `json:"query,omitempty"`
	Offset *int    `json:"offset,omitempty"`
	Limit  *int    `json:"limit,omitempty"`
}

// PlayRadioStationParams selects a radio station UUID.
type PlayRadioStationParams struct {
	StationUUID string `json:"stationUuid"`
}

// ExportPlaylistFileParams writes a portable playlist file.
type ExportPlaylistFileParams struct {
	Title string        `json:"title"`
	Items []model.Track `json:"items"`
	Path  string        `json:"path"`
}

// ImportPlaylistFileResult is a validated imported playlist.
type ImportPlaylistFileResult struct {
	Title string        `json:"title"`
	Items []model.Track `json:"items"`
}

// RateParams changes one track rating.
type RateParams struct {
	VideoID string       `json:"videoId"`
	Rating  model.Rating `json:"rating"`
}

// SetAlbumSavedParams changes album library membership.
type SetAlbumSavedParams struct {
	PlaylistID string `json:"playlistId"`
	Saved      bool   `json:"saved"`
}

// SetArtistSubscribedParams changes an artist channel subscription.
type SetArtistSubscribedParams struct {
	ID         string `json:"id"`
	Subscribed bool   `json:"subscribed"`
}

// SetArtistSubscribedResult reports the provider-confirmed subscription state.
type SetArtistSubscribedResult struct {
	Subscribed bool `json:"subscribed"`
}

// PlaylistTrackParams identifies a playlist and track.
type PlaylistTrackParams struct {
	PlaylistID string `json:"playlistId"`
	VideoID    string `json:"videoId"`
}

// AddToLocalPlaylistParams adds a full track to a device playlist.
type AddToLocalPlaylistParams struct {
	PlaylistID string      `json:"playlistId"`
	Item       model.Track `json:"item"`
}

// RemoveFromPlaylistParams removes a provider or device playlist row.
type RemoveFromPlaylistParams struct {
	PlaylistID string  `json:"playlistId"`
	VideoID    string  `json:"videoId"`
	SetVideoID *string `json:"setVideoId,omitempty"`
}

// CreatePlaylistParams names a new playlist.
type CreatePlaylistParams struct {
	Title string `json:"title"`
}

// PlaylistIDResult is the created playlist ID.
type PlaylistIDResult string

// EditPlaylistDetailsParams edits supported playlist metadata.
type EditPlaylistDetailsParams struct {
	PlaylistID  string  `json:"playlistId"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Public      *bool   `json:"public,omitempty"`
}

// SetPlaylistCoverParams selects or clears local playlist artwork.
type SetPlaylistCoverParams struct {
	PlaylistID string  `json:"playlistId"`
	Pick       bool    `json:"pick"`
	Path       *string `json:"path,omitempty"`
}

// SetPlaylistCoverResult reports the active custom and display artwork.
type SetPlaylistCoverResult struct {
	Cover     *string `json:"cover"`
	Thumbnail *string `json:"thumbnail"`
}

// SetPlaylistSortParams persists a provider playlist ordering.
type SetPlaylistSortParams struct {
	PlaylistID string             `json:"playlistId"`
	Sort       model.PlaylistSort `json:"sort"`
}

// PlaylistIDParams selects a playlist.
type PlaylistIDParams struct {
	PlaylistID string `json:"playlistId"`
}

// ListeningStatsParams selects a day, week, or month period.
type ListeningStatsParams struct {
	Period model.ListeningPeriod `json:"period"`
}

// LTServerURLParams updates the Listen Together relay URL.
type LTServerURLParams struct {
	URL string `json:"url"`
}

// LTCreateRoomParams names the local Listen Together user.
type LTCreateRoomParams struct {
	Username string `json:"username"`
}

// LTJoinRoomParams joins a room as a named user.
type LTJoinRoomParams struct {
	Code     string `json:"code"`
	Username string `json:"username"`
}

// LTUserParams selects a Listen Together user.
type LTUserParams struct {
	UserID string `json:"userId"`
}

// LTSuggestParams submits one track suggestion.
type LTSuggestParams struct {
	Item model.Track `json:"item"`
}

// LTSuggestionParams selects one pending suggestion.
type LTSuggestionParams struct {
	ID string `json:"id"`
}

// LyricsParams identifies a track for lyric resolution.
type LyricsParams struct {
	VideoID  string   `json:"videoId"`
	Title    string   `json:"title"`
	Artists  string   `json:"artists"`
	Album    *string  `json:"album,omitempty"`
	Duration *float64 `json:"duration,omitempty"`
}

// URLParams carries a web URL.
type URLParams struct {
	URL string `json:"url"`
}

// SetDownloadSettingsParams replaces download settings.
type SetDownloadSettingsParams struct {
	Settings model.DownloadSettings `json:"settings"`
}

// EnqueueDownloadParams admits one track download.
type EnqueueDownloadParams struct {
	VideoID   string  `json:"videoId"`
	Title     string  `json:"title"`
	Artists   *string `json:"artists,omitempty"`
	Thumbnail *string `json:"thumbnail,omitempty"`
}

// EnqueueCollectionParams admits a collection of downloads.
type EnqueueCollectionParams struct {
	Entries []model.DownloadCollectionEntry `json:"entries"`
}

// DownloadIDParams selects one download.
type DownloadIDParams struct {
	ID string `json:"id"`
}

// GetLogsParams filters diagnostics.
type GetLogsParams struct {
	Level  *string `json:"level,omitempty"`
	Source *string `json:"source,omitempty"`
	Query  *string `json:"query,omitempty"`
	Limit  *int    `json:"limit,omitempty"`
}

// LogClientErrorParams records one client-side diagnostic.
type LogClientErrorParams struct {
	Message string  `json:"message"`
	Target  *string `json:"target,omitempty"`
	Level   *string `json:"level,omitempty"`
}

// InstallUpdateParams selects the already-checked release version.
type InstallUpdateParams struct {
	Version string `json:"version"`
}

// RawResult is an explicitly unstructured compatibility result.
type RawResult = json.RawMessage
