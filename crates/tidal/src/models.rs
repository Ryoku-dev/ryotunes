//! Catalogue types.
//!
//! Every struct here is the tolerant shape: TIDAL adds fields without warning and omits others per
//! market, so nothing outside an id is required and a missing field degrades a label rather than
//! failing a page. Ids are numeric on the wire and stay numeric here; the `tidal:` prefixes the
//! rest of Ryotunes uses are the bridge's business, not this crate's.

use serde::Deserialize;

/// Artwork is addressed by a uuid with dashes replaced by slashes, at a size the caller picks.
/// TIDAL serves a fixed ladder; 320 is the card size and 1280 the now-playing size.
pub fn cover_url(cover: &str, size: u32) -> Option<String> {
    let cover = cover.trim();
    if cover.is_empty() {
        return None;
    }
    Some(format!(
        "https://resources.tidal.com/images/{}/{size}x{size}.jpg",
        cover.replace('-', "/")
    ))
}

#[derive(Clone, Debug, PartialEq, Eq, Deserialize)]
pub struct Artist {
    pub id: u64,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub picture: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Eq, Deserialize)]
pub struct Album {
    pub id: u64,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub cover: Option<String>,
    #[serde(rename = "numberOfTracks", default)]
    pub track_count: Option<u32>,
    #[serde(rename = "releaseDate", default)]
    pub release_date: Option<String>,
    #[serde(default)]
    pub artists: Vec<Artist>,
}

#[derive(Clone, Debug, PartialEq, Eq, Deserialize)]
pub struct Track {
    pub id: u64,
    #[serde(default)]
    pub title: String,
    /// Seconds, as TIDAL reports it.
    #[serde(default)]
    pub duration: Option<u32>,
    #[serde(rename = "trackNumber", default)]
    pub track_number: Option<u32>,
    #[serde(default)]
    pub artists: Vec<Artist>,
    #[serde(default)]
    pub album: Option<Album>,
    /// The highest tier this track exists in, which is not the tier a given token may fetch.
    #[serde(rename = "audioQuality", default)]
    pub audio_quality: Option<String>,
    #[serde(rename = "streamReady", default)]
    pub stream_ready: Option<bool>,
}

impl Track {
    /// The artist line, joined the way a player bar wants it.
    pub fn artist_line(&self) -> String {
        self.artists
            .iter()
            .map(|a| a.name.as_str())
            .filter(|n| !n.is_empty())
            .collect::<Vec<_>>()
            .join(", ")
    }

    /// Cover art for this track, taken from its album.
    pub fn cover_url(&self, size: u32) -> Option<String> {
        self.album.as_ref().and_then(|a| a.cover.as_deref()).and_then(|c| cover_url(c, size))
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Deserialize)]
pub struct Playlist {
    /// Playlists are the one catalogue type keyed by uuid rather than a number.
    pub uuid: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub description: Option<String>,
    #[serde(rename = "numberOfTracks", default)]
    pub track_count: Option<u32>,
    #[serde(default)]
    pub image: Option<String>,
    #[serde(rename = "squareImage", default)]
    pub square_image: Option<String>,
}

impl Playlist {
    pub fn cover_url(&self, size: u32) -> Option<String> {
        self.square_image.as_deref().or(self.image.as_deref()).and_then(|c| cover_url(c, size))
    }
}

/// A paged collection. TIDAL reports the window it served and the total, and a caller walks it by
/// offset until `items` comes back short.
#[derive(Clone, Debug, PartialEq, Eq, Deserialize)]
pub struct Page<T> {
    #[serde(default = "Vec::new")]
    pub items: Vec<T>,
    #[serde(rename = "totalNumberOfItems", default)]
    pub total: Option<u32>,
    #[serde(default)]
    pub offset: Option<u32>,
    #[serde(default)]
    pub limit: Option<u32>,
}

impl<T> Default for Page<T> {
    fn default() -> Self {
        Page { items: Vec::new(), total: None, offset: None, limit: None }
    }
}

/// Personal collections wrap the thing in a dated envelope; the date is what the library sorts by.
#[derive(Clone, Debug, PartialEq, Eq, Deserialize)]
pub struct Favourite<T> {
    #[serde(default)]
    pub created: Option<String>,
    pub item: T,
}

/// One search, across every type at once.
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize)]
pub struct SearchResults {
    #[serde(default)]
    pub tracks: Page<Track>,
    #[serde(default)]
    pub albums: Page<Album>,
    #[serde(default)]
    pub artists: Page<Artist>,
    #[serde(default)]
    pub playlists: Page<Playlist>,
}

/// The account record behind `/users/{id}`. Only the profile name is read off it: the rest is
/// either already in the token response or personal data this client has no business holding.
#[derive(Debug, Deserialize)]
pub struct UserProfile {
    #[serde(rename = "profileName", default)]
    pub profile_name: Option<String>,
}

impl UserProfile {
    /// The name worth showing. An account that never made a profile answers `null`; a blank is
    /// not a name either, and printing one would leave a gap where the account should be.
    pub fn name(&self) -> Option<String> {
        self.profile_name.as_deref().map(str::trim).filter(|s| !s.is_empty()).map(str::to_owned)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cover_uuids_become_a_path() {
        assert_eq!(
            cover_url("1e01cdb6-f15d-4d8b-8440-a047976c1cac", 320).as_deref(),
            Some("https://resources.tidal.com/images/1e01cdb6/f15d/4d8b/8440/a047976c1cac/320x320.jpg")
        );
        assert_eq!(cover_url("  ", 320), None);
    }

    #[test]
    fn a_track_parses_without_any_optional_field() {
        let t: Track = serde_json::from_str(r#"{"id":1}"#).expect("only the id is required");
        assert_eq!(t.id, 1);
        assert_eq!(t.artist_line(), "");
        assert_eq!(t.cover_url(320), None);
    }

    #[test]
    fn an_unknown_field_does_not_break_a_page() {
        let json = r#"{"items":[{"id":5,"title":"x","somethingNew":true}],"totalNumberOfItems":1}"#;
        let page: Page<Track> = serde_json::from_str(json).expect("new fields must be ignored");
        assert_eq!(page.items.len(), 1);
        assert_eq!(page.total, Some(1));
    }

    #[test]
    fn artists_join_in_order() {
        let t: Track = serde_json::from_str(
            r#"{"id":1,"artists":[{"id":1,"name":"A"},{"id":2,"name":""},{"id":3,"name":"B"}]}"#,
        )
        .unwrap();
        assert_eq!(t.artist_line(), "A, B", "empty names are dropped, order is kept");
    }

    #[test]
    fn a_profile_name_is_read_and_a_blank_one_counts_as_absent() {
        let named: UserProfile =
            serde_json::from_str(r#"{"profileName":"vlad","username":"a@b.c"}"#).unwrap();
        assert_eq!(named.name().as_deref(), Some("vlad"));
        for raw in
            [r#"{"profileName":null}"#, r#"{}"#, r#"{"profileName":""}"#, r#"{"profileName":"  "}"#]
        {
            let p: UserProfile = serde_json::from_str(raw).unwrap();
            assert_eq!(p.name(), None, "{raw} must not reach the status line");
        }
    }

    #[test]
    fn a_playlist_prefers_its_square_image() {
        let p: Playlist = serde_json::from_str(
            r#"{"uuid":"u","image":"aaaaaaaa-bbbb","squareImage":"cccccccc-dddd"}"#,
        )
        .unwrap();
        assert!(p.cover_url(320).unwrap().contains("cccccccc/dddd"));
    }
}
