//! Every socket method. Names, parameter keys and result shapes are the Tauri commands'
//! (`src-tauri/src/commands.rs`); parameter objects use the same camelCase keys the Svelte
//! `ui/src/lib/api.ts` sends. The bodies here are ports of those command bodies: the seams that
//! needed Tauri (native file/folder dialogs) take an explicit `path` over the socket instead, and
//! the ones that only used the `AppHandle` for asset-scope allow-listing drop it (the daemon serves
//! no webview assets). Plus the control methods `hello`, `subscribe`, `quit` and `sign_in`.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use innertube::{BrowseItem, PlaylistPage, PlaylistSort, Rating, SongItem, YouTubeClient};
use ryotunes_core::db::LocalPlaylist;
use ryotunes_core::soundcloud_bridge;
use ryotunes_core::spotify::{
    sc_playlist_id, sc_system_id, sc_track_id, sc_user_id, spotify_track_id, Provider,
};
use ryotunes_core::spotify_bridge;
use ryotunes_core::state::{
    is_local_playlist_id, is_smart_playlist_id, song_to_track, AppState, RepeatMode,
    LOCAL_PLAYLIST_PREFIX, ON_REPEAT_ID, ON_REPEAT_LIMIT, ON_REPEAT_WINDOW_SECS,
    RECENTLY_PLAYED_ID, RECENTLY_PLAYED_WINDOW_SECS, REDISCOVER_ID, REDISCOVER_OLDER_THAN_SECS,
    SMART_PLAYLIST_LIMIT, UI_SETTINGS,
};
use ryotunes_core::{local, radio};
use ryotunes_protocol::{ErrorBody, PROTOCOL_VERSION};
use serde::de::DeserializeOwned;
use serde_json::{json, Value};

use crate::server::{Connection, Dispatch};

pub struct Methods {
    pub state: Arc<AppState>,
    pub quit: tokio::sync::mpsc::UnboundedSender<()>,
}

fn arg<T: DeserializeOwned>(params: &Value, key: &str) -> Result<T, ErrorBody> {
    serde_json::from_value(params.get(key).cloned().unwrap_or(Value::Null))
        .map_err(|e| ErrorBody { code: "bad_params".into(), message: format!("{key}: {e}") })
}

fn err(e: impl ToString) -> ErrorBody {
    ErrorBody { code: "failed".into(), message: e.to_string() }
}

fn ok<T: serde::Serialize>(v: T) -> Result<Value, ErrorBody> {
    serde_json::to_value(v).map_err(err)
}

fn null() -> Result<Value, ErrorBody> {
    Ok(Value::Null)
}

// --- Spotify routing --------------------------------------------------------------------------

/// Guards the sign-in flow so two OAuth attempts never run at once (it is fire-and-forget).
static SPOTIFY_SIGNING_IN: AtomicBool = AtomicBool::new(false);
/// The authorization URL of the flow in progress, so a repeat `spotify_sign_in` (the user clicked
/// again because the browser never opened, or closed it) reopens the same page instead of
/// answering "started" and doing nothing visible.
static SPOTIFY_AUTH_URL: std::sync::Mutex<Option<String>> = std::sync::Mutex::new(None);
/// One `client.liked` page (mirrors the crate's private `LIKED_PAGE`); the liked-songs view pages
/// until a batch comes back short.
const SPOTIFY_LIKED_PAGE: usize = 100;

/// Resets [`SPOTIFY_SIGNING_IN`] whenever the spawned flow ends — success, failure or panic.
struct SignInGuard;
impl Drop for SignInGuard {
    fn drop(&mut self) {
        SPOTIFY_SIGNING_IN.store(false, Ordering::SeqCst);
        *SPOTIFY_AUTH_URL.lock().unwrap_or_else(|e| e.into_inner()) = None;
    }
}

/// A Spotify crate error as the daemon's error body.
fn spotify_err(e: impl ToString) -> ErrorBody {
    ErrorBody { code: "spotify".into(), message: e.to_string() }
}

/// The error a by-prefix Spotify command returns when no session is signed in.
fn spotify_signed_out() -> ErrorBody {
    ErrorBody { code: "spotify_signed_out".into(), message: "Sign in to Spotify".into() }
}

/// A SoundCloud crate error as the daemon's error body.
fn soundcloud_err(e: impl ToString) -> ErrorBody {
    ErrorBody { code: "soundcloud".into(), message: e.to_string() }
}

#[async_trait::async_trait]
impl Dispatch for Methods {
    async fn call(
        &self,
        method: &str,
        params: Value,
        conn: &Connection,
    ) -> Result<Value, ErrorBody> {
        let st = &self.state;
        match method {
            // --- control methods -------------------------------------------------------------
            "hello" => {
                ok(json!({ "protocol": PROTOCOL_VERSION, "daemon": env!("CARGO_PKG_VERSION") }))
            }
            "subscribe" => {
                conn.subscribe();
                let provider = st.spotify.selected().as_str();
                let spotify = spotify_status_json(st).await;
                ok(json!({
                    "playback": st.playback_snapshot().await,
                    "queue": st.queue_snapshot().await,
                    "settings": st.settings_snapshot(),
                    "auth": st.account_snapshot(),
                    "audioFx": st.audio_fx(),
                    "provider": provider,
                    "spotify": spotify,
                }))
            }
            "quit" => {
                let _ = self.quit.send(());
                null()
            }
            "sign_in" => {
                st.sign_in().await;
                null()
            }
            "show" => {
                // Raise a subscribed client's window, or launch the client when none is listening.
                crate::tray::show(&conn.sink);
                null()
            }

            // --- search ----------------------------------------------------------------------
            "search" => ok(st.search(&arg::<String>(&params, "query")?).await.map_err(err)?),
            "search_page" => {
                let query = arg::<String>(&params, "query")?;
                if st.spotify.browsing_soundcloud() {
                    soundcloud_search_page(st, &query).await
                } else if st.spotify.browsing_spotify().await {
                    spotify_search_page(st, &query).await
                } else {
                    ok(st.search_page(&query).await.map_err(err)?)
                }
            }
            "search_page_more" => {
                if st.spotify.browsing_soundcloud() || st.spotify.browsing_spotify().await {
                    ok(json!({ "items": [], "continuation": null }))
                } else {
                    ok(st.search_page_more(&arg::<String>(&params, "token")?).await.map_err(err)?)
                }
            }
            "search_all" => {
                let query = arg::<String>(&params, "query")?;
                if st.spotify.browsing_soundcloud() {
                    soundcloud_search_all(st, &query).await
                } else if st.spotify.browsing_spotify().await {
                    spotify_search_all(st, &query).await
                } else {
                    ok(st.search_all(&query).await.map_err(err)?)
                }
            }
            "search_all_more" => {
                ok(st.search_all_more(&arg::<String>(&params, "token")?).await.map_err(err)?)
            }
            "search_cards" => {
                let query = arg::<String>(&params, "query")?;
                let category = arg::<String>(&params, "category")?;
                if st.spotify.browsing_soundcloud() {
                    soundcloud_search_cards(st, &query, &category).await
                } else if st.spotify.browsing_spotify().await {
                    spotify_search_cards(st, &query, &category).await
                } else {
                    ok(st.search_cards(&query, &category).await.map_err(err)?)
                }
            }
            "search_cards_page" => ok(st
                .search_cards_page(
                    &arg::<String>(&params, "query")?,
                    &arg::<String>(&params, "category")?,
                )
                .await
                .map_err(err)?),
            "search_cards_more" => {
                ok(st.search_cards_more(&arg::<String>(&params, "token")?).await.map_err(err)?)
            }

            // --- playback / queue ------------------------------------------------------------
            "play" => {
                st.clone().play_song(arg(&params, "item")?).await;
                null()
            }
            "prefetch_stream" => {
                let st2 = st.clone();
                let video_id = arg::<String>(&params, "videoId")?;
                let is_upload = arg::<Option<bool>>(&params, "isUpload")?.unwrap_or(false);
                tokio::spawn(async move { st2.prefetch_stream(video_id, is_upload).await });
                null()
            }
            "play_index" => {
                st.clone().play_index(arg(&params, "index")?).await;
                null()
            }
            "remove_from_queue" => {
                st.clone().remove_from_queue(arg(&params, "index")?).await;
                null()
            }
            "clear_queued" => {
                st.clone().clear_queued().await;
                null()
            }
            "add_to_queue" => {
                st.clone()
                    .add_to_queue(
                        arg(&params, "items")?,
                        arg(&params, "from")?,
                        arg(&params, "continuation")?,
                    )
                    .await;
                null()
            }
            "move_in_queue" => {
                st.clone().move_in_queue(arg(&params, "from")?, arg(&params, "to")?).await;
                null()
            }
            "play_next" => {
                st.clone().play_next(arg(&params, "items")?, arg(&params, "from")?).await;
                null()
            }
            "next_track" => {
                st.clone().next_in_queue().await;
                null()
            }
            "prev_track" => {
                st.clone().prev_in_queue().await;
                null()
            }
            "toggle_shuffle" => {
                st.clone().toggle_shuffle().await;
                null()
            }
            "set_repeat" => {
                let mode = match arg::<String>(&params, "mode")?.as_str() {
                    "off" => RepeatMode::Off,
                    "all" => RepeatMode::All,
                    "one" => RepeatMode::One,
                    other => return Err(err(format!("unknown repeat mode: {other}"))),
                };
                st.clone().set_repeat(mode).await;
                null()
            }
            "set_stop_after_current" => {
                st.clone().set_stop_after_current(arg(&params, "enabled")?).await;
                null()
            }
            "toggle_pause" => {
                st.clone().resume_or_toggle().await;
                null()
            }
            "seek" => {
                let position = arg::<f64>(&params, "position")?;
                if !position.is_finite() || position < 0.0 {
                    return Err(err("Seek position must be a non-negative finite number."));
                }
                st.user_seek(position).await.map_err(err)?;
                null()
            }
            "set_volume" => {
                st.set_volume(arg(&params, "volume")?).map_err(err)?;
                null()
            }
            "set_playback_params" => {
                st.set_playback_params(arg(&params, "speed")?, arg(&params, "semitones")?)
                    .map_err(err)?;
                null()
            }
            "set_audio_fx" => {
                let fx: player::AudioFx = serde_json::from_value(params.clone()).map_err(err)?;
                st.set_audio_fx(fx).map_err(err)?;
                null()
            }
            "get_audio_fx" => ok(st.audio_fx()),
            "get_queue" => ok(st.queue_snapshot().await),
            "get_playback" => ok(st.playback_snapshot().await),

            // --- settings --------------------------------------------------------------------
            "get_settings" => ok(st.settings_snapshot()),
            "discord_status" => ok(st.discord_status()),
            "set_setting" => set_setting(st, &params),
            "get_personal" => ok(json!({ "personal": st.personal() })),
            "set_personal" => {
                st.set_personal(arg::<Value>(&params, "personal")?).map_err(err)?;
                null()
            }
            "get_stream_clients" => {
                let mut v = vec![innertube::MAIN_CLIENT.to_string()];
                v.extend(innertube::STREAM_FALLBACK_ORDER.iter().map(|s| s.to_string()));
                for key in innertube::UPLOAD_FALLBACK_ORDER {
                    if !v.iter().any(|s| s == key) {
                        v.push(key.to_string());
                    }
                }
                ok(v)
            }
            "clear_caches" => {
                st.clear_caches();
                null()
            }

            // --- auth ------------------------------------------------------------------------
            "get_account" => ok(st.account_snapshot()),
            "get_account_identities" => ok(st.account_identities().await.map_err(err)?),
            "switch_account" => ok(st
                .switch_account(&arg::<String>(&params, "selectionKey")?)
                .await
                .map_err(err)?),
            "sign_out" => {
                st.clone().sign_out().await;
                null()
            }

            // --- provider + Spotify auth -----------------------------------------------------
            "set_provider" => {
                let provider = arg::<String>(&params, "provider")?;
                let selected = Provider::parse(&provider)
                    .ok_or_else(|| err(format!("unknown provider: {provider}")))?;
                st.db.set_setting("provider", selected.as_str());
                st.spotify.select(selected);
                st.emit("provider-changed", json!({ "provider": selected.as_str() }));
                ok(json!({ "provider": selected.as_str() }))
            }
            "get_provider" => ok(json!({ "provider": st.spotify.selected().as_str() })),
            "spotify_status" => ok(spotify_status_json(st).await),
            "spotify_sign_in" => {
                if SPOTIFY_SIGNING_IN
                    .compare_exchange(false, true, Ordering::SeqCst, Ordering::SeqCst)
                    .is_err()
                {
                    // A flow is already waiting on the browser: hand the same URL out again.
                    let url = SPOTIFY_AUTH_URL.lock().unwrap_or_else(|e| e.into_inner()).clone();
                    if let Some(url) = url {
                        st.emit("spotify-auth", json!({ "state": "url", "url": url }));
                    }
                    return ok(json!({ "started": true }));
                }
                let task = st.clone();
                tokio::spawn(async move {
                    let _guard = SignInGuard;
                    let emitter = task.clone();
                    let result = task
                        .spotify
                        .sign_in(move |url| {
                            *SPOTIFY_AUTH_URL.lock().unwrap_or_else(|e| e.into_inner()) =
                                Some(url.clone());
                            emitter.emit("spotify-auth", json!({ "state": "url", "url": url }));
                        })
                        .await;
                    match result {
                        Ok(()) => {
                            let name = spotify_name(&task).await;
                            task.emit(
                                "spotify-auth",
                                json!({ "state": "signed_in", "name": name }),
                            );
                        }
                        Err(e) => {
                            task.emit(
                                "spotify-auth",
                                json!({
                                    "state": "error",
                                    "message": format!("Spotify sign-in failed: {e:#}"),
                                }),
                            );
                        }
                    }
                });
                ok(json!({ "started": true }))
            }
            "spotify_sign_out" => {
                st.spotify.sign_out().await;
                st.emit("spotify-auth", json!({ "state": "signed_out" }));
                ok(json!({ "signedIn": false }))
            }

            // --- browse / library ------------------------------------------------------------
            "get_home" => {
                if st.spotify.browsing_soundcloud() {
                    soundcloud_home(st).await
                } else if st.spotify.browsing_spotify().await {
                    spotify_home(st).await
                } else {
                    ok(st
                        .home(arg::<Option<String>>(&params, "params")?.as_deref())
                        .await
                        .map_err(err)?)
                }
            }
            "get_home_more" => {
                if st.spotify.browsing_soundcloud() || st.spotify.browsing_spotify().await {
                    ok(json!({ "sections": [], "continuation": null }))
                } else {
                    ok(st.home_more(&arg::<String>(&params, "token")?).await.map_err(err)?)
                }
            }
            "get_library" => {
                if st.spotify.browsing_soundcloud() {
                    ok(Vec::<BrowseItem>::new())
                } else if st.spotify.browsing_spotify().await {
                    spotify_library(st).await
                } else {
                    get_library(st).await
                }
            }
            "get_library_albums" => {
                if st.spotify.browsing_soundcloud() {
                    ok(Vec::<BrowseItem>::new())
                } else if st.spotify.browsing_spotify().await {
                    spotify_library_albums(st).await
                } else {
                    ok(st.library_albums().await.map_err(err)?)
                }
            }
            "get_library_artists" => {
                if st.spotify.browsing_soundcloud() {
                    ok(Vec::<BrowseItem>::new())
                } else if st.spotify.browsing_spotify().await {
                    spotify_library_artists(st).await
                } else {
                    ok(st.library_artists().await.map_err(err)?)
                }
            }
            "get_playlist" => get_playlist(st, &params).await,
            "get_playlist_more" => {
                ok(st.playlist_more(&arg::<String>(&params, "token")?).await.map_err(err)?)
            }
            "playlist_index" => ok(st.db.playlist_memberships()),
            "sync_playlist_index" => sync_playlist_index(st).await,
            "play_counts" => ok(st
                .db
                .play_counts(now_secs() - ON_REPEAT_WINDOW_SECS)
                .into_iter()
                .collect::<std::collections::HashMap<String, i64>>()),
            "listening_stats" => listening_stats(st, &params),
            "get_album" => {
                let id = arg::<String>(&params, "id")?;
                if let Some(aid) = id.strip_prefix("spotify:album:") {
                    spotify_album(st, aid).await
                } else if let Some(pid) = sc_playlist_id(&id) {
                    soundcloud_album(st, pid).await
                } else {
                    ok(st.album(&id).await.map_err(err)?)
                }
            }
            "get_artist" => {
                let id = arg::<String>(&params, "id")?;
                if let Some(aid) = id.strip_prefix("spotify:artist:") {
                    spotify_artist(st, aid).await
                } else if let Some(uid) = sc_user_id(&id) {
                    soundcloud_artist(st, uid).await
                } else {
                    ok(st.artist(&id).await.map_err(err)?)
                }
            }
            "get_waveform" => {
                let tid = sc_track_id(&arg::<String>(&params, "id")?)
                    .ok_or_else(|| err("get_waveform expects a SoundCloud track id"))?;
                soundcloud_waveform(st, tid).await
            }
            "get_sc_user" => {
                let uid = sc_user_id(&arg::<String>(&params, "id")?)
                    .ok_or_else(|| err("get_sc_user expects a SoundCloud user id"))?;
                soundcloud_artist(st, uid).await
            }
            "get_browse_grid" => ok(st
                .browse_grid(
                    &arg::<String>(&params, "id")?,
                    arg::<Option<String>>(&params, "params")?.as_deref(),
                )
                .await
                .map_err(err)?),

            // --- local music (dialogs replaced by an explicit `path`) ------------------------
            "get_local_library" => ok(scan_local(st).await.map_err(err)?),
            "add_local_folder" => {
                let path = canonical_music_folder(&arg::<String>(&params, "path")?).map_err(err)?;
                local::add_folder(&st.db, path);
                ok(Some(scan_local(st).await.map_err(err)?))
            }
            "remove_local_folder" => {
                local::remove_folder(&st.db, &arg::<String>(&params, "path")?);
                ok(scan_local(st).await.map_err(err)?)
            }

            // --- playback of collections -----------------------------------------------------
            "play_playlist" => {
                let items = arg::<Vec<SongItem>>(&params, "items")?;
                let start = arg::<Option<usize>>(&params, "start")?;
                let source_id = arg::<Option<String>>(&params, "sourceId")?
                    .filter(|id| !is_local_playlist_id(id));
                let source_name = arg::<Option<String>>(&params, "sourceName")?;
                let shuffle = arg::<Option<bool>>(&params, "shuffle")?.unwrap_or(false);
                let continuation = arg::<Option<String>>(&params, "continuation")?;
                st.clone()
                    .play_tracks(items, start, source_id, source_name, shuffle, continuation)
                    .await;
                null()
            }
            "start_radio" => {
                let kind = arg::<String>(&params, "kind")?;
                let id = arg::<String>(&params, "id")?;
                let name = arg::<Option<String>>(&params, "name")?;
                if radio::is_radio_id(&id) || is_local_playlist_id(&id) {
                    return Err(err("This item does not have a YouTube Music radio seed."));
                }
                st.clone().start_radio(&kind, &id, name).await.map_err(err)?;
                null()
            }
            "radio_stations" => ok(radio::stations(
                arg::<Option<String>>(&params, "query")?.as_deref(),
                arg::<Option<usize>>(&params, "offset")?.unwrap_or(0),
                arg::<Option<usize>>(&params, "limit")?.unwrap_or(36),
            )
            .await
            .map_err(err)?),
            "play_radio_station" => {
                let station = radio::station_by_uuid(&arg::<String>(&params, "stationUuid")?)
                    .await
                    .map_err(err)?;
                st.clone().play_radio_station(station).await.map_err(err)?;
                null()
            }

            // --- portable playlist transfer (dialogs replaced by an explicit `path`) ---------
            "export_playlist_file" => export_playlist_file(&params),
            "import_playlist_file" => import_playlist_file(&params),

            // --- write actions ---------------------------------------------------------------
            "rate" => {
                let video_id = arg::<String>(&params, "videoId")?;
                let rating = arg::<Rating>(&params, "rating")?;
                if let Some(tid) = spotify_track_id(&video_id) {
                    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
                    client
                        .set_track_saved(tid, matches!(rating, Rating::Like))
                        .await
                        .map_err(spotify_err)?;
                    return null();
                }
                if radio::is_radio_id(&video_id) || local::is_local_song(&video_id) {
                    return Err(err("This track does not have a YouTube Music rating."));
                }
                let client = require_login(st).map_err(err)?;
                st.rate_epoch.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
                st.it.rate(client, &video_id, rating).await.map_err(err)?;
                null()
            }
            "set_album_saved" => {
                let playlist_id = arg::<String>(&params, "playlistId")?;
                let saved = arg::<bool>(&params, "saved")?;
                if let Some(aid) = playlist_id.strip_prefix("spotify:album:") {
                    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
                    client.set_album_saved(aid, saved).await.map_err(spotify_err)?;
                    return null();
                }
                let client = require_login(st).map_err(err)?;
                st.it.like_playlist(client, &playlist_id, saved).await.map_err(err)?;
                null()
            }
            "add_to_playlist" => {
                let playlist_id = arg::<String>(&params, "playlistId")?;
                let video_id = arg::<String>(&params, "videoId")?;
                if let Some(pid) = playlist_id.strip_prefix("spotify:playlist:") {
                    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
                    let tid = spotify_track_id(&video_id)
                        .ok_or_else(|| spotify_err("Not a Spotify track."))?;
                    if pid == "liked" {
                        client.set_track_saved(tid, true).await.map_err(spotify_err)?;
                    } else {
                        client.add_track_to_playlist(pid, tid).await.map_err(spotify_err)?;
                    }
                    return null();
                }
                if radio::is_radio_id(&video_id) {
                    return Err(err("Live radio stations cannot be added to song playlists."));
                }
                let client = editable_playlist(st, &playlist_id).map_err(err)?;
                let added =
                    st.it.playlist_add(client, &playlist_id, &video_id).await.map_err(err)?;
                st.db.add_playlist_track(&playlist_id, &video_id);
                ok(added)
            }
            "add_to_local_playlist" => {
                let playlist_id = arg::<String>(&params, "playlistId")?;
                let item = arg::<SongItem>(&params, "item")?;
                if !is_local_playlist_id(&playlist_id) {
                    return Err(err("not a device playlist"));
                }
                if radio::is_radio_id(&item.video_id) {
                    return Err(err("Live radio stations cannot be added to song playlists."));
                }
                let item = shed_queue_context(item);
                let json = serde_json::to_string(&item).map_err(err)?;
                ok(st
                    .db
                    .add_local_playlist_track(&playlist_id, &item.video_id, &json)
                    .map_err(|e| err(format!("device playlist: {e}")))?)
            }
            "remove_from_playlist" => {
                let playlist_id = arg::<String>(&params, "playlistId")?;
                let video_id = arg::<String>(&params, "videoId")?;
                if let Some(pid) = playlist_id.strip_prefix("spotify:playlist:") {
                    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
                    let tid = spotify_track_id(&video_id)
                        .ok_or_else(|| spotify_err("Not a Spotify track."))?;
                    if pid == "liked" {
                        client.set_track_saved(tid, false).await.map_err(spotify_err)?;
                    } else {
                        client.remove_track_from_playlist(pid, tid).await.map_err(spotify_err)?;
                    }
                    return null();
                }
                let set_video_id = arg::<String>(&params, "setVideoId")?;
                if is_local_playlist_id(&playlist_id) {
                    st.db
                        .remove_local_playlist_track(&playlist_id, &video_id)
                        .map_err(|e| err(format!("device playlist: {e}")))?;
                    return null();
                }
                let client = editable_playlist(st, &playlist_id).map_err(err)?;
                st.it
                    .playlist_remove(client, &playlist_id, &video_id, &set_video_id)
                    .await
                    .map_err(err)?;
                st.db.remove_playlist_track(&playlist_id, &video_id);
                null()
            }
            "create_playlist" => {
                let title = arg::<String>(&params, "title")?;
                let title = title.trim();
                if title.is_empty() {
                    return Err(err("Playlist name cannot be empty."));
                }
                let title: String = title.chars().take(150).collect();
                if st.spotify.browsing_spotify().await {
                    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
                    let id = client.create_playlist(&title).await.map_err(spotify_err)?;
                    return ok(format!("spotify:playlist:{id}"));
                }
                if st.it.is_logged_in() {
                    let client = require_login(st).map_err(err)?;
                    return ok(st.it.create_playlist(client, &title).await.map_err(err)?);
                }
                // Internal namespace, never sent to YouTube. A random suffix (seeded by the OS via
                // `RandomState`) avoids collisions between rapid creates; `rand` is not a dependency.
                let rnd = {
                    use std::hash::{BuildHasher, Hasher};
                    std::collections::hash_map::RandomState::new().build_hasher().finish()
                };
                let id = format!("{LOCAL_PLAYLIST_PREFIX}{}-{rnd:016x}", now_secs());
                st.db
                    .create_local_playlist(&id, &title)
                    .map_err(|e| err(format!("device playlist: {e}")))?;
                ok(id)
            }
            "edit_playlist_details" => {
                let playlist_id = arg::<String>(&params, "playlistId")?;
                if let Some(pid) = playlist_id.strip_prefix("spotify:playlist:") {
                    spotify_edit_playlist(st, &params, pid).await
                } else {
                    edit_playlist_details(st, &params).await
                }
            }
            "set_playlist_cover" => set_playlist_cover(st, &params).await,
            "set_playlist_sort" => {
                let playlist_id = arg::<String>(&params, "playlistId")?;
                let sort = arg::<PlaylistSort>(&params, "sort")?;
                let client = editable_playlist(st, &playlist_id).map_err(err)?;
                st.it.playlist_set_sort(client, &playlist_id, sort).await.map_err(err)?;
                null()
            }
            "delete_playlist" => {
                let playlist_id = arg::<String>(&params, "playlistId")?;
                if let Some(pid) = playlist_id.strip_prefix("spotify:playlist:") {
                    if pid == "liked" {
                        return Err(spotify_err("The liked songs playlist cannot be removed."));
                    }
                    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
                    client.delete_playlist(pid).await.map_err(spotify_err)?;
                    return null();
                }
                if is_local_playlist_id(&playlist_id) {
                    st.db
                        .delete_local_playlist(&playlist_id)
                        .map_err(|e| err(format!("device playlist: {e}")))?;
                    st.db.delete_setting(&cover_key(&playlist_id));
                    st.db.delete_setting(&synced_key(&playlist_id));
                    return null();
                }
                let client = editable_playlist(st, &playlist_id).map_err(err)?;
                st.it.delete_playlist(client, &playlist_id).await.map_err(err)?;
                st.db.forget_playlist(&playlist_id);
                null()
            }

            // --- Listen Together -------------------------------------------------------------
            "lt_get_state" => ok(st.lt.snapshot().await),
            "lt_set_server_url" => {
                let url = normalize_lt_server_url(&arg::<String>(&params, "url")?).map_err(err)?;
                st.db.set_setting("lt_server_url", &url);
                st.lt.set_server_url(url).await;
                null()
            }
            "lt_create_room" => {
                st.lt.create_room(arg::<String>(&params, "username")?).await;
                null()
            }
            "lt_join_room" => {
                st.lt
                    .join_room(arg::<String>(&params, "code")?, arg::<String>(&params, "username")?)
                    .await;
                null()
            }
            "lt_leave" => {
                st.lt.leave().await;
                null()
            }
            "lt_approve_join" => {
                st.lt.approve_join(arg::<String>(&params, "userId")?).await;
                null()
            }
            "lt_reject_join" => {
                st.lt.reject_join(arg::<String>(&params, "userId")?).await;
                null()
            }
            "lt_kick" => {
                st.lt.kick(arg::<String>(&params, "userId")?).await;
                null()
            }
            "lt_transfer_host" => {
                st.lt.transfer_host(arg::<String>(&params, "userId")?).await;
                null()
            }
            "lt_suggest" => {
                st.lt.suggest(song_to_track(&arg::<SongItem>(&params, "item")?)).await;
                null()
            }
            "lt_approve_suggestion" => {
                if let Some(track) = st.lt.approve_suggestion(arg::<String>(&params, "id")?).await {
                    st.clone().lt_enqueue_track(track).await;
                }
                null()
            }
            "lt_reject_suggestion" => {
                st.lt.reject_suggestion(arg::<String>(&params, "id")?).await;
                null()
            }
            "lt_request_sync" => {
                st.lt.request_sync().await;
                null()
            }

            // --- lyrics + misc ---------------------------------------------------------------
            "get_lyrics" => {
                let video_id = arg::<String>(&params, "videoId")?;
                if let Some(tid) = spotify_track_id(&video_id) {
                    let Some(client) = st.spotify.client().await else {
                        return null();
                    };
                    let lyrics = client.lyrics(tid).await.map_err(spotify_err)?;
                    return ok(lyrics.as_ref().map(spotify_bridge::lyrics_to_lyrics));
                }
                if radio::is_radio_id(&video_id) {
                    return null();
                }
                ok(ryotunes_core::lyrics::get_lyrics(
                    st,
                    ryotunes_core::lyrics::LyricsRequest {
                        video_id,
                        title: arg::<String>(&params, "title")?,
                        artists: arg::<String>(&params, "artists")?,
                        album: arg::<Option<String>>(&params, "album")?,
                        duration: arg::<Option<f64>>(&params, "duration")?,
                    },
                )
                .await)
            }
            "open_external" => {
                ryotunes_core::lastfm::open_browser(&arg::<String>(&params, "url")?)
                    .map_err(err)?;
                null()
            }
            "lastfm_connect" => {
                ryotunes_core::lastfm::connect(st.clone()).await.map_err(err)?;
                null()
            }
            "lastfm_disconnect" => {
                ryotunes_core::lastfm::disconnect(st);
                null()
            }
            "lastfm_status" => ok(ryotunes_core::lastfm::status(st)),

            // --- client-side / not ported ----------------------------------------------------
            "frontend_ready" | "open_mini" | "close_mini" | "login_webview" => Err(ErrorBody {
                code: "client_side".into(),
                message: format!("{method} is handled by the client"),
            }),
            "ryoku_theme_tokens" => Err(ErrorBody {
                code: "client_side".into(),
                message: "the client reads the Ryoku palette singletons directly".into(),
            }),

            _ => Err(ErrorBody { code: "unknown_method".into(), message: method.into() }),
        }
    }
}

// --- Spotify command bodies -------------------------------------------------------------------

/// The `{signedIn, stored, name, premium}` block for `spotify_status` and the subscribe snapshot.
/// A signed-in session implies Premium (the crate gates both sign-in and restore on it).
async fn spotify_status_json(st: &Arc<AppState>) -> Value {
    let signed_in = st.spotify.status().await;
    let name = if signed_in { spotify_name(st).await } else { None };
    let premium = signed_in.then_some(true);
    json!({
        "signedIn": signed_in,
        "stored": st.spotify.stored(),
        "name": name,
        "premium": premium,
    })
}

/// The signed-in profile's display name, fetched fresh. `None` when signed out or unreachable.
async fn spotify_name(st: &Arc<AppState>) -> Option<String> {
    let client = st.spotify.client().await?;
    client.profile().await.ok().map(|profile| profile.display_name)
}

async fn spotify_home(st: &Arc<AppState>) -> Result<Value, ErrorBody> {
    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
    let feed = client.home().await.map_err(spotify_err)?;
    ok(spotify_bridge::home_page(&feed))
}

async fn spotify_search_all(st: &Arc<AppState>, query: &str) -> Result<Value, ErrorBody> {
    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
    let results = client.search(query).await.map_err(spotify_err)?;
    ok(spotify_bridge::search_all(&results))
}

async fn spotify_search_page(st: &Arc<AppState>, query: &str) -> Result<Value, ErrorBody> {
    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
    let results = client.search(query).await.map_err(spotify_err)?;
    ok(json!({ "items": spotify_bridge::search_songs(&results), "continuation": null }))
}

async fn spotify_search_cards(
    st: &Arc<AppState>,
    query: &str,
    category: &str,
) -> Result<Value, ErrorBody> {
    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
    let results = client.search(query).await.map_err(spotify_err)?;
    ok(spotify_bridge::search_cards(&results, category))
}

async fn spotify_library(st: &Arc<AppState>) -> Result<Value, ErrorBody> {
    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
    let playlists = client.playlists(200).await.map_err(spotify_err)?;
    let mut items = vec![spotify_bridge::liked_card()];
    items.extend(playlists.iter().map(spotify_bridge::playlist_to_card));
    ok(items)
}

async fn spotify_library_albums(st: &Arc<AppState>) -> Result<Value, ErrorBody> {
    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
    let albums = client.saved_albums(200).await.map_err(spotify_err)?;
    ok(albums.iter().map(spotify_bridge::album_to_card).collect::<Vec<_>>())
}

async fn spotify_library_artists(st: &Arc<AppState>) -> Result<Value, ErrorBody> {
    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
    let artists = client.saved_artists(200).await.map_err(spotify_err)?;
    ok(artists.iter().map(spotify_bridge::saved_artist_to_card).collect::<Vec<_>>())
}

/// `spotify:playlist:liked` pages every saved track; any other id is a real Spotify playlist.
async fn spotify_playlist(st: &Arc<AppState>, pid: &str) -> Result<Value, ErrorBody> {
    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
    if pid == "liked" {
        let mut tracks = Vec::new();
        let mut page = 0u32;
        loop {
            let batch = client.liked(page).await.map_err(spotify_err)?;
            let full = batch.len() >= SPOTIFY_LIKED_PAGE;
            tracks.extend(batch);
            if !full {
                break;
            }
            page += 1;
        }
        ok(spotify_bridge::liked_page(&tracks))
    } else {
        let detail = client.playlist(pid).await.map_err(spotify_err)?;
        ok(spotify_bridge::playlist_page(&detail))
    }
}

async fn spotify_album(st: &Arc<AppState>, id: &str) -> Result<Value, ErrorBody> {
    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
    let detail = client.album(id).await.map_err(spotify_err)?;
    ok(spotify_bridge::album_page(&detail))
}

async fn spotify_artist(st: &Arc<AppState>, id: &str) -> Result<Value, ErrorBody> {
    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
    let artist = client.artist(id).await.map_err(spotify_err)?;
    ok(spotify_bridge::artist_page(id, &artist))
}

/// Rename / set-public for a Spotify playlist. Description is dropped (the crate has no setter);
/// the liked-songs view is not editable.
async fn spotify_edit_playlist(
    st: &Arc<AppState>,
    params: &Value,
    pid: &str,
) -> Result<Value, ErrorBody> {
    if pid == "liked" {
        return Err(spotify_err("The liked songs playlist cannot be edited."));
    }
    let client = st.spotify.client().await.ok_or_else(spotify_signed_out)?;
    if let Some(name) = arg::<Option<String>>(params, "name")? {
        let name = name.trim();
        if name.is_empty() {
            return Err(err("Playlist name cannot be empty."));
        }
        let name: String = name.chars().take(150).collect();
        client.rename_playlist(pid, &name).await.map_err(spotify_err)?;
    }
    if let Some(public) = arg::<Option<bool>>(params, "public")? {
        client.set_playlist_public(pid, public).await.map_err(spotify_err)?;
    }
    null()
}

// --- SoundCloud command bodies ----------------------------------------------------------------

/// SoundCloud home: the "discover" selections, one shelf per selection in the order SoundCloud
/// returns them ("Trending by genre", "Curated by SoundCloud", "Artists to watch out for"). No
/// account needed; a fetch failure surfaces as an error toast rather than a blank page.
async fn soundcloud_home(st: &Arc<AppState>) -> Result<Value, ErrorBody> {
    let selections = st.soundcloud.discover().await.map_err(soundcloud_err)?;
    ok(soundcloud_bridge::discover_home(&selections))
}

async fn soundcloud_search_all(st: &Arc<AppState>, query: &str) -> Result<Value, ErrorBody> {
    let results = st.soundcloud.search(query).await.map_err(soundcloud_err)?;
    ok(soundcloud_bridge::search_all(&results))
}

async fn soundcloud_search_page(st: &Arc<AppState>, query: &str) -> Result<Value, ErrorBody> {
    let results = st.soundcloud.search(query).await.map_err(soundcloud_err)?;
    ok(json!({ "items": soundcloud_bridge::search_songs(&results), "continuation": null }))
}

async fn soundcloud_search_cards(
    st: &Arc<AppState>,
    query: &str,
    category: &str,
) -> Result<Value, ErrorBody> {
    let results = st.soundcloud.search(query).await.map_err(soundcloud_err)?;
    ok(soundcloud_bridge::search_cards(&results, category))
}

async fn soundcloud_album(st: &Arc<AppState>, id: u64) -> Result<Value, ErrorBody> {
    let detail = st.soundcloud.playlist(id).await.map_err(soundcloud_err)?;
    ok(soundcloud_bridge::album_page(&detail))
}

async fn soundcloud_playlist(st: &Arc<AppState>, id: u64) -> Result<Value, ErrorBody> {
    let detail = st.soundcloud.playlist(id).await.map_err(soundcloud_err)?;
    ok(soundcloud_bridge::playlist_page(&detail))
}

async fn soundcloud_system_playlist(
    st: &Arc<AppState>,
    permalink: &str,
) -> Result<Value, ErrorBody> {
    let detail = st.soundcloud.system_playlist(permalink).await.map_err(soundcloud_err)?;
    ok(soundcloud_bridge::system_playlist_page(&detail))
}

/// The Orange/SoundCloud artist page: the user profile plus their tracks, albums, playlists and
/// likes, fetched concurrently. A missing sub-list degrades to empty rather than failing the page.
async fn soundcloud_artist(st: &Arc<AppState>, id: u64) -> Result<Value, ErrorBody> {
    let sc = &st.soundcloud;
    let user = sc.user(id).await.map_err(soundcloud_err)?;
    let (tracks, albums, playlists, likes) = tokio::join!(
        sc.user_tracks(id, 0),
        sc.user_albums(id),
        sc.user_playlists(id),
        sc.user_likes(id),
    );
    let tracks = tracks.map(|page| page.items).unwrap_or_default();
    let page = soundcloud_bridge::artist_page(
        &user,
        &tracks,
        &albums.unwrap_or_default(),
        &playlists.unwrap_or_default(),
        &likes.unwrap_or_default(),
    );
    ok(page)
}

/// The waveform for a SoundCloud track: 240 peak samples (0..=100) for the seek bar.
async fn soundcloud_waveform(st: &Arc<AppState>, id: u64) -> Result<Value, ErrorBody> {
    let track = st.soundcloud.track(id).await.map_err(soundcloud_err)?;
    let samples = st.soundcloud.waveform(&track).await.map_err(soundcloud_err)?;
    ok(json!({ "samples": samples }))
}

// --- ported command bodies that are too large for a match arm ---------------------------------

fn set_setting(st: &Arc<AppState>, params: &Value) -> Result<Value, ErrorBody> {
    let key = arg::<String>(params, "key")?;
    let value = arg::<String>(params, "value")?;
    if !UI_SETTINGS.contains(&key.as_str()) {
        return Err(err(format!("unknown setting: {key}")));
    }
    let value = normalize_ui_setting(&key, &value).map_err(err)?;
    // Autostart is an OS/desktop-file concern the daemon does not manage; it persists the value
    // like any other so the setting round-trips, but performs no launcher registration.
    if key == "discord_presence_name" {
        let name = ryotunes_core::discord::normalize_presence_name(&value).map_err(err)?;
        st.db.set_setting(&key, &name);
        st.set_discord_name(name);
        return null();
    }
    st.db.set_setting(&key, &value);
    if key == "discord_rpc" {
        st.set_discord_enabled(value == "true");
    }
    if key == "low_resource_mode" {
        st.set_low_resource_mode(value == "true");
    }
    if key == "lyrics_boidu" {
        st.db.clear_lyrics_cache();
    }
    null()
}

async fn get_library(st: &Arc<AppState>) -> Result<Value, ErrorBody> {
    let client = metadata_client(st).map_err(err)?;
    let mut items = if st.it.is_logged_in() {
        st.it.library_playlists(client).await.map_err(err)?
    } else {
        Vec::new()
    };
    let songs = on_repeat_songs(st);
    if !songs.is_empty() {
        items.insert(
            0,
            BrowseItem {
                kind: "playlist",
                id: ON_REPEAT_ID.into(),
                title: "On Repeat".into(),
                subtitle: Some(format!("{} songs", songs.len())),
                thumbnail: None,
                duration: None,
                artist_runs: Vec::new(),
                play_count: None,
                is_video: false,
                is_upload: false,
                explicit: false,
            },
        );
    }
    let recent = recent_songs(st);
    if !recent.is_empty() {
        items.insert(
            usize::from(!songs.is_empty()),
            smart_playlist_card(RECENTLY_PLAYED_ID, "Recently Played", &recent),
        );
    }
    let rediscover = rediscover_songs(st);
    if !rediscover.is_empty() {
        let at = usize::from(!songs.is_empty()) + usize::from(!recent.is_empty());
        items.insert(at, smart_playlist_card(REDISCOVER_ID, "Rediscover", &rediscover));
    }
    let device: Vec<BrowseItem> =
        st.db.local_playlists().into_iter().map(local_playlist_card).collect();
    let smart_count = usize::from(!songs.is_empty())
        + usize::from(!recent.is_empty())
        + usize::from(!rediscover.is_empty());
    items.splice(smart_count..smart_count, device);
    for item in &mut items {
        if let Some(cover) = custom_cover(st, &item.id) {
            item.thumbnail = Some(cover);
        }
    }
    ok(items)
}

async fn get_playlist(st: &Arc<AppState>, params: &Value) -> Result<Value, ErrorBody> {
    let id = arg::<String>(params, "id")?;
    if let Some(pid) = id.strip_prefix("spotify:playlist:") {
        return spotify_playlist(st, pid).await;
    }
    if let Some(permalink) = sc_system_id(&id) {
        return soundcloud_system_playlist(st, permalink).await;
    }
    if let Some(pid) = sc_playlist_id(&id) {
        return soundcloud_playlist(st, pid).await;
    }
    let sort = arg::<Option<PlaylistSort>>(params, "sort")?;
    let desc = arg::<Option<bool>>(params, "desc")?;
    if is_local_playlist_id(&id) {
        return ok(local_playlist_page(st, &id)
            .ok_or_else(|| err("That device playlist no longer exists."))?);
    }
    if id == ON_REPEAT_ID {
        let items = on_repeat_songs(st);
        return ok(PlaylistPage {
            title: Some("On Repeat".into()),
            subtitle: Some(format!("{} songs you've played most this month", items.len())),
            thumbnail: None,
            description: None,
            privacy: None,
            cover: None,
            items,
            continuation: None,
            owned: false,
            collaborative: false,
            sort_menu: None,
        });
    }
    if id == RECENTLY_PLAYED_ID {
        let items = recent_songs(st);
        return ok(smart_playlist_page(
            "Recently Played",
            format!("{} songs you've returned to this week", items.len()),
            items,
        ));
    }
    if id == REDISCOVER_ID {
        let items = rediscover_songs(st);
        return ok(smart_playlist_page(
            "Rediscover",
            format!("{} songs ready for another listen", items.len()),
            items,
        ));
    }
    let client = metadata_client(st).map_err(err)?;
    let sort = sort.map(|s| (s, desc.unwrap_or(false)));
    let mut page = st.it.playlist(client, &id, sort).await.map_err(err)?;
    page.cover = custom_cover(st, &id);
    ok(page)
}

async fn sync_playlist_index(st: &Arc<AppState>) -> Result<Value, ErrorBody> {
    const LIKED_MUSIC_ID: &str = "VLLM";
    const PLAYLIST_INDEX_TTL_SECS: i64 = 6 * 3600;
    const PLAYLIST_INDEX_MAX_PAGES: usize = 50;

    if !st.it.is_logged_in() {
        st.db.clear_playlist_index();
        return ok(st.db.playlist_memberships());
    }
    let fresh_until = st
        .db
        .get_setting("playlist_index_synced_at")
        .and_then(|at| at.parse::<i64>().ok())
        .map(|at| at + PLAYLIST_INDEX_TTL_SECS);
    if fresh_until.is_some_and(|until| now_secs() < until) {
        return ok(st.db.playlist_memberships());
    }
    let client = metadata_client(st).map_err(err)?;
    let library = st.it.library_playlists(client).await.map_err(err)?;
    if library.is_empty() {
        return ok(st.db.playlist_memberships());
    }
    let mut indexed: Vec<String> = Vec::new();
    for item in library {
        if is_smart_playlist_id(&item.id) || item.id == LIKED_MUSIC_ID {
            continue;
        }
        let Ok(page) = st.it.playlist(client, &item.id, None).await else {
            indexed.push(item.id);
            continue;
        };
        if !page.owned && !page.collaborative {
            continue;
        }
        let mut video_ids: Vec<String> = page.items.into_iter().map(|song| song.video_id).collect();
        let mut token = page.continuation;
        for _ in 0..PLAYLIST_INDEX_MAX_PAGES {
            let Some(next) = token.take() else { break };
            let Ok(more) = st.it.playlist_continuation(client, &next).await else { break };
            video_ids.extend(more.items.into_iter().map(|song| song.video_id));
            token = more.continuation;
        }
        st.db.set_playlist_tracks(&item.id, &video_ids);
        indexed.push(item.id);
    }
    st.db.retain_playlists(&indexed);
    st.db.set_setting("playlist_index_synced_at", &now_secs().to_string());
    ok(st.db.playlist_memberships())
}

fn listening_stats(st: &Arc<AppState>, params: &Value) -> Result<Value, ErrorBody> {
    use std::collections::HashMap;
    let period = arg::<String>(params, "period")?;
    let seconds = match period.as_str() {
        "day" => 24 * 60 * 60,
        "month" => ON_REPEAT_WINDOW_SECS,
        _ => 7 * 24 * 60 * 60,
    };
    let rows = st.db.play_rows(now_secs() - seconds);
    let mut artists: HashMap<String, i64> = HashMap::new();
    let mut tracks: HashMap<String, (String, String, i64)> = HashMap::new();
    let mut known_duration_secs = 0u64;
    for json in &rows {
        let Ok(song) = serde_json::from_str::<SongItem>(json) else { continue };
        *artists.entry(song.artists.clone()).or_insert(0) += 1;
        let entry = tracks.entry(song.video_id.clone()).or_insert((
            song.title.clone(),
            song.artists.clone(),
            0,
        ));
        entry.2 += 1;
        if let Some(duration) = song.duration.as_deref().and_then(duration_secs) {
            known_duration_secs += duration;
        }
    }
    let mut top_artists: Vec<_> = artists.into_iter().collect();
    top_artists.sort_by(|a, b| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
    top_artists.truncate(8);
    let mut top_tracks: Vec<_> = tracks.into_values().collect();
    top_tracks.sort_by(|a, b| b.2.cmp(&a.2).then_with(|| a.0.cmp(&b.0)));
    top_tracks.truncate(8);
    ok(json!({
        "period": period,
        "plays": rows.len(),
        "knownDurationSeconds": known_duration_secs,
        "topArtists": top_artists.into_iter().map(|(name, plays)| json!({"name": name, "plays": plays})).collect::<Vec<_>>(),
        "topTracks": top_tracks.into_iter().map(|(title, artists, plays)| json!({"title": title, "artists": artists, "plays": plays})).collect::<Vec<_>>(),
    }))
}

async fn edit_playlist_details(st: &Arc<AppState>, params: &Value) -> Result<Value, ErrorBody> {
    let playlist_id = arg::<String>(params, "playlistId")?;
    let name = arg::<Option<String>>(params, "name")?;
    let description = arg::<Option<String>>(params, "description")?;
    let public = arg::<Option<bool>>(params, "public")?;
    if is_local_playlist_id(&playlist_id) {
        if description.is_some() || public.is_some() {
            return Err(err("Device playlists only store a name and local artwork."));
        }
        if let Some(name) = name {
            let name = name.trim();
            if name.is_empty() {
                return Err(err("Playlist name cannot be empty."));
            }
            let name: String = name.chars().take(150).collect();
            st.db
                .rename_local_playlist(&playlist_id, &name)
                .map_err(|e| err(format!("device playlist: {e}")))?;
        }
        return null();
    }
    let client = editable_playlist(st, &playlist_id).map_err(err)?;
    let privacy = public.map(|p| if p { "PUBLIC" } else { "PRIVATE" });
    st.it
        .playlist_edit_details(
            client,
            &playlist_id,
            name.as_deref(),
            description.as_deref(),
            privacy,
        )
        .await
        .map_err(err)?;
    null()
}

/// What the UI needs to draw after a cover changed: where the local copy is, and (on a removal)
/// the thumbnail YouTube rebuilt in its place.
#[derive(serde::Serialize)]
struct CoverResult {
    cover: Option<String>,
    thumbnail: Option<String>,
}

async fn set_playlist_cover(st: &Arc<AppState>, params: &Value) -> Result<Value, ErrorBody> {
    use std::io::Read;
    const MAX_BYTES: u64 = 8 * 1024 * 1024;
    const PNG_MAGIC: &[u8; 8] = b"\x89PNG\r\n\x1a\n";

    let playlist_id = arg::<String>(params, "playlistId")?;
    let pick = arg::<bool>(params, "pick")?;
    let key = cover_key(&playlist_id);
    let stored = st.db.get_setting(&key);

    if !pick {
        let thumbnail = match clear_cover_on_youtube(st, &playlist_id).await {
            Ok(t) => {
                st.db.delete_setting(&synced_key(&playlist_id));
                t
            }
            Err(e) => {
                tracing::warn!(playlist_id, error = %e, "custom cover not cleared on YouTube Music");
                if st.db.get_setting(&synced_key(&playlist_id)).is_some() {
                    st.emit(
                        "cover-error",
                        json!({ "message": "Removed here, but YouTube Music kept its copy." }),
                    );
                }
                None
            }
        };
        st.db.delete_setting(&key);
        if let Some(old) = stored {
            let _ = std::fs::remove_file(old);
        }
        return ok(CoverResult { cover: None, thumbnail });
    }

    // The renderer never supplies a filesystem path in the Tauri app; over the socket the client
    // has already run its own picker and hands us the chosen file.
    let src = std::path::PathBuf::from(arg::<String>(params, "path")?);
    let ext = src.extension().and_then(|e| e.to_str()).unwrap_or_default().to_ascii_lowercase();
    if !matches!(ext.as_str(), "jpg" | "jpeg" | "png") {
        return Err(err("Pick a JPEG or PNG image."));
    }
    let meta = src.metadata().map_err(|_| err("That image cannot be read."))?;
    if !meta.is_file() || meta.len() == 0 || meta.len() > MAX_BYTES {
        return Err(err("Artwork must be a readable JPEG/PNG up to 8 MB."));
    }
    let mut head = [0u8; 8];
    let mut file = std::fs::File::open(&src).map_err(err)?;
    let read = file.read(&mut head).map_err(err)?;
    let is_png = read >= PNG_MAGIC.len() && &head[..PNG_MAGIC.len()] == PNG_MAGIC;
    let is_jpeg = read >= 3 && head[..3] == [0xFF, 0xD8, 0xFF];
    if !is_png && !is_jpeg {
        return Err(err("That file is not a valid JPEG or PNG image."));
    }
    let dir = local::covers_dir(&st.paths).join("playlists");
    std::fs::create_dir_all(&dir).map_err(err)?;
    let stem: String = playlist_id
        .chars()
        .filter(|c| c.is_ascii_alphanumeric() || *c == '-' || *c == '_')
        .collect();
    let stem = if stem.is_empty() { "playlist" } else { stem.as_str() };
    let dest = dir.join(format!("{stem}-{}.{ext}", ryotunes_core::db::now_secs()));
    std::fs::copy(&src, &dest).map_err(err)?;
    if let Some(old) = stored {
        let _ = std::fs::remove_file(old);
    }
    let dest = dest.to_string_lossy().to_string();
    st.db.set_setting(&key, &dest);
    sync_cover(st, &playlist_id, dest.clone());
    ok(CoverResult { cover: Some(dest), thumbnail: None })
}

#[derive(serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
struct PlaylistTransfer {
    version: u32,
    title: String,
    items: Vec<SongItem>,
}

fn export_playlist_file(params: &Value) -> Result<Value, ErrorBody> {
    let title = arg::<String>(params, "title")?;
    let items = arg::<Vec<SongItem>>(params, "items")?;
    if items.len() > 5_000 {
        return Err(err("Export is limited to 5,000 tracks."));
    }
    if items.iter().any(|item| !portable_song(item)) {
        return Err(err(
            "Portable playlist export supports YouTube Music tracks only; local files and live radio stay on this device.",
        ));
    }
    let path =
        transfer_path(std::path::PathBuf::from(arg::<String>(params, "path")?)).map_err(err)?;
    let transfer = PlaylistTransfer {
        version: 1,
        title: title.trim().chars().take(150).collect(),
        items: items.into_iter().map(shed_queue_context).collect(),
    };
    let bytes = serde_json::to_vec_pretty(&transfer).map_err(err)?;
    let tmp = path.with_extension("json.tmp");
    std::fs::write(&tmp, bytes).map_err(err)?;
    std::fs::rename(&tmp, &path)
        .or_else(|_| {
            let bytes = serde_json::to_vec_pretty(&transfer).map_err(std::io::Error::other)?;
            std::fs::write(&path, bytes)
        })
        .map_err(err)?;
    ok(true)
}

fn import_playlist_file(params: &Value) -> Result<Value, ErrorBody> {
    let path =
        transfer_path(std::path::PathBuf::from(arg::<String>(params, "path")?)).map_err(err)?;
    let meta = std::fs::metadata(&path).map_err(err)?;
    if meta.len() > 12 * 1024 * 1024 {
        return Err(err("That playlist file is too large."));
    }
    let bytes = std::fs::read(&path).map_err(err)?;
    let mut transfer: PlaylistTransfer =
        serde_json::from_slice(&bytes).map_err(|_| err("That is not a Ryotunes playlist file."))?;
    if transfer.version != 1 || transfer.items.len() > 5_000 {
        return Err(err("That playlist file version or size is not supported."));
    }
    if transfer.items.iter().any(|item| !portable_song(item)) {
        return Err(err("That playlist contains unsupported or unsafe track metadata."));
    }
    transfer.title = transfer.title.trim().chars().filter(|c| !c.is_control()).take(150).collect();
    transfer.items = transfer.items.into_iter().map(shed_queue_context).collect();
    ok(json!({ "title": transfer.title, "items": transfer.items }))
}

// --- ported private helpers ------------------------------------------------------------------

fn metadata_client(state: &Arc<AppState>) -> Result<&YouTubeClient, String> {
    state.clients.get(innertube::METADATA_CLIENT).ok_or_else(|| "metadata client missing".into())
}

fn require_login(state: &Arc<AppState>) -> Result<&YouTubeClient, String> {
    if !state.it.is_logged_in() {
        return Err("Sign in first to use this.".into());
    }
    metadata_client(state)
}

fn editable_playlist<'a>(
    state: &'a Arc<AppState>,
    playlist_id: &str,
) -> Result<&'a YouTubeClient, String> {
    if is_smart_playlist_id(playlist_id) {
        return Err("Smart playlists build themselves from your listening history.".into());
    }
    if playlist_id == "VLLM" {
        return Err("Liked Music follows your likes; like the song instead.".into());
    }
    require_login(state)
}

fn now_secs() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

fn duration_secs(raw: &str) -> Option<u64> {
    let mut total = 0u64;
    for part in raw.split(':') {
        total = total.checked_mul(60)?.checked_add(part.parse::<u64>().ok()?)?;
    }
    Some(total)
}

/// Strip the queue-slot metadata a play record carried, so a song replayed out of a smart playlist
/// does not wear the session member / autoplay / set-video-id it was played from.
fn shed_queue_context(s: SongItem) -> SongItem {
    SongItem {
        queued: false,
        queued_end: false,
        queued_from: None,
        queued_by: None,
        autoplay: false,
        set_video_id: None,
        added_by: None,
        added_by_avatar: None,
        ..s
    }
}

fn parse_play_rows(rows: impl IntoIterator<Item = String>) -> Vec<SongItem> {
    rows.into_iter()
        .filter_map(|json| serde_json::from_str(&json).ok())
        .map(shed_queue_context)
        .collect()
}

fn recent_songs(state: &Arc<AppState>) -> Vec<SongItem> {
    parse_play_rows(
        state
            .db
            .recent_unique_plays(now_secs() - RECENTLY_PLAYED_WINDOW_SECS, SMART_PLAYLIST_LIMIT),
    )
}

fn rediscover_songs(state: &Arc<AppState>) -> Vec<SongItem> {
    let now = now_secs();
    parse_play_rows(state.db.rediscover_plays(
        now - ON_REPEAT_WINDOW_SECS,
        now - REDISCOVER_OLDER_THAN_SECS,
        SMART_PLAYLIST_LIMIT,
    ))
}

fn on_repeat_songs(state: &Arc<AppState>) -> Vec<SongItem> {
    let since = now_secs() - ON_REPEAT_WINDOW_SECS;
    state
        .db
        .top_plays(since, ON_REPEAT_LIMIT)
        .into_iter()
        .filter_map(|(json, _plays)| serde_json::from_str(&json).ok())
        .map(shed_queue_context)
        .collect()
}

fn smart_playlist_card(id: &str, title: &str, songs: &[SongItem]) -> BrowseItem {
    BrowseItem {
        kind: "playlist",
        id: id.into(),
        title: title.into(),
        subtitle: Some(format!("{} songs", songs.len())),
        thumbnail: songs.iter().find_map(|s| s.thumbnail.clone()),
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

fn smart_playlist_page(title: &str, subtitle: String, items: Vec<SongItem>) -> PlaylistPage {
    PlaylistPage {
        title: Some(title.into()),
        subtitle: Some(subtitle),
        thumbnail: items.iter().find_map(|s| s.thumbnail.clone()),
        description: None,
        privacy: None,
        cover: None,
        items,
        continuation: None,
        owned: false,
        collaborative: false,
        sort_menu: None,
    }
}

fn local_playlist_card(row: LocalPlaylist) -> BrowseItem {
    BrowseItem {
        kind: "playlist",
        id: row.id,
        title: row.title,
        subtitle: Some(format!(
            "{} track{} · On this device",
            row.track_count,
            if row.track_count == 1 { "" } else { "s" }
        )),
        thumbnail: None,
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

fn local_playlist_page(state: &Arc<AppState>, id: &str) -> Option<PlaylistPage> {
    let row = state.db.local_playlist(id)?;
    let items: Vec<SongItem> = state
        .db
        .local_playlist_track_json(id)
        .into_iter()
        .filter_map(|json| serde_json::from_str::<SongItem>(&json).ok())
        .map(|song| {
            let mut song = shed_queue_context(song);
            song.set_video_id = Some(song.video_id.clone());
            song
        })
        .collect();
    Some(PlaylistPage {
        title: Some(row.title),
        subtitle: Some(format!(
            "{} track{} · Stored on this device",
            items.len(),
            if items.len() == 1 { "" } else { "s" }
        )),
        thumbnail: items.iter().find_map(|song| song.thumbnail.clone()),
        description: None,
        privacy: None,
        cover: custom_cover(state, id),
        items,
        continuation: None,
        owned: true,
        collaborative: false,
        sort_menu: None,
    })
}

fn cover_key(playlist_id: &str) -> String {
    format!("playlist_cover:{}", playlist_id.strip_prefix("VL").unwrap_or(playlist_id))
}

fn synced_key(playlist_id: &str) -> String {
    format!("{}:synced", cover_key(playlist_id))
}

fn custom_cover(state: &Arc<AppState>, playlist_id: &str) -> Option<String> {
    let path = state.db.get_setting(&cover_key(playlist_id))?;
    std::path::Path::new(&path).is_file().then_some(path)
}

/// Push a just-saved cover on to YouTube Music behind the caller's back (the local copy is already
/// the answer). A failure is a `cover-error` event, not a rollback.
fn sync_cover(state: &Arc<AppState>, playlist_id: &str, path: String) {
    if is_smart_playlist_id(playlist_id)
        || is_local_playlist_id(playlist_id)
        || !state.it.is_logged_in()
    {
        return;
    }
    let state = Arc::clone(state);
    let playlist_id = playlist_id.to_owned();
    tokio::spawn(async move {
        let Some(client) = state.clients.get(innertube::METADATA_CLIENT) else {
            return;
        };
        let result = match std::fs::read(&path) {
            Ok(image) => state.it.playlist_set_cover(client, &playlist_id, image).await,
            Err(e) => Err(innertube::Error::Other(e.to_string())),
        };
        match result {
            Ok(()) => state.db.set_setting(&synced_key(&playlist_id), "1"),
            Err(e) => {
                tracing::warn!(playlist_id, error = %e, "playlist cover didn't reach YouTube Music");
                let message = match e {
                    innertube::Error::CoverRefused => format!("Artwork saved on this device. {e}"),
                    e => format!("Artwork saved here, but the upload to YouTube Music failed: {e}"),
                };
                state.emit("cover-error", json!({ "message": message }));
            }
        }
    });
}

async fn clear_cover_on_youtube(
    state: &Arc<AppState>,
    playlist_id: &str,
) -> Result<Option<String>, String> {
    if is_smart_playlist_id(playlist_id)
        || is_local_playlist_id(playlist_id)
        || !state.it.is_logged_in()
    {
        return Ok(None);
    }
    let client = metadata_client(state)?;
    state.it.playlist_clear_cover(client, playlist_id).await.map_err(|e| e.to_string())
}

async fn scan_local(state: &Arc<AppState>) -> Result<local::LocalLibrary, String> {
    let covers = local::covers_dir(&state.paths);
    let state = state.clone();
    // Disk IO + tag parsing off the async worker threads. The daemon serves no webview assets, so
    // the Tauri host's asset-scope allow-listing has no equivalent here.
    tokio::task::spawn_blocking(move || local::scan(&state.db, &covers))
        .await
        .map_err(|e| e.to_string())
}

const MAX_CONFIG_URL: usize = 2_048;

pub(crate) fn normalize_proxy_setting(raw: &str) -> Result<String, String> {
    let raw = raw.trim();
    if raw.is_empty() {
        return Ok(String::new());
    }
    if raw.len() > MAX_CONFIG_URL {
        return Err("Proxy URL is too long.".into());
    }
    let url = url::Url::parse(raw).map_err(|_| "Enter a valid HTTP or HTTPS proxy URL.")?;
    if !matches!(url.scheme(), "http" | "https") || url.host_str().is_none() {
        return Err("Proxy must use http:// or https:// and include a host.".into());
    }
    if !url.username().is_empty() || url.password().is_some() {
        return Err(
            "Authenticated proxy URLs are not supported in renderer-visible settings.".into()
        );
    }
    if url.fragment().is_some() || url.query().is_some() || !matches!(url.path(), "" | "/") {
        return Err("Proxy URL must contain only scheme, host and port.".into());
    }
    Ok(url.to_string())
}

fn normalize_lt_server_url(raw: &str) -> Result<String, String> {
    let raw = raw.trim();
    if raw.len() > MAX_CONFIG_URL {
        return Err("Listen Together server URL is too long.".into());
    }
    let url = url::Url::parse(raw).map_err(|_| "Enter a valid ws:// or wss:// server URL.")?;
    if !matches!(url.scheme(), "ws" | "wss") || url.host_str().is_none() {
        return Err("Listen Together server must use ws:// or wss:// and include a host.".into());
    }
    if !url.username().is_empty() || url.password().is_some() || url.fragment().is_some() {
        return Err("Listen Together server URLs cannot contain credentials or fragments.".into());
    }
    Ok(url.to_string())
}

fn known_stream_client(name: &str) -> bool {
    name == innertube::MAIN_CLIENT
        || innertube::STREAM_FALLBACK_ORDER.contains(&name)
        || innertube::UPLOAD_FALLBACK_ORDER.contains(&name)
}

fn normalize_ui_setting(key: &str, raw: &str) -> Result<String, String> {
    match key {
        "volume" => {
            let value: i64 = raw.parse().map_err(|_| "Volume must be a number from 0 to 100.")?;
            (0..=100)
                .contains(&value)
                .then(|| value.to_string())
                .ok_or_else(|| "Volume must be between 0 and 100.".into())
        }
        "proxy" => normalize_proxy_setting(raw),
        "quality" => matches!(raw, "LOW" | "AUTO" | "HIGH")
            .then(|| raw.to_owned())
            .ok_or_else(|| "Unknown audio quality.".into()),
        "enable_history" | "discord_rpc" | "close_to_tray" | "autostart" | "autoplay"
        | "prevent_duplicates" | "lyrics_boidu" | "low_resource_mode" => {
            matches!(raw, "true" | "false")
                .then(|| raw.to_owned())
                .ok_or_else(|| format!("{key} must be true or false."))
        }
        "disabled_stream_clients" => {
            let mut names: Vec<&str> =
                raw.split(',').map(str::trim).filter(|name| !name.is_empty()).collect();
            if names.iter().any(|name| !known_stream_client(name)) {
                return Err("Unknown stream client in disabled-client list.".into());
            }
            names.sort_unstable();
            names.dedup();
            Ok(names.join(","))
        }
        "ui_scale" => {
            let value: i32 = raw.parse().map_err(|_| "Interface scale must be a percentage.")?;
            ((80..=140).contains(&value) && value % 10 == 0)
                .then(|| value.to_string())
                .ok_or_else(|| "Interface scale must be 80–140% in 10% steps.".into())
        }
        "discord_presence_name" => Ok(raw.to_owned()),
        _ => Err(format!("unknown setting: {key}")),
    }
}

fn canonical_music_folder(raw: &str) -> Result<String, String> {
    let raw = raw.trim();
    if raw.is_empty() {
        return Err("Choose a music folder.".into());
    }
    let path = std::path::Path::new(raw)
        .canonicalize()
        .map_err(|_| "That music folder no longer exists or cannot be accessed.")?;
    if !path.is_dir() {
        return Err("Choose a directory, not a file.".into());
    }
    if path.parent().is_none() {
        return Err("For safety, choose a music folder instead of the filesystem root.".into());
    }
    Ok(path.to_string_lossy().into_owned())
}

fn transfer_path(mut path: std::path::PathBuf) -> Result<std::path::PathBuf, String> {
    match path.extension().and_then(|e| e.to_str()).map(|e| e.to_ascii_lowercase()) {
        None => {
            path.set_extension("json");
            Ok(path)
        }
        Some(ext) if ext == "json" => Ok(path),
        _ => Err("Ryotunes playlist files must end in .json".into()),
    }
}

fn portable_song(item: &SongItem) -> bool {
    let text_ok = |value: &str, max: usize| {
        !value.is_empty() && value.len() <= max && !value.chars().any(char::is_control)
    };
    let thumbnail_ok = item.thumbnail.as_deref().is_none_or(|raw| {
        raw.len() <= 4_096
            && url::Url::parse(raw).ok().is_some_and(|url| {
                matches!(url.scheme(), "http" | "https")
                    && url.host_str().is_some()
                    && url.username().is_empty()
                    && url.password().is_none()
            })
    });
    !local::is_local_song(&item.video_id)
        && !radio::is_radio_id(&item.video_id)
        && text_ok(&item.video_id, 256)
        && text_ok(&item.title, 1_024)
        && text_ok(&item.artists, 1_024)
        && thumbnail_ok
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_tauri_command_has_a_method() {
        let lib = include_str!("../../../src-tauri/src/lib.rs");
        let list = &lib[lib.find("generate_handler![").unwrap()..];
        let list = &list[..list.find(']').unwrap()];
        let me = include_str!("methods.rs");
        for name in list
            .split(',')
            .map(|s| s.trim().trim_start_matches("commands::"))
            .filter(|s| !s.is_empty() && !s.starts_with("generate_handler"))
        {
            assert!(me.contains(&format!("\"{name}\"")), "no socket method for command {name}");
        }
    }

    #[test]
    fn arg_decodes_camel_case_and_defaults_missing_to_null() {
        let params = json!({ "videoId": "abc", "isUpload": true });
        assert_eq!(arg::<String>(&params, "videoId").unwrap(), "abc");
        assert_eq!(arg::<Option<bool>>(&params, "isUpload").unwrap(), Some(true));
        // A missing key decodes as an absent optional, and as an error for a required scalar.
        assert_eq!(arg::<Option<String>>(&params, "from").unwrap(), None);
        assert!(arg::<String>(&params, "missing").is_err());
    }

    #[test]
    fn proxy_setting_is_validated() {
        assert!(normalize_proxy_setting("http://127.0.0.1:8080").is_ok());
        assert!(normalize_proxy_setting("file:///tmp/socket").is_err());
        assert!(normalize_proxy_setting("http://user:pw@proxy.example:8080").is_err());
    }

    #[test]
    fn listen_together_url_must_be_a_websocket() {
        assert!(normalize_lt_server_url("wss://example.com/room").is_ok());
        assert!(normalize_lt_server_url("https://example.com").is_err());
    }
}
