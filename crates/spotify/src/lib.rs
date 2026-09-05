//! Ryotunes Spotify provider.
//!
//! Lifted from nolight132/sonora (`crates/music/src/spotify`, GPL-3.0-or-later), commit 695242e,
//! and rewired for Ryotunes. See `README.md` for provenance, the `SongItem` mapping, and the
//! daemon-wiring notes.
//!
//! Boundary: this crate knows nothing about GPUI, Tauri, mpv, or UI state. Metadata comes from
//! librespot's session (Pathfinder GraphQL + protobuf extended-metadata + the collection v2 API).
//! Audio reaches Ryotunes' single libmpv engine through librespot-playback's `pipe` backend written
//! to a per-session FIFO that mpv opens as raw S16LE / 44100 Hz / stereo — never cpal or rodio.

pub mod credentials;
pub mod models;

mod albums;
mod artists;
mod auth;
mod client;
mod collection;
mod collection2;
mod lyrics;
mod pathfinder;
mod pb;
mod playlists;
mod profiles;
mod provider;
mod radio;
mod search;
mod stream;
mod wire;

pub use auth::{SignInFailure, SignInProblem};
pub use client::{Client, SearchResults};
pub use models::*;
pub use provider::{SpotifyConfig, SpotifyProvider};
pub use stream::{StreamControls, StreamEvent, StreamHandle};

/// The media kinds [`Client::share_url`] can address.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MediaKind {
    Track,
    Album,
    Artist,
    Playlist,
}

/// The first `wanted` distinct cover URLs across `tracks`, in playback order — used to build the
/// composite covers a playlist shows before its own artwork is known.
pub fn distinct_covers(tracks: &[models::Track], wanted: usize) -> Vec<String> {
    let mut covers: Vec<String> = Vec::with_capacity(wanted);
    for cover in tracks.iter().filter_map(|track| track.cover.as_deref()) {
        if covers.len() == wanted {
            break;
        }
        if !covers.iter().any(|kept| kept == cover) {
            covers.push(cover.to_owned());
        }
    }

    covers
}
