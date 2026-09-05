//! Pure mapping from the Spotify provider's models ([`ryotunes_spotify`]) to the YouTube-shaped
//! browse JSON the client already renders. The daemon calls these when the selected provider is
//! Spotify (browsing) or an id carries a `spotify:` prefix (a track/album/artist/playlist page).
//!
//! Every id the daemon emits is prefixed `spotify:<kind>:<base62>`; the crate returns bare base62
//! ids, so these functions add the prefix on the way out (and the daemon strips it on the way back
//! in). Durations are rendered "m:ss" (or "h:mm:ss" past an hour), play counts abbreviated the way
//! YouTube writes them ("547M"). Cards and song rows are built as the real [`innertube`] structs and
//! serialised through them, so serde field naming stays byte-identical to the YouTube surfaces.
//!
//! These are pure: no network, no `AppState`, no async. The daemon fetches the crate models; this
//! turns them into the client's shapes.

use std::time::Duration;

use innertube::models::metadata::ArtistRun;
use innertube::{
    AlbumPage, ArtistCarousel, ArtistPage, BrowseItem, HomePage, PlaylistPage, Rating,
    SearchResults, Section, SongItem,
};
use ryotunes_spotify::{
    Album, AlbumDetail, Artist, ArtistRef, GenreItem, HomeFeed, Lyrics, LyricsLine, Playlist,
    PlaylistDetail, ReleaseType, SavedArtist, SearchResults as SpotifyResults, Track,
};

use crate::lyrics::{LyricLine, LyricWord, Lyrics as DaemonLyrics};

const TRACK: &str = "spotify:track:";
const ALBUM: &str = "spotify:album:";
const ARTIST: &str = "spotify:artist:";
const PLAYLIST: &str = "spotify:playlist:";

/// The stable id of the liked-songs pseudo-playlist (the daemon pages `client.liked` behind it).
pub const LIKED_PLAYLIST_ID: &str = "spotify:playlist:liked";

// --- tracks ----------------------------------------------------------------------------------

/// A Spotify [`Track`] as a queue/playlist [`SongItem`]. `is_upload`/`is_video` are always false
/// (Spotify has neither), the thumbnail is the track cover, and the rating reads "indifferent" —
/// Spotify's saved state is a library flag, not a per-row like the row UI can toggle.
pub fn track_to_song(track: &Track) -> SongItem {
    SongItem {
        video_id: format!("{TRACK}{}", track.id.clone().unwrap_or_default()),
        title: track.name.clone(),
        artists: track.artists.clone(),
        artist_id: first_artist_id(&track.artist_refs),
        artist_runs: artist_runs(&track.artist_refs),
        album: (!track.album.is_empty()).then(|| track.album.clone()),
        album_id: track.album_id.as_ref().map(|id| format!("{ALBUM}{id}")),
        duration: Some(fmt_duration(track.duration)),
        play_count: track.playcount.map(abbreviate),
        thumbnail: track.cover.clone(),
        set_video_id: None,
        added_by: None,
        added_by_avatar: None,
        rating: Some(Rating::Indifferent),
        queued_by: None,
        queued: false,
        queued_end: false,
        queued_from: None,
        autoplay: false,
        is_video: false,
        is_upload: false,
        explicit: track.explicit,
    }
}

/// A Spotify [`Track`] as a `song` [`BrowseItem`] (a home/search card).
pub fn track_to_card(track: &Track) -> BrowseItem {
    BrowseItem {
        kind: "song",
        id: format!("{TRACK}{}", track.id.clone().unwrap_or_default()),
        title: track.name.clone(),
        subtitle: (!track.artists.is_empty()).then(|| track.artists.clone()),
        thumbnail: track.cover.clone(),
        duration: Some(fmt_duration(track.duration)),
        artist_runs: artist_runs(&track.artist_refs),
        play_count: track.playcount.map(abbreviate),
        is_video: false,
        is_upload: false,
        explicit: track.explicit,
    }
}

// --- cards -----------------------------------------------------------------------------------

/// A Spotify [`Album`] as an `album` [`BrowseItem`].
pub fn album_to_card(album: &Album) -> BrowseItem {
    BrowseItem {
        kind: "album",
        id: format!("{ALBUM}{}", album.id),
        title: album.name.clone(),
        subtitle: (!album.artists.is_empty()).then(|| album.artists.clone()),
        thumbnail: album.cover.clone().or_else(|| album.cover_large.clone()),
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

/// A saved-library artist as an `artist` [`BrowseItem`].
pub fn saved_artist_to_card(artist: &SavedArtist) -> BrowseItem {
    BrowseItem {
        kind: "artist",
        id: format!("{ARTIST}{}", artist.id),
        title: artist.name.clone(),
        subtitle: None,
        thumbnail: artist.cover.clone(),
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

/// A search-result [`ArtistRef`] as an `artist` [`BrowseItem`]. `None` when the ref links no artist
/// id (Spotify search derives artists from track hits; an unlinked name cannot open a page).
pub fn artist_ref_to_card(artist: &ArtistRef) -> Option<BrowseItem> {
    let id = artist.id.as_ref()?;
    Some(BrowseItem {
        kind: "artist",
        id: format!("{ARTIST}{id}"),
        title: artist.name.clone(),
        subtitle: None,
        thumbnail: None,
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    })
}

/// A Spotify [`Playlist`] as a `playlist` [`BrowseItem`].
pub fn playlist_to_card(playlist: &Playlist) -> BrowseItem {
    BrowseItem {
        kind: "playlist",
        id: format!("{PLAYLIST}{}", playlist.id),
        title: playlist.name.clone(),
        subtitle: (!playlist.owner.is_empty()).then(|| playlist.owner.clone()),
        thumbnail: playlist.cover.clone(),
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

/// The fixed liked-songs row that heads the Spotify library ([`LIKED_PLAYLIST_ID`]).
pub fn liked_card() -> BrowseItem {
    BrowseItem {
        kind: "playlist",
        id: LIKED_PLAYLIST_ID.to_owned(),
        title: "Liked Songs".to_owned(),
        subtitle: None,
        thumbnail: None,
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

// --- pages -----------------------------------------------------------------------------------

/// A Spotify [`AlbumDetail`] as the album page the client renders. `playlist_id` carries the
/// `spotify:album:` id so the library-save toggle round-trips back to `set_album_saved`.
pub fn album_page(detail: &AlbumDetail) -> AlbumPage {
    let album = &detail.album;
    AlbumPage {
        title: Some(album.name.clone()),
        artist: (!album.artists.is_empty()).then(|| album.artists.clone()),
        artist_id: first_artist_id(&album.artist_refs),
        artist_runs: artist_runs(&album.artist_refs),
        artist_thumbnail: None,
        subtitle: Some(album_type_line(album)),
        second_subtitle: Some(song_count(detail.tracks.len().max(album.track_count as usize))),
        description: None,
        thumbnail: album.cover_large.clone().or_else(|| album.cover.clone()),
        items: detail.tracks.iter().map(track_to_song).collect(),
        continuation: None,
        explicit: detail.tracks.iter().any(|track| track.explicit),
        playlist_id: Some(format!("{ALBUM}{}", album.id)),
        in_library: false,
        sections: Vec::new(),
    }
}

/// A Spotify artist ([`Artist`], from `client.artist`) as the artist page: top tracks become the
/// "Popular" shelf, and the discography is grouped by release type into card carousels.
pub fn artist_page(id: &str, artist: &Artist) -> ArtistPage {
    ArtistPage {
        name: Some(artist.name.clone()),
        thumbnail: artist.cover_large.clone(),
        description: artist.biography.clone(),
        subscribers: None,
        monthly_listeners: artist
            .monthly_listeners
            .map(|n| format!("{} monthly listeners", group_thousands(n))),
        channel_id: format!("{ARTIST}{id}"),
        subscribed: false,
        radio_playlist_id: None,
        top_songs: artist.top_tracks.iter().map(track_to_song).collect(),
        top_songs_id: None,
        sections: artist_sections(&artist.albums),
    }
}

/// A Spotify [`PlaylistDetail`] as the playlist page.
pub fn playlist_page(detail: &PlaylistDetail) -> PlaylistPage {
    let playlist = &detail.playlist;
    PlaylistPage {
        title: Some(playlist.name.clone()),
        subtitle: (!playlist.owner.is_empty()).then(|| playlist.owner.clone()),
        thumbnail: playlist.cover.clone(),
        description: None,
        privacy: Some(if playlist.public { "PUBLIC" } else { "PRIVATE" }.to_owned()),
        cover: None,
        items: detail.tracks.iter().map(track_to_song).collect(),
        continuation: None,
        owned: playlist.owned,
        collaborative: playlist.collaborative,
        sort_menu: None,
    }
}

/// The liked-songs page: the paged saved tracks under the "Liked Songs" title.
pub fn liked_page(tracks: &[Track]) -> PlaylistPage {
    let items: Vec<SongItem> = tracks.iter().map(track_to_song).collect();
    PlaylistPage {
        title: Some("Liked Songs".to_owned()),
        subtitle: Some(song_count(items.len())),
        thumbnail: None,
        description: None,
        privacy: Some("PRIVATE".to_owned()),
        cover: None,
        items,
        continuation: None,
        owned: true,
        collaborative: false,
        sort_menu: None,
    }
}

/// A Spotify [`HomeFeed`] as the home page: chips are always empty, then "Listen again",
/// "Quick picks", then each genre section (playlists/albums; bare genres are dropped — they have
/// no navigable card kind). Empty sections are omitted.
pub fn home_page(feed: &HomeFeed) -> HomePage {
    let mut sections = Vec::new();
    if !feed.listen_again.is_empty() {
        sections.push(card_section(
            "Listen again",
            feed.listen_again.iter().map(track_to_card).collect(),
        ));
    }
    if let Some(quick) = feed.quick_picks.as_ref().filter(|quick| !quick.is_empty()) {
        sections.push(card_section("Quick picks", quick.iter().map(track_to_card).collect()));
    }
    for section in &feed.sections {
        let items: Vec<BrowseItem> = section.items.iter().filter_map(genre_item_to_card).collect();
        if !items.is_empty() {
            sections.push(card_section(section.title.clone(), items));
        }
    }
    HomePage { chips: Vec::new(), sections, continuation: None }
}

/// Spotify [`SpotifyResults`] as the `search_all` payload. Spotify has no "top result" shelf, so
/// `top` is empty; artists come from the track hits and drop any that link no id.
pub fn search_all(results: &SpotifyResults) -> SearchResults {
    SearchResults {
        top: Vec::new(),
        songs: results.tracks.iter().map(track_to_card).collect(),
        albums: results.albums.iter().map(album_to_card).collect(),
        artists: results.artists.iter().filter_map(artist_ref_to_card).collect(),
        playlists: results.playlists.iter().map(playlist_to_card).collect(),
        continuation: None,
    }
}

/// One `search_cards` category (`songs`|`albums`|`artists`|`playlists`) as a flat card list.
pub fn search_cards(results: &SpotifyResults, category: &str) -> Vec<BrowseItem> {
    match category {
        "songs" => results.tracks.iter().map(track_to_card).collect(),
        "albums" => results.albums.iter().map(album_to_card).collect(),
        "artists" => results.artists.iter().filter_map(artist_ref_to_card).collect(),
        "playlists" => results.playlists.iter().map(playlist_to_card).collect(),
        _ => Vec::new(),
    }
}

/// The `search_page` items: the track hits as song rows.
pub fn search_songs(results: &SpotifyResults) -> Vec<SongItem> {
    results.tracks.iter().map(track_to_song).collect()
}

// --- lyrics ----------------------------------------------------------------------------------

/// Spotify [`Lyrics`] as the daemon's lyrics shape. Synced lines carry per-line and (when present)
/// per-word timings; plain lyrics become un-timed lines.
pub fn lyrics_to_lyrics(lyrics: &Lyrics) -> DaemonLyrics {
    match lyrics {
        Lyrics::Synced { lines } => DaemonLyrics {
            source: "Spotify".to_owned(),
            synced: true,
            instrumental: false,
            lines: lines.iter().map(synced_line).collect(),
        },
        Lyrics::Plain { text, .. } => DaemonLyrics {
            source: "Spotify".to_owned(),
            synced: false,
            instrumental: false,
            lines: text.lines().map(|line| LyricLine::simple(None, line.to_owned())).collect(),
        },
    }
}

fn synced_line(line: &LyricsLine) -> LyricLine {
    LyricLine {
        time_ms: Some(millis(line.start)),
        end_time_ms: line.end.map(millis),
        text: line.text.clone(),
        words: line.words.as_ref().map(|words| {
            words
                .iter()
                .map(|word| LyricWord {
                    text: word.text.clone(),
                    start_ms: millis(word.start),
                    end_ms: millis(word.end),
                })
                .collect()
        }),
        translation: None,
    }
}

// --- helpers ---------------------------------------------------------------------------------

fn card_section(title: impl Into<String>, items: Vec<BrowseItem>) -> Section {
    Section {
        title: title.into(),
        title_is_artist: false,
        items,
        more_browse_id: None,
        more_params: None,
    }
}

fn genre_item_to_card(item: &GenreItem) -> Option<BrowseItem> {
    match item {
        GenreItem::Playlist(playlist) => Some(playlist_to_card(playlist)),
        GenreItem::Album(album) => Some(album_to_card(album)),
        GenreItem::Genre(_) => None,
    }
}

fn artist_sections(albums: &[Album]) -> Vec<ArtistCarousel> {
    let mut order: Vec<ReleaseType> = Vec::new();
    for album in albums {
        if !order.contains(&album.release_type) {
            order.push(album.release_type);
        }
    }
    order
        .into_iter()
        .map(|kind| ArtistCarousel {
            title: format!("{}s", kind.label()),
            items: albums
                .iter()
                .filter(|album| album.release_type == kind)
                .map(album_to_card)
                .collect(),
            more_browse_id: None,
            more_params: None,
        })
        .collect()
}

fn artist_runs(refs: &[ArtistRef]) -> Vec<ArtistRun> {
    let mut runs = Vec::with_capacity(refs.len().saturating_mul(2));
    for (i, artist) in refs.iter().enumerate() {
        if i > 0 {
            runs.push(ArtistRun { text: ", ".to_owned(), id: None });
        }
        runs.push(ArtistRun {
            text: artist.name.clone(),
            id: artist.id.as_ref().map(|id| format!("{ARTIST}{id}")),
        });
    }
    runs
}

fn first_artist_id(refs: &[ArtistRef]) -> Option<String> {
    refs.iter().find_map(|artist| artist.id.as_ref().map(|id| format!("{ARTIST}{id}")))
}

fn album_type_line(album: &Album) -> String {
    if album.year > 0 {
        format!("{} • {}", album.release_type.label(), album.year)
    } else {
        album.release_type.label().to_owned()
    }
}

fn song_count(count: usize) -> String {
    format!("{count} {}", if count == 1 { "song" } else { "songs" })
}

fn millis(duration: Duration) -> u64 {
    duration.as_millis() as u64
}

/// "m:ss", or "h:mm:ss" once the track runs an hour or more.
fn fmt_duration(duration: Duration) -> String {
    let total = duration.as_secs();
    let (hours, minutes, seconds) = (total / 3600, (total % 3600) / 60, total % 60);
    if hours > 0 {
        format!("{hours}:{minutes:02}:{seconds:02}")
    } else {
        format!("{minutes}:{seconds:02}")
    }
}

/// A play count the way YouTube abbreviates it: "547M", "2.5B", "1.2K", "999".
fn abbreviate(n: u64) -> String {
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

/// A full count with thousands separators ("12,345,678"), for the artist listener line.
fn group_thousands(n: u64) -> String {
    let digits = n.to_string();
    let len = digits.len();
    let mut out = String::with_capacity(len + len / 3);
    for (i, ch) in digits.chars().enumerate() {
        if i > 0 && (len - i) % 3 == 0 {
            out.push(',');
        }
        out.push(ch);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use ryotunes_spotify::{GenreSection, LyricsWord};
    use std::sync::Arc;

    fn track() -> Track {
        Track {
            id: Some("6rqhFgbbKwnb9MLmUQDhG6".to_owned()),
            name: "So Long, London".to_owned(),
            playable: true,
            artists: "Taylor Swift, Bon Iver".to_owned(),
            artist_refs: vec![
                ArtistRef {
                    name: "Taylor Swift".to_owned(),
                    id: Some("06HL4z0CvFAxyc27GXpf02".to_owned()),
                },
                ArtistRef { name: "Bon Iver".to_owned(), id: None },
            ],
            album: "The Tortured Poets Department".to_owned(),
            album_id: Some("1Mo4aZ8ho7Gm2ao5VOZjzna".to_owned()),
            cover: Some("https://i.scdn.co/image/cover".to_owned()),
            duration: Duration::from_secs(4 * 60 + 22),
            added_at: None,
            added_by: None,
            playcount: Some(547_000_000),
            popularity: 88,
            explicit: true,
            track_number: 5,
            disc_number: 1,
            tags: Vec::new(),
            languages: Vec::new(),
            credits: Vec::new(),
        }
    }

    fn album() -> Album {
        Album {
            id: "1Mo4aZ8ho7Gm2ao5VOZjzna".to_owned(),
            name: "The Tortured Poets Department".to_owned(),
            artists: "Taylor Swift".to_owned(),
            artist_refs: vec![ArtistRef {
                name: "Taylor Swift".to_owned(),
                id: Some("06HL4z0CvFAxyc27GXpf02".to_owned()),
            }],
            cover: Some("https://i.scdn.co/image/album".to_owned()),
            cover_large: Some("https://i.scdn.co/image/album-large".to_owned()),
            release_type: ReleaseType::Album,
            year: 2024,
            track_count: 16,
            release_date: "2024-04-19".to_owned(),
            label: "Republic".to_owned(),
            copyrights: Vec::new(),
            added_at: None,
        }
    }

    fn single() -> Album {
        Album { id: "single1".to_owned(), release_type: ReleaseType::Single, ..album() }
    }

    fn playlist() -> Playlist {
        Playlist {
            id: "37i9dQZF1DXcBWIGoYBM5M".to_owned(),
            name: "Today's Top Hits".to_owned(),
            owner: "Spotify".to_owned(),
            owner_id: "spotify".to_owned(),
            owned: false,
            collaborative: false,
            blend: false,
            public: true,
            cover: Some("https://i.scdn.co/image/playlist".to_owned()),
            track_count: 50,
            modified_at: None,
        }
    }

    #[test]
    fn track_to_song_prefixes_ids_and_formats() {
        let song = track_to_song(&track());
        assert_eq!(song.video_id, "spotify:track:6rqhFgbbKwnb9MLmUQDhG6");
        assert_eq!(song.title, "So Long, London");
        assert_eq!(song.artists, "Taylor Swift, Bon Iver");
        assert_eq!(song.artist_id.as_deref(), Some("spotify:artist:06HL4z0CvFAxyc27GXpf02"));
        assert_eq!(song.album.as_deref(), Some("The Tortured Poets Department"));
        assert_eq!(song.album_id.as_deref(), Some("spotify:album:1Mo4aZ8ho7Gm2ao5VOZjzna"));
        assert_eq!(song.duration.as_deref(), Some("4:22"));
        assert_eq!(song.play_count.as_deref(), Some("547M"));
        assert!(song.explicit);
        assert!(!song.is_video && !song.is_upload);
        assert_eq!(song.rating, Some(Rating::Indifferent));
        // Runs reproduce the display line, each linked name carrying a spotify: artist id.
        assert_eq!(song.artist_runs.len(), 3);
        assert_eq!(song.artist_runs[0].text, "Taylor Swift");
        assert_eq!(
            song.artist_runs[0].id.as_deref(),
            Some("spotify:artist:06HL4z0CvFAxyc27GXpf02")
        );
        assert_eq!(song.artist_runs[1].text, ", ");
        assert_eq!(song.artist_runs[1].id, None);
        assert_eq!(song.artist_runs[2].text, "Bon Iver");
        assert_eq!(song.artist_runs[2].id, None);
    }

    #[test]
    fn song_item_serialises_snake_case() {
        let json = serde_json::to_value(track_to_song(&track())).unwrap();
        assert_eq!(json["video_id"], "spotify:track:6rqhFgbbKwnb9MLmUQDhG6");
        assert_eq!(json["artist_runs"][0]["text"], "Taylor Swift");
        assert_eq!(json["duration"], "4:22");
        assert_eq!(json["play_count"], "547M");
        assert_eq!(json["rating"], "indifferent");
        assert_eq!(json["is_video"], false);
    }

    #[test]
    fn track_to_card_is_a_song_card() {
        let json = serde_json::to_value(track_to_card(&track())).unwrap();
        assert_eq!(json["kind"], "song");
        assert_eq!(json["id"], "spotify:track:6rqhFgbbKwnb9MLmUQDhG6");
        assert_eq!(json["subtitle"], "Taylor Swift, Bon Iver");
        assert_eq!(json["duration"], "4:22");
        assert_eq!(json["playCount"], "547M");
        assert_eq!(json["artistRuns"][0]["id"], "spotify:artist:06HL4z0CvFAxyc27GXpf02");
    }

    #[test]
    fn album_and_playlist_cards() {
        let a = serde_json::to_value(album_to_card(&album())).unwrap();
        assert_eq!(a["kind"], "album");
        assert_eq!(a["id"], "spotify:album:1Mo4aZ8ho7Gm2ao5VOZjzna");
        assert_eq!(a["subtitle"], "Taylor Swift");
        assert_eq!(a["thumbnail"], "https://i.scdn.co/image/album");

        let p = serde_json::to_value(playlist_to_card(&playlist())).unwrap();
        assert_eq!(p["kind"], "playlist");
        assert_eq!(p["id"], "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M");
        assert_eq!(p["subtitle"], "Spotify");
    }

    #[test]
    fn liked_card_is_fixed() {
        let json = serde_json::to_value(liked_card()).unwrap();
        assert_eq!(json["kind"], "playlist");
        assert_eq!(json["id"], "spotify:playlist:liked");
        assert_eq!(json["title"], "Liked Songs");
    }

    #[test]
    fn album_page_groups_and_saves() {
        let detail = AlbumDetail { album: album(), tracks: vec![track()] };
        let page = album_page(&detail);
        assert_eq!(page.title.as_deref(), Some("The Tortured Poets Department"));
        assert_eq!(page.subtitle.as_deref(), Some("Album • 2024"));
        assert_eq!(page.second_subtitle.as_deref(), Some("16 songs"));
        assert_eq!(page.artist_id.as_deref(), Some("spotify:artist:06HL4z0CvFAxyc27GXpf02"));
        assert_eq!(page.playlist_id.as_deref(), Some("spotify:album:1Mo4aZ8ho7Gm2ao5VOZjzna"));
        assert!(page.explicit);
        assert_eq!(page.items.len(), 1);
    }

    #[test]
    fn artist_page_popular_and_release_sections() {
        let artist = Artist {
            name: "Taylor Swift".to_owned(),
            cover_large: Some("https://i.scdn.co/image/artist".to_owned()),
            biography: Some("An American singer-songwriter.".to_owned()),
            monthly_listeners: Some(84_231_004),
            top_tracks: vec![track()],
            albums: vec![album(), single()],
        };
        let page = artist_page("06HL4z0CvFAxyc27GXpf02", &artist);
        assert_eq!(page.channel_id, "spotify:artist:06HL4z0CvFAxyc27GXpf02");
        assert_eq!(page.monthly_listeners.as_deref(), Some("84,231,004 monthly listeners"));
        assert_eq!(page.top_songs.len(), 1);
        assert_eq!(page.sections.len(), 2);
        assert_eq!(page.sections[0].title, "Albums");
        assert_eq!(page.sections[1].title, "Singles");
        assert_eq!(page.sections[0].items[0].id, "spotify:album:1Mo4aZ8ho7Gm2ao5VOZjzna");
    }

    #[test]
    fn playlist_and_liked_pages() {
        let detail = PlaylistDetail { playlist: playlist(), tracks: vec![track(), track()] };
        let page = playlist_page(&detail);
        assert_eq!(page.title.as_deref(), Some("Today's Top Hits"));
        assert_eq!(page.subtitle.as_deref(), Some("Spotify"));
        assert_eq!(page.privacy.as_deref(), Some("PUBLIC"));
        assert!(!page.owned);
        assert_eq!(page.items.len(), 2);

        let liked = liked_page(&[track()]);
        assert_eq!(liked.title.as_deref(), Some("Liked Songs"));
        assert_eq!(liked.subtitle.as_deref(), Some("1 song"));
        assert!(liked.owned);
    }

    #[test]
    fn home_page_orders_sections_and_drops_empties() {
        let feed = HomeFeed {
            listen_again: vec![track()],
            quick_picks: Some(vec![track(), track()]),
            sections: vec![
                GenreSection {
                    title: "Made for you".to_owned(),
                    items: vec![GenreItem::Playlist(playlist()), GenreItem::Album(album())],
                },
                GenreSection { title: "Empty".to_owned(), items: Vec::new() },
            ],
        };
        let page = home_page(&feed);
        assert!(page.chips.is_empty());
        assert_eq!(page.sections.len(), 3);
        assert_eq!(page.sections[0].title, "Listen again");
        assert_eq!(page.sections[0].items.len(), 1);
        assert_eq!(page.sections[1].title, "Quick picks");
        assert_eq!(page.sections[1].items.len(), 2);
        assert_eq!(page.sections[2].title, "Made for you");
        assert_eq!(page.sections[2].items.len(), 2);
        assert!(page.continuation.is_none());
    }

    #[test]
    fn search_all_and_cards() {
        let results = SpotifyResults {
            tracks: vec![track()],
            albums: vec![album()],
            artists: vec![
                ArtistRef {
                    name: "Taylor Swift".to_owned(),
                    id: Some("06HL4z0CvFAxyc27GXpf02".to_owned()),
                },
                ArtistRef { name: "Unlinked".to_owned(), id: None },
            ],
            playlists: vec![playlist()],
        };
        let all = search_all(&results);
        assert!(all.top.is_empty());
        assert_eq!(all.songs.len(), 1);
        assert_eq!(all.albums.len(), 1);
        assert_eq!(all.artists.len(), 1); // the id-less ref is dropped
        assert_eq!(all.playlists.len(), 1);

        assert_eq!(search_cards(&results, "albums").len(), 1);
        assert_eq!(search_cards(&results, "artists")[0].kind, "artist");
        assert!(search_cards(&results, "unknown").is_empty());
        assert_eq!(search_songs(&results).len(), 1);
    }

    #[test]
    fn duration_and_abbreviation_edges() {
        assert_eq!(fmt_duration(Duration::from_secs(9)), "0:09");
        assert_eq!(fmt_duration(Duration::from_secs(62)), "1:02");
        assert_eq!(fmt_duration(Duration::from_secs(3 * 3600 + 4 * 60 + 5)), "3:04:05");
        assert_eq!(abbreviate(999), "999");
        assert_eq!(abbreviate(1_200), "1.2K");
        assert_eq!(abbreviate(547_000_000), "547M");
        assert_eq!(abbreviate(2_500_000_000), "2.5B");
        assert_eq!(abbreviate(1_000_000), "1M");
        assert_eq!(group_thousands(84_231_004), "84,231,004");
    }

    #[test]
    fn lyrics_synced_words_map_through() {
        let lyrics = Lyrics::Synced {
            lines: Arc::from(vec![LyricsLine {
                start: Duration::from_millis(1500),
                end: Some(Duration::from_millis(4200)),
                text: "So long, London".to_owned(),
                romanized: None,
                words: Some(vec![LyricsWord {
                    start: Duration::from_millis(1500),
                    end: Duration::from_millis(2000),
                    text: "So".to_owned(),
                }]),
                secondary: Vec::new(),
                voice: Default::default(),
            }]),
        };
        let mapped = lyrics_to_lyrics(&lyrics);
        assert!(mapped.synced);
        assert_eq!(mapped.source, "Spotify");
        assert_eq!(mapped.lines.len(), 1);
        assert_eq!(mapped.lines[0].time_ms, Some(1500));
        assert_eq!(mapped.lines[0].end_time_ms, Some(4200));
        let words = mapped.lines[0].words.as_ref().unwrap();
        assert_eq!(words[0].text, "So");
        assert_eq!(words[0].start_ms, 1500);
        assert_eq!(words[0].end_ms, 2000);

        let plain = lyrics_to_lyrics(&Lyrics::plain("line one\nline two"));
        assert!(!plain.synced);
        assert_eq!(plain.lines.len(), 2);
        assert_eq!(plain.lines[1].text, "line two");
    }
}
