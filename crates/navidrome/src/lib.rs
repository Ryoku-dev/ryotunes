//! Navidrome provider for Ryotunes.
//!
//! Navidrome speaks Subsonic, a protocol designed for a phone client on a slow connection: one
//! `GET`, one JSON envelope, no OAuth and no refresh token. The crate owns that funnel, the salted
//! token every request carries, the slice of the catalogue a player needs, and the two URLs a
//! track and its artwork are opened by. It persists nothing: the host stores the [`NavidromeAuth`]
//! blob, which never contains the password.
//!
//! Two rules run through it. Server ids are opaque strings, placed into URLs only through query
//! pairs so an id containing `&`, `#` or a space is encoded rather than mangled. And an error a
//! Subsonic server reports is mapped once, here, so the daemon can tell "sign in again" from
//! "the server is unreachable" without reading status codes.
//!
//! The `nd:` id prefixes the rest of Ryotunes uses are the bridge's business, not this crate's.

mod auth;
mod client;
mod models;
mod stream;

/// The protocol version every request declares. Navidrome implements 1.16.1; OpenSubsonic
/// extensions are negotiated on top of it and never change the version number.
pub const SUBSONIC_VERSION: &str = "1.16.1";
/// The `c` parameter. Servers show it in their logs, so it names the application, not the user.
pub const CLIENT_NAME: &str = "Ryotunes";

pub use auth::{normalise_server, Capabilities, Credentials, NavidromeAuth};
pub use client::{AlbumList, Navidrome, StarKind};
pub use models::{
    Album, Artist, ArtistIndex, Artists, Extension, LyricLine, LyricsList, Playlist, SearchResults,
    Song, Starred, StructuredLyrics, User,
};
pub use stream::Covers;

/// Everything that can go wrong, kept narrow enough that a caller can act on each arm.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// The transport failed before an answer arrived.
    #[error("navidrome request failed: {0}")]
    Http(#[from] reqwest::Error),
    /// The server answered, but not with the Subsonic envelope.
    #[error("navidrome sent json that did not parse: {0}")]
    Json(#[from] serde_json::Error),
    /// The server answered and refused, with a code from the Subsonic error table.
    #[error("navidrome answered error {code}: {message}")]
    Api { code: u32, message: String },
    /// Wrong user or password (code 40). The host clears the saved auth and asks for it again.
    #[error("navidrome rejected the username or password")]
    AuthFailed,
    /// The server does not let this account use that feature (codes 41 through 44).
    #[error("this navidrome account is not allowed to do that")]
    AuthUnsupported,
    /// A call that needs a signed-in user was made without one.
    #[error("not signed in to navidrome")]
    SignedOut,
    /// The server does not have the item (code 70).
    #[error("navidrome does not have that item")]
    NotFound,
    /// A local precondition failed: a malformed server address, a missing field. The message is
    /// written for the sign-in form, because that is where these surface.
    #[error("{0}")]
    Unsupported(&'static str),
}

impl Error {
    /// Map a Subsonic error code to the arm the daemon acts on. Codes 40 and 70 are the two the
    /// protocol gives a fixed meaning; 41–44 all mean "this account may not", which one message
    /// covers. Anything else keeps the server's own text.
    pub fn from_api(code: u32, message: impl Into<String>) -> Self {
        match code {
            40 => Error::AuthFailed,
            41..=44 => Error::AuthUnsupported,
            70 => Error::NotFound,
            code => Error::Api { code, message: message.into() },
        }
    }
}

pub type Result<T, E = Error> = std::result::Result<T, E>;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_error_table_maps_the_codes_the_daemon_acts_on() {
        assert!(matches!(Error::from_api(40, "Wrong username or password"), Error::AuthFailed));
        for code in 41..=44 {
            assert!(matches!(Error::from_api(code, "nope"), Error::AuthUnsupported), "{code}");
        }
        assert!(matches!(Error::from_api(70, "not found"), Error::NotFound));
    }

    #[test]
    fn an_unknown_code_keeps_the_servers_own_words() {
        let err = Error::from_api(10, "Required parameter is missing");
        assert!(matches!(err, Error::Api { code: 10, .. }));
        assert_eq!(err.to_string(), "navidrome answered error 10: Required parameter is missing");
    }

    #[test]
    fn auth_failure_text_asks_for_a_password_without_quoting_one() {
        let text = Error::AuthFailed.to_string();
        assert!(text.contains("username or password"));
    }
}
