//! Pure mapping from the SoundCloud provider's models ([`ryotunes_soundcloud`]) to the
//! YouTube-shaped browse JSON the client already renders — the SoundCloud twin of
//! [`crate::spotify_bridge`]. The daemon calls these when the selected provider is SoundCloud
//! (browsing) or an id carries an `sc:` prefix (a track/user/playlist page).
//!
//! Ids are `sc:track:<num>` / `sc:user:<num>` / `sc:playlist:<num>` (albums are just playlists on
//! SoundCloud); the crate uses bare numeric ids, so these functions add the prefix on the way out
//! (the daemon strips it on the way back in). Durations render "m:ss" (or "h:mm:ss" past an hour),
//! play counts abbreviated the way YouTube writes them ("547M"). Cards and song rows are built as
//! the real [`innertube`] structs and serialised through them, so serde field naming stays
//! byte-identical to the YouTube surfaces.
//!
//! Every page reuses the YouTube page structs verbatim except the artist page, which is a
//! SoundCloud-specific superset ([`SoundcloudArtistPage`]): the Orange artist view (round avatar,
//! banner, followers, and the albums/playlists/likes side columns) needs fields the YouTube
//! [`innertube::ArtistPage`] has no room for.
//!
//! These are pure: no network, no `AppState`, no async. The daemon fetches the crate models; this
//! turns them into the client's shapes.

use innertube::models::metadata::ArtistRun;
use innertube::{AlbumPage, BrowseItem, HomePage, PlaylistPage, SearchResults, Section, SongItem};
use ryotunes_soundcloud::{
    Playlist, PlaylistDetail, SearchResults as ScResults, Track, User, UserRef,
};

const TRACK: &str = "sc:track:";
const USER: &str = "sc:user:";
const PLAYLIST: &str = "sc:playlist:";

// --- tracks ----------------------------------------------------------------------------------

/// A SoundCloud [`Track`] as a queue/playlist [`SongItem`]. The artist is the uploading user
/// (SoundCloud has no separate album artist), the thumbnail is the track art falling back to the
/// uploader's avatar, and `play_count` carries the play total abbreviated the way YouTube writes
/// it. `rating` is `None` (a guest cannot like), `album`/`is_upload`/`is_video`/`explicit` are all
/// empty/false — SoundCloud has none of them.
pub fn track_to_song(track: &Track) -> SongItem {
    SongItem {
        video_id: format!("{TRACK}{}", track.id),
        title: track.title.clone(),
        artists: track.user.username.clone(),
        artist_id: Some(format!("{USER}{}", track.user.id)),
        artist_runs: user_ref_runs(&track.user),
        album: None,
        album_id: None,
        duration: Some(fmt_duration_ms(track.duration_ms)),
        play_count: track.plays.map(abbreviate),
        thumbnail: track_art(track),
        set_video_id: None,
        added_by: None,
        added_by_avatar: None,
        rating: None,
        queued_by: None,
        queued: false,
        queued_end: false,
        queued_from: None,
        autoplay: false,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

/// A SoundCloud [`Track`] as a `song` [`BrowseItem`] (a home/search card).
pub fn track_to_card(track: &Track) -> BrowseItem {
    BrowseItem {
        kind: "song",
        id: format!("{TRACK}{}", track.id),
        title: track.title.clone(),
        subtitle: Some(track.user.username.clone()),
        thumbnail: track_art(track),
        duration: Some(fmt_duration_ms(track.duration_ms)),
        artist_runs: user_ref_runs(&track.user),
        play_count: track.plays.map(abbreviate),
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

// --- cards -----------------------------------------------------------------------------------

/// A SoundCloud [`User`] as an `artist` [`BrowseItem`].
pub fn user_to_card(user: &User) -> BrowseItem {
    BrowseItem {
        kind: "artist",
        id: format!("{USER}{}", user.id),
        title: user.username.clone(),
        subtitle: None,
        thumbnail: user.avatar.clone(),
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

/// A SoundCloud [`Playlist`] with `is_album` as an `album` [`BrowseItem`]; the subtitle reads
/// "Album · <year>".
pub fn album_to_card(pl: &Playlist) -> BrowseItem {
    BrowseItem {
        kind: "album",
        id: format!("{PLAYLIST}{}", pl.id),
        title: pl.title.clone(),
        subtitle: Some(album_line(pl)),
        thumbnail: pl.artwork.clone(),
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

/// A SoundCloud [`Playlist`] (not an album) as a `playlist` [`BrowseItem`]; the subtitle is its
/// track count.
pub fn playlist_to_card(pl: &Playlist) -> BrowseItem {
    BrowseItem {
        kind: "playlist",
        id: format!("{PLAYLIST}{}", pl.id),
        title: pl.title.clone(),
        subtitle: Some(track_count_line(pl.track_count)),
        thumbnail: pl.artwork.clone(),
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

// --- pages -----------------------------------------------------------------------------------

/// A SoundCloud album ([`PlaylistDetail`] whose playlist is `is_album`) as the album page. The
/// `artist_id`/`playlist_id` carry `sc:` ids so the artist link and any downstream navigation
/// round-trip back through the daemon.
pub fn album_page(detail: &PlaylistDetail) -> AlbumPage {
    let pl = &detail.playlist;
    let count = detail.tracks.len().max(pl.track_count as usize) as u64;
    AlbumPage {
        title: Some(pl.title.clone()),
        artist: Some(pl.user.username.clone()),
        artist_id: Some(format!("{USER}{}", pl.user.id)),
        artist_runs: user_ref_runs(&pl.user),
        artist_thumbnail: pl.user.avatar.clone(),
        subtitle: Some(album_line(pl)),
        second_subtitle: Some(track_count_line(count)),
        description: detail.description.clone(),
        thumbnail: pl.artwork.clone(),
        items: detail.tracks.iter().map(track_to_song).collect(),
        continuation: None,
        explicit: false,
        playlist_id: Some(format!("{PLAYLIST}{}", pl.id)),
        in_library: false,
        sections: Vec::new(),
    }
}

/// A SoundCloud [`PlaylistDetail`] as the playlist page.
pub fn playlist_page(detail: &PlaylistDetail) -> PlaylistPage {
    let pl = &detail.playlist;
    PlaylistPage {
        title: Some(pl.title.clone()),
        subtitle: Some(pl.user.username.clone()),
        thumbnail: pl.artwork.clone(),
        description: detail.description.clone(),
        privacy: None,
        cover: None,
        items: detail.tracks.iter().map(track_to_song).collect(),
        continuation: None,
        owned: false,
        collaborative: false,
        sort_menu: None,
    }
}

/// The home page for SoundCloud: the daemon's chart shelves (`title`, tracks) as titled card
/// carousels. Empty shelves are dropped so a failed chart fetch never leaves a blank row.
pub fn home_page(shelves: Vec<(String, Vec<Track>)>) -> HomePage {
    let sections = shelves
        .into_iter()
        .filter(|(_, tracks)| !tracks.is_empty())
        .map(|(title, tracks)| card_section(title, tracks.iter().map(track_to_card).collect()))
        .collect();
    HomePage { chips: Vec::new(), sections, continuation: None }
}

/// SoundCloud [`ScResults`] as the `search_all` payload. SoundCloud has no "top result" shelf, so
/// `top` is empty; albums are the `is_album` playlists, playlists the rest.
pub fn search_all(results: &ScResults) -> SearchResults {
    SearchResults {
        top: Vec::new(),
        songs: results.tracks.iter().map(track_to_card).collect(),
        albums: results.playlists.iter().filter(|p| p.is_album).map(album_to_card).collect(),
        artists: results.users.iter().map(user_to_card).collect(),
        playlists: results.playlists.iter().filter(|p| !p.is_album).map(playlist_to_card).collect(),
        continuation: None,
    }
}

/// One `search_cards` category (`songs`|`albums`|`artists`|`playlists`) as a flat card list.
pub fn search_cards(results: &ScResults, category: &str) -> Vec<BrowseItem> {
    match category {
        "songs" => results.tracks.iter().map(track_to_card).collect(),
        "albums" => results.playlists.iter().filter(|p| p.is_album).map(album_to_card).collect(),
        "artists" => results.users.iter().map(user_to_card).collect(),
        "playlists" => {
            results.playlists.iter().filter(|p| !p.is_album).map(playlist_to_card).collect()
        }
        _ => Vec::new(),
    }
}

/// The `search_page` items: the track hits as song rows.
pub fn search_songs(results: &ScResults) -> Vec<SongItem> {
    results.tracks.iter().map(track_to_song).collect()
}

// --- artist page -----------------------------------------------------------------------------

/// The Orange/SoundCloud artist page. A superset of the YouTube [`innertube::ArtistPage`]: the
/// user's card fields (round avatar, banner, follower/track counts, verified tick) plus the four
/// content columns the view renders — the user's `tracks`, their `albums`, `playlists`, and
/// `likes`. `kind` is always `"soundcloud"` so the client switches to the bespoke view.
#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SoundcloudArtistPage {
    pub kind: &'static str,
    /// `sc:user:<id>` — the id the page navigated to.
    pub channel_id: String,
    pub name: String,
    /// The round avatar.
    pub thumbnail: Option<String>,
    /// The wide profile banner, blurred to paper behind the header.
    pub banner: Option<String>,
    pub followers: u64,
    pub track_count: u64,
    pub city: Option<String>,
    pub country: Option<String>,
    pub description: Option<String>,
    pub verified: bool,
    /// The user's own tracks (the left column list).
    pub tracks: Vec<SongItem>,
    /// "Albums from this user".
    pub albums: Vec<BrowseItem>,
    pub playlists: Vec<BrowseItem>,
    pub likes: Vec<SongItem>,
}

/// Build the [`SoundcloudArtistPage`] from a resolved user and their four content lists.
pub fn artist_page(
    user: &User,
    tracks: &[Track],
    albums: &[Playlist],
    playlists: &[Playlist],
    likes: &[Track],
) -> SoundcloudArtistPage {
    SoundcloudArtistPage {
        kind: "soundcloud",
        channel_id: format!("{USER}{}", user.id),
        name: user.username.clone(),
        thumbnail: user.avatar.clone(),
        banner: user.banner.clone(),
        followers: user.followers,
        track_count: user.track_count,
        city: user.city.clone(),
        country: user.country.clone(),
        description: user.description.clone(),
        verified: user.verified,
        tracks: tracks.iter().map(track_to_song).collect(),
        albums: albums.iter().map(album_to_card).collect(),
        playlists: playlists.iter().map(playlist_to_card).collect(),
        likes: likes.iter().map(track_to_song).collect(),
    }
}

// --- helpers ---------------------------------------------------------------------------------

fn card_section(title: String, items: Vec<BrowseItem>) -> Section {
    Section { title, title_is_artist: false, items, more_browse_id: None, more_params: None }
}

fn user_ref_runs(user: &UserRef) -> Vec<ArtistRun> {
    vec![ArtistRun { text: user.username.clone(), id: Some(format!("{USER}{}", user.id)) }]
}

fn track_art(track: &Track) -> Option<String> {
    track.artwork.clone().or_else(|| track.user.avatar.clone())
}

fn album_line(pl: &Playlist) -> String {
    match pl.release_date.as_deref().and_then(year_of) {
        Some(year) => format!("Album · {year}"),
        None => "Album".to_owned(),
    }
}

fn track_count_line(n: u64) -> String {
    format!("{n} track{}", if n == 1 { "" } else { "s" })
}

/// The leading four-digit year of a SoundCloud date string (`2019-05-10T00:00:00Z`), if it starts
/// with one.
fn year_of(date: &str) -> Option<&str> {
    let year = date.get(0..4)?;
    year.chars().all(|c| c.is_ascii_digit()).then_some(year)
}

/// "m:ss", or "h:mm:ss" once the track runs an hour or more.
pub(crate) fn fmt_duration_ms(ms: u64) -> String {
    let total = ms / 1000;
    let (hours, minutes, seconds) = (total / 3600, (total % 3600) / 60, total % 60);
    if hours > 0 {
        format!("{hours}:{minutes:02}:{seconds:02}")
    } else {
        format!("{minutes}:{seconds:02}")
    }
}

/// A play/like count the way YouTube abbreviates it: "547M", "2.5B", "1.2K", "999".
pub(crate) fn abbreviate(n: u64) -> String {
    fn scale(value: f64, unit: &str) -> String {
        if value >= 10.0 {
            format!("{}{unit}", value.round() as u64)
        } else {
            let text = format!("{value:.1}");
            format!("{}{unit}", text.strip_suffix(".0").unwrap_or(&text))
        }
    }
    if n >= 1_000_000_000 {
        scale(n as f64 / 1e9, "B")
    } else if n >= 1_000_000 {
        scale(n as f64 / 1e6, "M")
    } else if n >= 1_000 {
        scale(n as f64 / 1e3, "K")
    } else {
        n.to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ryotunes_soundcloud::UserRef;

    fn track(id: u64) -> Track {
        Track {
            id,
            title: "Jubel".to_owned(),
            user: UserRef {
                id: 42,
                username: "Klingande".to_owned(),
                permalink: "klingande".to_owned(),
                avatar: Some("https://cdn/avatar-large.jpg".to_owned()),
            },
            duration_ms: 425_000,
            artwork: Some("https://cdn/art-t500x500.jpg".to_owned()),
            genre: Some("Deep House".to_owned()),
            tags: vec![],
            plays: Some(12_500_000),
            likes: Some(210_000),
            created_at: "2013-09-01T00:00:00Z".to_owned(),
            permalink_url: "https://soundcloud.com/klingande/jubel".to_owned(),
            waveform_url: Some("https://wave/j.json".to_owned()),
            authorization: Some("tok".to_owned()),
            transcodings: vec![],
            description: None,
        }
    }

    #[test]
    fn track_maps_to_prefixed_song_row() {
        let s = track_to_song(&track(580_008_897));
        assert_eq!(s.video_id, "sc:track:580008897");
        assert_eq!(s.artists, "Klingande");
        assert_eq!(s.artist_id.as_deref(), Some("sc:user:42"));
        assert_eq!(s.duration.as_deref(), Some("7:05"));
        assert_eq!(s.play_count.as_deref(), Some("13M"));
        assert_eq!(s.thumbnail.as_deref(), Some("https://cdn/art-t500x500.jpg"));
        assert!(s.rating.is_none());
        assert!(!s.is_upload && !s.is_video && !s.explicit);
    }

    #[test]
    fn artist_page_carries_soundcloud_keys() {
        let user = User {
            id: 42,
            username: "Klingande".to_owned(),
            full_name: None,
            permalink: "klingande".to_owned(),
            avatar: Some("https://cdn/avatar-large.jpg".to_owned()),
            banner: Some("https://cdn/banner.jpg".to_owned()),
            followers: 500_000,
            track_count: 20,
            city: Some("Paris".to_owned()),
            country: Some("France".to_owned()),
            description: Some("bio".to_owned()),
            verified: true,
        };
        let album = Playlist {
            id: 7,
            title: "The Album".to_owned(),
            user: user_ref(),
            artwork: Some("https://cdn/al.jpg".to_owned()),
            is_album: true,
            set_type: Some("album".to_owned()),
            track_count: 10,
            release_date: Some("2016-04-22T00:00:00Z".to_owned()),
            duration_ms: 2_400_000,
        };
        let page = artist_page(&user, &[track(1)], &[album], &[], &[track(2)]);
        let json = serde_json::to_value(&page).unwrap();
        let obj = json.as_object().unwrap();
        for key in [
            "kind",
            "channelId",
            "name",
            "thumbnail",
            "banner",
            "followers",
            "trackCount",
            "city",
            "country",
            "description",
            "verified",
            "tracks",
            "albums",
            "playlists",
            "likes",
        ] {
            assert!(obj.contains_key(key), "artist_page missing key {key}");
        }
        assert_eq!(obj["kind"], "soundcloud");
        assert_eq!(obj["channelId"], "sc:user:42");
        assert_eq!(obj["albums"][0]["subtitle"], "Album · 2016");
        assert_eq!(obj["albums"][0]["id"], "sc:playlist:7");
    }

    fn user_ref() -> UserRef {
        UserRef {
            id: 42,
            username: "Klingande".to_owned(),
            permalink: "klingande".to_owned(),
            avatar: None,
        }
    }

    #[test]
    fn albums_and_playlists_split_by_is_album() {
        let mk = |id: u64, is_album: bool| Playlist {
            id,
            title: format!("p{id}"),
            user: user_ref(),
            artwork: None,
            is_album,
            set_type: None,
            track_count: 3,
            release_date: None,
            duration_ms: 0,
        };
        let results =
            ScResults { tracks: vec![], users: vec![], playlists: vec![mk(1, true), mk(2, false)] };
        let all = search_all(&results);
        assert_eq!(all.albums.len(), 1);
        assert_eq!(all.albums[0].kind, "album");
        assert_eq!(all.playlists.len(), 1);
        assert_eq!(all.playlists[0].kind, "playlist");
    }
}
