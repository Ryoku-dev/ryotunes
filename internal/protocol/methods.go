package protocol

import (
	"reflect"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

const (
	// MethodAddLocalFolder adds and scans a local music folder.
	MethodAddLocalFolder = "add_local_folder"
	// MethodAddToLocalPlaylist adds a track to a device playlist.
	MethodAddToLocalPlaylist = "add_to_local_playlist"
	// MethodAddToPlaylist adds a provider track to a playlist.
	MethodAddToPlaylist = "add_to_playlist"
	// MethodAddToQueue appends tracks to the manual queue.
	MethodAddToQueue = "add_to_queue"
	// MethodCancelDownload cancels an active or queued download.
	MethodCancelDownload = "cancel_download"
	// MethodCheckForUpdates checks the latest packaged release.
	MethodCheckForUpdates = "check_for_updates"
	// MethodClearCaches clears playback and lyrics caches.
	MethodClearCaches = "clear_caches"
	// MethodClearDownloadHistory removes terminal download rows.
	MethodClearDownloadHistory = "clear_download_history"
	// MethodClearQueued removes manually queued tracks.
	MethodClearQueued = "clear_queued"
	// MethodCreatePlaylist creates a provider or device playlist.
	MethodCreatePlaylist = "create_playlist"
	// MethodDeletePlaylist deletes a playlist.
	MethodDeletePlaylist = "delete_playlist"
	// MethodDiscordStatus returns Discord RPC state.
	MethodDiscordStatus = "discord_status"
	// MethodEditPlaylistDetails edits playlist metadata.
	MethodEditPlaylistDetails = "edit_playlist_details"
	// MethodEnqueueCollection queues a collection for download.
	MethodEnqueueCollection = "enqueue_collection"
	// MethodEnqueueDownload queues one track for download.
	MethodEnqueueDownload = "enqueue_download"
	// MethodExportPlaylistFile writes a portable playlist file.
	MethodExportPlaylistFile = "export_playlist_file"
	// MethodGetAccount returns the YouTube account snapshot.
	MethodGetAccount = "get_account"
	// MethodGetAccountIdentities returns switchable YouTube identities.
	MethodGetAccountIdentities = "get_account_identities"
	// MethodGetAlbum returns an album page.
	MethodGetAlbum = "get_album"
	// MethodGetArtist returns an artist page.
	MethodGetArtist = "get_artist"
	// MethodGetAudioFx returns playback effects.
	MethodGetAudioFx = "get_audio_fx"
	// MethodGetBrowseGrid returns one provider browse grid.
	MethodGetBrowseGrid = "get_browse_grid"
	// MethodGetDownloads returns the full download snapshot.
	MethodGetDownloads = "get_downloads"
	// MethodGetDownloadSettings returns download settings.
	MethodGetDownloadSettings = "get_download_settings"
	// MethodGetHome returns the provider home feed.
	MethodGetHome = "get_home"
	// MethodGetHomeMore continues a home feed.
	MethodGetHomeMore = "get_home_more"
	// MethodGetLibrary returns playlist library cards.
	MethodGetLibrary = "get_library"
	// MethodGetLibraryAlbums returns album library cards.
	MethodGetLibraryAlbums = "get_library_albums"
	// MethodGetLibraryArtists returns artist library cards.
	MethodGetLibraryArtists = "get_library_artists"
	// MethodGetLocalLibrary scans and returns the local library.
	MethodGetLocalLibrary = "get_local_library"
	// MethodGetLogs returns filtered diagnostics.
	MethodGetLogs = "get_logs"
	// MethodGetLyrics resolves lyrics for a track.
	MethodGetLyrics = "get_lyrics"
	// MethodGetPersonal returns renderer-owned personalization.
	MethodGetPersonal = "get_personal"
	// MethodGetPlayback returns the player snapshot.
	MethodGetPlayback = "get_playback"
	// MethodGetPlaylist returns a playlist page.
	MethodGetPlaylist = "get_playlist"
	// MethodGetPlaylistMore continues a playlist page.
	MethodGetPlaylistMore = "get_playlist_more"
	// MethodGetProvider returns provider selection.
	MethodGetProvider = "get_provider"
	// MethodGetQueue returns the complete queue.
	MethodGetQueue = "get_queue"
	// MethodGetSCUser returns a SoundCloud user page.
	MethodGetSCUser = "get_sc_user"
	// MethodGetSettings returns legacy string settings.
	MethodGetSettings = "get_settings"
	// MethodGetStreamClients returns YouTube stream clients.
	MethodGetStreamClients = "get_stream_clients"
	// MethodGetWaveform returns a normalized SoundCloud waveform.
	MethodGetWaveform = "get_waveform"
	// MethodHello performs the protocol handshake.
	MethodHello = "hello"
	// MethodImportPlaylistFile reads a portable playlist file.
	MethodImportPlaylistFile = "import_playlist_file"
	// MethodInstallUpdate installs a checked packaged release.
	MethodInstallUpdate = "install_update"
	// MethodLastfmConnect begins Last.fm authorization.
	MethodLastfmConnect = "lastfm_connect"
	// MethodLastfmDisconnect clears Last.fm authorization.
	MethodLastfmDisconnect = "lastfm_disconnect"
	// MethodLastfmStatus returns Last.fm state.
	MethodLastfmStatus = "lastfm_status"
	// MethodListeningStats returns period listening aggregates.
	MethodListeningStats = "listening_stats"
	// MethodLogClientError records one client diagnostic.
	MethodLogClientError = "log_client_error"
	// MethodLTApproveJoin approves a pending room join.
	MethodLTApproveJoin = "lt_approve_join"
	// MethodLTApproveSuggestion approves a track suggestion.
	MethodLTApproveSuggestion = "lt_approve_suggestion"
	// MethodLTCreateRoom creates a Listen Together room.
	MethodLTCreateRoom = "lt_create_room"
	// MethodLTGetState returns Listen Together state.
	MethodLTGetState = "lt_get_state"
	// MethodLTJoinRoom requests entry to a room.
	MethodLTJoinRoom = "lt_join_room"
	// MethodLTKick removes a room user.
	MethodLTKick = "lt_kick"
	// MethodLTLeave leaves the current room.
	MethodLTLeave = "lt_leave"
	// MethodLTRejectJoin rejects a pending room join.
	MethodLTRejectJoin = "lt_reject_join"
	// MethodLTRejectSuggestion rejects a track suggestion.
	MethodLTRejectSuggestion = "lt_reject_suggestion"
	// MethodLTRequestSync requests authoritative room state.
	MethodLTRequestSync = "lt_request_sync"
	// MethodLTSetServerURL updates the Listen Together relay.
	MethodLTSetServerURL = "lt_set_server_url"
	// MethodLTSuggest submits a guest track suggestion.
	MethodLTSuggest = "lt_suggest"
	// MethodLTTransferHost transfers room ownership.
	MethodLTTransferHost = "lt_transfer_host"
	// MethodMoveInQueue reorders one queue row.
	MethodMoveInQueue = "move_in_queue"
	// MethodNextTrack advances the queue.
	MethodNextTrack = "next_track"
	// MethodOpenDownload opens a completed download's folder.
	MethodOpenDownload = "open_download"
	// MethodOpenDownloadFolder opens the configured download folder.
	MethodOpenDownloadFolder = "open_download_folder"
	// MethodOpenExternal opens a safe web link.
	MethodOpenExternal = "open_external"
	// MethodOpenLogsFolder opens the diagnostics folder.
	MethodOpenLogsFolder = "open_logs_folder"
	// MethodPlay starts one track.
	MethodPlay = "play"
	// MethodPlayCounts returns trailing play counts.
	MethodPlayCounts = "play_counts"
	// MethodPlayIndex starts a queue row.
	MethodPlayIndex = "play_index"
	// MethodPlaylistIndex returns cached playlist memberships.
	MethodPlaylistIndex = "playlist_index"
	// MethodPlayNext inserts tracks after the current row.
	MethodPlayNext = "play_next"
	// MethodPlayPlaylist replaces the queue with a collection.
	MethodPlayPlaylist = "play_playlist"
	// MethodPlayRadioStation starts a live radio station.
	MethodPlayRadioStation = "play_radio_station"
	// MethodPrefetchStream resolves a stream in advance.
	MethodPrefetchStream = "prefetch_stream"
	// MethodPrevTrack moves to the previous queue row.
	MethodPrevTrack = "prev_track"
	// MethodQuit shuts the daemon down.
	MethodQuit = "quit"
	// MethodRadioStations searches the internet radio directory.
	MethodRadioStations = "radio_stations"
	// MethodRate changes one track rating.
	MethodRate = "rate"
	// MethodRemoveFromPlaylist removes a playlist row.
	MethodRemoveFromPlaylist = "remove_from_playlist"
	// MethodRemoveFromQueue removes a queue row.
	MethodRemoveFromQueue = "remove_from_queue"
	// MethodRemoveLocalFolder removes a local scan root.
	MethodRemoveLocalFolder = "remove_local_folder"
	// MethodRetryDownload retries a terminal download.
	MethodRetryDownload = "retry_download"
	// MethodRyokuThemeTokens is a client-side compatibility method.
	MethodRyokuThemeTokens = "ryoku_theme_tokens"
	// MethodSearch searches tracks.
	MethodSearch = "search"
	// MethodSearchAll searches mixed result categories.
	MethodSearchAll = "search_all"
	// MethodSearchAllMore continues mixed search.
	MethodSearchAllMore = "search_all_more"
	// MethodSearchCards searches one card category.
	MethodSearchCards = "search_cards"
	// MethodSearchCardsMore continues a card search.
	MethodSearchCardsMore = "search_cards_more"
	// MethodSearchCardsPage starts a card search page.
	MethodSearchCardsPage = "search_cards_page"
	// MethodSearchPage starts a provider track search page.
	MethodSearchPage = "search_page"
	// MethodSearchPageMore continues a track search page.
	MethodSearchPageMore = "search_page_more"
	// MethodSeek changes playback position.
	MethodSeek = "seek"
	// MethodSetAlbumSaved changes album library membership.
	MethodSetAlbumSaved = "set_album_saved"
	// MethodSetArtistSubscribed changes an artist channel subscription.
	MethodSetArtistSubscribed = "set_artist_subscribed"
	// MethodSetAudioFx replaces playback effects.
	MethodSetAudioFx = "set_audio_fx"
	// MethodSetDownloadSettings replaces download policy.
	MethodSetDownloadSettings = "set_download_settings"
	// MethodSetPersonal replaces renderer-owned personalization.
	MethodSetPersonal = "set_personal"
	// MethodSetPlaybackParams changes tempo and pitch.
	MethodSetPlaybackParams = "set_playback_params"
	// MethodSetPlaylistCover selects or clears playlist art.
	MethodSetPlaylistCover = "set_playlist_cover"
	// MethodSetPlaylistSort changes provider playlist order.
	MethodSetPlaylistSort = "set_playlist_sort"
	// MethodSetProvider changes provider selection.
	MethodSetProvider = "set_provider"
	// MethodSetRepeat changes repeat mode.
	MethodSetRepeat = "set_repeat"
	// MethodSetSetting writes one renderer-visible setting.
	MethodSetSetting = "set_setting"
	// MethodSetStopAfterCurrent changes the one-shot stop flag.
	MethodSetStopAfterCurrent = "set_stop_after_current"
	// MethodSetVolume changes volume percent.
	MethodSetVolume = "set_volume"
	// MethodShow raises or launches the client.
	MethodShow = "show"
	// MethodSignIn begins YouTube authorization.
	MethodSignIn = "sign_in"
	// MethodSignOut clears YouTube authorization.
	MethodSignOut = "sign_out"
	// MethodSoundCloudSignIn begins SoundCloud authorization.
	MethodSoundCloudSignIn = "soundcloud_sign_in"
	// MethodSoundCloudSignOut clears SoundCloud authorization.
	MethodSoundCloudSignOut = "soundcloud_sign_out"
	// MethodSoundCloudStatus returns SoundCloud account state.
	MethodSoundCloudStatus = "soundcloud_status"
	// MethodSpotifySignIn begins Spotify authorization.
	MethodSpotifySignIn = "spotify_sign_in"
	// MethodSpotifySignOut clears Spotify authorization.
	MethodSpotifySignOut = "spotify_sign_out"
	// MethodSpotifyStatus returns Spotify account state.
	MethodSpotifyStatus = "spotify_status"
	// MethodStartRadio starts provider radio.
	MethodStartRadio = "start_radio"
	// MethodSubscribe registers for events and returns a snapshot.
	MethodSubscribe = "subscribe"
	// MethodSwitchAccount switches YouTube identity.
	MethodSwitchAccount = "switch_account"
	// MethodSyncPlaylistIndex refreshes playlist memberships.
	MethodSyncPlaylistIndex = "sync_playlist_index"
	// MethodTidalSetCredentials updates TIDAL flow credentials.
	MethodTidalSetCredentials = "tidal_set_credentials"
	// MethodTidalSignIn begins TIDAL authorization.
	MethodTidalSignIn = "tidal_sign_in"
	// MethodTidalSignOut clears TIDAL authorization.
	MethodTidalSignOut = "tidal_sign_out"
	// MethodTidalStatus returns TIDAL account state.
	MethodTidalStatus = "tidal_status"
	// MethodTogglePause toggles playback pause.
	MethodTogglePause = "toggle_pause"
	// MethodToggleShuffle toggles queue shuffle.
	MethodToggleShuffle = "toggle_shuffle"
)

// Method describes one typed RPC endpoint.
type Method struct {
	Name   string
	Params reflect.Type
	Result reflect.Type
	Doc    string
}

// Methods is the complete version-one RPC registry.
var Methods = []Method{
	def[PathParams, model.LocalLibrary](MethodAddLocalFolder, "Add and scan a local music folder."),
	def[AddToLocalPlaylistParams, bool](MethodAddToLocalPlaylist, "Add a track to a device playlist."),
	def[PlaylistTrackParams, *bool](MethodAddToPlaylist, "Add a track to a provider playlist."),
	def[AddToQueueParams, Null](MethodAddToQueue, "Append tracks to the manual queue."),
	def[DownloadIDParams, Null](MethodCancelDownload, "Cancel a queued or active download."),
	def[Null, model.UpdateInfo](MethodCheckForUpdates, "Check the latest packaged release."),
	def[ClearCachesParams, ClearCachesResult](MethodClearCaches, "Clear playback and lyrics caches."),
	def[Null, Null](MethodClearDownloadHistory, "Remove terminal download history."),
	def[Null, Null](MethodClearQueued, "Remove manually queued tracks."),
	def[CreatePlaylistParams, PlaylistIDResult](MethodCreatePlaylist, "Create a playlist."),
	def[PlaylistIDParams, Null](MethodDeletePlaylist, "Delete a playlist."),
	def[Null, model.DiscordStatus](MethodDiscordStatus, "Return Discord RPC state."),
	def[EditPlaylistDetailsParams, Null](MethodEditPlaylistDetails, "Edit playlist metadata."),
	def[EnqueueCollectionParams, model.DownloadCollectionResult](MethodEnqueueCollection, "Queue a collection for download."),
	def[EnqueueDownloadParams, model.DownloadEntry](MethodEnqueueDownload, "Queue one track for download."),
	def[ExportPlaylistFileParams, bool](MethodExportPlaylistFile, "Write a portable playlist file."),
	def[Null, model.Account](MethodGetAccount, "Return the YouTube account snapshot."),
	def[Null, model.AccountIdentities](MethodGetAccountIdentities, "Return selectable YouTube identities."),
	def[IDParams, model.AlbumPage](MethodGetAlbum, "Return an album page."),
	def[IDParams, model.ArtistPage](MethodGetArtist, "Return an artist page."),
	def[Null, model.AudioFx](MethodGetAudioFx, "Return playback effects."),
	def[BrowseGridParams, []model.BrowseItem](MethodGetBrowseGrid, "Return a browse grid."),
	def[Null, model.DownloadSnapshot](MethodGetDownloads, "Return the download snapshot."),
	def[Null, model.DownloadSettings](MethodGetDownloadSettings, "Return download settings."),
	def[HomeParams, model.Feed](MethodGetHome, "Return the provider home feed."),
	def[TokenParams, model.Feed](MethodGetHomeMore, "Continue a home feed."),
	def[Null, []model.BrowseItem](MethodGetLibrary, "Return playlist library cards."),
	def[Null, []model.BrowseItem](MethodGetLibraryAlbums, "Return album library cards."),
	def[Null, []model.BrowseItem](MethodGetLibraryArtists, "Return artist library cards."),
	def[Null, model.LocalLibrary](MethodGetLocalLibrary, "Scan and return the local library."),
	def[GetLogsParams, model.LogsResult](MethodGetLogs, "Return filtered diagnostics."),
	def[LyricsParams, *model.Lyrics](MethodGetLyrics, "Resolve lyrics for a track."),
	def[Null, PersonalResult](MethodGetPersonal, "Return renderer-owned personalization."),
	def[Null, model.PlaybackSnapshot](MethodGetPlayback, "Return the player snapshot."),
	def[PlaylistParams, model.PlaylistPage](MethodGetPlaylist, "Return a playlist page."),
	def[TokenParams, model.PlaylistTracksPage](MethodGetPlaylistMore, "Continue a playlist page."),
	def[Null, ProviderResult](MethodGetProvider, "Return provider selection."),
	def[Null, model.QueueSnapshot](MethodGetQueue, "Return the complete queue."),
	def[IDParams, model.ArtistPage](MethodGetSCUser, "Return a SoundCloud user page."),
	def[Null, model.SettingsSnapshot](MethodGetSettings, "Return legacy string settings."),
	def[Null, []model.StreamClient](MethodGetStreamClients, "Return YouTube stream clients."),
	def[IDParams, model.Waveform](MethodGetWaveform, "Return a normalized SoundCloud waveform."),
	def[Null, Hello](MethodHello, "Perform the protocol handshake."),
	def[PathParams, ImportPlaylistFileResult](MethodImportPlaylistFile, "Read a portable playlist file."),
	def[InstallUpdateParams, model.UpdateResult](MethodInstallUpdate, "Install a checked packaged release."),
	def[Null, Null](MethodLastfmConnect, "Begin Last.fm authorization."),
	def[Null, Null](MethodLastfmDisconnect, "Clear Last.fm authorization."),
	def[Null, model.LastfmStatus](MethodLastfmStatus, "Return Last.fm state."),
	def[ListeningStatsParams, model.ListeningStats](MethodListeningStats, "Return period listening aggregates."),
	def[LogClientErrorParams, Null](MethodLogClientError, "Record one client diagnostic."),
	def[LTUserParams, Null](MethodLTApproveJoin, "Approve a pending room join."),
	def[LTSuggestionParams, Null](MethodLTApproveSuggestion, "Approve a track suggestion."),
	def[LTCreateRoomParams, Null](MethodLTCreateRoom, "Create a Listen Together room."),
	def[Null, model.LTState](MethodLTGetState, "Return Listen Together state."),
	def[LTJoinRoomParams, Null](MethodLTJoinRoom, "Request entry to a room."),
	def[LTUserParams, Null](MethodLTKick, "Remove a room user."),
	def[Null, Null](MethodLTLeave, "Leave the current room."),
	def[LTUserParams, Null](MethodLTRejectJoin, "Reject a pending room join."),
	def[LTSuggestionParams, Null](MethodLTRejectSuggestion, "Reject a track suggestion."),
	def[Null, Null](MethodLTRequestSync, "Request authoritative room state."),
	def[LTServerURLParams, Null](MethodLTSetServerURL, "Update the Listen Together relay."),
	def[LTSuggestParams, Null](MethodLTSuggest, "Submit a guest track suggestion."),
	def[LTUserParams, Null](MethodLTTransferHost, "Transfer room ownership."),
	def[MoveInQueueParams, Null](MethodMoveInQueue, "Reorder one queue row."),
	def[Null, Null](MethodNextTrack, "Advance the queue."),
	def[DownloadIDParams, Null](MethodOpenDownload, "Open a completed download folder."),
	def[Null, Null](MethodOpenDownloadFolder, "Open the configured download folder."),
	def[URLParams, Null](MethodOpenExternal, "Open a safe web link."),
	def[Null, Null](MethodOpenLogsFolder, "Open the diagnostics folder."),
	def[PlayParams, Null](MethodPlay, "Start one track."),
	def[Null, model.PlayCounts](MethodPlayCounts, "Return trailing play counts."),
	def[IndexParams, Null](MethodPlayIndex, "Start a queue row."),
	def[Null, model.PlaylistIndex](MethodPlaylistIndex, "Return cached playlist memberships."),
	def[PlayNextParams, Null](MethodPlayNext, "Insert tracks after the current row."),
	def[PlayPlaylistParams, Null](MethodPlayPlaylist, "Replace the queue with a collection."),
	def[PlayRadioStationParams, Null](MethodPlayRadioStation, "Start a live radio station."),
	def[PrefetchStreamParams, Null](MethodPrefetchStream, "Resolve a stream in advance."),
	def[Null, Null](MethodPrevTrack, "Move to the previous queue row."),
	def[Null, Null](MethodQuit, "Shut the daemon down."),
	def[RadioStationsParams, []model.RadioStation](MethodRadioStations, "Search the internet radio directory."),
	def[RateParams, Null](MethodRate, "Change one track rating."),
	def[RemoveFromPlaylistParams, Null](MethodRemoveFromPlaylist, "Remove a playlist row."),
	def[IndexParams, Null](MethodRemoveFromQueue, "Remove a queue row."),
	def[PathParams, model.LocalLibrary](MethodRemoveLocalFolder, "Remove a local scan root."),
	def[DownloadIDParams, model.DownloadEntry](MethodRetryDownload, "Retry a terminal download."),
	def[Null, Null](MethodRyokuThemeTokens, "Report that palette tokens are client-owned."),
	def[QueryParams, []model.Track](MethodSearch, "Search tracks."),
	def[QueryParams, model.SearchAll](MethodSearchAll, "Search mixed result categories."),
	def[TokenParams, model.SearchAll](MethodSearchAllMore, "Continue mixed search."),
	def[SearchCardsParams, []model.BrowseItem](MethodSearchCards, "Search one card category."),
	def[TokenParams, model.CardsPage](MethodSearchCardsMore, "Continue a card search."),
	def[SearchCardsParams, model.CardsPage](MethodSearchCardsPage, "Start a card search page."),
	def[QueryParams, model.SearchPage](MethodSearchPage, "Start a provider track search page."),
	def[TokenParams, model.SearchPage](MethodSearchPageMore, "Continue a track search page."),
	def[SeekParams, Null](MethodSeek, "Change playback position."),
	def[SetAlbumSavedParams, Null](MethodSetAlbumSaved, "Change album library membership."),
	def[SetArtistSubscribedParams, SetArtistSubscribedResult](MethodSetArtistSubscribed, "Change an artist channel subscription."),
	def[model.AudioFx, Null](MethodSetAudioFx, "Replace playback effects."),
	def[SetDownloadSettingsParams, model.DownloadSettings](MethodSetDownloadSettings, "Replace download policy."),
	def[SetPersonalParams, Null](MethodSetPersonal, "Replace renderer-owned personalization."),
	def[model.PlaybackParams, Null](MethodSetPlaybackParams, "Change tempo and pitch."),
	def[SetPlaylistCoverParams, SetPlaylistCoverResult](MethodSetPlaylistCover, "Select or clear playlist art."),
	def[SetPlaylistSortParams, Null](MethodSetPlaylistSort, "Change provider playlist order."),
	def[ProviderParams, ProviderResult](MethodSetProvider, "Change provider selection."),
	def[SetRepeatParams, Null](MethodSetRepeat, "Change repeat mode."),
	def[SetSettingParams, Null](MethodSetSetting, "Write one renderer-visible setting."),
	def[SetStopAfterCurrentParams, Null](MethodSetStopAfterCurrent, "Change the one-shot stop flag."),
	def[SetVolumeParams, Null](MethodSetVolume, "Change volume percent."),
	def[Null, Null](MethodShow, "Raise or launch the client."),
	def[Null, AuthFlowResult](MethodSignIn, "Begin YouTube authorization and return its browser URL."),
	def[Null, Null](MethodSignOut, "Clear YouTube authorization."),
	def[Null, AuthFlowResult](MethodSoundCloudSignIn, "Begin SoundCloud authorization and return its browser URL."),
	def[Null, SignedOutResult](MethodSoundCloudSignOut, "Clear SoundCloud authorization."),
	def[Null, model.SoundCloudStatus](MethodSoundCloudStatus, "Return SoundCloud account state."),
	def[Null, StartedResult](MethodSpotifySignIn, "Begin Spotify authorization."),
	def[Null, SignedOutResult](MethodSpotifySignOut, "Clear Spotify authorization."),
	def[Null, model.SpotifyStatus](MethodSpotifyStatus, "Return Spotify account state."),
	def[StartRadioParams, Null](MethodStartRadio, "Start provider radio."),
	def[SubscribeParams, Snapshot](MethodSubscribe, "Register for events and return a snapshot."),
	def[SwitchAccountParams, model.Account](MethodSwitchAccount, "Switch YouTube identity."),
	def[Null, model.PlaylistIndex](MethodSyncPlaylistIndex, "Refresh playlist memberships."),
	def[TidalCredentialsParams, model.TidalStatus](MethodTidalSetCredentials, "Update TIDAL flow credentials."),
	def[Null, StartedResult](MethodTidalSignIn, "Begin TIDAL authorization."),
	def[Null, SignedOutResult](MethodTidalSignOut, "Clear TIDAL authorization."),
	def[Null, model.TidalStatus](MethodTidalStatus, "Return TIDAL account state."),
	def[Null, Null](MethodTogglePause, "Toggle playback pause."),
	def[Null, Null](MethodToggleShuffle, "Toggle queue shuffle."),
}

var methodsByName = func() map[string]Method {
	index := make(map[string]Method, len(Methods))
	for _, method := range Methods {
		index[method.Name] = method
	}
	return index
}()

// Lookup returns the typed method definition for a wire name.
func Lookup(name string) (Method, bool) {
	method, ok := methodsByName[name]
	return method, ok
}

func def[P, R any](name, doc string) Method {
	return Method{
		Name:   name,
		Params: reflect.TypeOf((*P)(nil)).Elem(),
		Result: reflect.TypeOf((*R)(nil)).Elem(),
		Doc:    doc,
	}
}
