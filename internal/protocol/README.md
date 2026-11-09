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
- `get_playback`, `get_queue`, `prefetch_stream`: state and stream warm-up.
- `start_radio`, `radio_stations`, `play_radio_station`, `get_waveform`: provider radio, internet radio, and SoundCloud waveform.

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
- `get_account`, `get_account_identities`, `switch_account`, `sign_in`, `sign_out`: YouTube account lifecycle.
- `get_provider`, `set_provider`: active catalogue selection.

## Provider authentication

- `spotify_status`, `spotify_sign_in`, `spotify_sign_out`: Spotify account lifecycle.
- `soundcloud_status`, `soundcloud_sign_in`, `soundcloud_sign_out`: SoundCloud account lifecycle.
- `tidal_status`, `tidal_set_credentials`, `tidal_sign_in`, `tidal_sign_out`: TIDAL credentials and account lifecycle.

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
