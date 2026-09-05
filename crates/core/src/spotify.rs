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

/// Owns the Spotify sign-in and the current client. Construct once with the daemon's data
/// directory; `spotify` credentials land under `<data_dir>/spotify`.
pub struct SpotifyState {
    provider: SpotifyProvider,
    client: RwLock<Option<Arc<Client>>>,
}

impl SpotifyState {
    pub fn new(data_dir: PathBuf) -> Self {
        Self { provider: SpotifyProvider::new(data_dir), client: RwLock::new(None) }
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
