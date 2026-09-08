//! Tauri commands — the ONLY API the UI calls. UI state contract. No YouTube shapes leak
//! past here; the UI never sees a stream URL.

use std::sync::Arc;

use innertube::{
    AlbumPage, ArtistPage, BrowseItem, HomePage, PlaylistContinuation, PlaylistPage, PlaylistSort,
    Rating, SearchCardPage, SearchResult, SearchResults, SongItem,
};
use tauri::State;
use tauri_plugin_dialog::DialogExt;

use crate::state::{
    is_local_playlist_id, is_smart_playlist_id, AppState, LOCAL_PLAYLIST_PREFIX, ON_REPEAT_ID,
    ON_REPEAT_LIMIT, ON_REPEAT_WINDOW_SECS, RECENTLY_PLAYED_ID, RECENTLY_PLAYED_WINDOW_SECS,
    REDISCOVER_ID, REDISCOVER_OLDER_THAN_SECS, SMART_PLAYLIST_LIMIT, UI_SETTINGS,
};

type St<'a> = State<'a, Arc<AppState>>;

const MAX_CONFIG_URL: usize = 2_048;

pub(crate) fn normalize_proxy_setting(raw: &str) -> Result<String, String> {
    let raw = raw.trim();
    if raw.is_empty() {
        return Ok(String::new());
    }
    if raw.len() > MAX_CONFIG_URL {
        return Err("Proxy URL is too long.".into());
    }
    let url = tauri::Url::parse(raw).map_err(|_| "Enter a valid HTTP or HTTPS proxy URL.")?;
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
        // Discord has its own Unicode-aware 2–128 character validator below.
        "discord_presence_name" => Ok(raw.to_owned()),
        _ => Err(format!("unknown setting: {key}")),
    }
}

fn normalize_lt_server_url(raw: &str) -> Result<String, String> {
    let raw = raw.trim();
    if raw.len() > MAX_CONFIG_URL {
        return Err("Listen Together server URL is too long.".into());
    }
    let url = tauri::Url::parse(raw).map_err(|_| "Enter a valid ws:// or wss:// server URL.")?;
    if !matches!(url.scheme(), "ws" | "wss") || url.host_str().is_none() {
        return Err("Listen Together server must use ws:// or wss:// and include a host.".into());
    }
    if !url.username().is_empty() || url.password().is_some() || url.fragment().is_some() {
        return Err("Listen Together server URLs cannot contain credentials or fragments.".into());
    }
    Ok(url.to_string())
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

#[tauri::command]
pub async fn search(state: St<'_>, query: String) -> Result<Vec<SongItem>, String> {
    state.search(&query).await
}

/// Songs-only first search page, including YouTube's opaque continuation token.
#[tauri::command]
pub async fn search_page(state: St<'_>, query: String) -> Result<SearchResult, String> {
    state.search_page(&query).await
}

/// Continue a songs-only search page.
#[tauri::command]
pub async fn search_page_more(state: St<'_>, token: String) -> Result<SearchResult, String> {
    state.search_page_more(&token).await
}

/// Unfiltered search → categorized sections for the search page.
#[tauri::command]
pub async fn search_all(state: St<'_>, query: String) -> Result<SearchResults, String> {
    state.search_all(&query).await
}

/// Filtered "Show more" search for one category (albums / artists / playlists).
#[tauri::command]
pub async fn search_cards(
    state: St<'_>,
    query: String,
    category: String,
) -> Result<Vec<BrowseItem>, String> {
    state.search_cards(&query, &category).await
}

/// First filtered card page plus continuation.
#[tauri::command]
pub async fn search_cards_page(
    state: St<'_>,
    query: String,
    category: String,
) -> Result<SearchCardPage, String> {
    state.search_cards_page(&query, &category).await
}

/// Continue a filtered card search.
#[tauri::command]
pub async fn search_cards_more(state: St<'_>, token: String) -> Result<SearchCardPage, String> {
    state.search_cards_more(&token).await
}

/// Continue the mixed result stream if YouTube supplied one.
#[tauri::command]
pub async fn search_all_more(state: St<'_>, token: String) -> Result<SearchResults, String> {
    state.search_all_more(&token).await
}

/// Play a track (from a search result). The UI passes the full item so we can seed the queue
/// with its metadata without another round-trip.
#[tauri::command]
pub async fn play(state: St<'_>, item: SongItem) -> Result<(), String> {
    let state = state.inner().clone();
    state.play_song(item).await;
    Ok(())
}

/// Warm a likely next click without changing queue/player state. Return immediately; the resolve
/// continues on the async runtime and populates the same stream cache used by normal playback.
#[tauri::command]
pub async fn prefetch_stream(
    state: St<'_>,
    video_id: String,
    is_upload: Option<bool>,
) -> Result<(), String> {
    let state = state.inner().clone();
    tauri::async_runtime::spawn(async move {
        state.prefetch_stream(video_id, is_upload.unwrap_or(false)).await
    });
    Ok(())
}

#[tauri::command]
pub async fn play_index(state: St<'_>, index: usize) -> Result<(), String> {
    let state = state.inner().clone();
    state.play_index(index).await;
    Ok(())
}

/// Remove an upcoming track from the queue (not the one playing). Guests are add-only — blocked
/// inside AppState.
#[tauri::command]
pub async fn remove_from_queue(state: St<'_>, index: usize) -> Result<(), String> {
    state.inner().clone().remove_from_queue(index).await;
    Ok(())
}

/// "Play next" from a ⋯ menu: one track or a whole album/playlist, inserted right after the
/// current song (behind any earlier manual adds). `from` is the album/playlist title, which heads
/// the block in the queue panel.
#[tauri::command]
pub async fn play_next(
    state: St<'_>,
    items: Vec<SongItem>,
    from: Option<String>,
) -> Result<(), String> {
    state.inner().clone().play_next(items, from).await;
    Ok(())
}

/// Drag-to-reorder in the queue panel: move the upcoming track at `from` to `to`. Out-of-range or
/// already-played indices are ignored.
#[tauri::command]
pub async fn move_in_queue(state: St<'_>, from: usize, to: usize) -> Result<(), String> {
    state.inner().clone().move_in_queue(from, to).await;
    Ok(())
}

/// "Add to queue": the tracks go at the back of the "Next in queue" block, so they play after
/// everything already queued by hand and ahead of the playing context (and its radio/filler).
/// `from` heads the block in the queue panel; `continuation` is the source page's next-page token —
/// the rest of a long playlist is walked in in the background.
#[tauri::command]
pub async fn add_to_queue(
    state: St<'_>,
    items: Vec<SongItem>,
    from: Option<String>,
    continuation: Option<String>,
) -> Result<(), String> {
    state.inner().clone().add_to_queue(items, from, continuation).await;
    Ok(())
}

/// Clear every upcoming manually-queued track (the queue panel's "Next in queue" section).
#[tauri::command]
pub async fn clear_queued(state: St<'_>) -> Result<(), String> {
    state.inner().clone().clear_queued().await;
    Ok(())
}

#[tauri::command]
pub async fn next_track(state: St<'_>) -> Result<(), String> {
    state.inner().clone().next_in_queue().await;
    Ok(())
}

#[tauri::command]
pub async fn prev_track(state: St<'_>) -> Result<(), String> {
    state.inner().clone().prev_in_queue().await;
    Ok(())
}

#[tauri::command]
pub async fn toggle_shuffle(state: St<'_>) -> Result<(), String> {
    state.inner().clone().toggle_shuffle().await;
    Ok(())
}

/// `mode` ∈ "off" | "all" | "one".
#[tauri::command]
pub async fn set_repeat(state: St<'_>, mode: String) -> Result<(), String> {
    let mode = match mode.as_str() {
        "off" => crate::state::RepeatMode::Off,
        "all" => crate::state::RepeatMode::All,
        "one" => crate::state::RepeatMode::One,
        other => return Err(format!("unknown repeat mode: {other}")),
    };
    state.inner().clone().set_repeat(mode).await;
    Ok(())
}

#[tauri::command]
pub async fn set_stop_after_current(state: St<'_>, enabled: bool) -> Result<(), String> {
    state.inner().clone().set_stop_after_current(enabled).await;
    Ok(())
}

#[tauri::command]
pub async fn toggle_pause(state: St<'_>) -> Result<(), String> {
    let state = state.inner().clone();
    state.resume_or_toggle().await;
    Ok(())
}

#[tauri::command]
pub async fn seek(state: St<'_>, position: f64) -> Result<(), String> {
    if !position.is_finite() || position < 0.0 {
        return Err("Seek position must be a non-negative finite number.".into());
    }
    // Routed through AppState so a Listen Together host broadcasts the seek and a guest is blocked.
    state.user_seek(position).await
}

#[tauri::command]
pub async fn set_volume(state: St<'_>, volume: i64) -> Result<(), String> {
    state.set_volume(volume)
}

/// Tempo (0.25–2.0) and pitch (−12..=12 semitones), the "Advanced" dialog. Volatile by design:
/// both reset to 1.0 / 0 on restart, so nobody wonders next week why everything sounds wrong.
#[tauri::command]
pub async fn set_playback_params(state: St<'_>, speed: f64, semitones: i32) -> Result<(), String> {
    state.set_playback_params(speed, semitones)
}

#[tauri::command]
pub async fn get_queue(state: St<'_>) -> Result<serde_json::Value, String> {
    Ok(state.queue_snapshot().await)
}

#[tauri::command]
pub async fn check_for_updates() -> Result<ryotunes_core::update::UpdateInfo, String> {
    ryotunes_core::update::check_for_updates().await
}

#[tauri::command]
pub async fn install_update(
    version: String,
) -> Result<ryotunes_core::update::InstalledUpdate, String> {
    ryotunes_core::update::install_update(version).await
}

#[tauri::command]
pub async fn get_settings(
    app: tauri::AppHandle,
    state: St<'_>,
) -> Result<serde_json::Value, String> {
    // The UI-visible subset is computed in the core so the daemon serves the exact same shape.
    let mut snapshot = state.settings_snapshot();

    // The OS is authoritative for autostart. Reconcile the persisted UI value whenever Settings
    // reads it so a deleted/failed desktop registration can never leave a misleading ON switch.
    use tauri_plugin_autostart::ManagerExt;
    if let (Ok(enabled), Some(map)) = (app.autolaunch().is_enabled(), snapshot.as_object_mut()) {
        let value = if enabled { "true" } else { "false" };
        state.db.set_setting("autostart", value);
        map.insert("autostart".into(), serde_json::Value::String(value.into()));
    }
    Ok(snapshot)
}

/// Resolve the same live Material-role chain used by Ryoku.Ui.Singletons.Tokens.
#[tauri::command]
pub fn ryoku_theme_tokens() -> serde_json::Value {
    crate::ryoku_theme::tokens()
}

#[tauri::command]
pub fn discord_status(state: St<'_>) -> serde_json::Value {
    state.discord_status()
}

#[tauri::command]
pub async fn set_setting(
    app: tauri::AppHandle,
    state: St<'_>,
    key: String,
    value: String,
) -> Result<(), String> {
    if !UI_SETTINGS.contains(&key.as_str()) {
        return Err(format!("unknown setting: {key}"));
    }
    let value = normalize_ui_setting(&key, &value)?;
    // Autostart is transactional: update the real OS registration first, verify it, and persist
    // only the resulting truth. This prevents the Settings switch drifting away from Linux.
    if key == "autostart" {
        use tauri_plugin_autostart::ManagerExt;
        let desired = value == "true";
        let al = app.autolaunch();
        let current = al.is_enabled().map_err(|e| format!("autostart: {e}"))?;
        if desired != current {
            if desired {
                al.enable().map_err(|e| format!("autostart: {e}"))?;
            } else {
                al.disable().map_err(|e| format!("autostart: {e}"))?;
            }
        }
        let actual = al.is_enabled().map_err(|e| format!("autostart verify: {e}"))?;
        if actual != desired {
            return Err("autostart registration did not reach the requested state".into());
        }
        state.db.set_setting("autostart", if actual { "true" } else { "false" });
        return Ok(());
    }

    if key == "discord_presence_name" {
        let name = crate::discord::normalize_presence_name(&value)?;
        state.db.set_setting(&key, &name);
        state.set_discord_name(name);
        return Ok(());
    }

    state.db.set_setting(&key, &value);
    // Presence connects/clears the moment it's toggled — the user shouldn't have to skip a track
    // to see it take effect.
    if key == "discord_rpc" {
        state.set_discord_enabled(value == "true");
    }
    if key == "low_resource_mode" {
        state.set_low_resource_mode(value == "true");
    }
    // Cached lyrics outlive the setting that produced them, so a track fetched while Boidu was on
    // would keep its word timings (and one fetched while off would never gain them) forever.
    if key == "lyrics_boidu" {
        state.db.clear_lyrics_cache();
    }
    Ok(())
}

/// The personal store (Home Shortcuts, sidebar pins, play recency, Home arrangement). Wrapped in
/// `{ personal }` so a client tells "nothing saved yet" (`{}`) apart from a transport failure. The
/// blob's shape and reducers live in the clients; the backend only persists it.
#[tauri::command]
pub async fn get_personal(state: St<'_>) -> Result<serde_json::Value, String> {
    Ok(serde_json::json!({ "personal": state.personal() }))
}

/// Replace the personal store and emit `personal-changed` to every window (so the mini follows the
/// main), rejecting a non-object or an oversized blob inside `AppState::set_personal`.
#[tauri::command]
pub async fn set_personal(state: St<'_>, personal: serde_json::Value) -> Result<(), String> {
    state.set_personal(personal)
}

/// The streamable client keys the orchestrator tries, for the "disabled clients" setting. Names
/// come from the innertube crate so the UI stays free of YouTube-shaped identity strings.
#[tauri::command]
pub async fn get_stream_clients() -> Result<Vec<String>, String> {
    let mut v = vec![innertube::MAIN_CLIENT.to_string()];
    v.extend(innertube::STREAM_FALLBACK_ORDER.iter().map(|s| s.to_string()));
    for key in innertube::UPLOAD_FALLBACK_ORDER {
        if !v.iter().any(|s| s == key) {
            v.push(key.to_string());
        }
    }
    Ok(v)
}

/// Wipe both cache tiers: resolved stream URLs and mpv's on-disk audio cache.
#[tauri::command]
pub async fn clear_caches(state: St<'_>) -> Result<(), String> {
    state.clear_caches();
    Ok(())
}

// --- auth (authentication flow) ---------------------------------------------------------------------

#[tauri::command]
pub async fn get_account(state: St<'_>) -> Result<serde_json::Value, String> {
    Ok(state.account_snapshot())
}

#[tauri::command]
pub async fn get_account_identities(state: St<'_>) -> Result<Vec<serde_json::Value>, String> {
    state.account_identities().await
}

#[tauri::command]
pub async fn switch_account(
    state: St<'_>,
    selection_key: String,
) -> Result<serde_json::Value, String> {
    state.switch_account(&selection_key).await
}

#[tauri::command]
pub async fn sign_out(state: St<'_>) -> Result<(), String> {
    let state = state.inner().clone();
    state.sign_out().await;
    Ok(())
}

/// Open the in-app Google sign-in webview (authentication flow Path A). Completes asynchronously; the UI
/// hears back via `auth-changed` (success) or `login-error`.
#[tauri::command]
pub async fn login_webview(state: St<'_>) -> Result<(), String> {
    state.inner().sign_in().await;
    Ok(())
}

/// The current track, play state, position and duration in one shot. Events are the normal
/// channel; this is for a webview that started after them (the mini player, or the main window
/// on a cold start, where the queue is restored before the UI subscribes).
#[tauri::command]
pub async fn get_playback(state: St<'_>) -> Result<serde_json::Value, String> {
    Ok(state.playback_snapshot().await)
}

/// Frontend-to-native first-paint handshake for cold start and hibernation reconstruction.
#[tauri::command]
pub fn frontend_ready(app: tauri::AppHandle, label: String) -> Result<(), String> {
    match label.as_str() {
        "main" => crate::main_window::frontend_ready(&app),
        crate::mini::LABEL => crate::mini::frontend_ready(&app),
        other => Err(format!("unknown frontend window label: {other}")),
    }
}

// --- mini player (mini.rs) ------------------------------------------------------------------

/// Swap the app for the floating widget: the main window hides to the tray behind it.
#[tauri::command]
pub async fn open_mini(app: tauri::AppHandle) -> Result<(), String> {
    // GTK wants window creation on the main thread, so hop and post the result back rather than
    // logging a failure the user would only see as a click that did nothing.
    let (tx, rx) = tokio::sync::oneshot::channel();
    let handle = app.clone();
    app.run_on_main_thread(move || {
        let _ = tx.send(crate::mini::open(&handle));
    })
    .map_err(|e| e.to_string())?;
    rx.await.map_err(|_| "the mini player never answered".to_string())?
}

/// Swap back. Same path as the tray, so the widget and the tray can't disagree about what
/// "show Ryotunes" means.
#[tauri::command]
pub async fn close_mini(app: tauri::AppHandle) -> Result<(), String> {
    crate::tray::show_main(&app);
    Ok(())
}

// --- browse / library (browse parser) ---------------------------------------------------------

fn metadata_client(state: &Arc<AppState>) -> Result<&innertube::YouTubeClient, String> {
    state.clients.get(innertube::METADATA_CLIENT).ok_or_else(|| "metadata client missing".into())
}

#[tauri::command]
pub async fn get_home(state: St<'_>, params: Option<String>) -> Result<HomePage, String> {
    state.home(params.as_deref()).await
}

#[tauri::command]
pub async fn get_home_more(state: St<'_>, token: String) -> Result<HomePage, String> {
    state.home_more(&token).await
}

#[tauri::command]
pub async fn get_library(state: St<'_>) -> Result<Vec<BrowseItem>, String> {
    let client = metadata_client(&state)?;
    // Signed out there is no YouTube library to ask for (the browse would come back as a sign-in
    // shell), but On Repeat is built from this machine's play history and is still real.
    let mut items = if state.it.is_logged_in() {
        state.it.library_playlists(client).await.map_err(|e| e.to_string())?
    } else {
        Vec::new()
    };
    // On Repeat leads the library once there's anything in it. Hidden while empty rather than
    // shown as a dead tile on a fresh install.
    let songs = on_repeat_songs(&state);
    if !songs.is_empty() {
        items.insert(
            0,
            BrowseItem {
                kind: "playlist",
                id: ON_REPEAT_ID.into(),
                title: "On Repeat".into(),
                subtitle: Some(format!("{} songs", songs.len())),
                thumbnail: None, // the UI draws an icon cover for this one
                duration: None,
                artist_runs: Vec::new(),
                play_count: None,
                is_video: false,
                is_upload: false,
                explicit: false,
            },
        );
    }
    // Two lighter smart views use the same bounded local history. They are intentionally local:
    // no extra startup network calls, and they remain useful while signed out.
    let recent = recent_songs(&state);
    if !recent.is_empty() {
        items.insert(
            usize::from(!songs.is_empty()),
            smart_playlist_card(RECENTLY_PLAYED_ID, "Recently Played", &recent),
        );
    }
    let rediscover = rediscover_songs(&state);
    if !rediscover.is_empty() {
        let at = usize::from(!songs.is_empty()) + usize::from(!recent.is_empty());
        items.insert(at, smart_playlist_card(REDISCOVER_ID, "Rediscover", &rediscover));
    }

    // Device playlists are independent of Google sign-in. Keep smart playlists first, then put
    // playlists created on this machine ahead of account rows so the signed-out Library is useful
    // rather than an empty instruction screen.
    let device: Vec<BrowseItem> =
        state.db.local_playlists().into_iter().map(local_playlist_card).collect();
    let smart_count = usize::from(!songs.is_empty())
        + usize::from(!recent.is_empty())
        + usize::from(!rediscover.is_empty());
    items.splice(smart_count..smart_count, device);

    // A card has nowhere to put two images, so a custom cover simply is the artwork here.
    for item in &mut items {
        if let Some(cover) = custom_cover(&state, &item.id) {
            item.thumbnail = Some(cover);
        }
    }
    Ok(items)
}

/// Empty rather than an error when signed out: the Library page merges the user's local saves into
/// these grids, so "nothing of yours on YouTube" is an answer, not a failure.
#[tauri::command]
pub async fn get_library_albums(state: St<'_>) -> Result<Vec<BrowseItem>, String> {
    state.library_albums().await
}

#[tauri::command]
pub async fn get_library_artists(state: St<'_>) -> Result<Vec<BrowseItem>, String> {
    state.library_artists().await
}

/// A playlist or album page. `id` is the browseId (`VL…` / `MPRE…`); Liked Songs is `VLLM`, and
/// `RYOTUNES_ON_REPEAT` is the local auto-playlist rather than anything YouTube knows about.
///
/// `sort` asks YouTube for the tracks in a given order; `None` gets whatever order the account
/// already has the list in, which is what a fresh visit wants (it matches YouTube Music).
#[tauri::command]
pub async fn get_playlist(
    state: St<'_>,
    id: String,
    sort: Option<PlaylistSort>,
    desc: Option<bool>,
) -> Result<PlaylistPage, String> {
    if is_local_playlist_id(&id) {
        return local_playlist_page(&state, &id)
            .ok_or_else(|| "That device playlist no longer exists.".to_string());
    }
    if id == ON_REPEAT_ID {
        let items = on_repeat_songs(&state);
        return Ok(PlaylistPage {
            title: Some("On Repeat".into()),
            subtitle: Some(format!("{} songs you've played most this month", items.len())),
            thumbnail: None,
            description: None,
            privacy: None,
            cover: None,
            items,
            continuation: None,
            owned: false, // nothing to rename or delete; it rebuilds itself from what you play
            collaborative: false,
            sort_menu: None, // built from local history, so YouTube has no order to give
        });
    }
    if id == RECENTLY_PLAYED_ID {
        let items = recent_songs(&state);
        return Ok(smart_playlist_page(
            "Recently Played",
            format!("{} songs you've returned to this week", items.len()),
            items,
        ));
    }
    if id == REDISCOVER_ID {
        let items = rediscover_songs(&state);
        return Ok(smart_playlist_page(
            "Rediscover",
            format!("{} songs ready for another listen", items.len()),
            items,
        ));
    }
    let client = metadata_client(&state)?;
    let sort = sort.map(|s| (s, desc.unwrap_or(false)));
    let mut page = state.it.playlist(client, &id, sort).await.map_err(|e| e.to_string())?;
    // Alongside YouTube's own thumbnail, not over it: the dialog offers to drop the custom one.
    page.cover = custom_cover(&state, &id);
    Ok(page)
}

/// Store a sort order on a playlist, so YouTube Music and every other client show it the same way.
///
/// Only for a playlist whose `sortMenu.editable` said the options are writes. Everywhere else the
/// order is view-only and this would 400.
#[tauri::command]
pub async fn set_playlist_sort(
    state: St<'_>,
    playlist_id: String,
    sort: PlaylistSort,
) -> Result<(), String> {
    let client = editable_playlist(&state, &playlist_id)?;
    state.it.playlist_set_sort(client, &playlist_id, sort).await.map_err(|e| e.to_string())
}

/// videoId → how many times it was played, over the same trailing window On Repeat is built from
/// (the history table is pruned to it, so there is no older data to offer). Feeds the playlist
/// page's "Most played" sort; a track the map doesn't mention has not been played this month.
#[tauri::command]
pub fn play_counts(state: St<'_>) -> std::collections::HashMap<String, i64> {
    state.db.play_counts(now_secs() - ON_REPEAT_WINDOW_SECS).into_iter().collect()
}

/// Lightweight local listening summary. No background timer and no WebKit-side history scan: the
/// bounded SQLite table is aggregated once when the Insights tab is opened/refreshed.
#[tauri::command]
pub fn listening_stats(state: St<'_>, period: String) -> serde_json::Value {
    use std::collections::HashMap;
    let seconds = match period.as_str() {
        "day" => 24 * 60 * 60,
        "month" => ON_REPEAT_WINDOW_SECS,
        _ => 7 * 24 * 60 * 60,
    };
    let rows = state.db.play_rows(now_secs() - seconds);
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
    serde_json::json!({
        "period": period,
        "plays": rows.len(),
        "knownDurationSeconds": known_duration_secs,
        "topArtists": top_artists.into_iter().map(|(name, plays)| serde_json::json!({"name": name, "plays": plays})).collect::<Vec<_>>(),
        "topTracks": top_tracks.into_iter().map(|(title, artists, plays)| serde_json::json!({"title": title, "artists": artists, "plays": plays})).collect::<Vec<_>>(),
    })
}

fn duration_secs(raw: &str) -> Option<u64> {
    let mut total = 0u64;
    for part in raw.split(':') {
        total = total.checked_mul(60)?.checked_add(part.parse::<u64>().ok()?)?;
    }
    Some(total)
}

/// The On Repeat track list: most-played first, over the trailing window. Rows whose stored JSON
/// no longer parses (a `SongItem` shape change) are dropped rather than failing the whole page.
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

/// A play record is the whole `SongItem` as it sat in the queue, so it carries that slot's queue
/// metadata: `queued`/`queued_by` when the track was "added to queue" (in a Listen Together session,
/// stamped with who added it), `autoplay` when radio appended it, `set_video_id` from whatever
/// playlist it was played from. None of that describes the song, so On Repeat sheds it: otherwise
/// the row wears a session member's name forever, and playing On Repeat drops it into "Next in
/// queue" instead of the playlist. Strips on read so rows already stored this way are fixed too.
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

fn now_secs() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

fn local_playlist_card(row: crate::db::LocalPlaylist) -> BrowseItem {
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
            // The playlist page uses this field only as a per-row "removable" marker. Device
            // playlists remove by video/file id, so the id itself is a stable local marker.
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

#[tauri::command]
pub async fn get_playlist_more(
    state: St<'_>,
    token: String,
) -> Result<PlaylistContinuation, String> {
    state.playlist_more(&token).await
}

/// An album page. `id` is the album browseId (`MPRE…`).
#[tauri::command]
pub async fn get_album(state: St<'_>, id: String) -> Result<AlbumPage, String> {
    state.album(&id).await
}

/// An artist page. `id` is the channel browseId (`UC…`).
#[tauri::command]
pub async fn get_artist(state: St<'_>, id: String) -> Result<ArtistPage, String> {
    state.artist(&id).await
}

/// A card grid reached from a carousel's "More" button (e.g. an artist's full albums list).
#[tauri::command]
pub async fn get_browse_grid(
    state: St<'_>,
    id: String,
    params: Option<String>,
) -> Result<Vec<BrowseItem>, String> {
    state.browse_grid(&id, params.as_deref()).await
}

/// Play a playlist/album: the given items become the queue (no radio). `start` is the clicked
/// track index; `None`/omitted means "just play it" (random opener when shuffle is on).
/// `source_id` (the page's playlist/album playlist id) makes autoplay continue with that
/// context's radio when the queue runs out. `source_name` (the page title) feeds the queue
/// panel's "Next from" header; `shuffle: true` (page Shuffle buttons) turns shuffle on for
/// this queue — pass the items in their real order, the backend shuffles. `continuation` is the
/// page's next-page token when it has one: pass the tracks that are loaded and the backend walks
/// the rest into the queue in the background, so playback starts on page 1.
#[tauri::command]
pub async fn play_playlist(
    state: St<'_>,
    items: Vec<SongItem>,
    start: Option<usize>,
    source_id: Option<String>,
    source_name: Option<String>,
    shuffle: Option<bool>,
    continuation: Option<String>,
) -> Result<(), String> {
    // A device playlist is a local container, not a YouTube playlist seed. Passing its synthetic
    // id as radio_seed would make queue exhaustion try a YouTube /next request with that id.
    let source_id = source_id.filter(|id| !is_local_playlist_id(id));
    let state = state.inner().clone();
    state
        .play_tracks(items, start, source_id, source_name, shuffle.unwrap_or(false), continuation)
        .await;
    Ok(())
}

/// Start a radio seeded on a song, artist, album or playlist (browse parser). `kind` is
/// `song` | `artist` | `album` | `playlist`; `id` is the videoId (song) or browseId/playlistId
/// (everything else) — the backend resolves it to a radio playlist. `name` titles the queue.
///
/// Starting a song radio on the track that's already playing keeps it playing and replaces only
/// what comes after it; every other case replaces the queue.
#[tauri::command]
pub async fn start_radio(
    state: St<'_>,
    kind: String,
    id: String,
    name: Option<String>,
) -> Result<(), String> {
    if crate::radio::is_radio_id(&id) || is_local_playlist_id(&id) {
        return Err("This item does not have a YouTube Music radio seed.".into());
    }
    let state = state.inner().clone();
    state.start_radio(&kind, &id, name).await
}

// --- Internet Radio --------------------------------------------------------------------------

#[tauri::command]
pub async fn radio_stations(
    query: Option<String>,
    offset: Option<usize>,
    limit: Option<usize>,
) -> Result<Vec<crate::radio::RadioStation>, String> {
    crate::radio::stations(query.as_deref(), offset.unwrap_or(0), limit.unwrap_or(36)).await
}

#[tauri::command]
pub async fn play_radio_station(state: St<'_>, station_uuid: String) -> Result<(), String> {
    // WebKit only returns the opaque id it received. Native code resolves the actual station
    // record again, so this command can never become an arbitrary native HTTP fetch primitive.
    let station = crate::radio::station_by_uuid(&station_uuid).await?;
    state.inner().clone().play_radio_station(station).await
}

// --- portable playlist transfer ---------------------------------------------------------------

#[derive(serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
struct PlaylistTransfer {
    version: u32,
    title: String,
    items: Vec<SongItem>,
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

fn safe_transfer_name(title: &str) -> String {
    let cleaned: String = title
        .trim()
        .chars()
        .filter(|c| !c.is_control())
        .map(|c| if r#"\/:*?"<>|"#.contains(c) { '-' } else { c })
        .take(80)
        .collect();
    if cleaned.trim_matches([' ', '.', '-']).is_empty() {
        "Ryotunes Playlist".into()
    } else {
        cleaned
    }
}

fn portable_song(item: &SongItem) -> bool {
    let text_ok = |value: &str, max: usize| {
        !value.is_empty() && value.len() <= max && !value.chars().any(char::is_control)
    };
    let thumbnail_ok = item.thumbnail.as_deref().is_none_or(|raw| {
        raw.len() <= 4_096
            && tauri::Url::parse(raw).ok().is_some_and(|url| {
                matches!(url.scheme(), "http" | "https")
                    && url.host_str().is_some()
                    && url.username().is_empty()
                    && url.password().is_none()
            })
    });
    !crate::local::is_local_song(&item.video_id)
        && !crate::radio::is_radio_id(&item.video_id)
        && text_ok(&item.video_id, 256)
        && text_ok(&item.title, 1_024)
        && text_ok(&item.artists, 1_024)
        && thumbnail_ok
}

/// Export only portable song metadata. The renderer never supplies a filesystem path: Rust opens
/// the native save dialog and writes only after the user explicitly chooses a destination.
#[tauri::command]
pub async fn export_playlist_file(
    app: tauri::AppHandle,
    title: String,
    items: Vec<SongItem>,
) -> Result<bool, String> {
    if items.len() > 5_000 {
        return Err("Export is limited to 5,000 tracks.".into());
    }
    if items.iter().any(|item| !portable_song(item)) {
        return Err(
            "Portable playlist export supports YouTube Music tracks only; local files and live radio stay on this device."
                .into(),
        );
    }
    let file_name = format!("{}.json", safe_transfer_name(&title));
    let Some(chosen) = app
        .dialog()
        .file()
        .set_title("Export Ryotunes playlist")
        .set_file_name(file_name)
        .add_filter("Ryotunes playlist", &["json"])
        .blocking_save_file()
    else {
        return Ok(false);
    };
    let path = transfer_path(
        chosen
            .into_path()
            .map_err(|_| "That save location is not a local filesystem path.".to_string())?,
    )?;
    let transfer = PlaylistTransfer {
        version: 1,
        title: title.trim().chars().take(150).collect(),
        items: items.into_iter().map(shed_queue_context).collect(),
    };
    let bytes = serde_json::to_vec_pretty(&transfer).map_err(|e| e.to_string())?;
    let tmp = path.with_extension("json.tmp");
    std::fs::write(&tmp, bytes).map_err(|e| e.to_string())?;
    std::fs::rename(&tmp, &path)
        .or_else(|_| {
            let bytes = serde_json::to_vec_pretty(&transfer).map_err(std::io::Error::other)?;
            std::fs::write(&path, bytes)
        })
        .map_err(|e| e.to_string())?;
    Ok(true)
}

/// Import is a narrow parser, and the path comes only from a native picker. Renderer code can ask
/// to open the picker, but cannot silently point Rust at an arbitrary local JSON file.
#[tauri::command]
pub async fn import_playlist_file(
    app: tauri::AppHandle,
) -> Result<Option<serde_json::Value>, String> {
    let Some(chosen) = app
        .dialog()
        .file()
        .set_title("Import Ryotunes playlist")
        .add_filter("Ryotunes playlist", &["json"])
        .blocking_pick_file()
    else {
        return Ok(None);
    };
    let path = transfer_path(
        chosen
            .into_path()
            .map_err(|_| "That playlist is not a local filesystem file.".to_string())?,
    )?;
    let meta = std::fs::metadata(&path).map_err(|e| e.to_string())?;
    if meta.len() > 12 * 1024 * 1024 {
        return Err("That playlist file is too large.".into());
    }
    let bytes = std::fs::read(&path).map_err(|e| e.to_string())?;
    let mut transfer: PlaylistTransfer = serde_json::from_slice(&bytes)
        .map_err(|_| "That is not a Ryotunes playlist file.".to_string())?;
    if transfer.version != 1 || transfer.items.len() > 5_000 {
        return Err("That playlist file version or size is not supported.".into());
    }
    if transfer.items.iter().any(|item| !portable_song(item)) {
        return Err("That playlist contains unsupported or unsafe track metadata.".into());
    }
    transfer.title = transfer.title.trim().chars().filter(|c| !c.is_control()).take(150).collect();
    transfer.items = transfer.items.into_iter().map(shed_queue_context).collect();
    Ok(Some(serde_json::json!({"title": transfer.title, "items": transfer.items})))
}

// --- write actions (write API ✎, authentication flow) ----------------------------------------------

fn require_login(state: &Arc<AppState>) -> Result<&innertube::YouTubeClient, String> {
    if !state.it.is_logged_in() {
        return Err("Sign in first to use this.".into());
    }
    metadata_client(state)
}

/// Like, dislike, or clear a track's rating. One command for all three: YouTube's states are
/// mutually exclusive, so a dislike un-likes in the same call and the UI never has to send two.
#[tauri::command]
pub async fn rate(state: St<'_>, video_id: String, rating: Rating) -> Result<(), String> {
    if crate::radio::is_radio_id(&video_id) || crate::local::is_local_song(&video_id) {
        return Err("This track does not have a YouTube Music rating.".into());
    }
    let client = require_login(&state)?;
    // Any detached refresh already in flight was asked before this write existed.
    state.rate_epoch.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
    state.it.rate(client, &video_id, rating).await.map_err(|e| e.to_string())
}

/// Save an album to the library, or remove it. `playlist_id` is the album's `OLAK5uy_…`
/// (`AlbumPage.playlistId`).
#[tauri::command]
pub async fn set_album_saved(
    state: St<'_>,
    playlist_id: String,
    saved: bool,
) -> Result<(), String> {
    let client = require_login(&state)?;
    state.it.like_playlist(client, &playlist_id, saved).await.map_err(|e| e.to_string())
}

/// Login, plus the guard every playlist edit needs. Two ids never reach `edit_playlist`: On Repeat
/// has no YouTube playlist behind it, and Liked Music is an auto-playlist YouTube edits through the
/// rating endpoint instead. Both answer 400 there.
fn editable_playlist<'a>(
    state: &'a Arc<AppState>,
    playlist_id: &str,
) -> Result<&'a innertube::YouTubeClient, String> {
    if is_smart_playlist_id(playlist_id) {
        return Err("Smart playlists build themselves from your listening history.".into());
    }
    if playlist_id == LIKED_MUSIC_ID {
        return Err("Liked Music follows your likes; like the song instead.".into());
    }
    require_login(state)
}

/// Liked Music is a library playlist like any other, but the row already carries its thumbs-up
/// state, so a second mark saying the same thing is noise. It is also the one list that runs to
/// thousands of tracks, which would double the crawl on its own.
const LIKED_MUSIC_ID: &str = "VLLM";
/// How long the membership index is trusted before a re-crawl. Adds and removes made in this app
/// patch it as they happen, so this window only ever covers edits made somewhere else.
const PLAYLIST_INDEX_TTL_SECS: i64 = 6 * 3600;
/// Continuation pages per playlist. YouTube hands back 100 tracks a page, so this covers 5000 of
/// them. Note: a hard stop, not paging state. A playlist past it marks its first 5000 tracks
/// and no more, which beats one pathological list turning a sync into hundreds of requests.
const PLAYLIST_INDEX_MAX_PAGES: usize = 50;

/// videoId → the ids of your playlists holding it, straight from SQLite with no network at all,
/// so a track list can draw the "saved" mark on its first paint. Empty until the first sync.
#[tauri::command]
pub fn playlist_index(state: St<'_>) -> std::collections::HashMap<String, Vec<String>> {
    state.db.playlist_memberships()
}

/// Rebuild that index by walking the playlists you own, then answer with it.
///
/// Nothing else knows playlist membership: the library browse gives cards, a playlist browse gives
/// one list's tracks, and InnerTube's per-video add-to-playlist dialog would be a request per row.
/// So the crawl is the price, and it is paid at most once every `PLAYLIST_INDEX_TTL_SECS`, on a
/// launch or a sign-in. Playlists you merely saved are skipped: they are someone else's, so "you
/// saved this song to it" would be a lie, and their long mixes would double the walk.
#[tauri::command]
pub async fn sync_playlist_index(
    state: St<'_>,
) -> Result<std::collections::HashMap<String, Vec<String>>, String> {
    if !state.it.is_logged_in() {
        state.db.clear_playlist_index();
        return Ok(state.db.playlist_memberships());
    }
    let fresh_until = state
        .db
        .get_setting("playlist_index_synced_at")
        .and_then(|at| at.parse::<i64>().ok())
        .map(|at| at + PLAYLIST_INDEX_TTL_SECS);
    if fresh_until.is_some_and(|until| now_secs() < until) {
        return Ok(state.db.playlist_memberships());
    }
    let client = metadata_client(&state)?;
    let library = state.it.library_playlists(client).await.map_err(|e| e.to_string())?;
    // A degraded response that parses as an empty library would otherwise wipe every mark and
    // then call the wipe fresh for six hours. Nothing to index is nothing to trust: keep what is
    // stored and try again on the next launch.
    if library.is_empty() {
        return Ok(state.db.playlist_memberships());
    }
    let mut indexed: Vec<String> = Vec::new();
    for item in library {
        if is_smart_playlist_id(&item.id) || item.id == LIKED_MUSIC_ID {
            continue;
        }
        // One playlist failing (a deleted id, a hiccup) must not abandon the rest of the crawl,
        // and must not drop what is already indexed for it either: leaving it out of `indexed`
        // would have `retain_playlists` forget the tracks we do know about.
        let Ok(page) = state.it.playlist(client, &item.id, None).await else {
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
            let Ok(more) = state.it.playlist_continuation(client, &next).await else { break };
            video_ids.extend(more.items.into_iter().map(|song| song.video_id));
            token = more.continuation;
        }
        state.db.set_playlist_tracks(&item.id, &video_ids);
        indexed.push(item.id);
    }
    state.db.retain_playlists(&indexed);
    state.db.set_setting("playlist_index_synced_at", &now_secs().to_string());
    Ok(state.db.playlist_memberships())
}

/// Add a complete track snapshot to a playlist stored on this device. Keeping the whole SongItem
/// means the list can reopen while signed out/offline without a metadata request.
#[tauri::command]
pub fn add_to_local_playlist(
    state: St<'_>,
    playlist_id: String,
    item: SongItem,
) -> Result<bool, String> {
    if !is_local_playlist_id(&playlist_id) {
        return Err("not a device playlist".into());
    }
    if crate::radio::is_radio_id(&item.video_id) {
        return Err("Live radio stations cannot be added to song playlists.".into());
    }
    let item = shed_queue_context(item);
    let json = serde_json::to_string(&item).map_err(|e| e.to_string())?;
    state
        .db
        .add_local_playlist_track(&playlist_id, &item.video_id, &json)
        .map_err(|e| format!("device playlist: {e}"))
}

/// `false` means the playlist already had the track and YouTube added nothing — not an error, but
/// the UI must not draw an optimistic row for it (there is no real row to remove later).
#[tauri::command]
pub async fn add_to_playlist(
    state: St<'_>,
    playlist_id: String,
    video_id: String,
) -> Result<bool, String> {
    if crate::radio::is_radio_id(&video_id) {
        return Err("Live radio stations cannot be added to song playlists.".into());
    }
    let client = editable_playlist(&state, &playlist_id)?;
    let added =
        state.it.playlist_add(client, &playlist_id, &video_id).await.map_err(|e| e.to_string())?;
    // Also on `false`: YouTube refusing a duplicate means the playlist holds the track, which is
    // exactly what the index should say. A stale index is how it got asked in the first place.
    state.db.add_playlist_track(&playlist_id, &video_id);
    Ok(added)
}

#[tauri::command]
pub async fn remove_from_playlist(
    state: St<'_>,
    playlist_id: String,
    video_id: String,
    set_video_id: String,
) -> Result<(), String> {
    if is_local_playlist_id(&playlist_id) {
        return state
            .db
            .remove_local_playlist_track(&playlist_id, &video_id)
            .map_err(|e| format!("device playlist: {e}"));
    }
    let client = editable_playlist(&state, &playlist_id)?;
    state
        .it
        .playlist_remove(client, &playlist_id, &video_id, &set_video_id)
        .await
        .map_err(|e| e.to_string())?;
    state.db.remove_playlist_track(&playlist_id, &video_id);
    Ok(())
}

#[tauri::command]
pub async fn create_playlist(state: St<'_>, title: String) -> Result<String, String> {
    let title = title.trim();
    if title.is_empty() {
        return Err("Playlist name cannot be empty.".into());
    }
    let title: String = title.chars().take(150).collect();
    if state.it.is_logged_in() {
        let client = require_login(&state)?;
        return state.it.create_playlist(client, &title).await.map_err(|e| e.to_string());
    }

    // Random suffix avoids collisions between rapid creates while the wall-clock prefix keeps ids
    // debuggable. It is an internal namespace, never sent to YouTube.
    let id = format!("{LOCAL_PLAYLIST_PREFIX}{}-{:016x}", now_secs(), rand::random::<u64>());
    state.db.create_local_playlist(&id, &title).map_err(|e| format!("device playlist: {e}"))?;
    Ok(id)
}

/// Edit a playlist you own, from the "Edit playlist" dialog: name, description, visibility.
///
/// Each field is `None` when the user left it alone, and only what changed is sent: an edit of
/// the name must not blank a description we failed to read back off the page.
#[tauri::command]
pub async fn edit_playlist_details(
    state: St<'_>,
    playlist_id: String,
    name: Option<String>,
    description: Option<String>,
    public: Option<bool>,
) -> Result<(), String> {
    if is_local_playlist_id(&playlist_id) {
        if description.is_some() || public.is_some() {
            return Err("Device playlists only store a name and local artwork.".into());
        }
        if let Some(name) = name {
            let name = name.trim();
            if name.is_empty() {
                return Err("Playlist name cannot be empty.".into());
            }
            let name: String = name.chars().take(150).collect();
            state
                .db
                .rename_local_playlist(&playlist_id, &name)
                .map_err(|e| format!("device playlist: {e}"))?;
        }
        return Ok(());
    }
    let client = editable_playlist(&state, &playlist_id)?;
    // The switch is two-state; YouTube's third value (UNLISTED) is only ever left as it was.
    let privacy = public.map(|p| if p { "PUBLIC" } else { "PRIVATE" });
    state
        .it
        .playlist_edit_details(
            client,
            &playlist_id,
            name.as_deref(),
            description.as_deref(),
            privacy,
        )
        .await
        .map_err(|e| e.to_string())
}

/// Custom playlist artwork, in both places it lives.
///
/// Setting one is local-first: the picked image is copied in beside the local-music covers and
/// answered straight back, then pushed to YouTube Music in the background (`sync_cover`), because
/// the upload is three round trips and nobody should watch a spinner for their own file.
///
/// Dropping one waits, and that is deliberate. Once a cover has been up there, YouTube's own
/// thumbnail *is* that cover, so a local-first removal would fall back to the very image being
/// removed and only reach the rebuilt collage a beat later: two swaps, the first of them pointless.
/// The clear is a single small call, so it answers with the thumbnail YouTube rebuilt and the UI
/// changes once.
#[tauri::command]
pub async fn set_playlist_cover(
    app: tauri::AppHandle,
    state: St<'_>,
    playlist_id: String,
    pick: bool,
) -> Result<Option<CoverResult>, String> {
    use std::io::Read;
    use tauri::Manager;

    const MAX_BYTES: u64 = 8 * 1024 * 1024;
    const PNG_MAGIC: &[u8; 8] = b"\x89PNG\r\n\x1a\n";

    let key = cover_key(&playlist_id);
    let stored = state.db.get_setting(&key);

    if !pick {
        let thumbnail = match clear_cover_on_youtube(&state, &playlist_id).await {
            Ok(t) => {
                state.db.delete_setting(&synced_key(&playlist_id));
                t
            }
            Err(e) => {
                tracing::warn!(playlist_id, error = %e, "custom cover not cleared on YouTube Music");
                if state.db.get_setting(&synced_key(&playlist_id)).is_some() {
                    state.emit(
                        "cover-error",
                        serde_json::json!({
                            "message": "Removed here, but YouTube Music kept its copy.",
                        }),
                    );
                }
                None
            }
        };
        state.db.delete_setting(&key);
        if let Some(old) = stored {
            let _ = std::fs::remove_file(old);
        }
        return Ok(Some(CoverResult { cover: None, thumbnail }));
    }

    let Some(chosen) = app
        .dialog()
        .file()
        .set_title("Choose playlist artwork")
        .add_filter("Images", &["jpg", "jpeg", "png"])
        .blocking_pick_file()
    else {
        return Ok(None);
    };
    let src = chosen
        .into_path()
        .map_err(|_| "That artwork is not a local filesystem file.".to_string())?;
    let ext = src.extension().and_then(|e| e.to_str()).unwrap_or_default().to_ascii_lowercase();
    if !matches!(ext.as_str(), "jpg" | "jpeg" | "png") {
        return Err("Pick a JPEG or PNG image.".into());
    }
    let meta = src.metadata().map_err(|_| "That image cannot be read.".to_string())?;
    if !meta.is_file() || meta.len() == 0 || meta.len() > MAX_BYTES {
        return Err("Artwork must be a readable JPEG/PNG up to 8 MB.".into());
    }

    let mut head = [0u8; 8];
    let mut file = std::fs::File::open(&src).map_err(|e| e.to_string())?;
    let read = file.read(&mut head).map_err(|e| e.to_string())?;
    let is_png = read >= PNG_MAGIC.len() && &head[..PNG_MAGIC.len()] == PNG_MAGIC;
    let is_jpeg = read >= 3 && head[..3] == [0xFF, 0xD8, 0xFF];
    if !is_png && !is_jpeg {
        return Err("That file is not a valid JPEG or PNG image.".into());
    }

    let dir = crate::local::covers_dir(&state.paths).join("playlists");
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    let stem: String = playlist_id
        .chars()
        .filter(|c| c.is_ascii_alphanumeric() || *c == '-' || *c == '_')
        .collect();
    let stem = if stem.is_empty() { "playlist" } else { stem.as_str() };
    let dest = dir.join(format!("{stem}-{}.{ext}", crate::db::now_secs()));
    std::fs::copy(&src, &dest).map_err(|e| e.to_string())?;
    if let Some(old) = stored {
        let _ = std::fs::remove_file(old);
    }
    let dest = dest.to_string_lossy().to_string();
    let _ = app.asset_protocol_scope().allow_file(&dest);
    state.db.set_setting(&key, &dest);
    sync_cover(&state, &playlist_id, dest.clone());
    Ok(Some(CoverResult { cover: Some(dest), thumbnail: None }))
}

/// What the UI needs to draw after a cover changed: where the local copy is, and (on a removal)
/// the thumbnail YouTube rebuilt in its place.
#[derive(serde::Serialize)]
pub struct CoverResult {
    cover: Option<String>,
    thumbnail: Option<String>,
}

/// Send the cover on to YouTube Music behind the picker's back: the local copy is already on
/// screen, and the upload is a three-call round trip nobody should wait through.
///
/// A failure is a toast, not a rollback: the artwork is still right here, and it is still this
/// playlist's cover on this machine. Signed out (or On Repeat, which YouTube has never heard of),
/// there is nothing to sync and local is all there ever was.
fn sync_cover(state: &Arc<AppState>, playlist_id: &str, path: String) {
    if is_smart_playlist_id(playlist_id)
        || is_local_playlist_id(playlist_id)
        || !state.it.is_logged_in()
    {
        return;
    }
    let state = Arc::clone(state);
    let playlist_id = playlist_id.to_owned();
    tauri::async_runtime::spawn(async move {
        let Some(client) = state.clients.get(innertube::METADATA_CLIENT) else {
            return;
        };
        // Read here, not on the command's thread: the file was just written and the caller has its
        // answer already.
        let result = match std::fs::read(&path) {
            Ok(image) => state.it.playlist_set_cover(client, &playlist_id, image).await,
            Err(e) => Err(innertube::Error::Other(e.to_string())),
        };
        match result {
            // Remembered so a later removal knows whether YouTube has anything of ours to drop.
            Ok(()) => state.db.set_setting(&synced_key(&playlist_id), "1"),
            Err(e) => {
                tracing::warn!(playlist_id, error = %e, "playlist cover didn't reach YouTube Music");
                let message = match e {
                    // The one refusal with a known cause and no fix inside this app. Say it once,
                    // plainly, and leave the cover where it already is: on this machine.
                    innertube::Error::CoverRefused => format!("Artwork saved on this device. {e}"),
                    e => format!("Artwork saved here, but the upload to YouTube Music failed: {e}"),
                };
                state.emit("cover-error", serde_json::json!({ "message": message }));
            }
        }
    });
}

/// Drop the custom thumbnail from the account, answering the one YouTube rebuilt from the tracks.
/// Nothing to do (and nothing to answer with) when there is no account behind the playlist.
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

fn cover_key(playlist_id: &str) -> String {
    // Browse ids arrive `VL`-prefixed and playlist ids don't; one playlist, one key either way.
    format!("playlist_cover:{}", playlist_id.strip_prefix("VL").unwrap_or(playlist_id))
}

/// Set once a cover of ours has actually landed on the account, so a removal knows whether there
/// is anything up there to warn about failing to clear.
fn synced_key(playlist_id: &str) -> String {
    format!("{}:synced", cover_key(playlist_id))
}

/// The custom artwork stored for a playlist, if the file is still there. The user owns that
/// directory and can empty it, and a dead path renders as a broken image.
fn custom_cover(state: &Arc<AppState>, playlist_id: &str) -> Option<String> {
    let path = state.db.get_setting(&cover_key(playlist_id))?;
    std::path::Path::new(&path).is_file().then_some(path)
}

#[tauri::command]
pub async fn delete_playlist(state: St<'_>, playlist_id: String) -> Result<(), String> {
    if is_local_playlist_id(&playlist_id) {
        state
            .db
            .delete_local_playlist(&playlist_id)
            .map_err(|e| format!("device playlist: {e}"))?;
        state.db.delete_setting(&cover_key(&playlist_id));
        state.db.delete_setting(&synced_key(&playlist_id));
        return Ok(());
    }
    let client = editable_playlist(&state, &playlist_id)?;
    state.it.delete_playlist(client, &playlist_id).await.map_err(|e| e.to_string())?;
    state.db.forget_playlist(&playlist_id);
    Ok(())
}

#[tauri::command]
pub async fn subscribe(state: St<'_>, channel_id: String, subscribed: bool) -> Result<(), String> {
    let client = require_login(&state)?;
    state.it.subscribe(client, &channel_id, subscribed).await.map_err(|e| e.to_string())
}

// --- local music (local.rs) ------------------------------------------------------------------

/// Rescan the watched folders and return the library. The scan is the deletion check too: its
/// `removed` list is every id that was on screen but is gone from disk, so the UI can drop those
/// tiles without waiting for anyone to click a dead one.
#[tauri::command]
pub async fn get_local_library(
    app: tauri::AppHandle,
    state: St<'_>,
) -> Result<crate::local::LocalLibrary, String> {
    scan_local(&app, &state).await
}

#[tauri::command]
pub async fn add_local_folder(
    app: tauri::AppHandle,
    state: St<'_>,
) -> Result<Option<crate::local::LocalLibrary>, String> {
    let Some(chosen) = app.dialog().file().set_title("Add a music folder").blocking_pick_folder()
    else {
        return Ok(None);
    };
    let chosen = chosen
        .into_path()
        .map_err(|_| "That folder is not a local filesystem path.".to_string())?;
    let path = canonical_music_folder(chosen.to_string_lossy().as_ref())?;
    crate::local::add_folder(&state.db, path);
    scan_local(&app, &state).await.map(Some)
}

#[tauri::command]
pub async fn remove_local_folder(
    app: tauri::AppHandle,
    state: St<'_>,
    path: String,
) -> Result<crate::local::LocalLibrary, String> {
    crate::local::remove_folder(&state.db, &path);
    scan_local(&app, &state).await
}

/// Disk IO + tag parsing off the async runtime's worker threads.
async fn scan_local(
    app: &tauri::AppHandle,
    state: &Arc<AppState>,
) -> Result<crate::local::LocalLibrary, String> {
    let covers = crate::local::covers_dir(&state.paths);
    let state = state.clone();
    let lib = tauri::async_runtime::spawn_blocking(move || crate::local::scan(&state.db, &covers))
        .await
        .map_err(|e| e.to_string())?;
    // Artwork reaches the page over the asset protocol, which starts out allowing nothing.
    crate::local_scope::allow_covers(app, &lib.songs);
    Ok(lib)
}

// --- Listen Together (session protocol) ----------------------------------------------------------

/// Current client-side LT state (status, role, room, participants, pending joins, suggestions).
#[tauri::command]
pub async fn lt_get_state(state: St<'_>) -> Result<serde_json::Value, String> {
    Ok(state.lt.snapshot().await)
}

/// Set and persist the Listen Together WebSocket server URL.
#[tauri::command]
pub async fn lt_set_server_url(state: St<'_>, url: String) -> Result<(), String> {
    let url = normalize_lt_server_url(&url)?;
    state.db.set_setting("lt_server_url", &url);
    state.lt.set_server_url(url).await;
    Ok(())
}

#[tauri::command]
pub async fn lt_create_room(state: St<'_>, username: String) -> Result<(), String> {
    state.lt.create_room(username).await;
    Ok(())
}

#[tauri::command]
pub async fn lt_join_room(state: St<'_>, code: String, username: String) -> Result<(), String> {
    state.lt.join_room(code, username).await;
    Ok(())
}

#[tauri::command]
pub async fn lt_leave(state: St<'_>) -> Result<(), String> {
    state.lt.leave().await;
    Ok(())
}

#[tauri::command]
pub async fn lt_approve_join(state: St<'_>, user_id: String) -> Result<(), String> {
    state.lt.approve_join(user_id).await;
    Ok(())
}

#[tauri::command]
pub async fn lt_reject_join(state: St<'_>, user_id: String) -> Result<(), String> {
    state.lt.reject_join(user_id).await;
    Ok(())
}

#[tauri::command]
pub async fn lt_kick(state: St<'_>, user_id: String) -> Result<(), String> {
    state.lt.kick(user_id).await;
    Ok(())
}

#[tauri::command]
pub async fn lt_transfer_host(state: St<'_>, user_id: String) -> Result<(), String> {
    state.lt.transfer_host(user_id).await;
    Ok(())
}

/// Guest: send a track to the session queue (auto-approved by the host client, which stamps
/// who added it).
#[tauri::command]
pub async fn lt_suggest(state: St<'_>, item: SongItem) -> Result<(), String> {
    state.lt.suggest(crate::state::song_to_track(&item)).await;
    Ok(())
}

/// Host: approve a suggestion — add it to the real queue and notify the suggester. (Unused since
/// guest adds auto-approve, kept for a future "require approval" setting.)
#[tauri::command]
pub async fn lt_approve_suggestion(state: St<'_>, id: String) -> Result<(), String> {
    if let Some(track) = state.lt.approve_suggestion(id).await {
        state.inner().clone().lt_enqueue_track(track).await;
    }
    Ok(())
}

#[tauri::command]
pub async fn lt_reject_suggestion(state: St<'_>, id: String) -> Result<(), String> {
    state.lt.reject_suggestion(id).await;
    Ok(())
}

/// Guest: force a re-sync with the room (drift correction).
#[tauri::command]
pub async fn lt_request_sync(state: St<'_>) -> Result<(), String> {
    state.lt.request_sync().await;
    Ok(())
}

// --- lyrics ---------------------------------------------------------------------------------

/// Lyrics for a track (cached). The UI passes the metadata it already has from `now-playing`;
/// `duration` is mpv's length in seconds. `None` = no lyrics found anywhere.
#[tauri::command]
pub async fn get_lyrics(
    state: St<'_>,
    video_id: String,
    title: String,
    artists: String,
    album: Option<String>,
    duration: Option<f64>,
) -> Result<Option<crate::lyrics::Lyrics>, String> {
    if crate::radio::is_radio_id(&video_id) {
        return Ok(None);
    }
    Ok(crate::lyrics::get_lyrics(
        state.inner(),
        crate::lyrics::LyricsRequest { video_id, title, artists, album, duration },
    )
    .await)
}

/// Open a link from the UI in the real browser. An `<a href>` inside the webview would navigate
/// the app itself off the SPA, with no way back.
#[tauri::command]
pub async fn open_external(url: String) -> Result<(), String> {
    crate::lastfm::open_browser(&url)
}

// --- Last.fm scrobbling ---------------------------------------------------------------------

/// Start the browser auth flow. Returns once the authorize page is open; the outcome (session
/// stored, or an error) arrives via the `lastfm-state` event.
#[tauri::command]
pub async fn lastfm_connect(state: St<'_>) -> Result<(), String> {
    crate::lastfm::connect(state.inner().clone()).await
}

#[tauri::command]
pub async fn lastfm_disconnect(state: St<'_>) -> Result<(), String> {
    crate::lastfm::disconnect(&state);
    Ok(())
}

/// `{ connected, username }` from the persisted session — seeds the titlebar button on mount.
#[tauri::command]
pub async fn lastfm_status(state: St<'_>) -> Result<serde_json::Value, String> {
    Ok(crate::lastfm::status(&state))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn renderer_settings_are_validated_at_the_native_boundary() {
        assert_eq!(normalize_ui_setting("quality", "HIGH").unwrap(), "HIGH");
        assert!(normalize_ui_setting("quality", "DROP TABLE").is_err());
        assert_eq!(normalize_ui_setting("volume", "75").unwrap(), "75");
        assert!(normalize_ui_setting("volume", "101").is_err());
        assert_eq!(normalize_ui_setting("ui_scale", "110").unwrap(), "110");
        assert!(normalize_ui_setting("ui_scale", "111").is_err());
        assert!(normalize_proxy_setting("file:///tmp/socket").is_err());
        assert!(normalize_proxy_setting("http://127.0.0.1:8080").is_ok());
        assert!(normalize_proxy_setting("http://user:pw@proxy.example:8080").is_err());
    }

    #[test]
    fn listen_together_accepts_only_websocket_urls() {
        assert!(normalize_lt_server_url("wss://example.com/room").is_ok());
        assert!(normalize_lt_server_url("ws://127.0.0.1:4000").is_ok());
        assert!(normalize_lt_server_url("https://example.com").is_err());
        assert!(normalize_lt_server_url("javascript:alert(1)").is_err());
        assert!(normalize_lt_server_url("wss://user:pass@example.com").is_err());
    }

    #[test]
    fn local_music_picker_cannot_register_the_filesystem_root() {
        let temp = std::env::temp_dir();
        assert!(canonical_music_folder(temp.to_string_lossy().as_ref()).is_ok());
        #[cfg(unix)]
        assert!(canonical_music_folder("/").is_err());
    }

    #[test]
    fn on_repeat_rows_shed_the_queue_slot_they_were_played_from() {
        let played = SongItem {
            video_id: "abc".into(),
            title: "Grace".into(),
            queued: true,
            queued_by: Some("ryotunes-test".into()),
            autoplay: true,
            set_video_id: Some("SVI".into()),
            ..Default::default()
        };
        let row = shed_queue_context(played.clone());
        assert_eq!(
            row,
            SongItem { video_id: "abc".into(), title: "Grace".into(), ..Default::default() }
        );
        assert_eq!(row.title, played.title, "the song itself survives");
    }
}
