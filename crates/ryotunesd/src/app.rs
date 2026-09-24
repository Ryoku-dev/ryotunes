//! Build the core exactly as `src-tauri/src/lib.rs` `setup` does, then pump mpv/media/Listen
//! Together into it. The host seams (`JsBridge`, `LoginFlow`, `EventSink`) are the GTK/socket ones;
//! everything else is the same construction the Tauri app runs, so the daemon opens the app's
//! existing database, cookies and caches.

use std::path::PathBuf;
use std::sync::Arc;
use std::time::{Duration, Instant};

use innertube::{Clients, InnerTube, Locale, Session};
use player::{Player, PlayerEvent};
use ryotunes_core::cipher::{CipherDeobfuscator, PlayerConfigStore};
use ryotunes_core::db::Db;
use ryotunes_core::host::{EventSink, JsBridge, LoginFlow, Paths};
use ryotunes_core::listentogether::{LtSession, SyncCommand};
use ryotunes_core::media::{MediaControlEvent, MediaPosition, SeekDirection};
use ryotunes_core::orchestrator::Orchestrator;
use ryotunes_core::potoken::PoTokenGenerator;
use ryotunes_core::state::{self, AppState};
use ryotunes_core::{discord, lastfm, media};
use tokio::runtime::Handle;
use tokio::sync::mpsc::UnboundedReceiver;

use crate::lifecycle::Lifecycle;
use crate::methods::normalize_proxy_setting;
use crate::sink::SocketSink;

/// The XDG directories Tauri resolves on Linux for `dev.ryoku.ryotunes`, so the daemon shares the
/// app's data (`$XDG_DATA_HOME/dev.ryoku.ryotunes`) and audio cache
/// (`$XDG_CACHE_HOME/dev.ryoku.ryotunes/audio`). A relative/unset `XDG_*` is ignored per the spec.
pub fn paths() -> Paths {
    const APP_ID: &str = "dev.ryoku.ryotunes";
    Paths {
        data_dir: xdg_dir("XDG_DATA_HOME", ".local/share").join(APP_ID),
        cache_dir: xdg_dir("XDG_CACHE_HOME", ".cache").join(APP_ID).join("audio"),
    }
}

fn xdg_dir(var: &str, fallback: &str) -> PathBuf {
    std::env::var_os(var)
        .map(PathBuf::from)
        .filter(|p| p.is_absolute())
        .unwrap_or_else(|| home_dir().join(fallback))
}

fn home_dir() -> PathBuf {
    std::env::var_os("HOME").map(PathBuf::from).unwrap_or_else(|| PathBuf::from("/"))
}

/// Construct the whole core: db + transport + player + orchestrator + integrations + `AppState`,
/// then wire the post-construction background tasks Tauri's `setup` spawns (queue restore, first-run
/// visitorData bootstrap, cipher prewarm, cipher/PoToken idle teardown). Returns the receivers the
/// caller hands to [`spawn_pumps`]. This is a verbatim port of `src-tauri/src/lib.rs` lines 169-334
/// with the Tauri host seams replaced by the daemon's.
pub fn build(
    paths: Paths,
    sink: Arc<SocketSink>,
    js: Arc<dyn JsBridge>,
    login: Arc<dyn LoginFlow>,
    rt: &Handle,
) -> anyhow::Result<(
    Arc<AppState>,
    UnboundedReceiver<PlayerEvent>,
    UnboundedReceiver<MediaControlEvent>,
    UnboundedReceiver<SyncCommand>,
)> {
    let data_dir = paths.data_dir.clone();
    let cache_dir = paths.cache_dir.clone();
    std::fs::create_dir_all(&data_dir)?;
    std::fs::create_dir_all(&cache_dir)?;

    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let private_dir = std::fs::Permissions::from_mode(0o700);
        let _ = std::fs::set_permissions(&data_dir, private_dir.clone());
        if let Some(cache_root) = cache_dir.parent() {
            let _ = std::fs::set_permissions(cache_root, private_dir.clone());
        }
        let _ = std::fs::set_permissions(&cache_dir, private_dir);
    }

    // Same database name and legacy-name migration as the Tauri app: adopt the sole sibling
    // `.sqlite` when the new file is absent, so a build switch never abandons state.
    let db_path = data_dir.join("ryotunes.sqlite");
    if !db_path.exists() {
        if let Ok(entries) = std::fs::read_dir(&data_dir) {
            let mut candidates = entries.filter_map(Result::ok).map(|e| e.path()).filter(|p| {
                p.extension().and_then(|x| x.to_str()) == Some("sqlite") && p != &db_path
            });
            if let Some(previous) = candidates.next() {
                if candidates.next().is_none() && std::fs::rename(&previous, &db_path).is_err() {
                    let _ = std::fs::copy(&previous, &db_path);
                }
            }
        }
    }

    let db = Arc::new(Db::open(&db_path)?);
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let _ = std::fs::set_permissions(&db_path, std::fs::Permissions::from_mode(0o600));
    }

    // Session bootstrap: persisted proxy + login session (cookie/dataSyncId/visitorData). A missing
    // visitorData is fetched anonymously in the background after construction.
    let proxy = db.get_setting("proxy").and_then(|raw| match normalize_proxy_setting(&raw) {
        Ok(value) if !value.is_empty() => Some(value),
        Ok(_) => None,
        Err(error) => {
            tracing::warn!(%error, "discarding invalid persisted proxy setting");
            db.delete_setting("proxy");
            None
        }
    });
    let cookie = db.get_setting("session_cookie").filter(|s| !s.is_empty());
    let data_sync_id = state::persisted_data_sync_id(&db);
    let visitor_data = db.get_setting("visitor_data").filter(|s| !s.is_empty());
    let needs_visitor_bootstrap = visitor_data.is_none();
    if cookie.is_some() {
        tracing::info!("loaded persisted login session");
    }

    let session = Session { locale: Locale::default(), visitor_data, data_sync_id, cookie };
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
    let _ = player.set_volume(state::saved_volume(&db));
    let events = player.take_events().expect("player events");

    // Cipher and PoToken helpers run in the hidden GTK webviews behind the orchestrator.
    let config = Arc::new(PlayerConfigStore::new(&data_dir));
    let cipher = Arc::new(CipherDeobfuscator::new(js.clone(), &data_dir, config));
    let potoken = Arc::new(PoTokenGenerator::new(js.clone(), db.clone()));
    let orchestrator =
        Arc::new(Orchestrator::new(it.clone(), clients.clone(), cipher.clone(), potoken.clone()));

    let sink_dyn: Arc<dyn EventSink> = sink.clone();

    // OS media controls (MPRIS). Presses arrive over `media_rx`; the pump drains them into AppState.
    let (media_tx, media_rx) = tokio::sync::mpsc::unbounded_channel();
    let media = media::spawn(sink_dyn.clone(), media_tx);

    let discord = discord::spawn(
        db.get_setting("discord_rpc").as_deref() == Some("true"),
        db.get_setting("discord_presence_name")
            .unwrap_or_else(|| discord::DEFAULT_PRESENCE_NAME.into()),
    );

    let lastfm = lastfm::spawn(rt, db.get_setting("lastfm_session_key").filter(|s| !s.is_empty()));

    let lt_url = db.get_setting("lt_server_url").filter(|u| !u.is_empty()).unwrap_or_default();
    let (lt, lt_sync_rx) = LtSession::new(sink_dyn.clone(), lt_url);

    let app_state = Arc::new(AppState::new(
        it,
        clients,
        player,
        db,
        sink_dyn,
        login,
        paths,
        orchestrator,
        lt,
        media,
        discord,
        lastfm,
    ));

    // Restore the last session's queue (paused, not autoplaying).
    {
        let st = app_state.clone();
        rt.spawn(async move {
            st.restore_queue().await;
        });
    }

    // First-run visitorData bootstrap, off the startup path. A missing visitorData makes every
    // anonymous fallback /player request hit YouTube's "confirm you're not a bot" gate, so one
    // failed fetch (offline at boot, consent interstitial, transient 5xx) used to leave the daemon
    // unable to play *any* track until the next restart. Retry with backoff until it lands.
    if needs_visitor_bootstrap {
        let st = app_state.clone();
        rt.spawn(async move {
            const DELAYS: [Duration; 5] = [
                Duration::from_secs(5),
                Duration::from_secs(20),
                Duration::from_secs(60),
                Duration::from_secs(180),
                Duration::from_secs(600),
            ];
            for (attempt, delay) in DELAYS.iter().enumerate() {
                tokio::time::sleep(*delay).await;
                match st.it.fetch_visitor_data().await {
                    Ok(vd) => {
                        st.it.set_visitor_data(Some(vd.clone()));
                        st.db.set_setting("visitor_data", &vd);
                        tracing::info!("visitorData bootstrapped (background)");
                        return;
                    }
                    Err(e) => tracing::warn!(
                        attempt = attempt + 1,
                        error = %e,
                        "visitorData bootstrap failed; will retry"
                    ),
                }
            }
            tracing::warn!("visitorData bootstrap gave up; playback will retry on demand");
        });
    }

    // Fetch player.js off the first-play path once the daemon has settled.
    {
        let cipher = cipher.clone();
        let st = app_state.clone();
        rt.spawn(async move {
            tokio::time::sleep(Duration::from_secs(20)).await;
            if !st.low_resource_mode() {
                cipher.prewarm().await;
            }
        });
    }

    // Keep the hidden webviews resident while media is loaded; release them after a long idle.
    {
        let cipher = cipher.clone();
        let potoken = potoken.clone();
        let st = app_state.clone();
        rt.spawn(async move {
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

    Ok((app_state, events, media_rx, lt_sync_rx))
}

/// Spawn the three drains that feed `AppState`: the mpv event pump, the OS media-control drain, and
/// the Listen Together sync bridge. Ports `src-tauri/src/lib.rs` `spawn_event_pump` and the two
/// `setup` drains, with `has_ui()` replaced by "any connection subscribed" and the Tauri-only tray
/// / idle-exit calls routed into Task 5's [`Lifecycle`] and [`crate::tray`] instead.
pub fn spawn_pumps(
    state: Arc<AppState>,
    events: UnboundedReceiver<PlayerEvent>,
    mut media_rx: UnboundedReceiver<MediaControlEvent>,
    mut lt_rx: UnboundedReceiver<SyncCommand>,
    sink: Arc<SocketSink>,
    lifecycle: Arc<Lifecycle>,
) {
    spawn_event_pump(state.clone(), events, sink, lifecycle.clone());

    {
        let st = state.clone();
        let lifecycle = lifecycle.clone();
        tokio::spawn(async move {
            while let Some(ev) = media_rx.recv().await {
                handle_media_event(&st, ev, &lifecycle).await;
            }
        });
    }
    {
        let st = state;
        tokio::spawn(async move {
            while let Some(cmd) = lt_rx.recv().await {
                st.apply_sync(cmd).await;
            }
        });
    }
}

fn spawn_event_pump(
    state: Arc<AppState>,
    mut events: UnboundedReceiver<PlayerEvent>,
    sink: Arc<SocketSink>,
    lifecycle: Arc<Lifecycle>,
) {
    tokio::spawn(async move {
        let mut throttle = PositionThrottle::new();
        while let Some(ev) = events.recv().await {
            // A subscribed connection is the daemon's equivalent of a visible UI: with none, the
            // pump drops to ~1 Hz and emits no socket event, only the backend bookkeeping.
            let ui = sink.subscriber_count() > 0;
            match ev {
                PlayerEvent::Position(p) => {
                    state.note_position_sample(p);
                    let cadence = if ui {
                        if state.low_resource_mode() {
                            Duration::from_millis(500)
                        } else {
                            Duration::from_millis(250)
                        }
                    } else {
                        Duration::from_secs(1)
                    };
                    if throttle.should_emit(p, Instant::now(), cadence) {
                        if ui {
                            state.emit("position", serde_json::json!({ "position": p }));
                        }
                        state.on_position(p).await;
                    }
                }
                PlayerEvent::Duration(d) => {
                    if ui {
                        state.emit("duration", serde_json::json!({ "duration": d }));
                    }
                    state.on_duration(d).await;
                }
                PlayerEvent::Playing(playing) => {
                    if ui {
                        state.emit("playback-state", if playing { "playing" } else { "paused" });
                    }
                    if !playing {
                        state.flush_position();
                        if ui {
                            state.emit(
                                "position",
                                serde_json::json!({ "position": state.current_position() }),
                            );
                        }
                    }
                    state.media_set_playing(playing);
                    state.lt_on_play_state(playing).await;
                    // Task 5: keep the tray label and idle-exit deadline in step with playback,
                    // where the Tauri host called `tray::set_playing` / `schedule_idle_exit`.
                    crate::tray::set_playing(playing);
                    lifecycle.playing_changed(playing);
                }
                PlayerEvent::TrackEnded => {
                    state.on_track_ended().await;
                }
                PlayerEvent::TrackFailed(msg) => {
                    tracing::warn!(error = %msg, "track failed");
                    if !state.on_track_failed().await && ui {
                        state.emit("playback-error", serde_json::json!({ "message": msg }));
                    }
                }
                PlayerEvent::Error(msg) => {
                    tracing::error!(error = %msg, "player error");
                    if ui {
                        state.emit("playback-error", serde_json::json!({ "message": msg }));
                    }
                }
            }
        }
    });
}

/// Route an OS media-control press into the same `AppState` methods the socket commands use. Ports
/// `src-tauri/src/lib.rs` `handle_media_event`; the Tauri Stop-arm idle-exit scheduling is now the
/// [`Lifecycle`] call below.
async fn handle_media_event(
    state: &Arc<AppState>,
    event: MediaControlEvent,
    lifecycle: &Arc<Lifecycle>,
) {
    match event {
        MediaControlEvent::Play => state.resume().await,
        MediaControlEvent::Toggle => state.resume_or_toggle().await,
        MediaControlEvent::Pause => {
            let _ = state.player.pause();
        }
        MediaControlEvent::Stop => {
            let _ = state.player.stop();
            state.media_set_playing(false);
            lifecycle.playing_changed(false);
        }
        MediaControlEvent::Next => state.next_in_queue().await,
        MediaControlEvent::Previous => state.prev_in_queue().await,
        MediaControlEvent::SetPosition(MediaPosition(pos)) => {
            let _ = state.player.seek(pos.as_secs_f64());
        }
        MediaControlEvent::SeekBy(dir, by) => {
            let delta = if matches!(dir, SeekDirection::Forward) {
                by.as_secs_f64()
            } else {
                -by.as_secs_f64()
            };
            let _ = state.player.seek((state.current_position() + delta).max(0.0));
        }
        MediaControlEvent::Seek(dir) => {
            let delta = if matches!(dir, SeekDirection::Forward) { 10.0 } else { -10.0 };
            let _ = state.player.seek((state.current_position() + delta).max(0.0));
        }
        _ => {}
    }
}

/// Decide whether a position sample is worth emitting at the current cadence. Discontinuities
/// (seek / track change) pass immediately; steady samples are batched. Ported verbatim from the
/// Tauri host so the two share one policy.
struct PositionThrottle {
    last_emit: Instant,
    last_pos: f64,
}

impl PositionThrottle {
    fn new() -> Self {
        Self { last_emit: Instant::now() - Duration::from_secs(1), last_pos: f64::NAN }
    }
    fn should_emit(&mut self, pos: f64, now: Instant, cadence: Duration) -> bool {
        let dt = now.duration_since(self.last_emit);
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

#[cfg(test)]
mod tests {
    use super::PositionThrottle;
    use std::time::{Duration, Instant};

    #[test]
    fn steady_playback_throttles_to_250ms() {
        let mut t = PositionThrottle::new();
        let base = Instant::now();
        assert!(t.should_emit(0.0, base, Duration::from_millis(250)));
        assert!(!t.should_emit(0.1, base + Duration::from_millis(100), Duration::from_millis(250)));
        assert!(!t.should_emit(0.2, base + Duration::from_millis(200), Duration::from_millis(250)));
        assert!(t.should_emit(0.25, base + Duration::from_millis(250), Duration::from_millis(250)));
    }

    #[test]
    fn discontinuities_emit_immediately() {
        let mut t = PositionThrottle::new();
        let base = Instant::now();
        assert!(t.should_emit(10.0, base, Duration::from_millis(250)));
        // Forward jump (media-key seek) despite a short dt.
        assert!(t.should_emit(40.0, base + Duration::from_millis(50), Duration::from_millis(250)));
        // Backward jump too.
        assert!(t.should_emit(10.0, base + Duration::from_millis(80), Duration::from_millis(250)));
    }

    #[test]
    fn background_cadence_is_one_second() {
        let mut t = PositionThrottle::new();
        let base = Instant::now();
        assert!(t.should_emit(0.0, base, Duration::from_secs(1)));
        assert!(!t.should_emit(0.5, base + Duration::from_millis(500), Duration::from_secs(1)));
        assert!(t.should_emit(1.0, base + Duration::from_secs(1), Duration::from_secs(1)));
    }
}
