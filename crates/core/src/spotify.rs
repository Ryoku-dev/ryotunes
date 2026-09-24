//! Spotify provider state, held for the daemon.
//!
//! A thin wrapper around [`ryotunes_spotify::SpotifyProvider`] plus the currently signed-in
//! [`ryotunes_spotify::Client`], behind a [`tokio::sync::RwLock`] so many readers (playback,
//! browsing) share one client. The daemon adds the actual command methods on top of this (see the
//! `## Daemon wiring` section of `crates/spotify/README.md`); this file deliberately wires none.

use std::path::PathBuf;
use std::sync::Arc;

use anyhow::Result;
use ryotunes_spotify::{Client, SpotifyProvider};
use tokio::sync::RwLock;

/// Which catalogue the browsing commands (search, home, library, album, artist, playlist) read
/// from. YouTube Music and Spotify are peers: the queue may hold tracks from both, told apart by
/// their ids (`spotify:track:…` vs a video id), and playback of either goes through the one mpv.
#[derive(Clone, Copy, Debug, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Provider {
    Youtube,
    Spotify,
    Soundcloud,
}

impl Provider {
    pub fn as_str(self) -> &'static str {
        match self {
            Provider::Youtube => "youtube",
            Provider::Spotify => "spotify",
            Provider::Soundcloud => "soundcloud",
        }
    }
    pub fn parse(s: &str) -> Option<Self> {
        match s {
            "youtube" => Some(Provider::Youtube),
            "spotify" => Some(Provider::Spotify),
            "soundcloud" => Some(Provider::Soundcloud),
            _ => None,
        }
    }
}

pub const SPOTIFY_TRACK_PREFIX: &str = "spotify:track:";

/// True for any id the daemon minted for a Spotify item (`spotify:<kind>:<base62>`).
pub fn is_spotify_id(id: &str) -> bool {
    id.starts_with("spotify:")
}

/// The bare base62 id of a Spotify track id, or None for anything else.
pub fn spotify_track_id(id: &str) -> Option<&str> {
    id.strip_prefix(SPOTIFY_TRACK_PREFIX).filter(|s| !s.is_empty())
}

pub const SC_TRACK_PREFIX: &str = "sc:track:";
pub const SC_USER_PREFIX: &str = "sc:user:";
pub const SC_PLAYLIST_PREFIX: &str = "sc:playlist:";
pub const SC_SYSTEM_PREFIX: &str = "sc:system:";

/// True for any id the daemon minted for a SoundCloud item (`sc:<kind>:<num>`). SoundCloud tracks
/// stream over plain HTTPS (an HLS m3u8), not the Spotify FIFO — see `AppState::resolve`.
pub fn is_sc_id(id: &str) -> bool {
    id.starts_with("sc:")
}

/// The numeric id of a SoundCloud track id (`sc:track:<num>`), or None for anything else.
pub fn sc_track_id(id: &str) -> Option<u64> {
    id.strip_prefix(SC_TRACK_PREFIX)?.parse().ok()
}

/// The numeric id of a SoundCloud user id (`sc:user:<num>`).
pub fn sc_user_id(id: &str) -> Option<u64> {
    id.strip_prefix(SC_USER_PREFIX)?.parse().ok()
}

/// The numeric id of a SoundCloud playlist/album id (`sc:playlist:<num>`).
pub fn sc_playlist_id(id: &str) -> Option<u64> {
    id.strip_prefix(SC_PLAYLIST_PREFIX)?.parse().ok()
}

/// The permalink of a SoundCloud system-playlist id (`sc:system:<permalink>`) — a curated/charts
/// playlist keyed by permalink rather than a numeric id. `None` for anything else.
pub fn sc_system_id(id: &str) -> Option<&str> {
    id.strip_prefix(SC_SYSTEM_PREFIX).filter(|s| !s.is_empty())
}

/// Owns the Spotify sign-in, the current client and the catalogue selector. Construct once with the
/// daemon's data directory; `spotify` credentials land under `<data_dir>/spotify`.
pub struct SpotifyState {
    provider: SpotifyProvider,
    client: RwLock<Option<Arc<Client>>>,
    /// The one on-demand session recovery this process is allowed (see `client_or_recover`).
    /// Reset by a fresh sign-in or sign-out, which start a new credential lifetime.
    recovery_used: std::sync::atomic::AtomicBool,
    /// Serialises session restores so an on-demand recovery never runs a second librespot
    /// connect against the same cached credentials while the startup restore is mid-flight.
    recover_lock: tokio::sync::Mutex<()>,
    selected: parking_lot::RwLock<Provider>,
}

impl SpotifyState {
    pub fn new(data_dir: PathBuf, selected: Provider) -> Self {
        Self {
            provider: SpotifyProvider::new(data_dir),
            client: RwLock::new(None),
            recovery_used: std::sync::atomic::AtomicBool::new(false),
            recover_lock: tokio::sync::Mutex::new(()),
            selected: parking_lot::RwLock::new(selected),
        }
    }

    /// The catalogue the browsing commands read from right now.
    pub fn selected(&self) -> Provider {
        *self.selected.read()
    }

    pub fn select(&self, p: Provider) {
        *self.selected.write() = p;
    }

    /// Whether browsing should go to Spotify: selected AND signed in. Selected without a session
    /// is the sign-in state the client shows, not a catalogue.
    pub async fn browsing_spotify(&self) -> bool {
        self.selected() == Provider::Spotify && self.status().await
    }

    /// Whether browsing should go to SoundCloud. SoundCloud is guest-only (no account), so this
    /// is just the selection — there is no sign-in gate like [`Self::browsing_spotify`].
    pub fn browsing_soundcloud(&self) -> bool {
        self.selected() == Provider::Soundcloud
    }

    /// Whether a credential file exists to restore from (no network).
    pub fn stored(&self) -> bool {
        self.provider.stored()
    }

    /// Whether a usable client exists. A session librespot invalidated (network drop, Spotify
    /// killed it server-side) is *not* usable: it is dropped here so a later `restore`/
    /// `client_or_recover` sees the truth and announces the sign-out to the daemon's event sink.
    pub async fn status(&self) -> bool {
        self.live_client().await.is_some()
    }

    /// The signed-in client, cloned out for use without holding the lock. A dead session reads as
    /// signed out (and is dropped), so callers never hand a doomed client to librespot.
    pub async fn client(&self) -> Option<Arc<Client>> {
        self.live_client().await
    }

    /// A session that is both present and still connected; drops (and logs) one that died.
    async fn live_client(&self) -> Option<Arc<Client>> {
        let live = {
            let guard = self.client.read().await;
            match guard.as_ref() {
                Some(c) if c.alive() => Some(Arc::clone(c)),
                Some(_) => None,
                None => return None,
            }
        };
        if live.is_none() {
            // Drops a session librespot invalidated; `restore_locked` serialises actual
            // (re)connects so this never races a connect in flight.
            let dead = self.client.write().await.take();
            if dead.is_some() {
                tracing::warn!("spotify: the session was invalidated; treating as signed out");
            }
        }
        live
    }

    /// The client for playback/browsing, recovering a dead or missing one from the saved
    /// credentials the first time this process needs it. This is the fix for "every song is
    /// skipped as unavailable, and it asks me to sign in every time I open it": a startup
    /// restore that raced with server bind, or a session that died mid-life, used to leave the
    /// daemon permanently signed-out for that run. Recovery is attempted exactly once per
    /// credential lifetime so a genuinely dead credential pair cannot stall every track start.
    pub async fn client_or_recover(&self) -> Option<Arc<Client>> {
        if let Some(client) = self.live_client().await {
            return Some(client);
        }
        if !self.provider.stored()
            || self.recovery_used.swap(true, std::sync::atomic::Ordering::Relaxed)
        {
            return None;
        }
        // `restore` serializes against the startup restore itself.
        match self.restore().await {
            Ok(true) => {
                tracing::info!("spotify: recovered the cached session on demand");
                self.live_client().await
            }
            Ok(false) => None,
            Err(e) => {
                tracing::warn!(error = %e, "spotify: on-demand session recovery failed");
                None
            }
        }
    }

    /// Restore a client from cached credentials. Returns whether one was restored (or was
    /// already live). Serialized on `recover_lock`: a startup restore still connecting and an
    /// on-demand recovery racing over one credential cache is how a good session gets
    /// invalidated — and under the lock the restore we waited for may already have landed, in
    /// which case this simply returns true.
    pub async fn restore(&self) -> Result<bool> {
        let _guard = self.recover_lock.lock().await;
        self.restore_locked().await
    }

    /// The caller holds `recover_lock`.
    async fn restore_locked(&self) -> Result<bool> {
        if self.live_client().await.is_some() {
            return Ok(true);
        }
        match self.provider.restore().await? {
            Some(client) => {
                *self.client.write().await = Some(Arc::new(client));
                Ok(true)
            }
            None => Ok(false),
        }
    }

    /// Run the OAuth flow. `on_url` receives the authorization URL to hand to the client; on success
    /// the resulting client is stored, and a fresh credential lifetime arms recovery again.
    pub async fn sign_in<F>(&self, on_url: F) -> Result<()>
    where
        F: FnOnce(String) + Send + 'static,
    {
        let client = self.provider.sign_in(on_url).await?;
        *self.client.write().await = Some(Arc::new(client));
        self.recovery_used.store(false, std::sync::atomic::Ordering::Relaxed);
        Ok(())
    }

    /// Forget the cached credentials and drop the current client.
    pub async fn sign_out(&self) {
        self.provider.sign_out();
        *self.client.write().await = None;
        self.recovery_used.store(false, std::sync::atomic::Ordering::Relaxed);
    }
}
