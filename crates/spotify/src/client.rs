//! The signed-in Spotify client. Adapted from nolight132/sonora `spotify/client.rs`: Sonora's
//! `impl MusicApi for LibrespotClient` becomes inherent async methods on [`Client`] (Ryotunes has no
//! `MusicApi` trait), and the client now owns the one librespot player/FIFO ([`stream::Engine`]) that
//! streams every track into mpv.

use std::collections::HashSet;

use anyhow::{Context as _, Result};
use librespot_core::Session;
use librespot_protocol::playlist4_external::SelectedListContent as RootList;
use protobuf::Message as _;

use crate::stream::{self, StreamHandle};
use crate::{
    albums, artists, collection, collection2, lyrics, pathfinder, playlists, profiles, radio,
    search, wire, Album, AlbumDetail, Artist, ArtistProfile, ArtistRef, Genre, GenreDetail,
    HomeFeed, Lyrics, MediaKind, Playlist, PlaylistDetail, SavedArtist, Track, UserDetail,
    UserProfile,
};

const MADE_FOR_YOU: &str = "0JQ5DAt0tbjZptfcdMSKl3";
/// How many liked tracks make up one [`Client::liked`] page.
const LIKED_PAGE: u32 = 100;

/// The four result lists a query returns. `artists` is derived from the track hits' artist refs
/// (Spotify's Pathfinder search covers albums and playlists but not artists); see the README.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct SearchResults {
    pub tracks: Vec<Track>,
    pub albums: Vec<Album>,
    pub artists: Vec<ArtistRef>,
    pub playlists: Vec<Playlist>,
}

pub struct Client {
    session: Session,
    engine: stream::Engine,
}

impl Client {
    pub(crate) fn new(session: Session) -> Result<Self> {
        let engine = stream::Engine::new(session.clone())
            .context("cannot start the Spotify playback engine")?;
        Ok(Self { session, engine })
    }

    /// The underlying librespot session, for callers that need it directly.
    pub fn session(&self) -> &Session {
        &self.session
    }

    /// Whether the session is still connected.
    pub fn alive(&self) -> bool {
        !self.session.is_invalid()
    }

    /// A public `open.spotify.com` URL for a piece of media.
    pub fn share_url(&self, kind: MediaKind, id: &str) -> String {
        let kind = match kind {
            MediaKind::Track => "track",
            MediaKind::Album => "album",
            MediaKind::Artist => "artist",
            MediaKind::Playlist => "playlist",
        };
        format!("https://open.spotify.com/{kind}/{id}")
    }

    // -- Playback -----------------------------------------------------------------------------

    /// Start streaming `track_id` (bare id or `spotify:track:` URI) through the FIFO mpv reads.
    pub fn stream(&self, track_id: &str) -> Result<StreamHandle> {
        self.engine.stream(track_id)
    }

    // -- Profile / library --------------------------------------------------------------------

    pub async fn profile(&self) -> Result<UserProfile> {
        let username = self.session.username();
        let body = self
            .session
            .spclient()
            .get_user_profile(&username, None, None)
            .await?;

        let profile: wire::Named = serde_json::from_slice(&body).unwrap_or_default();
        Ok(UserProfile {
            display_name: profile.label().unwrap_or(&username).to_owned(),
            id: username,
        })
    }

    pub async fn user(&self, user_id: &str) -> Result<UserDetail> {
        profiles::profile(&self.session, user_id).await
    }

    /// One page of the user's liked (saved) tracks, newest first. Spotify's collection API pages by
    /// an opaque continuation token rather than an offset, so page `n` is served by reading the first
    /// `(n + 1) * LIKED_PAGE` saved tracks and returning the tail slice.
    pub async fn liked(&self, page: u32) -> Result<Vec<Track>> {
        let limit = page.saturating_add(1).saturating_mul(LIKED_PAGE);
        let mut tracks = collection::saved_tracks(&self.session, limit).await?;
        let skip = (page.saturating_mul(LIKED_PAGE)) as usize;
        if skip >= tracks.len() {
            return Ok(Vec::new());
        }
        Ok(tracks.split_off(skip))
    }

    pub async fn saved_tracks(&self, limit: u32) -> Result<Vec<Track>> {
        collection::saved_tracks(&self.session, limit).await
    }

    pub async fn set_track_saved(&self, track_id: &str, saved: bool) -> Result<()> {
        collection2::set_track_saved(&self.session, track_id, saved).await
    }

    pub async fn saved_albums(&self, limit: u32) -> Result<Vec<Album>> {
        albums::saved_albums(&self.session, limit).await
    }

    pub async fn set_album_saved(&self, album_id: &str, saved: bool) -> Result<()> {
        collection2::set_album_saved(&self.session, album_id, saved).await
    }

    pub async fn saved_artists(&self, limit: u32) -> Result<Vec<SavedArtist>> {
        artists::saved_artists(&self.session, limit).await
    }

    pub async fn set_artist_saved(&self, artist_id: &str, saved: bool) -> Result<()> {
        collection2::set_artist_saved(&self.session, artist_id, saved).await
    }

    // -- Tracks / albums / artists ------------------------------------------------------------

    pub async fn track(&self, track_id: &str) -> Result<Track> {
        collection::track(&self.session, track_id).await
    }

    pub async fn track_playcount(&self, track_id: &str) -> Result<Option<u64>> {
        pathfinder::track(&self.session, track_id).await
    }

    pub async fn track_radio(&self, track_id: &str) -> Result<Vec<Track>> {
        radio::track_radio(&self.session, track_id).await
    }

    pub async fn lyrics(&self, track_id: &str) -> Result<Option<Lyrics>> {
        lyrics::lyrics(&self.session, track_id).await
    }

    pub async fn album(&self, album_id: &str) -> Result<AlbumDetail> {
        albums::album(&self.session, album_id).await
    }

    pub async fn album_tracks(&self, album_id: &str) -> Result<Vec<Track>> {
        albums::album_tracks(&self.session, album_id).await
    }

    pub async fn artist(&self, artist_id: &str) -> Result<Artist> {
        artists::artist(&self.session, artist_id).await
    }

    pub async fn artist_profile(&self, artist_id: &str) -> Result<ArtistProfile> {
        artists::profile(&self.session, artist_id).await
    }

    pub async fn artist_images(
        &self,
        ids: Vec<String>,
    ) -> Result<std::collections::HashMap<String, String>> {
        artists::images(&self.session, &ids).await
    }

    // -- Playlists ----------------------------------------------------------------------------

    pub async fn playlist(&self, playlist_id: &str) -> Result<PlaylistDetail> {
        let mut detail = playlists::playlist(&self.session, playlist_id).await?;
        let owner = detail.playlist.owner_id.clone();
        if !owner.is_empty() {
            let names = profiles::display_names(&self.session, HashSet::from([owner.clone()])).await;
            if let Some(name) = names.get(&owner) {
                detail.playlist.owner = name.clone();
            }
        }
        Ok(detail)
    }

    pub async fn playlist_tracks(&self, playlist_id: &str) -> Result<Vec<Track>> {
        playlists::playlist_tracks(&self.session, playlist_id).await
    }

    pub async fn playlist_covers(&self, playlist_id: &str, wanted: usize) -> Result<Vec<String>> {
        playlists::covers(&self.session, playlist_id, wanted).await
    }

    pub async fn playlists(&self, limit: u32) -> Result<Vec<Playlist>> {
        let body = self
            .session
            .spclient()
            .get_rootlist(0, Some(limit as usize))
            .await?;

        let rootlist =
            RootList::parse_from_bytes(&body).context("cannot decode the rootlist protobuf")?;
        let mut playlists = wire::playlists_from(&rootlist);

        let owners = playlists
            .iter()
            .map(|playlist| playlist.owner_id.clone())
            .filter(|owner| !owner.is_empty())
            .collect();
        let ids = playlists.iter().map(|playlist| playlist.id.clone()).collect();
        let (names, stamps) = tokio::join!(
            profiles::display_names(&self.session, owners),
            playlists::modified(&self.session, ids)
        );

        for playlist in &mut playlists {
            playlist.owned = playlist.owner_id == self.session.username();
            if let Some(name) = names.get(&playlist.owner_id) {
                playlist.owner = name.clone();
            }
            playlist.modified_at = stamps.get(&playlist.id).copied();
        }

        Ok(playlists)
    }

    pub async fn create_playlist(&self, name: &str) -> Result<String> {
        playlists::create(&self.session, name).await
    }

    pub async fn rename_playlist(&self, playlist_id: &str, name: &str) -> Result<()> {
        playlists::rename(&self.session, playlist_id, name).await
    }

    pub async fn delete_playlist(&self, playlist_id: &str) -> Result<()> {
        playlists::delete(&self.session, playlist_id).await
    }

    pub async fn remove_playlist_from_library(&self, playlist_id: &str) -> Result<()> {
        playlists::remove_from_library(&self.session, playlist_id).await
    }

    pub async fn add_playlist_to_library(&self, playlist_id: &str) -> Result<()> {
        playlists::add_to_library(&self.session, playlist_id).await
    }

    pub async fn set_playlist_public(&self, playlist_id: &str, public: bool) -> Result<()> {
        playlists::set_public(&self.session, playlist_id, public).await
    }

    pub async fn add_track_to_playlist(&self, playlist_id: &str, track_id: &str) -> Result<()> {
        playlists::add_track(&self.session, playlist_id, track_id).await
    }

    pub async fn remove_track_from_playlist(&self, playlist_id: &str, track_id: &str) -> Result<()> {
        playlists::remove_track(&self.session, playlist_id, track_id).await
    }

    // -- Discovery ----------------------------------------------------------------------------

    /// Search tracks, albums, and playlists in one shot; artists are derived from the track hits. A
    /// track failure propagates; album/playlist failures degrade to empty (a stale Pathfinder hash
    /// should not sink the whole query).
    pub async fn search(&self, query: &str) -> Result<SearchResults> {
        let (tracks, albums, playlists) = tokio::join!(
            search::search(&self.session, query),
            pathfinder::search_albums(&self.session, query),
            pathfinder::search_playlists(&self.session, query),
        );

        let tracks = tracks?;
        let albums = albums.unwrap_or_else(|error| {
            log::warn!("search: cannot search albums: {error:#}");
            Vec::new()
        });
        let playlists = playlists.unwrap_or_else(|error| {
            log::warn!("search: cannot search playlists: {error:#}");
            Vec::new()
        });
        let artists = artists_of(&tracks);

        Ok(SearchResults {
            tracks,
            albums,
            artists,
            playlists,
        })
    }

    pub async fn search_albums(&self, query: &str) -> Result<Vec<Album>> {
        pathfinder::search_albums(&self.session, query).await
    }

    pub async fn search_playlists(&self, query: &str) -> Result<Vec<Playlist>> {
        pathfinder::search_playlists(&self.session, query).await
    }

    pub async fn home(&self) -> Result<HomeFeed> {
        let mut sections = pathfinder::genre(&self.session, MADE_FOR_YOU).await?.sections;
        playlists::name_blanks(&self.session, &mut sections).await;

        Ok(HomeFeed {
            sections,
            ..HomeFeed::default()
        })
    }

    pub async fn genres(&self) -> Result<Vec<Genre>> {
        pathfinder::genres(&self.session).await
    }

    pub async fn genre(&self, genre_id: &str) -> Result<GenreDetail> {
        pathfinder::genre(&self.session, genre_id).await
    }
}

/// The distinct artists across a set of track hits, in first-seen order — Spotify search has no
/// artist entity, so results surface the artists that its tracks credit.
fn artists_of(tracks: &[Track]) -> Vec<ArtistRef> {
    let mut seen: HashSet<String> = HashSet::new();
    let mut artists = Vec::new();
    for artist in tracks.iter().flat_map(|track| track.artist_refs.iter()) {
        let key = artist.id.clone().unwrap_or_else(|| artist.name.clone());
        if seen.insert(key) {
            artists.push(artist.clone());
        }
    }
    artists
}
