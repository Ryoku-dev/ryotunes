// Register every application command with Tauri's runtime authority. Without an AppManifest,
// application commands registered through invoke_handler are allowed to every WebView by default,
// including the hidden cipher/PoToken runtimes and the remote Google sign-in surface.
const APP_COMMANDS: &[&str] = &[
    "search",
    "search_page",
    "search_page_more",
    "search_all",
    "search_all_more",
    "search_cards",
    "search_cards_page",
    "search_cards_more",
    "play",
    "prefetch_stream",
    "play_index",
    "remove_from_queue",
    "clear_queued",
    "add_to_queue",
    "move_in_queue",
    "play_next",
    "next_track",
    "prev_track",
    "toggle_shuffle",
    "set_repeat",
    "set_stop_after_current",
    "toggle_pause",
    "seek",
    "set_volume",
    "set_playback_params",
    "get_queue",
    "get_playback",
    "frontend_ready",
    "get_settings",
    "check_for_updates",
    "install_update",
    "discord_status",
    "ryoku_theme_tokens",
    "set_setting",
    "get_personal",
    "set_personal",
    "get_stream_clients",
    "clear_caches",
    "get_account",
    "get_account_identities",
    "switch_account",
    "sign_out",
    "login_webview",
    "open_mini",
    "close_mini",
    "get_home",
    "get_home_more",
    "get_library",
    "get_library_albums",
    "get_library_artists",
    "get_playlist",
    "get_playlist_more",
    "playlist_index",
    "sync_playlist_index",
    "play_counts",
    "listening_stats",
    "get_album",
    "get_local_library",
    "add_local_folder",
    "remove_local_folder",
    "get_artist",
    "get_browse_grid",
    "play_playlist",
    "start_radio",
    "radio_stations",
    "play_radio_station",
    "export_playlist_file",
    "import_playlist_file",
    "rate",
    "set_album_saved",
    "add_to_playlist",
    "add_to_local_playlist",
    "remove_from_playlist",
    "create_playlist",
    "edit_playlist_details",
    "set_playlist_cover",
    "set_playlist_sort",
    "delete_playlist",
    "subscribe",
    "lt_get_state",
    "lt_set_server_url",
    "lt_create_room",
    "lt_join_room",
    "lt_leave",
    "lt_approve_join",
    "lt_reject_join",
    "lt_kick",
    "lt_transfer_host",
    "lt_suggest",
    "lt_approve_suggestion",
    "lt_reject_suggestion",
    "lt_request_sync",
    "get_lyrics",
    "lastfm_connect",
    "lastfm_disconnect",
    "lastfm_status",
    "open_external",
];

fn main() {
    // Last.fm API credentials live in a gitignored `lastfm.keys` next to this file (the repo is
    // public — they must never be tracked). Format: `RYOTUNES_LASTFM_API_KEY=…` and
    // `RYOTUNES_LASTFM_API_SECRET=…`, one per line. Exported as compile-time env for `option_env!`
    // in lastfm.rs; missing file just means the scrobbler reports "not configured".
    println!("cargo:rerun-if-changed=lastfm.keys");
    if let Ok(keys) = std::fs::read_to_string("lastfm.keys") {
        for line in keys.lines() {
            if let Some((k, v)) = line.split_once('=') {
                let (k, v) = (k.trim(), v.trim());
                if k == "RYOTUNES_LASTFM_API_KEY" || k == "RYOTUNES_LASTFM_API_SECRET" {
                    println!("cargo:rustc-env={k}={v}");
                }
            }
        }
    }
    tauri_build::try_build(
        tauri_build::Attributes::new()
            .app_manifest(tauri_build::AppManifest::new().commands(APP_COMMANDS)),
    )
    .expect("Tauri build configuration failed");
}
