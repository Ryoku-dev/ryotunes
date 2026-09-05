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
}

impl Provider {
    pub fn as_str(self) -> &'static str {
        match self {
            Provider::Youtube => "youtube",
            Provider::Spotify => "spotify",
        }
    }
    pub fn parse(s: &str) -> Option<Self> {
        match s {
            "youtube" => Some(Provider::Youtube),
            "spotify" => Some(Provider::Spotify),
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

/// Owns the Spotify sign-in, the current client and the catalogue selector. Construct once with the
/// daemon's data directory; `spotify` credentials land under `<data_dir>/spotify`.
pub struct SpotifyState {
    provider: SpotifyProvider,
    client: RwLock<Option<Arc<Client>>>,
    selected: parking_lot::RwLock<Provider>,
}

impl SpotifyState {
    pub fn new(data_dir: PathBuf, selected: Provider) -> Self {
        Self {
            provider: SpotifyProvider::new(data_dir),
            client: RwLock::new(None),
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

    /// Whether a credential file exists to restore from (no network).
    pub fn stored(&self) -> bool {
        self.provider.stored()
    }

    /// Whether a client is currently signed in.
    pub async fn status(&self) -> bool {
        self.client.read().await.is_some()
    }

    /// The signed-in client, cloned out for use without holding the lock.
    pub async fn client(&self) -> Option<Arc<Client>> {
        self.client.read().await.clone()
    }

    /// Restore a client from cached credentials. Returns whether one was restored.
    pub async fn restore(&self) -> Result<bool> {
        match self.provider.restore().await? {
            Some(client) => {
                *self.client.write().await = Some(Arc::new(client));
                Ok(true)
            }
            None => Ok(false),
        }
    }

    /// Run the OAuth flow. `on_url` receives the authorization URL to hand to the client; on success
    /// the resulting client is stored.
    pub async fn sign_in<F>(&self, on_url: F) -> Result<()>
    where
        F: FnOnce(String) + Send + 'static,
    {
        let client = self.provider.sign_in(on_url).await?;
        *self.client.write().await = Some(Arc::new(client));
        Ok(())
    }

    /// Forget the cached credentials and drop the current client.
    pub async fn sign_out(&self) {
        self.provider.sign_out();
        *self.client.write().await = None;
    }
}
