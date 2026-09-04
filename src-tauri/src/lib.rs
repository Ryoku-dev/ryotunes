//! Ryotunes Tauri app. Wires transport + player + db + orchestrator behind the command boundary.

mod cipher;
mod commands;
mod db;
mod discord;
mod http;
mod lastfm;
mod listentogether;
mod local;
mod lyrics;
mod main_window;
mod media;
mod mini;
mod orchestrator;
mod potoken;
mod radio;
mod ryoku_theme;
mod session;
mod state;
#[cfg(target_os = "windows")]
mod taskbar;
mod tray;
mod webview;

use std::sync::Arc;
use std::time::Duration;

use innertube::{Clients, InnerTube, Locale, Session};
use player::{Player, PlayerEvent};
use tauri::{Emitter, Manager};

use cipher::{CipherDeobfuscator, PlayerConfigStore};
use db::Db;
use orchestrator::Orchestrator;
use potoken::PoTokenGenerator;
use state::AppState;

/// Hand glibc's freed-but-retained heap back to the OS every few minutes.
///
/// glibc gives each thread its own arena and never returns those pages on `free`, so this process
/// (45 threads across tokio, GTK, mpv and souvlaki) accumulates empty heap it will never reuse.
/// Measured against a running 0.3.2 build: `malloc_trim(0)` dropped it from 211 MiB to 160 MiB PSS
/// and the slack came back at roughly 15 MiB per 15 minutes, so a periodic trim keeps it flat.
///
/// Note: trim only. `mallopt(M_ARENA_MAX, 2)` would cap the sprawl at the source, but it
/// serialises allocation across all those threads for a win the trim already gets. Reach for it
/// only if RSS starts climbing between trims.
#[cfg(target_os = "linux")]
fn spawn_heap_trimmer() {
    tauri::async_runtime::spawn(async {
        loop {
            tokio::time::sleep(Duration::from_secs(180)).await;
            // Safe: no arguments, no allocation, glibc walks its own arenas.
            unsafe { libc::malloc_trim(0) };
        }
    });
}

/// Pull WebKitGTK off its full-browser defaults, which wry never touches.
///
/// **Caches.** WebKitGTK defaults to `WEBKIT_CACHE_MODEL_WEB_BROWSER` ("cache a very large number of
/// resources and previously viewed content"), sized against total system RAM. A music client
/// browsing YTM shelves fills that with thumbnails: measured 627 MiB of on-disk WebKitCache and a
/// web process that would not give the memory back (`malloc_trim` there freed 1 MiB, so it is all
/// live cache). `DocumentViewer` is the smallest cache model: Ryotunes already owns browse/page caches and
/// thumbnail sizing, so retaining a second browser-sized resource cache only inflates the renderer. wry also hard-enables the back/forward page cache (`webkitgtk/mod.rs:438`), which
/// keeps whole previous documents alive; this is a SvelteKit SPA doing client-side routing, so it
/// never gets a back/forward navigation to restore and that memory is pure waste.
///
/// **Subsystems.** Ryotunes is audio-only: libmpv owns playback and the webview renders artwork,
/// queue and lyrics. WebKit's media/GStreamer/WebGL stacks stay disabled permanently. This avoids
/// waking the discrete GPU for a feature the app does not expose.
///
/// Applies to one webview, because WebKit settings are per-view: the main window and the mini
/// player each cost their own web process, so each has to be told. The hidden cipher/PoToken
/// webviews are deliberately left at the defaults, since the fingerprinting code they exist to run
/// is entitled to probe whatever it likes.
#[cfg(target_os = "linux")]
fn tune_webview(win: &tauri::WebviewWindow) {
    use webkit2gtk::{CacheModel, SettingsExt, WebContextExt, WebViewExt};

    let label = win.label().to_owned();
    let res = win.with_webview(move |wv| {
        let webview = wv.inner();
        // Context-wide, so the second call is a no-op. Set here anyway: whichever window comes up
        // first should not depend on the other existing.
        if let Some(ctx) = WebViewExt::context(&webview) {
            ctx.set_cache_model(CacheModel::DocumentViewer);
        }
        if let Some(settings) = WebViewExt::settings(&webview) {
            // Audio playback is handled by libmpv, so WebKit's media stack stays off. Keep
            // ordinary UI compositing on WebKitGTK's default renderer instead of forcing CPU
            // software rendering. On a Hybrid/Optimus desktop this lands on the display/iGPU and
            // avoids turning a few CSS layers into a 20%+ WebKit CPU workload.
            settings.set_enable_page_cache(false);
            settings.set_enable_media(false);
            settings.set_enable_mediasource(false);
            settings.set_enable_media_stream(false);
            settings.set_enable_media_capabilities(false);
            settings.set_enable_encrypted_media(false);
            settings.set_enable_webaudio(false);
            settings.set_enable_webrtc(false);
            settings.set_enable_webgl(false);
            settings.set_enable_html5_database(false); // WebSQL. localStorage is a separate switch.
        }
    });
    match res {
        Ok(()) => {
            tracing::info!(label, "webkit: audio-only, minimal document cache, media + page cache + webgl off; default UI compositor")
        }
        Err(e) => tracing::warn!(label, error = %e, "webkit tuning failed (continuing)"),
    }
}

/// [`tune_webview`] for a window looked up by label. No-op if it isn't up.
#[cfg(target_os = "linux")]
pub(crate) fn tune_webview_labelled(app: &tauri::AppHandle, label: &str) {
    if let Some(win) = app.get_webview_window(label) {
        tune_webview(&win);
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    // NVIDIA + Wayland: WebKitGTK's DMABUF renderer trips over NVIDIA's explicit
    // sync (GBM buffer failures / blank window / Gdk Error 71). Disabling explicit
    // sync keeps hardware-accelerated rendering, unlike the old
    // WEBKIT_DISABLE_DMABUF_RENDERER=1 workaround which forced CPU software
    // rendering on WebKitGTK 2.46+ and made the whole UI laggy. Harmless no-op on
    // non-NVIDIA drivers. Note: blanket-set on Linux; probe driver/session if
    // an X11/NVIDIA blank-window report ever comes in.
    #[cfg(target_os = "linux")]
    {
        let wayland = std::env::var_os("WAYLAND_DISPLAY").is_some()
            || std::env::var("XDG_SESSION_TYPE").is_ok_and(|v| v.eq_ignore_ascii_case("wayland"));
        let nvidia = std::path::Path::new("/proc/driver/nvidia/version").exists()
            || std::path::Path::new("/sys/module/nvidia").exists();
        if wayland && nvidia && std::env::var_os("__NV_DISABLE_EXPLICIT_SYNC").is_none() {
            std::env::set_var("__NV_DISABLE_EXPLICIT_SYNC", "1");
        }
    }

    // Public builds stay quiet by default so normal playback does not become a listening-history
    // log. Developers can opt into detail with RUST_LOG=info (or a narrower target filter).
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "warn,app_lib=warn,discord_rich_presence=error".into()),
        )
        .init();

    // On Hyprland the compositor owns float/tile policy. Install the exact main-window rule before
    // Tauri creates a visible surface; the config keeps main hidden until the SPA has mounted.
    main_window::install_hyprland_map_rules();

    tauri::Builder::default()
        // Must be the first plugin registered (its documented requirement). A second launch —
        // e.g. clicking the app icon while we're hidden in the tray — re-shows this instance
        // instead of spawning a second one (which would fight over SQLite and mpv).
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            tray::show_main(app);
        }))
        // Folder picker for the local-music library (local.rs).
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_autostart::init(
            tauri_plugin_autostart::MacosLauncher::LaunchAgent,
            None,
        ))
        .setup(|app| {
            let handle = app.handle().clone();

            // Session cookies and the queue live here, so never fall back to a shared temp
            // directory. Tauri resolves this through the platform's per-user application-data
            // location (XDG_DATA_HOME on Linux).
            let data_dir = app.path().app_data_dir()?;
            std::fs::create_dir_all(&data_dir)?;

            // Large, disposable audio belongs in the cache location instead of application data.
            // If a platform cannot provide a cache directory, keep it private under app data rather
            // than failing startup or leaking it into a process-wide temp directory.
            let cache_root = app.path().app_cache_dir().unwrap_or_else(|_| data_dir.join("cache"));
            let cache_dir = cache_root.join("audio");
            std::fs::create_dir_all(&cache_dir)?;

            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                let private_dir = std::fs::Permissions::from_mode(0o700);
                let _ = std::fs::set_permissions(&data_dir, private_dir.clone());
                let _ = std::fs::set_permissions(&cache_root, private_dir.clone());
                let _ = std::fs::set_permissions(&cache_dir, private_dir);
            }

            // Use the Ryotunes database name while preserving state from previous local builds
            // without keeping any legacy product identifier in source: when the new database is
            // absent, adopt the sole sibling .sqlite file in this app-data directory.
            let db_path = data_dir.join("ryotunes.sqlite");
            if !db_path.exists() {
                if let Ok(entries) = std::fs::read_dir(&data_dir) {
                    let mut candidates =
                        entries.filter_map(Result::ok).map(|e| e.path()).filter(|p| {
                            p.extension().and_then(|x| x.to_str()) == Some("sqlite")
                                && p != &db_path
                        });
                    if let Some(previous) = candidates.next() {
                        if candidates.next().is_none() {
                            if std::fs::rename(&previous, &db_path).is_err() {
                                let _ = std::fs::copy(&previous, &db_path);
                            }
                        }
                    }
                }
            }

            // Shared: the PoToken generator persists its session token through the same file,
            // and it is built before AppState takes ownership of everything else.
            let db = Arc::new(Db::open(&db_path)?);
            #[cfg(unix)]
            {
                use std::os::unix::fs::PermissionsExt;
                let _ = std::fs::set_permissions(&db_path, std::fs::Permissions::from_mode(0o600));
            }

            // Session bootstrap (authentication flow startup ordering): load the persisted login session
            // (cookie/dataSyncId/visitorData) from settings; fetch visitorData anonymously
            // (PoToken flow §A) only if we've never stored one.
            let proxy = db.get_setting("proxy").and_then(|raw| {
                match commands::normalize_proxy_setting(&raw) {
                    Ok(value) if !value.is_empty() => Some(value),
                    Ok(_) => None,
                    Err(error) => {
                        tracing::warn!(%error, "discarding invalid persisted proxy setting");
                        db.delete_setting("proxy");
                        None
                    }
                }
            });
            let cookie = db.get_setting("session_cookie").filter(|s| !s.is_empty());
            let data_sync_id = state::persisted_data_sync_id(&db);
            let visitor_data = db.get_setting("visitor_data").filter(|s| !s.is_empty());
            // First run (no stored visitorData): bootstrap it in the background after the window is
            // up, rather than blocking setup on a network GET (up to 60s on a bad connection). See
            // the spawned task after AppState is created.
            let needs_visitor_bootstrap = visitor_data.is_none();
            if cookie.is_some() {
                tracing::info!("loaded persisted login session");
            }

            let session = Session { locale: Locale::from_env(), visitor_data, data_sync_id, cookie };
            let it = match InnerTube::new(session.clone(), proxy.as_deref()) {
                Ok(it) => it,
                Err(error) => {
                    tracing::warn!(%error, "stored proxy is invalid — starting without it");
                    db.delete_setting("proxy");
                    InnerTube::new(session, None).expect("build InnerTube without proxy")
                }
            };
            let clients = Clients::bundled();

            let mut player = Player::new(cache_dir.to_str().unwrap()).expect("init libmpv");
            // Before anything can play: the first track of a restored queue has to come out at the
            // level the user left, not at 100.
            let _ = player.set_volume(state::saved_volume(&db));
            let events = player.take_events().expect("player events");

            // Cipher and PoToken helpers run in hidden webviews behind the orchestrator.
            let config = Arc::new(PlayerConfigStore::new(&data_dir));
            let cipher = Arc::new(CipherDeobfuscator::new(handle.clone(), &data_dir, config));
            let potoken = Arc::new(PoTokenGenerator::new(handle.clone(), db.clone()));
            let orchestrator = Arc::new(Orchestrator::new(
                it.clone(),
                clients.clone(),
                cipher.clone(),
                potoken.clone(),
            ));

            // OS media controls (MPRIS/SMTC/NowPlaying). Its callback resolves AppState lazily, so
            // it's fine to spawn before AppState is managed. fail-soft policy, D11.
            let media = media::spawn(handle.clone());
            // Taskbar preview buttons (#47). Windows-only, and a different API from the SMTC
            // session above.
            #[cfg(target_os = "windows")]
            taskbar::init(&handle);

            // Discord rich presence — off unless the user opted in; parks on its channel until then.
            // The activity label is local vanity text; the fixed APP_ID remains Ryotunes' identity.
            let discord = discord::spawn(
                db.get_setting("discord_rpc").as_deref() == Some("true"),
                db.get_setting("discord_presence_name")
                    .unwrap_or_else(|| discord::DEFAULT_PRESENCE_NAME.into()),
            );

            // Last.fm scrobbler — parks until a session key exists (titlebar connect flow).
            let lastfm =
                lastfm::spawn(db.get_setting("lastfm_session_key").filter(|s| !s.is_empty()));

            // Listen Together session (session protocol). Server URL is a DB setting so "home PC → VPS" is
            // config, not a rebuild. The sync channel feeds the guest-playback bridge below.
            // Listen Together has no baked-in server. Community builds can point it at any
            // compatible ryotunes-sync endpoint; a fresh install stays disconnected until the
            // user chooses one.
            let lt_url =
                db.get_setting("lt_server_url").filter(|u| !u.is_empty()).unwrap_or_default();
            let (lt, lt_sync_rx) = listentogether::LtSession::new(handle.clone(), lt_url);

            let app_state = Arc::new(AppState::new(
                it,
                clients,
                player,
                db,
                handle.clone(),
                orchestrator,
                lt,
                cache_dir.clone(),
                media,
                discord,
                lastfm,
            ));
            app.manage(app_state.clone());

            // Ryoku's own QML apps watch the live palette files. Mirror that with one blocking
            // inotify watcher rather than a JavaScript polling clock so named themes and wallpaper
            // palettes reach Ryotunes immediately while idle CPU stays asleep.
            ryoku_theme::spawn_watcher(handle.clone());

            // Local music artwork reaches the webview over the asset protocol, whose configured
            // scope is empty — the folders it may read are the ones the user picked (local.rs).
            local::allow_music_paths(&handle, &app_state.db);

            // System tray: playback controls + show/quit while running in the background.
            if let Err(e) = tray::init(&handle) {
                tracing::warn!(error = %e, "tray init failed (continuing without tray)");
                app_state.db.set_setting("close_to_tray", "false");
            }

            // Bridge: apply Listen Together sync commands (guest playback / host seed) to AppState.
            {
                let st = app_state.clone();
                let mut rx = lt_sync_rx;
                tauri::async_runtime::spawn(async move {
                    while let Some(cmd) = rx.recv().await {
                        st.apply_sync(cmd).await;
                    }
                });
            }

            // Restore the last session's queue (paused, not autoplaying). UI state §state.
            {
                let st = app_state.clone();
                tauri::async_runtime::spawn(async move {
                    st.restore_queue().await;
                });
            }

            // First-run visitorData bootstrap, off the startup path. `set_visitor_data` writes
            // through the shared session (Arc<RwLock>), so the orchestrator's InnerTube clone sees
            // it; resolves degrade gracefully (no PoToken) until it lands. PoToken flow §A.
            if needs_visitor_bootstrap {
                let st = app_state.clone();
                tauri::async_runtime::spawn(async move {
                    // Do not contend with first paint/TLS/session restoration on a cold launch.
                    tokio::time::sleep(Duration::from_secs(5)).await;
                    match st.it.fetch_visitor_data().await {
                        Ok(vd) => {
                            st.it.set_visitor_data(Some(vd.clone()));
                            st.db.set_setting("visitor_data", &vd);
                            tracing::info!("visitorData bootstrapped (background)");
                        }
                        Err(e) => {
                            tracing::warn!(error = %e, "visitorData bootstrap failed (continuing)")
                        }
                    }
                });
            }

            // Pump mpv events into UI events and queue advancement.
            spawn_event_pump(app_state.clone(), handle, events);

            // Fetch player.js off the first-play path. This is network-only: the cipher WebView is
            // still created lazily if a real stream needs JavaScript deciphering. PoToken is also
            // fully demand-driven; keeping its hidden WebKit process resident at startup costs more
            // than the small first-use latency it saves.
            {
                let cipher = cipher.clone();
                let st = app_state.clone();
                tauri::async_runtime::spawn(async move {
                    // Cold start should become interactive before optional network preparation.
                    tokio::time::sleep(Duration::from_secs(20)).await;
                    if !st.low_resource_mode() && !crate::main_window::is_background_mode() {
                        cipher.prewarm().await;
                    }
                });
            }
            // The hidden cipher/PoToken webviews are burst workers, but every track start needs
            // them: tearing them down 15 s after use meant a fresh WebKit process, a 3 MB JS
            // injection and a full analysis per track. Keep them resident while media is
            // loaded and only release them after a long idle with nothing playing.
            {
                let cipher = cipher.clone();
                let potoken = potoken.clone();
                let st = app_state.clone();
                tauri::async_runtime::spawn(async move {
                    loop {
                        tokio::time::sleep(Duration::from_secs(30)).await;
                        if st.player.has_loaded_media() {
                            continue;
                        }
                        cipher.teardown_if_idle(Duration::from_secs(300)).await;
                        potoken.teardown_if_idle(Duration::from_secs(300)).await;
                    }
                });
            }

            #[cfg(target_os = "linux")]
            {
                tune_webview_labelled(app.handle(), "main");
                // Do not trust compositor/session restore for initial geometry. Ryotunes starts as
                // the same centered floating instrument on every launch.
                main_window::enforce_floating_geometry(app.handle());
                main_window::request_hyprland_float(app.handle());
                // The main WebView starts hidden to avoid exposing an unpainted WebKit frame.
                // Svelte normally reveals it from `onMount`; this bounded native fallback only
                // guarantees that a lost readiness message can never leave a cold launch
                // permanently tray-only. A cold WebKitGTK + SvelteKit mount takes 1-3 s on a
                // laptop, so the fallback sits well past that or it fires before the handshake
                // and shows an unstyled frame.
                main_window::arm_reveal_failsafe(app.handle(), Duration::from_millis(4000));
                spawn_heap_trimmer();
            }
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            commands::search,
            commands::search_page,
            commands::search_page_more,
            commands::search_all,
            commands::search_all_more,
            commands::search_cards,
            commands::search_cards_page,
            commands::search_cards_more,
            commands::play,
            commands::prefetch_stream,
            commands::play_index,
            commands::remove_from_queue,
            commands::clear_queued,
            commands::add_to_queue,
            commands::move_in_queue,
            commands::play_next,
            commands::next_track,
            commands::prev_track,
            commands::toggle_shuffle,
            commands::set_repeat,
            commands::set_stop_after_current,
            commands::toggle_pause,
            commands::seek,
            commands::set_volume,
            commands::set_playback_params,
            commands::get_queue,
            commands::get_playback,
            commands::frontend_ready,
            commands::get_settings,
            commands::discord_status,
            commands::ryoku_theme_tokens,
            commands::set_setting,
            commands::get_stream_clients,
            commands::clear_caches,
            commands::get_account,
            commands::get_account_identities,
            commands::switch_account,
            commands::sign_out,
            commands::login_webview,
            commands::open_mini,
            commands::close_mini,
            commands::get_home,
            commands::get_home_more,
            commands::get_library,
            commands::get_library_albums,
            commands::get_library_artists,
            commands::get_playlist,
            commands::get_playlist_more,
            commands::playlist_index,
            commands::sync_playlist_index,
            commands::play_counts,
            commands::listening_stats,
            commands::get_album,
            commands::get_local_library,
            commands::add_local_folder,
            commands::remove_local_folder,
            commands::get_artist,
            commands::get_browse_grid,
            commands::play_playlist,
            commands::start_radio,
            commands::radio_stations,
            commands::play_radio_station,
            commands::export_playlist_file,
            commands::import_playlist_file,
            commands::rate,
            commands::set_album_saved,
            commands::add_to_playlist,
            commands::add_to_local_playlist,
            commands::remove_from_playlist,
            commands::create_playlist,
            commands::edit_playlist_details,
            commands::set_playlist_cover,
            commands::set_playlist_sort,
            commands::delete_playlist,
            commands::subscribe,
            commands::lt_get_state,
            commands::lt_set_server_url,
            commands::lt_create_room,
            commands::lt_join_room,
            commands::lt_leave,
            commands::lt_approve_join,
            commands::lt_reject_join,
            commands::lt_kick,
            commands::lt_transfer_host,
            commands::lt_suggest,
            commands::lt_approve_suggestion,
            commands::lt_reject_suggestion,
            commands::lt_request_sync,
            commands::get_lyrics,
            commands::lastfm_connect,
            commands::lastfm_disconnect,
            commands::lastfm_status,
            commands::open_external,
        ])
        .on_window_event(|window, event| {
            // Close-to-tray is a real hibernation on Linux: destroy the expensive main WebKit UI
            // while libmpv/backend/MPRIS stay alive. Other platforms keep the proven hide/show
            // behavior. `close_to_tray=false` still means a real application exit.
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                match window.label() {
                    "main" => {
                        let keep_background = window
                            .app_handle()
                            .try_state::<Arc<AppState>>()
                            .map(|s| close_hides(s.db.get_setting("close_to_tray").as_deref()))
                            .unwrap_or(true);
                        if keep_background {
                            #[cfg(target_os = "linux")]
                            {
                                // Allow this close to destroy the WebView. The run-event handler
                                // keeps the native application alive and arms idle shutdown.
                                main_window::prepare_hibernate(window.app_handle());
                            }
                            #[cfg(not(target_os = "linux"))]
                            {
                                api.prevent_close();
                                let _ = window.hide();
                            }
                        } else {
                            api.prevent_close();
                            main_window::request_quit(window.app_handle());
                        }
                    }
                    // Closing the compact player closes only that surface. Playback/backend stay
                    // alive; only the explicit in-widget “Back to Ryotunes” action rebuilds main.
                    mini::LABEL => {
                        mini::save_position(window.app_handle());
                        main_window::prepare_hibernate(window.app_handle());
                        // Do not prevent close: allow this mini WebView to be destroyed.
                    }
                    _ => {}
                }
            }
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|handle, event| {
            match &event {
                tauri::RunEvent::WindowEvent {
                    label,
                    event: tauri::WindowEvent::Destroyed,
                    ..
                } if label == "main" => {
                    if main_window::is_quitting() {
                        // request_quit owns teardown and process exit.
                    } else if main_window::is_background_mode() {
                        main_window::trim_after_hibernate();
                        main_window::schedule_idle_exit(handle);
                    } else {
                        main_window::request_quit(handle);
                    }
                }
                tauri::RunEvent::WindowEvent {
                    label,
                    event: tauri::WindowEvent::Destroyed,
                    ..
                } if label == mini::LABEL => {
                    if !main_window::is_quitting() {
                        main_window::schedule_idle_exit(handle);
                    }
                }
                // Destroying the last WebView normally asks Tauri to terminate. During Linux
                // background playback the tray/native backend intentionally outlive the WebView.
                // Explicit app.exit(0) carries a code and is never intercepted, so Quit and the
                // idle-kill path remain authoritative.
                tauri::RunEvent::ExitRequested { api, code, .. }
                    if code.is_none()
                        && main_window::is_background_mode()
                        && !main_window::is_quitting() =>
                {
                    api.prevent_exit();
                }
                _ => {}
            }
        });
}

/// ✕ hides to tray unless the user explicitly set close_to_tray=false (unset → default on).
fn close_hides(setting: Option<&str>) -> bool {
    setting != Some("false")
}

//// Decide whether a position sample is worth processing at the current runtime cadence.
/// Discontinuities (seek/track change) pass immediately; steady samples are batched. Pure so the
/// foreground and hibernated-background policies are covered by the same tests.
struct PositionThrottle {
    last_emit: std::time::Instant,
    last_pos: f64,
}

impl PositionThrottle {
    fn new() -> Self {
        Self {
            last_emit: std::time::Instant::now() - std::time::Duration::from_secs(1),
            last_pos: f64::NAN,
        }
    }
    fn should_emit(
        &mut self,
        pos: f64,
        now: std::time::Instant,
        cadence: std::time::Duration,
    ) -> bool {
        let dt = now.duration_since(self.last_emit);
        // A jump is any move that couldn't be normal playback since the last emit (+0.75s slack).
        let jumped =
            self.last_pos.is_nan() || (pos - self.last_pos).abs() > dt.as_secs_f64() + 0.75;
        if jumped || dt >= cadence {
            self.last_emit = now;
            self.last_pos = pos;
            return true;
        }
        false
    }
}

fn spawn_event_pump(
    state: Arc<AppState>,
    app: tauri::AppHandle,
    mut events: tokio::sync::mpsc::UnboundedReceiver<PlayerEvent>,
) {
    tauri::async_runtime::spawn(async move {
        let mut throttle = PositionThrottle::new();
        while let Some(ev) = events.recv().await {
            match ev {
                PlayerEvent::Position(p) => {
                    // mpv can publish time-pos much faster than either the UI or background media
                    // integrations need. Visible UI gets ~4 Hz; with no user-facing WebView the
                    // backend drops to ~1 Hz and emits no JavaScript event at all.
                    state.note_position_sample(p);
                    let ui = main_window::has_ui(&app);
                    let cadence = if ui {
                        if state.low_resource_mode() {
                            std::time::Duration::from_millis(500)
                        } else {
                            std::time::Duration::from_millis(250)
                        }
                    } else {
                        std::time::Duration::from_secs(1)
                    };
                    if throttle.should_emit(p, std::time::Instant::now(), cadence) {
                        if ui {
                            let _ = app.emit("position", serde_json::json!({ "position": p }));
                        }
                        state.on_position(p).await;
                    }
                }
                PlayerEvent::Duration(d) => {
                    if main_window::has_ui(&app) {
                        let _ = app.emit("duration", serde_json::json!({ "duration": d }));
                    }
                    state.on_duration(d).await;
                }
                PlayerEvent::Playing(playing) => {
                    let ui = main_window::has_ui(&app);
                    if ui {
                        let _ =
                            app.emit("playback-state", if playing { "playing" } else { "paused" });
                    }
                    if !playing {
                        state.flush_position(); // persist exact resume position on pause
                        if ui {
                            let _ = app.emit(
                                "position",
                                serde_json::json!({ "position": state.current_position() }),
                            );
                        }
                    }
                    state.media_set_playing(playing);
                    // Keep the tray's toggle label honest — this arm is the same chokepoint
                    // MPRIS uses, so tray state can't drift from media-key state.
                    tray::set_playing(&app, playing);
                    state.lt_on_play_state(playing).await; // Listen Together host → broadcast
                    if playing {
                        main_window::cancel_idle_exit();
                    } else {
                        main_window::schedule_idle_exit(&app);
                    }
                }
                PlayerEvent::TrackEnded => {
                    state.on_track_ended().await;
                }
                PlayerEvent::TrackFailed(msg) => {
                    // The track died (dead/403 URL etc). on_track_failed records a WEB_REMIX 403
                    // (stream selection §2), evicts the poisoned cache, and retries the track once via
                    // the fallback clients — only toast the error if it gave up and advanced.
                    tracing::warn!(error = %msg, "track failed");
                    if !state.on_track_failed().await && main_window::has_ui(&app) {
                        let _ = app.emit("playback-error", serde_json::json!({ "message": msg }));
                    }
                }
                PlayerEvent::Error(msg) => {
                    tracing::error!(error = %msg, "player error");
                    if main_window::has_ui(&app) {
                        let _ = app.emit("playback-error", serde_json::json!({ "message": msg }));
                    }
                }
            }
        }
    });
}

#[cfg(test)]
mod tests {
    use super::{close_hides, PositionThrottle};
    use std::time::{Duration, Instant};

    #[test]
    fn close_hides_unless_explicitly_disabled() {
        assert!(close_hides(None)); // fresh install → tray on
        assert!(close_hides(Some("true")));
        assert!(close_hides(Some("garbage")));
        assert!(!close_hides(Some("false")));
    }

    #[test]
    fn steady_playback_throttles_to_250ms() {
        let mut t = PositionThrottle::new();
        let base = Instant::now();
        // First tick ever → emitted regardless of cadence.
        assert!(t.should_emit(0.0, base, Duration::from_millis(250)));
        // 100ms later, small forward move → still within the 250ms window, suppressed.
        assert!(!t.should_emit(0.1, base + Duration::from_millis(100), Duration::from_millis(250)));
        assert!(!t.should_emit(0.2, base + Duration::from_millis(200), Duration::from_millis(250)));
        // 250ms accumulated since last emit → emitted again.
        assert!(t.should_emit(0.25, base + Duration::from_millis(250), Duration::from_millis(250)));
    }

    #[test]
    fn forward_jump_emits_immediately() {
        let mut t = PositionThrottle::new();
        let base = Instant::now();
        assert!(t.should_emit(10.0, base, Duration::from_millis(250)));
        // 50ms later but position jumped +30s (e.g. media-key seek) → emit despite short dt.
        assert!(t.should_emit(40.0, base + Duration::from_millis(50), Duration::from_millis(250)));
    }

    #[test]
    fn backward_jump_emits_immediately() {
        let mut t = PositionThrottle::new();
        let base = Instant::now();
        assert!(t.should_emit(60.0, base, Duration::from_millis(250)));
        // 50ms later but position jumped -30s → emit despite short dt.
        assert!(t.should_emit(30.0, base + Duration::from_millis(50), Duration::from_millis(250)));
    }

    #[test]
    fn background_playback_can_use_one_second_cadence() {
        let mut t = PositionThrottle::new();
        let base = Instant::now();
        assert!(t.should_emit(0.0, base, Duration::from_secs(1)));
        assert!(!t.should_emit(0.5, base + Duration::from_millis(500), Duration::from_secs(1)));
        assert!(t.should_emit(1.0, base + Duration::from_secs(1), Duration::from_secs(1)));
    }

    #[test]
    fn first_tick_ever_emits() {
        let mut t = PositionThrottle::new();
        // NaN last_pos (fresh throttle) → always emits on the very first tick, even at t=now.
        assert!(t.should_emit(5.0, Instant::now(), Duration::from_millis(250)));
    }
}
