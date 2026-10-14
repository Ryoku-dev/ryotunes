//! TIDAL provider for Ryotunes.
//!
//! The crate owns one HTTP funnel, the two OAuth flows TIDAL offers a desktop client, the slice of
//! the catalogue a player needs, and the step that turns a track into something openable. It holds
//! no credentials of its own and persists nothing except its own small cache: the host supplies the
//! client ids, stores the auth blob, and is called back whenever a token rotates.
//!
//! One rule runs through it. TIDAL will sometimes answer with an asset that is protected, or with a
//! thirty second preview in place of the track. Both are reported as errors, [`Error::Protected`]
//! and [`Error::Preview`]. Neither is worked around, and a caller that treats either as a retryable
//! condition has misread it.

mod auth;
mod client;
mod models;
mod stream;

pub use auth::{
    Credentials, DeviceLogin, Entitlement, PkceChallenge, TidalAuth, DEVICE_AUTH_URL,
    PKCE_AUTH_URL, SCOPE, TOKEN_URL,
};
pub use client::Tidal;
pub use models::{cover_url, Album, Artist, Favourite, Page, Playlist, SearchResults, Track};
pub use stream::{Quality, Source, Stream};

/// Everything that can go wrong, kept narrow enough that a caller can act on each arm.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("tidal request failed: {0}")]
    Http(#[from] reqwest::Error),
    #[error("tidal sent json that did not parse: {0}")]
    Json(#[from] serde_json::Error),
    /// The API answered, and said no.
    #[error("tidal answered {status} for {url}")]
    Api { status: u16, url: String },
    /// No client id for the flow the caller asked for. A configuration state, not a fault: the
    /// host decides where those ids come from, and this crate ships none.
    #[error("no tidal client credentials configured for the {0} flow")]
    NoCredentials(&'static str),
    /// A call that needs a signed-in user was made without one.
    #[error("not signed in to tidal")]
    SignedOut,
    /// The asset is protected. Not retryable, and not to be worked around.
    #[error("tidal returned a protected asset ({0}); this build does not play protected streams")]
    Protected(String),
    /// The account is not entitled to the tier that was asked for, so a sample came back instead.
    #[error("tidal returned a {0} asset instead of the full track")]
    Preview(String),
    /// The manifest arrived but could not be read.
    #[error("tidal manifest could not be read: {0}")]
    Manifest(String),
    /// The device flow is still waiting for the user to approve it.
    #[error("waiting for the tidal device login to be approved")]
    Pending,
    /// The device code died before the user approved it.
    #[error("the tidal device login expired before it was approved")]
    LoginExpired,
}

pub type Result<T, E = Error> = std::result::Result<T, E>;
