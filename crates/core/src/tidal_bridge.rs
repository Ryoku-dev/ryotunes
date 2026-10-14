//! Pure mapping from the TIDAL provider's models ([`ryotunes_tidal`]) to the YouTube-shaped browse
//! JSON the client already renders, the TIDAL twin of [`crate::soundcloud_bridge`]. The daemon
//! calls these when the selected provider is TIDAL (browsing) or an id carries a `tidal:` prefix.
//!
//! Ids are `tidal:track:<num>` / `tidal:artist:<num>` / `tidal:album:<num>` and, because TIDAL keys
//! playlists by uuid rather than a number, `tidal:playlist:<uuid>`. The crate speaks bare ids, so
//! these functions add the prefix on the way out and the daemon strips it on the way back in.
//!
//! The artist page reuses the YouTube [`ArtistPage`] rather than inventing a TIDAL superset the way
//! SoundCloud needed to: TIDAL's artist view carries nothing the shared shape cannot hold, and every
//! extra page kind buys a branch in the client.
//!
//! These are pure: no network, no `AppState`, no async.

use innertube::models::browse::ArtistCarousel;
use innertube::models::metadata::ArtistRun;
use innertube::{AlbumPage, BrowseItem, PlaylistPage, SearchResults, Section, SongItem};
use ryotunes_tidal::{Album, Artist, Playlist, SearchResults as TdResults, Track};

const TRACK: &str = "tidal:track:";
const ARTIST: &str = "tidal:artist:";
const ALBUM: &str = "tidal:album:";
const PLAYLIST: &str = "tidal:playlist:";

/// Card artwork, and the larger size a page header wants.
const CARD_ART: u32 = 320;
const PAGE_ART: u32 = 1280;

// --- tracks ----------------------------------------------------------------------------------

/// A TIDAL [`Track`] as a queue/playlist [`SongItem`]. `rating` stays `None`: favourites live in
/// TIDAL's own collection rather than in a per-track like the client's control writes to.
pub fn track_to_song(track: &Track) -> SongItem {
    SongItem {
        video_id: format!("{TRACK}{}", track.id),
        title: track.title.clone(),
        artists: track.artist_line(),
        artist_id: track.artists.first().map(|a| format!("{ARTIST}{}", a.id)),
        artist_runs: artist_runs(&track.artists),
        album: track.album.as_ref().map(|a| a.title.clone()),
        album_id: track.album.as_ref().map(|a| format!("{ALBUM}{}", a.id)),
        duration: track.duration.map(fmt_duration),
        play_count: None,
        thumbnail: track.cover_url(CARD_ART),
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

/// A TIDAL [`Track`] as a `song` [`BrowseItem`] (a home/search card).
pub fn track_to_card(track: &Track) -> BrowseItem {
    BrowseItem {
        kind: "song",
        id: format!("{TRACK}{}", track.id),
        title: track.title.clone(),
        subtitle: Some(track.artist_line()).filter(|s| !s.is_empty()),
        thumbnail: track.cover_url(CARD_ART),
        duration: track.duration.map(fmt_duration),
        artist_runs: artist_runs(&track.artists),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

// --- cards -----------------------------------------------------------------------------------

pub fn artist_to_card(artist: &Artist) -> BrowseItem {
    BrowseItem {
        kind: "artist",
        id: format!("{ARTIST}{}", artist.id),
        title: artist.name.clone(),
        subtitle: None,
        thumbnail: artist.picture.as_deref().and_then(|p| ryotunes_tidal::cover_url(p, CARD_ART)),
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

pub fn album_to_card(album: &Album) -> BrowseItem {
    BrowseItem {
        kind: "album",
        id: format!("{ALBUM}{}", album.id),
        title: album.title.clone(),
        subtitle: Some(album_line(album)),
        thumbnail: album.cover.as_deref().and_then(|c| ryotunes_tidal::cover_url(c, CARD_ART)),
        duration: None,
        artist_runs: artist_runs(&album.artists),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

pub fn playlist_to_card(pl: &Playlist) -> BrowseItem {
    BrowseItem {
        kind: "playlist",
        id: format!("{PLAYLIST}{}", pl.uuid),
        title: pl.title.clone(),
        subtitle: pl.track_count.map(|n| track_count_line(n as u64)),
        thumbnail: pl.cover_url(CARD_ART),
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

// --- pages -----------------------------------------------------------------------------------

/// A TIDAL album with its tracks as the album page.
pub fn album_page(album: &Album, tracks: &[Track]) -> AlbumPage {
    let count = tracks.len().max(album.track_count.unwrap_or(0) as usize) as u64;
    AlbumPage {
        title: Some(album.title.clone()),
        artist: Some(artist_line(&album.artists)).filter(|s| !s.is_empty()),
        artist_id: album.artists.first().map(|a| format!("{ARTIST}{}", a.id)),
        artist_runs: artist_runs(&album.artists),
        artist_thumbnail: album
            .artists
            .first()
            .and_then(|a| a.picture.as_deref())
            .and_then(|p| ryotunes_tidal::cover_url(p, CARD_ART)),
        subtitle: Some(album_line(album)),
        second_subtitle: Some(track_count_line(count)),
        description: None,
        thumbnail: album.cover.as_deref().and_then(|c| ryotunes_tidal::cover_url(c, PAGE_ART)),
        items: tracks.iter().map(track_to_song).collect(),
        continuation: None,
        explicit: false,
        playlist_id: Some(format!("{ALBUM}{}", album.id)),
        in_library: false,
        sections: Vec::new(),
    }
}

/// A TIDAL playlist with its tracks as the playlist page.
pub fn playlist_page(pl: &Playlist, tracks: &[Track]) -> PlaylistPage {
    PlaylistPage {
        title: Some(pl.title.clone()),
        subtitle: pl.track_count.map(|n| track_count_line(n as u64)),
        thumbnail: pl.cover_url(PAGE_ART),
        description: pl.description.clone(),
        privacy: None,
        cover: None,
        items: tracks.iter().map(track_to_song).collect(),
        continuation: None,
        owned: false,
        collaborative: false,
        sort_menu: None,
    }
}

/// A TIDAL artist page: the top tracks shelf plus one carousel of albums.
pub fn artist_page(artist: &Artist, top: &[Track], albums: &[Album]) -> innertube::ArtistPage {
    let sections = if albums.is_empty() {
        Vec::new()
    } else {
        vec![ArtistCarousel {
            title: "Albums".to_string(),
            items: albums.iter().map(album_to_card).collect(),
            more_browse_id: None,
            more_params: None,
        }]
    };
    innertube::ArtistPage {
        name: Some(artist.name.clone()),
        thumbnail: artist.picture.as_deref().and_then(|p| ryotunes_tidal::cover_url(p, PAGE_ART)),
        description: None,
        subscribers: None,
        monthly_listeners: None,
        channel_id: format!("{ARTIST}{}", artist.id),
        subscribed: false,
        radio_playlist_id: None,
        top_songs: top.iter().map(track_to_song).collect(),
        top_songs_id: None,
        sections,
    }
}

// --- search ----------------------------------------------------------------------------------

/// TIDAL [`TdResults`] as the `search_all` payload. TIDAL names no top result, so `top` is empty.
pub fn search_all(results: &TdResults) -> SearchResults {
    SearchResults {
        top: Vec::new(),
        songs: results.tracks.items.iter().map(track_to_card).collect(),
        albums: results.albums.items.iter().map(album_to_card).collect(),
        artists: results.artists.items.iter().map(artist_to_card).collect(),
        playlists: results.playlists.items.iter().map(playlist_to_card).collect(),
        continuation: None,
    }
}

/// One `search_cards` category (`songs`|`albums`|`artists`|`playlists`) as a flat card list.
pub fn search_cards(results: &TdResults, category: &str) -> Vec<BrowseItem> {
    match category {
        "songs" => results.tracks.items.iter().map(track_to_card).collect(),
        "albums" => results.albums.items.iter().map(album_to_card).collect(),
        "artists" => results.artists.items.iter().map(artist_to_card).collect(),
        "playlists" => results.playlists.items.iter().map(playlist_to_card).collect(),
        _ => Vec::new(),
    }
}

/// The `search_page` items: the track hits as song rows.
pub fn search_songs(results: &TdResults) -> Vec<SongItem> {
    results.tracks.items.iter().map(track_to_song).collect()
}

// --- home ------------------------------------------------------------------------------------

/// The signed-in account's own shelves. TIDAL has no guest catalogue, so this is the whole of
/// Home; an empty list drops its shelf rather than rendering a blank row.
pub fn library_sections(
    tracks: &[Track],
    albums: &[Album],
    playlists: &[Playlist],
) -> Vec<Section> {
    let mut sections = Vec::new();
    if !playlists.is_empty() {
        sections.push(card_section(
            "Your playlists".into(),
            playlists.iter().map(playlist_to_card).collect(),
        ));
    }
    if !tracks.is_empty() {
        sections
            .push(card_section("Your tracks".into(), tracks.iter().map(track_to_card).collect()));
    }
    if !albums.is_empty() {
        sections
            .push(card_section("Your albums".into(), albums.iter().map(album_to_card).collect()));
    }
    sections
}

// --- helpers ---------------------------------------------------------------------------------

fn card_section(title: String, items: Vec<BrowseItem>) -> Section {
    Section { title, title_is_artist: false, items, more_browse_id: None, more_params: None }
}

fn artist_runs(artists: &[Artist]) -> Vec<ArtistRun> {
    artists
        .iter()
        .filter(|a| !a.name.is_empty())
        .map(|a| ArtistRun { text: a.name.clone(), id: Some(format!("{ARTIST}{}", a.id)) })
        .collect()
}

fn artist_line(artists: &[Artist]) -> String {
    artists.iter().map(|a| a.name.as_str()).filter(|n| !n.is_empty()).collect::<Vec<_>>().join(", ")
}

/// "Album · 2019", or just "Album" when TIDAL names no release date.
fn album_line(album: &Album) -> String {
    match album.release_date.as_deref().and_then(|d| d.get(..4)) {
        Some(year) if !year.is_empty() => format!("Album · {year}"),
        _ => "Album".to_string(),
    }
}

fn track_count_line(n: u64) -> String {
    format!("{n} track{}", if n == 1 { "" } else { "s" })
}

/// TIDAL reports seconds; the client wants "m:ss", or "h:mm:ss" past an hour.
fn fmt_duration(seconds: u32) -> String {
    let (hours, minutes, seconds) = (seconds / 3600, (seconds % 3600) / 60, seconds % 60);
    if hours > 0 {
        format!("{hours}:{minutes:02}:{seconds:02}")
    } else {
        format!("{minutes}:{seconds:02}")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn track() -> Track {
        serde_json::from_str(
            r#"{"id":7,"title":"Song","duration":185,
                "artists":[{"id":1,"name":"A"},{"id":2,"name":"B"}],
                "album":{"id":9,"title":"Rec","cover":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}}"#,
        )
        .unwrap()
    }

    #[test]
    fn a_song_row_carries_prefixed_ids_everywhere() {
        let s = track_to_song(&track());
        assert_eq!(s.video_id, "tidal:track:7");
        assert_eq!(s.artist_id.as_deref(), Some("tidal:artist:1"));
        assert_eq!(s.album_id.as_deref(), Some("tidal:album:9"));
        assert_eq!(s.artists, "A, B");
        assert_eq!(s.duration.as_deref(), Some("3:05"));
        assert_eq!(s.artist_runs.len(), 2);
        assert_eq!(s.artist_runs[1].id.as_deref(), Some("tidal:artist:2"));
    }

    #[test]
    fn durations_grow_an_hour_field_only_when_needed() {
        assert_eq!(fmt_duration(59), "0:59");
        assert_eq!(fmt_duration(185), "3:05");
        assert_eq!(fmt_duration(3600), "1:00:00");
        assert_eq!(fmt_duration(3725), "1:02:05");
    }

    #[test]
    fn a_playlist_id_keeps_its_uuid_rather_than_a_number() {
        let pl: Playlist =
            serde_json::from_str(r#"{"uuid":"1f-2e-3d","title":"Mix","numberOfTracks":3}"#)
                .unwrap();
        let card = playlist_to_card(&pl);
        assert_eq!(card.id, "tidal:playlist:1f-2e-3d");
        assert_eq!(card.subtitle.as_deref(), Some("3 tracks"));
    }

    #[test]
    fn an_album_without_a_date_still_gets_a_subtitle() {
        let a: Album = serde_json::from_str(r#"{"id":1,"title":"X"}"#).unwrap();
        assert_eq!(album_line(&a), "Album");
        let b: Album =
            serde_json::from_str(r#"{"id":1,"title":"X","releaseDate":"2019-04-01"}"#).unwrap();
        assert_eq!(album_line(&b), "Album · 2019");
    }

    #[test]
    fn one_track_reads_singular() {
        assert_eq!(track_count_line(1), "1 track");
        assert_eq!(track_count_line(0), "0 tracks");
    }

    #[test]
    fn search_splits_into_the_four_card_lists() {
        let r: TdResults = serde_json::from_str(
            r#"{"tracks":{"items":[{"id":1,"title":"t"}]},
                "albums":{"items":[{"id":2,"title":"a"}]},
                "artists":{"items":[{"id":3,"name":"r"}]},
                "playlists":{"items":[{"uuid":"u","title":"p"}]}}"#,
        )
        .unwrap();
        let all = search_all(&r);
        assert!(all.top.is_empty(), "tidal names no top result");
        assert_eq!(all.songs.len(), 1);
        assert_eq!(all.albums[0].id, "tidal:album:2");
        assert_eq!(all.artists[0].id, "tidal:artist:3");
        assert_eq!(all.playlists[0].id, "tidal:playlist:u");
        assert!(search_cards(&r, "nonsense").is_empty());
    }

    #[test]
    fn empty_shelves_are_dropped_rather_than_rendered_blank() {
        assert!(library_sections(&[], &[], &[]).is_empty());
        let s = library_sections(&[track()], &[], &[]);
        assert_eq!(s.len(), 1);
        assert_eq!(s[0].title, "Your tracks");
    }
}
