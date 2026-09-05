//! Sign-in lifecycle. Adapted from nolight132/sonora `spotify/mod.rs`: Sonora's
//! `impl MusicProvider for SpotifyProvider` (with its GPUI prompt/input plumbing) becomes inherent
//! async methods returning a [`Client`], and sign-in surfaces the authorization URL through a
//! caller-supplied sink instead of opening a browser.

use std::path::PathBuf;

use anyhow::Result;

use crate::auth::{self, AuthConfig};
use crate::credentials;
use crate::Client;

/// Provider settings the daemon may override. Defaults are Sonora's public web client id and the
/// loopback redirect the OAuth listener binds.
#[derive(Clone, Debug)]
pub struct SpotifyConfig {
    pub client_id: String,
    pub redirect_uri: String,
}

impl Default for SpotifyConfig {
    fn default() -> Self {
        Self {
            client_id: auth::DEFAULT_CLIENT_ID.to_owned(),
            redirect_uri: auth::DEFAULT_REDIRECT_URI.to_owned(),
        }
    }
}

/// Owns the Spotify sign-in configuration and its on-disk credential cache. One per process.
pub struct SpotifyProvider {
    auth: AuthConfig,
}

impl SpotifyProvider {
    /// Build a provider whose credential cache and Pathfinder hash registry live under
    /// `data_dir/spotify`.
    pub fn new(data_dir: PathBuf) -> Self {
        Self::with_config(data_dir, SpotifyConfig::default())
    }

    /// Like [`SpotifyProvider::new`], with explicit provider settings.
    pub fn with_config(data_dir: PathBuf, config: SpotifyConfig) -> Self {
        let cache_dir = data_dir.join("spotify");
        credentials::set_root(cache_dir.clone());
        Self {
            auth: AuthConfig {
                client_id: config.client_id,
                redirect_uri: config.redirect_uri,
                cache_dir,
            },
        }
    }

    /// Whether a credential file exists to restore from.
    pub fn stored(&self) -> bool {
        self.auth.file().exists()
    }

    /// Restore a client from cached credentials. `Ok(None)` when nothing is cached.
    pub async fn restore(&self) -> Result<Option<Client>> {
        match auth::restore(&self.auth).await? {
            Some(session) => Ok(Some(Client::new(session)?)),
            None => Ok(None),
        }
    }

    /// Run the OAuth authorization-code flow. `on_url` receives the authorization URL to open (the
    /// daemon hands it to the client); this then completes the exchange and returns a signed-in
    /// client. Requires a Spotify Premium account.
    pub async fn sign_in<F>(&self, on_url: F) -> Result<Client>
    where
        F: FnOnce(String) + Send + 'static,
    {
        let session = auth::sign_in(&self.auth, on_url).await?;
        Client::new(session)
    }

    /// Forget the cached credentials.
    pub fn sign_out(&self) {
        auth::forget(&self.auth);
    }
}
