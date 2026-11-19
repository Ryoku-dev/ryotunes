//! The two absolute URLs a track needs: the file itself and its artwork.
//!
//! Both carry the session's auth query and are display and playback URLs only — they embed the
//! derived token and are never sent anywhere but the server that minted them. Both are built
//! through [`reqwest::Url`], so a server id containing `&`, `#` or a space is percent-encoded
//! rather than read as more query. Nothing here touches the network; the player opens the stream
//! URL itself, and the client renders the cover URL directly.

use crate::auth::{normalise_server, NavidromeAuth};
use crate::{Error, Result, CLIENT_NAME, SUBSONIC_VERSION};

/// The largest cover the client asks for. The panel size is 1280, and a server may round up.
const MAX_COVER_PX: u32 = 2_048;

/// Absolute URLs for a track and its artwork, carrying the session's auth query.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Covers {
    auth: NavidromeAuth,
}

impl Covers {
    pub fn new(auth: &NavidromeAuth) -> Result<Self> {
        let base_url = normalise_server(&auth.base_url)?;
        Ok(Covers { auth: NavidromeAuth { base_url, ..auth.clone() } })
    }

    /// `{base}/rest/getCoverArt.view?id=…&size=…`, or `None` when the item has no artwork.
    pub fn url(&self, cover_id: &str, px: u32) -> Option<String> {
        let id = cover_id.trim();
        if id.is_empty() {
            return None;
        }
        let size = px.clamp(1, MAX_COVER_PX).to_string();
        self.endpoint("getCoverArt", &[("id", id), ("size", &size)]).ok().map(String::from)
    }

    /// `{base}/rest/stream.view?id=…&format=raw`. `format=raw` keeps the server from transcoding,
    /// so the player receives the file the library holds. There is no expiry to cache: the URL
    /// carries the current token and is resolved once per play.
    pub fn stream_url(&self, song_id: &str) -> Option<String> {
        let id = song_id.trim();
        if id.is_empty() {
            return None;
        }
        self.endpoint("stream", &[("id", id), ("format", "raw")]).ok().map(String::from)
    }

    /// The request funnel, shared with the catalogue client: `{base}/rest/{method}.view` with the
    /// auth pairs, the protocol version, the client name, JSON output and then `params`.
    pub(crate) fn endpoint(&self, method: &str, params: &[(&str, &str)]) -> Result<reqwest::Url> {
        let mut url = reqwest::Url::parse(&format!("{}/rest/{method}.view", self.auth.base_url))
            .map_err(|_| {
                Error::Unsupported("the saved navidrome server address is not a valid url")
            })?;
        {
            let mut query = url.query_pairs_mut();
            for (key, value) in self.auth.query() {
                query.append_pair(key, value);
            }
            query.append_pair("v", SUBSONIC_VERSION);
            query.append_pair("c", CLIENT_NAME);
            query.append_pair("f", "json");
            for (key, value) in params {
                query.append_pair(key, value);
            }
        }
        Ok(url)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn covers() -> Covers {
        NavidromeAuth {
            base_url: "https://music.example.com".into(),
            username: "admin".into(),
            salt: "abc".into(),
            token: "c42b93a1ac3b382e88071510296a96ce".into(),
        }
        .covers()
        .expect("the base url is valid")
    }

    fn param(url: &reqwest::Url, key: &str) -> Option<String> {
        url.query_pairs().find(|(name, _)| name == key).map(|(_, value)| value.into_owned())
    }

    #[test]
    fn a_stream_url_is_self_contained() {
        let url = covers().stream_url("so-1").expect("an id builds a url");
        let parsed = reqwest::Url::parse(&url).unwrap();
        assert_eq!(parsed.path(), "/rest/stream.view");
        assert_eq!(param(&parsed, "id").as_deref(), Some("so-1"));
        assert_eq!(param(&parsed, "format").as_deref(), Some("raw"));
        for (key, value) in [
            ("u", "admin"),
            ("s", "abc"),
            ("t", "c42b93a1ac3b382e88071510296a96ce"),
            ("v", SUBSONIC_VERSION),
            ("c", CLIENT_NAME),
            ("f", "json"),
        ] {
            assert_eq!(param(&parsed, key).as_deref(), Some(value), "{key}");
        }
    }

    #[test]
    fn a_cover_url_carries_the_size_and_auth() {
        let url = covers().url("al-1", 320).expect("an id builds a url");
        let parsed = reqwest::Url::parse(&url).unwrap();
        assert_eq!(parsed.path(), "/rest/getCoverArt.view");
        assert_eq!(param(&parsed, "id").as_deref(), Some("al-1"));
        assert_eq!(param(&parsed, "size").as_deref(), Some("320"));
        assert_eq!(param(&parsed, "u").as_deref(), Some("admin"));
    }

    #[test]
    fn an_item_without_artwork_gets_no_cover_url() {
        let covers = covers();
        assert_eq!(covers.url("", 320), None);
        assert_eq!(covers.url("   ", 320), None);
    }

    #[test]
    fn an_id_with_reserved_characters_is_encoded_into_the_query() {
        let url = covers().stream_url("a&b c#d").expect("an id builds a url");
        assert!(url.contains("%26"), "& must not start a new parameter: {url}");
        assert!(url.contains("%23"), "# must not start a fragment: {url}");
        assert!(!url.contains("#d"), "{url}");
        let parsed = reqwest::Url::parse(&url).unwrap();
        assert_eq!(param(&parsed, "id").as_deref(), Some("a&b c#d"), "the id round-trips intact");
        assert_eq!(parsed.query_pairs().filter(|(k, _)| k == "id").count(), 1);
    }

    #[test]
    fn a_path_prefix_survives_into_the_request_path() {
        let auth = NavidromeAuth::with_salt(
            "https://example.com/navidrome/rest/",
            "admin",
            "secret",
            Some("abc"),
        )
        .unwrap();
        let covers = auth.covers().unwrap();
        let parsed = reqwest::Url::parse(&covers.stream_url("x").unwrap()).expect("a valid url");
        assert_eq!(parsed.path(), "/navidrome/rest/stream.view");
    }

    #[test]
    fn a_tampered_saved_address_is_refused_rather_than_panicking() {
        let auth = NavidromeAuth {
            base_url: "not a url".into(),
            username: "admin".into(),
            salt: "abc".into(),
            token: "t".into(),
        };
        assert!(Covers::new(&auth).is_err());
    }
}
