# Protocol method map

`Methods` in `methods.go` is authoritative; names below are grouped only for daemon/client navigation.

## Lifecycle and diagnostics

- `hello`, `subscribe`, `show`, `quit`: handshake, snapshot/event subscription, window raise, and shutdown.
- `get_logs`, `log_client_error`, `open_logs_folder`: diagnostics query, client ingestion, and folder reveal.
- `check_for_updates`, `install_update`: packaged-release discovery and installation.
- `ryoku_theme_tokens`: compatibility call that returns `client_side`; the Gio client reads the palette directly.

## Playback and queue

- `play`, `play_playlist`, `play_index`, `next_track`, `prev_track`, `toggle_pause`, `seek`: direct playback control.
- `add_to_queue`, `play_next`, `remove_from_queue`, `move_in_queue`, `clear_queued`: manual queue editing.
- `toggle_shuffle`, `set_repeat`, `set_stop_after_current`: queue modes.
- `set_volume`, `set_playback_params`, `set_audio_fx`, `get_audio_fx`: volume, tempo/pitch, and effects.
- `get_equalizer`, `set_equalizer`: read or atomically replace the complete parametric equalizer. A successful set emits `equalizer` with the accepted normalized state.
- `list_equalizer_presets`, `save_equalizer_preset`, `delete_equalizer_preset`: built-in and user preset management. Built-in names cannot be replaced or deleted. Saving names the active curve and emits `equalizer`; deleting emits it when the active preset name is cleared.
- `import_equalizer`: apply Equalizer APO or AutoEQ text and return any clamp or compatibility warnings; a non-empty name also saves it as a user preset. Every successful import emits `equalizer`.
- `get_playback`, `get_queue`, `prefetch_stream`: state and stream warm-up.
- `start_radio`, `radio_stations`, `play_radio_station`, `get_waveform`: provider radio, internet radio, and SoundCloud waveform.

The subscribe snapshot carries the complete `equalizer`. Its `preset` member is
omitted when the active curve is custom. Equalizer bands are labelled in mpv's
audio-filter chain. mpv 0.41 does not forward `af-command` through its `lavfi`
wrapper on the shipped build, so the player uses asynchronous complete-chain
replacement.

## Browse, search, and library

- `get_home`, `get_home_more`: home feed and continuation.
- `search`, `search_page`, `search_page_more`: track search and continuation.
- `search_all`, `search_all_more`: mixed search and continuation.
- `search_cards`, `search_cards_page`, `search_cards_more`: album/artist/playlist card search.
- `get_album`, `get_artist`, `get_sc_user`, `get_browse_grid`: detail and grid browsing.
- `set_artist_subscribed`: subscribe or unsubscribe from a YouTube Music artist channel.
- `get_library`, `get_library_albums`, `get_library_artists`: provider library cards.
- `get_local_library`, `add_local_folder`, `remove_local_folder`: local scan roots and catalogue.
- `get_playlist`, `get_playlist_more`, `playlist_index`, `sync_playlist_index`: playlist reads and membership index.
- `create_playlist`, `edit_playlist_details`, `delete_playlist`, `set_playlist_cover`, `set_playlist_sort`: playlist metadata writes.
- `add_to_playlist`, `add_to_local_playlist`, `remove_from_playlist`, `set_album_saved`, `rate`: playlist/library membership and ratings.
- `export_playlist_file`, `import_playlist_file`: portable JSON playlist transfer.
- `play_counts`, `listening_stats`: local listening history aggregates.

## Settings, personal data, and account

- `get_settings`, `set_setting`, `get_stream_clients`, `clear_caches`: string settings and playback caches.
- `get_personal`, `set_personal`: opaque renderer-owned personalization.
- `get_account`, `get_account_identities`, `switch_account`, `sign_out`: YouTube account lifecycle.
- `get_provider`, `set_provider`: active catalogue selection.

## Provider authentication

The client owns every browser handoff; the daemon never launches a browser.
`sign_in` and `soundcloud_sign_in` return `{"url": string}`. The URL is the
provider sign-in page, or empty when stored cookies already completed sign-in.
The cookie import continues watching after a non-empty URL is returned.

- `sign_in`: YouTube authorization; returns `https://music.youtube.com` when browser input is needed.
- `spotify_status`, `spotify_sign_in`, `spotify_sign_out`: Spotify account lifecycle. `spotify-auth` with `state: "url"` carries the PKCE URL; another sign-in request while pending re-emits the same event.
- `soundcloud_status`, `soundcloud_sign_in`, `soundcloud_sign_out`: SoundCloud account lifecycle. Browser input uses `https://soundcloud.com/signin`.
- `tidal_status`, `tidal_set_credentials`, `tidal_sign_in`, `tidal_sign_out`: TIDAL credentials and account lifecycle. A `tidal-auth` waiting event carries `{"state":"waiting","url":string,"code":string}`; `url` is the complete verification URI when TIDAL supplies it, otherwise the verification URI.

## Lyrics and integrations

- `get_lyrics`: provider/local lyrics resolution.
- `discord_status`: Discord RPC state; enablement and presence name are settings.
- `lastfm_status`, `lastfm_connect`, `lastfm_disconnect`: Last.fm lifecycle.
- `open_external`: validated web-link opening.
- `lt_get_state`, `lt_set_server_url`, `lt_create_room`, `lt_join_room`, `lt_leave`: Listen Together state and room lifecycle.
- `lt_approve_join`, `lt_reject_join`, `lt_kick`, `lt_transfer_host`: Listen Together membership control.
- `lt_suggest`, `lt_approve_suggestion`, `lt_reject_suggestion`, `lt_request_sync`: Listen Together suggestions and synchronization.

## Downloads

- `get_download_settings`, `set_download_settings`, `get_downloads`: download policy and snapshot.
- `enqueue_download`, `enqueue_collection`, `cancel_download`, `retry_download`: download admission and control.
- `clear_download_history`, `open_download`, `open_download_folder`: terminal history and filesystem reveal.

## Names that are not methods

`day`, `month`, and `week` are `listening_stats.period` values. `off`, `all`, and `one` are repeat modes. `volume`, `quality`, `proxy`, `ui_scale`, `disabled_stream_clients`, and `discord_presence_name` are `set_setting.key` values. They are intentionally absent from `Methods`.
