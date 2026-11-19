//! Sign-in state, the salted token Subsonic asks for, and what the server said it supports.
//!
//! Subsonic never transmits a password. A request carries the user, a salt and
//! `t = md5(password + salt)`; the server recomputes the digest from the password it already
//! stores. That is weaker than OAuth — the token is a password equivalent for as long as the salt
//! is unchanged — so the password itself is never persisted, only the derived token, and the salt
//! is reused rather than rotated on every launch so cover URLs already on screen keep working.

use md5::{Digest, Md5};
use rand::Rng;
use serde::{Deserialize, Serialize};

use crate::{Error, Result};

/// Subsonic only asks for six characters; sixteen alphanumerics keep the digest well away from a
/// birthday problem and keep the token URL-safe without escaping.
const SALT_LEN: usize = 16;
const SALT_ALPHABET: &[u8] = b"abcdefghijklmnopqrstuvwxyz0123456789";
/// A pasted address, with room for a reverse-proxy path. Anything longer is a mistake, not a host.
const MAX_SERVER_LEN: usize = 2_048;

/// Normalise a server address the user pasted into the sign-in form.
///
/// Accepts only an absolute `http`/`https` URL, because a Subsonic request is a plain `GET` the
/// daemon makes on the user's behalf — the scheme is the whole transport-security story. A trailing
/// slash and a pasted `/rest` suffix are dropped so every request appends `/rest/{method}.view`
/// exactly once; a path prefix survives, which is what a reverse proxy such as
/// `https://example.com/navidrome` needs.
pub fn normalise_server(raw: &str) -> Result<String> {
    let raw = raw.trim();
    if raw.is_empty() {
        return Err(Error::Unsupported("enter the navidrome server address"));
    }
    if raw.len() > MAX_SERVER_LEN {
        return Err(Error::Unsupported("that server address is too long"));
    }
    let mut url = reqwest::Url::parse(raw)
        .map_err(|_| Error::Unsupported("the server address needs http:// or https:// in front"))?;
    if !matches!(url.scheme(), "http" | "https") || url.host_str().is_none() {
        return Err(Error::Unsupported("the server address must be an http:// or https:// url"));
    }
    if !url.username().is_empty() || url.password().is_some() {
        return Err(Error::Unsupported(
            "put the username in its own field, not inside the server address",
        ));
    }
    if url.query().is_some() || url.fragment().is_some() {
        return Err(Error::Unsupported("the server address cannot carry a query or a fragment"));
    }

    let mut path = url.path().trim_end_matches('/').to_owned();
    if let Some(stripped) = path.strip_suffix("/rest") {
        path = stripped.trim_end_matches('/').to_owned();
    }
    url.set_path(if path.is_empty() { "/" } else { &path });
    url.set_query(None);
    url.set_fragment(None);
    Ok(url.as_str().trim_end_matches('/').to_owned())
}

/// Everything a request needs to authenticate, and everything the host persists.
///
/// Serializable verbatim under the daemon-only `navidrome_auth` setting. The password is not a
/// field: [`new`](Self::new) consumes it, derives the token and drops it.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct NavidromeAuth {
    /// Normalised by [`normalise_server`]: no trailing slash, may carry a path prefix.
    pub base_url: String,
    pub username: String,
    /// Sixteen alphanumerics, generated once per server and user.
    pub salt: String,
    /// Lowercase hex `md5(password + salt)`.
    pub token: String,
}

/// The same record under the name the sign-in path uses. There is nothing else to a Subsonic
/// credential once the salt is chosen, so the two names are one type rather than two structures
/// that could drift apart.
pub type Credentials = NavidromeAuth;

impl NavidromeAuth {
    /// Derive a token with a fresh salt. The returned value is what `sign_in` pings with and what
    /// the host persists after the ping succeeds.
    pub fn new(server: &str, username: &str, password: &str) -> Result<Self> {
        Self::with_salt(server, username, password, None)
    }

    /// Derive a token while keeping `salt` when one is supplied.
    ///
    /// Reconnecting to the same server and user reuses the old salt so cover-art URLs that are
    /// already on screen stay valid. A salt that no longer matches the server's copy of the
    /// password is not a problem at play time — stream URLs are always resolved fresh — and the
    /// next browse replaces the stale artwork.
    pub fn with_salt(
        server: &str,
        username: &str,
        password: &str,
        salt: Option<&str>,
    ) -> Result<Self> {
        let base_url = normalise_server(server)?;
        let username = username.trim();
        if username.is_empty() {
            return Err(Error::Unsupported("enter the username for that server"));
        }
        if password.is_empty() {
            return Err(Error::Unsupported("enter the password for that server"));
        }
        let salt = salt
            .map(str::trim)
            .filter(|s| !s.is_empty())
            .map(str::to_owned)
            .unwrap_or_else(new_salt);
        let token = salt_token(password, &salt);
        Ok(NavidromeAuth { base_url, username: username.to_owned(), salt, token })
    }

    /// The ordered auth pairs every request carries: user, salt and derived token.
    pub fn query(&self) -> [(&'static str, &str); 3] {
        [("u", &self.username), ("s", &self.salt), ("t", &self.token)]
    }

    /// The URL builder for this session. Fails only when the stored address was tampered with.
    pub fn covers(&self) -> Result<crate::Covers> {
        crate::Covers::new(self)
    }
}

/// A fresh salt: sixteen lowercase alphanumerics, which need no escaping inside a query string.
fn new_salt() -> String {
    let mut rng = rand::thread_rng();
    (0..SALT_LEN)
        .map(|_| {
            let index = rng.gen_range(0..SALT_ALPHABET.len());
            SALT_ALPHABET[index] as char
        })
        .collect()
}

/// `md5(password + salt)` as lowercase hex, the derivation every Subsonic client uses.
fn salt_token(password: &str, salt: &str) -> String {
    let mut hash = Md5::new();
    hash.update(password.as_bytes());
    hash.update(salt.as_bytes());
    format!("{:x}", hash.finalize())
}

/// What the server said it supports, read once at sign-in.
///
/// OpenSubsonic extensions are optional: a plain Subsonic server answers the extension call with
/// nothing, and every feature behind an extension then degrades instead of failing. The daemon
/// restores an auth blob without renegotiating, so an empty list here means "not asked yet", not
/// "nothing supported"; only a fresh sign-in settles it.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Capabilities {
    /// Extension names from `getOpenSubsonicExtensions`, for example `songLyrics` or `formPost`.
    pub extensions: Vec<String>,
}

impl Capabilities {
    pub fn supports(&self, extension: &str) -> bool {
        self.extensions.iter().any(|name| name == extension)
    }

    /// Whether `getLyricsBySongId` will answer. Navidrome's legacy `getLyrics` needs an external
    /// lyrics agent, so only the OpenSubsonic extension is worth calling.
    pub fn lyrics(&self) -> bool {
        self.supports("songLyrics")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_token_is_md5_of_password_and_salt() {
        // md5("secret" + "abc"), the derivation the Subsonic documentation specifies.
        assert_eq!(salt_token("secret", "abc"), "c42b93a1ac3b382e88071510296a96ce");
        let auth =
            NavidromeAuth::with_salt("https://music.example.com", "admin", "secret", Some("abc"))
                .unwrap();
        assert_eq!(auth.token, "c42b93a1ac3b382e88071510296a96ce");
        assert_eq!(auth.base_url, "https://music.example.com");
    }

    #[test]
    fn a_fresh_salt_is_sixteen_alphanumerics() {
        let salt = new_salt();
        assert_eq!(salt.len(), SALT_LEN);
        assert!(salt.chars().all(|c| c.is_ascii_alphanumeric()), "{salt}");
        assert_ne!(salt, new_salt(), "a salt must not be predictable from the last one");
    }

    #[test]
    fn reconnecting_with_the_same_salt_keeps_the_same_token() {
        let first = NavidromeAuth::new("https://music.example.com", "admin", "secret").unwrap();
        let again = NavidromeAuth::with_salt(
            "https://music.example.com/",
            "admin",
            "secret",
            Some(&first.salt),
        )
        .unwrap();
        assert_eq!(first, again, "stable cover urls need a stable token");
        let third = NavidromeAuth::new("https://music.example.com", "admin", "secret").unwrap();
        assert_ne!(first.salt, third.salt);
        assert_ne!(first.token, third.token, "a new salt is a new token");
    }

    #[test]
    fn the_auth_query_is_ordered_user_salt_token() {
        let auth =
            NavidromeAuth::with_salt("https://m.example", "admin", "secret", Some("abc")).unwrap();
        assert_eq!(
            auth.query(),
            [("u", "admin"), ("s", "abc"), ("t", "c42b93a1ac3b382e88071510296a96ce")]
        );
    }

    #[test]
    fn a_plain_server_loses_its_trailing_slash() {
        assert_eq!(
            normalise_server("https://music.example.com").unwrap(),
            "https://music.example.com"
        );
        assert_eq!(
            normalise_server("  https://music.example.com/  ").unwrap(),
            "https://music.example.com"
        );
    }

    #[test]
    fn a_reverse_proxy_path_is_kept() {
        assert_eq!(
            normalise_server("https://example.com/navidrome/").unwrap(),
            "https://example.com/navidrome"
        );
    }

    #[test]
    fn a_pasted_rest_suffix_is_dropped_once() {
        assert_eq!(
            normalise_server("https://example.com/navidrome/rest").unwrap(),
            "https://example.com/navidrome"
        );
        assert_eq!(
            normalise_server("https://example.com/navidrome/rest/").unwrap(),
            "https://example.com/navidrome"
        );
        assert_eq!(normalise_server("https://example.com/rest").unwrap(), "https://example.com");
    }

    #[test]
    fn a_missing_scheme_is_refused_and_the_fix_is_named() {
        let err = normalise_server("music.example.com").unwrap_err();
        assert!(err.to_string().contains("http://"), "{err}");
    }

    #[test]
    fn only_http_and_https_are_accepted() {
        assert!(normalise_server("javascript:alert(1)").is_err());
        assert!(normalise_server("ftp://music.example.com").is_err());
        assert!(normalise_server("file:///etc/passwd").is_err());
        assert!(normalise_server("   ").is_err());
    }

    #[test]
    fn credentials_query_and_fragment_are_refused_in_the_address() {
        assert!(normalise_server("https://admin:secret@music.example.com").is_err());
        assert!(normalise_server("https://music.example.com/?x=1").is_err());
        assert!(normalise_server("https://music.example.com/#top").is_err());
    }

    #[test]
    fn sign_in_needs_a_username_and_a_password() {
        assert!(NavidromeAuth::new("https://music.example.com", "  ", "secret").is_err());
        assert!(NavidromeAuth::new("https://music.example.com", "admin", "").is_err());
    }

    #[test]
    fn a_saved_auth_blob_round_trips_through_json() {
        let auth =
            NavidromeAuth::with_salt("https://music.example.com", "admin", "secret", Some("abc"))
                .unwrap();
        let blob = serde_json::to_string(&auth).unwrap();
        assert!(!blob.contains("secret"), "the password must never be serialised: {blob}");
        assert_eq!(serde_json::from_str::<NavidromeAuth>(&blob).unwrap(), auth);
    }

    #[test]
    fn capabilities_answer_by_extension_name() {
        let caps = Capabilities { extensions: vec!["songLyrics".into(), "formPost".into()] };
        assert!(caps.lyrics());
        assert!(!caps.supports("apiKeyAuthentication"));
        assert!(!Capabilities::default().lyrics(), "a plain subsonic server has no lyrics list");
    }
}
