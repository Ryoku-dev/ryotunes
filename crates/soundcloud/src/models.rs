//! Public models (the crate contract) and the tolerant wire structs they map from.
//!
//! SoundCloud's `api-v2` omits fields liberally and returns `{"id": …}` stubs for unhydrated
//! playlist tracks, so every wire field carries `#[serde(default)]` and mapping fills the gaps.

use serde::Deserialize;

use crate::{nonempty, parse_tag_list, upscale_artwork};

// ---------------------------------------------------------------------------
// Public models (contract)
// ---------------------------------------------------------------------------

/// A single track. `artwork` is the 500×500 variant; `transcodings` are the raw stream options
/// (`stream_url` resolves one to an m3u8 at play time, since those URLs expire).
#[derive(Clone, Debug)]
pub struct Track {
    pub id: u64,
    pub title: String,
    pub user: UserRef,
    pub duration_ms: u64,
    pub artwork: Option<String>,
    pub genre: Option<String>,
    pub tags: Vec<String>,
    pub plays: Option<u64>,
    pub likes: Option<u64>,
    pub created_at: String,
    pub permalink_url: String,
    pub waveform_url: Option<String>,
    pub authorization: Option<String>,
    pub transcodings: Vec<Transcoding>,
    pub description: Option<String>,
}

/// A stream option from a track's `media.transcodings`. Selection prefers `protocol == "hls"` with
/// an aac (`audio/mp4`) mime, then mp3 (`audio/mpeg`).
#[derive(Clone, Debug)]
pub struct Transcoding {
    pub url: String,
    pub protocol: String,
    pub mime_type: String,
    pub preset: Option<String>,
    pub quality: Option<String>,
}

/// The lightweight user embedded in a track/playlist.
#[derive(Clone, Debug, Default)]
pub struct UserRef {
    pub id: u64,
    pub username: String,
    pub permalink: String,
    pub avatar: Option<String>,
}

/// A fully resolved user (artist) profile.
#[derive(Clone, Debug, Default)]
pub struct User {
    pub id: u64,
    pub username: String,
    pub full_name: Option<String>,
    pub permalink: String,
    pub avatar: Option<String>,
    pub banner: Option<String>,
    pub followers: u64,
    pub track_count: u64,
    pub city: Option<String>,
    pub country: Option<String>,
    pub description: Option<String>,
    pub verified: bool,
}

/// A playlist or album (`is_album` distinguishes them; albums are just playlists on SoundCloud).
#[derive(Clone, Debug)]
pub struct Playlist {
    pub id: u64,
    pub title: String,
    pub user: UserRef,
    pub artwork: Option<String>,
    pub is_album: bool,
    pub set_type: Option<String>,
    pub track_count: u64,
    pub release_date: Option<String>,
    pub duration_ms: u64,
}

/// A playlist with its tracks fully hydrated (in original order).
#[derive(Clone, Debug)]
pub struct PlaylistDetail {
    pub playlist: Playlist,
    pub tracks: Vec<Track>,
    pub description: Option<String>,
}

/// A page of results. `next` is the offset to pass back for the following page, if any.
#[derive(Clone, Debug)]
pub struct Page<T> {
    pub items: Vec<T>,
    pub next: Option<u32>,
}

/// Mixed search results (the `/search` endpoint), split by kind. Albums surface as `playlists`
/// with `is_album == true`.
#[derive(Clone, Debug, Default)]
pub struct SearchResults {
    pub tracks: Vec<Track>,
    pub users: Vec<User>,
    pub playlists: Vec<Playlist>,
}

/// Which chart to pull (`/charts?kind=…`).
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ChartKind {
    Top,
    Trending,
}

impl ChartKind {
    pub(crate) fn as_str(self) -> &'static str {
        match self {
            ChartKind::Top => "top",
            ChartKind::Trending => "trending",
        }
    }
}

// ---------------------------------------------------------------------------
// Wire models (SoundCloud api-v2 JSON)
// ---------------------------------------------------------------------------

/// A `{collection, next_href}` envelope, used by search/listing endpoints.
#[derive(Deserialize, Default)]
pub(crate) struct WireCollection<T> {
    #[serde(default = "Vec::new")]
    pub collection: Vec<T>,
    #[serde(default)]
    pub next_href: Option<String>,
}

#[derive(Deserialize, Clone, Default)]
pub(crate) struct WireUserRef {
    #[serde(default)]
    pub id: u64,
    #[serde(default)]
    pub username: String,
    #[serde(default)]
    pub permalink: String,
    #[serde(default)]
    pub avatar_url: Option<String>,
}

#[derive(Deserialize, Clone, Default)]
pub(crate) struct WireUser {
    #[serde(default)]
    pub id: u64,
    #[serde(default)]
    pub username: String,
    #[serde(default)]
    pub full_name: Option<String>,
    #[serde(default)]
    pub permalink: String,
    #[serde(default)]
    pub avatar_url: Option<String>,
    #[serde(default)]
    pub followers_count: u64,
    #[serde(default)]
    pub track_count: u64,
    #[serde(default)]
    pub city: Option<String>,
    #[serde(default)]
    pub country: Option<String>,
    #[serde(default)]
    pub country_code: Option<String>,
    #[serde(default)]
    pub description: Option<String>,
    #[serde(default)]
    pub verified: bool,
    #[serde(default)]
    pub visuals: Option<WireVisuals>,
}

#[derive(Deserialize, Clone, Default)]
pub(crate) struct WireVisuals {
    #[serde(default)]
    pub visuals: Vec<WireVisual>,
}

#[derive(Deserialize, Clone, Default)]
pub(crate) struct WireVisual {
    #[serde(default)]
    pub visual_url: Option<String>,
}

#[derive(Deserialize, Clone, Default)]
pub(crate) struct WireFormat {
    #[serde(default)]
    pub protocol: String,
    #[serde(default)]
    pub mime_type: String,
}

#[derive(Deserialize, Clone, Default)]
pub(crate) struct WireTranscoding {
    #[serde(default)]
    pub url: String,
    #[serde(default)]
    pub preset: Option<String>,
    #[serde(default)]
    pub quality: Option<String>,
    #[serde(default)]
    pub format: WireFormat,
}

#[derive(Deserialize, Clone, Default)]
pub(crate) struct WireMedia {
    #[serde(default)]
    pub transcodings: Vec<WireTranscoding>,
}

#[derive(Deserialize, Clone, Default)]
pub(crate) struct WireTrack {
    #[serde(default)]
    pub id: u64,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub user: Option<WireUserRef>,
    #[serde(default)]
    pub duration: u64,
    #[serde(default)]
    pub artwork_url: Option<String>,
    #[serde(default)]
    pub genre: Option<String>,
    #[serde(default)]
    pub tag_list: Option<String>,
    #[serde(default)]
    pub playback_count: Option<u64>,
    #[serde(default)]
    pub likes_count: Option<u64>,
    #[serde(default)]
    pub created_at: String,
    #[serde(default)]
    pub permalink_url: String,
    #[serde(default)]
    pub waveform_url: Option<String>,
    #[serde(default)]
    pub track_authorization: Option<String>,
    #[serde(default)]
    pub media: WireMedia,
    #[serde(default)]
    pub description: Option<String>,
}

impl WireTrack {
    /// A bare `{"id": …}` stub (unhydrated playlist row) carries no user.
    pub(crate) fn is_hydrated(&self) -> bool {
        self.user.is_some()
    }
}

#[derive(Deserialize, Clone, Default)]
pub(crate) struct WirePlaylist {
    #[serde(default)]
    pub id: u64,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub user: Option<WireUserRef>,
    #[serde(default)]
    pub artwork_url: Option<String>,
    #[serde(default)]
    pub is_album: bool,
    #[serde(default)]
    pub set_type: Option<String>,
    #[serde(default)]
    pub track_count: u64,
    #[serde(default)]
    pub release_date: Option<String>,
    #[serde(default)]
    pub duration: u64,
    #[serde(default)]
    pub description: Option<String>,
    #[serde(default)]
    pub tracks: Vec<WireTrack>,
}

// ---------------------------------------------------------------------------
// Mapping (wire -> public)
// ---------------------------------------------------------------------------

pub(crate) fn map_user_ref(w: WireUserRef) -> UserRef {
    UserRef {
        id: w.id,
        username: w.username,
        permalink: w.permalink,
        avatar: nonempty(w.avatar_url).map(|u| upscale_artwork(&u)),
    }
}

pub(crate) fn map_user(w: WireUser) -> User {
    User {
        id: w.id,
        username: w.username,
        full_name: nonempty(w.full_name),
        permalink: w.permalink,
        avatar: nonempty(w.avatar_url).map(|u| upscale_artwork(&u)),
        banner: w
            .visuals
            .and_then(|v| v.visuals.into_iter().next())
            .and_then(|v| nonempty(v.visual_url)),
        followers: w.followers_count,
        track_count: w.track_count,
        city: nonempty(w.city),
        country: nonempty(w.country).or_else(|| nonempty(w.country_code)),
        description: nonempty(w.description),
        verified: w.verified,
    }
}

pub(crate) fn map_transcoding(w: WireTranscoding) -> Transcoding {
    Transcoding {
        url: w.url,
        protocol: w.format.protocol,
        mime_type: w.format.mime_type,
        preset: nonempty(w.preset),
        quality: nonempty(w.quality),
    }
}

pub(crate) fn map_track(w: WireTrack) -> Track {
    Track {
        id: w.id,
        title: w.title,
        user: w.user.map(map_user_ref).unwrap_or_default(),
        duration_ms: w.duration,
        artwork: nonempty(w.artwork_url).map(|u| upscale_artwork(&u)),
        genre: nonempty(w.genre),
        tags: w.tag_list.as_deref().map(parse_tag_list).unwrap_or_default(),
        plays: w.playback_count,
        likes: w.likes_count,
        created_at: w.created_at,
        permalink_url: w.permalink_url,
        waveform_url: nonempty(w.waveform_url),
        authorization: nonempty(w.track_authorization),
        transcodings: w.media.transcodings.into_iter().map(map_transcoding).collect(),
        description: nonempty(w.description),
    }
}

pub(crate) fn map_playlist(w: &WirePlaylist) -> Playlist {
    Playlist {
        id: w.id,
        title: w.title.clone(),
        user: w.user.clone().map(map_user_ref).unwrap_or_default(),
        artwork: nonempty(w.artwork_url.clone()).map(|u| upscale_artwork(&u)),
        is_album: w.is_album,
        set_type: nonempty(w.set_type.clone()),
        track_count: w.track_count,
        release_date: nonempty(w.release_date.clone()),
        duration_ms: w.duration,
    }
}
