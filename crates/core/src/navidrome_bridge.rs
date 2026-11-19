//! Pure mapping from the Navidrome provider's models ([`ryotunes_navidrome`]) to the
//! YouTube-shaped browse JSON the client already renders, the Navidrome twin of
//! [`crate::tidal_bridge`]. The daemon calls these when the selected provider is Navidrome
//! (browsing) or an id carries an `nd:` prefix (a track/album/artist/playlist page).
//!
//! Ids are `nd:track:<id>` / `nd:album:<id>` / `nd:artist:<id>` / `nd:playlist:<id>`. The crate
//! speaks the server's own opaque ids, so these functions add the prefix on the way out and the
//! daemon strips it on the way back in. Because a server id can be an MD5 hash, a UUID or a scan
//! number, nothing is parsed as a number here.
//!
//! A playlist row also carries its position: `set_video_id = "nd:idx:<n>"`. Subsonic removes songs
//! by index, and the same song may appear twice, so the index is the only unambiguous handle.
//!
//! Cover URLs come from [`Covers`], which is given to every mapping function rather than reached
//! for through the client: that keeps these pure — no network, no `AppState`, no async.
//!
//! Lyrics arrive already chosen ([`ryotunes_navidrome::LyricsList::best`]) and only need timing and
//! attribution translated to [`crate::lyrics::Lyrics`].

use innertube::models::browse::ArtistCarousel;
use innertube::models::metadata::{ArtistRun, Rating};
use innertube::{AlbumPage, BrowseItem, PlaylistPage, SearchResults, Section, SongItem};
use ryotunes_navidrome::{
    Album, Artist, Covers, Playlist, SearchResults as NdResults, Song, Starred, StructuredLyrics,
};

use crate::lyrics::{LyricLine, Lyrics};

const TRACK: &str = "nd:track:";
const ALBUM: &str = "nd:album:";
const ARTIST: &str = "nd:artist:";
const PLAYLIST: &str = "nd:playlist:";
/// A playlist row's position on the server, used to remove exactly that row.
const INDEX: &str = "nd:idx:";

/// Card artwork, and the larger size a page header wants.
const CARD_ART: u32 = 320;
const PAGE_ART: u32 = 1280;

// --- tracks ----------------------------------------------------------------------------------

/// A server [`Song`] as a queue/album-row [`SongItem`]. `rating` follows the server's star, so a
/// hearted track arrives hearted instead of waiting for a local echo to catch up.
pub fn track_to_song(track: &Song, covers: &Covers) -> SongItem {
    song_row(track, covers, None)
}

/// A server [`Song`] as a `song` [`BrowseItem`] (a home/search card).
pub fn track_to_card(track: &Song, covers: &Covers) -> BrowseItem {
    BrowseItem {
        kind: "song",
        id: format!("{TRACK}{}", track.id),
        title: track.title.clone(),
        subtitle: Some(track.artist_line()).filter(|line| !line.is_empty()),
        thumbnail: cover(covers, track.cover_art.as_deref(), CARD_ART),
        duration: track.duration.map(fmt_duration),
        artist_runs: row_artist_runs(track),
        play_count: track.play_count.map(|count| count.to_string()),
        is_video: track.is_video.unwrap_or(false),
        is_upload: false,
        explicit: false,
    }
}

/// The one row builder, with an optional playlist position.
fn song_row(track: &Song, covers: &Covers, index: Option<usize>) -> SongItem {
    SongItem {
        video_id: format!("{TRACK}{}", track.id),
        title: track.title.clone(),
        artists: track.artist_line(),
        artist_id: track.artist_id.as_deref().map(|id| format!("{ARTIST}{id}")),
        artist_runs: row_artist_runs(track),
        album: track.album.clone(),
        album_id: track.album_id.as_deref().map(|id| format!("{ALBUM}{id}")),
        duration: track.duration.map(fmt_duration),
        play_count: track.play_count.map(|count| count.to_string()),
        thumbnail: cover(covers, track.cover_art.as_deref(), CARD_ART),
        set_video_id: index.map(|index| format!("{INDEX}{index}")),
        added_by: None,
        added_by_avatar: None,
        rating: track.is_starred().then_some(Rating::Like),
        queued_by: None,
        queued: false,
        queued_end: false,
        queued_from: None,
        autoplay: false,
        is_video: track.is_video.unwrap_or(false),
        is_upload: false,
        explicit: false,
    }
}

// --- cards -----------------------------------------------------------------------------------

pub fn album_to_card(album: &Album, covers: &Covers) -> BrowseItem {
    BrowseItem {
        kind: "album",
        id: format!("{ALBUM}{}", album.id),
        title: album.title().to_owned(),
        subtitle: Some(album_subtitle(album)).filter(|line| !line.is_empty()),
        thumbnail: cover(covers, album.cover_art.as_deref(), CARD_ART),
        duration: None,
        artist_runs: album_artist_runs(album),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

pub fn artist_to_card(artist: &Artist, covers: &Covers) -> BrowseItem {
    BrowseItem {
        kind: "artist",
        id: format!("{ARTIST}{}", artist.id),
        title: artist.name.clone(),
        subtitle: None,
        thumbnail: cover(covers, artist.cover_art.as_deref(), CARD_ART),
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

pub fn playlist_to_card(playlist: &Playlist, covers: &Covers) -> BrowseItem {
    BrowseItem {
        kind: "playlist",
        id: format!("{PLAYLIST}{}", playlist.id),
        title: playlist.name.clone(),
        subtitle: Some(track_count_line(playlist_count(playlist))),
        thumbnail: cover(covers, playlist.cover_art.as_deref(), CARD_ART),
        duration: None,
        artist_runs: Vec::new(),
        play_count: None,
        is_video: false,
        is_upload: false,
        explicit: false,
    }
}

// --- pages -----------------------------------------------------------------------------------

/// A server album with its songs as the album page. `in_library` is the server's star: on a
/// music server "saved to the library" and "starred" are the same fact.
pub fn album_page(album: &Album, covers: &Covers) -> AlbumPage {
    let count = album.song.len().max(album.song_count.unwrap_or(0) as usize) as u64;
    AlbumPage {
        title: Some(album.title().to_owned()),
        artist: Some(album_artist(album)).filter(|line| !line.is_empty()),
        artist_id: album.artist_id.as_deref().map(|id| format!("{ARTIST}{id}")),
        artist_runs: album_artist_runs(album),
        // The album response carries no artist image; the album cover heads the page instead.
        artist_thumbnail: None,
        subtitle: Some(album_line(album)),
        second_subtitle: Some(track_count_line(count)),
        description: None,
        thumbnail: cover(covers, album.cover_art.as_deref(), PAGE_ART),
        items: album.song.iter().map(|song| track_to_song(song, covers)).collect(),
        continuation: None,
        explicit: false,
        playlist_id: Some(format!("{ALBUM}{}", album.id)),
        in_library: album.is_starred(),
        sections: Vec::new(),
    }
}

/// A server playlist with its entries as the playlist page. Each row carries its position so a
/// removal addresses the right row even when the same song appears twice.
pub fn playlist_page(playlist: &Playlist, covers: &Covers) -> PlaylistPage {
    PlaylistPage {
        title: Some(playlist.name.clone()),
        subtitle: Some(track_count_line(playlist_count(playlist))),
        thumbnail: cover(covers, playlist.cover_art.as_deref(), PAGE_ART),
        description: playlist.comment.clone(),
        privacy: None,
        cover: None,
        items: playlist
            .entry
            .iter()
            .enumerate()
            .map(|(index, song)| song_row(song, covers, Some(index)))
            .collect(),
        continuation: None,
        // One server, one account: the playlist belongs to the person signed in.
        owned: true,
        collaborative: false,
        sort_menu: None,
    }
}

/// A server artist page: its albums as a carousel. The top-songs shelf stays empty because
/// `getTopSongs` needs a Last.fm key that many self-hosted servers do not have.
pub fn artist_page(artist: &Artist, covers: &Covers) -> innertube::ArtistPage {
    let sections = if artist.albums.is_empty() {
        Vec::new()
    } else {
        vec![ArtistCarousel {
            title: "Albums".to_string(),
            items: artist.albums.iter().map(|album| album_to_card(album, covers)).collect(),
            more_browse_id: None,
            more_params: None,
        }]
    };
    innertube::ArtistPage {
        name: Some(artist.name.clone()),
        thumbnail: cover(covers, artist.cover_art.as_deref(), PAGE_ART),
        description: None,
        subscribers: None,
        monthly_listeners: None,
        channel_id: format!("{ARTIST}{}", artist.id),
        subscribed: false,
        radio_playlist_id: None,
        top_songs: Vec::new(),
        top_songs_id: None,
        sections,
    }
}

// --- search ----------------------------------------------------------------------------------

/// A server search as the `search_all` payload. Navidrome names no top result, and `search3` has
/// no playlist category, so both lists stay empty.
pub fn search_all(results: &NdResults, covers: &Covers) -> SearchResults {
    SearchResults {
        top: Vec::new(),
        songs: results.songs.iter().map(|song| track_to_card(song, covers)).collect(),
        albums: results.albums.iter().map(|album| album_to_card(album, covers)).collect(),
        artists: results.artists.iter().map(|artist| artist_to_card(artist, covers)).collect(),
        playlists: Vec::new(),
        continuation: None,
    }
}

/// One `search_cards` category (`songs`|`albums`|`artists`|`playlists`) as a flat card list.
pub fn search_cards(results: &NdResults, category: &str, covers: &Covers) -> Vec<BrowseItem> {
    match category {
        "songs" => results.songs.iter().map(|song| track_to_card(song, covers)).collect(),
        "albums" => results.albums.iter().map(|album| album_to_card(album, covers)).collect(),
        "artists" => results.artists.iter().map(|artist| artist_to_card(artist, covers)).collect(),
        // Subsonic's search3 has no playlist result, so the category stays empty.
        _ => Vec::new(),
    }
}

/// The `search_page` items: the song hits as rows.
pub fn search_songs(results: &NdResults, covers: &Covers) -> Vec<SongItem> {
    results.songs.iter().map(|song| track_to_song(song, covers)).collect()
}

// --- home ------------------------------------------------------------------------------------

/// The server shelves, in the order Home shows them. Empty shelves are dropped rather than
/// rendered blank, and "Starred" falls back to songs when no album is starred.
pub fn home_sections(
    recent: &[Album],
    newest: &[Album],
    starred: &Starred,
    playlists: &[Playlist],
    random: &[Song],
    covers: &Covers,
) -> Vec<Section> {
    let mut sections = Vec::new();
    if !recent.is_empty() {
        sections.push(card_section(
            "Recently played".into(),
            recent.iter().map(|album| album_to_card(album, covers)).collect(),
        ));
    }
    if !newest.is_empty() {
        sections.push(card_section(
            "Recently added".into(),
            newest.iter().map(|album| album_to_card(album, covers)).collect(),
        ));
    }
    if !starred.albums.is_empty() {
        sections.push(card_section(
            "Starred".into(),
            starred.albums.iter().map(|album| album_to_card(album, covers)).collect(),
        ));
    } else if !starred.songs.is_empty() {
        sections.push(card_section(
            "Starred".into(),
            starred.songs.iter().map(|song| track_to_card(song, covers)).collect(),
        ));
    }
    if !playlists.is_empty() {
        sections.push(card_section("Playlists".into(), library_playlists(playlists, covers)));
    }
    if !random.is_empty() {
        sections.push(card_section(
            "Random picks".into(),
            random.iter().map(|song| track_to_card(song, covers)).collect(),
        ));
    }
    sections
}

/// The Library playlists tab.
pub fn library_playlists(playlists: &[Playlist], covers: &Covers) -> Vec<BrowseItem> {
    playlists.iter().map(|playlist| playlist_to_card(playlist, covers)).collect()
}

// --- lyrics ----------------------------------------------------------------------------------

/// Server lyrics as the panel's [`Lyrics`].
///
/// The entry is picked before this point: a synced one when the server has it. `offset` is applied
/// the way OpenSubsonic defines it — positive means the lyric should appear sooner — and times are
/// clamped at zero so a negative offset on the first line cannot wrap.
pub fn lyrics_to_lyrics(lyrics: &StructuredLyrics, source: &str) -> Lyrics {
    let offset = lyrics.offset.unwrap_or(0);
    let lines = lyrics
        .line
        .iter()
        .map(|line| LyricLine {
            time_ms: shifted(line.start, offset),
            end_time_ms: shifted(line.end, offset),
            text: line.value.clone(),
            words: None,
            translation: None,
            romanized: None,
        })
        .collect();
    Lyrics { source: source.to_owned(), synced: lyrics.synced, instrumental: false, lines }
}

// --- helpers ---------------------------------------------------------------------------------

fn card_section(title: String, items: Vec<BrowseItem>) -> Section {
    Section { title, title_is_artist: false, items, more_browse_id: None, more_params: None }
}

fn cover(covers: &Covers, cover_id: Option<&str>, px: u32) -> Option<String> {
    cover_id.and_then(|id| covers.url(id, px))
}

/// A single server artist id cannot describe a collaboration line, so the whole line links to the
/// one artist the server named. When it named none, the row keeps plain text and no runs.
fn row_artist_runs(song: &Song) -> Vec<ArtistRun> {
    artist_runs(&song.artist_line(), song.artist_id.as_deref())
}

fn album_artist_runs(album: &Album) -> Vec<ArtistRun> {
    artist_runs(&album_artist(album), album.artist_id.as_deref())
}

fn artist_runs(name: &str, id: Option<&str>) -> Vec<ArtistRun> {
    let name = name.trim();
    match id.filter(|_| !name.is_empty()) {
        Some(id) => vec![ArtistRun { text: name.to_owned(), id: Some(format!("{ARTIST}{id}")) }],
        None => Vec::new(),
    }
}

fn album_artist(album: &Album) -> String {
    album.artist.as_deref().map(str::trim).unwrap_or_default().to_owned()
}

/// The card's second line: the artist when the server named one, else the release line.
fn album_subtitle(album: &Album) -> String {
    let artist = album_artist(album);
    if artist.is_empty() {
        album_line(album)
    } else {
        artist
    }
}

/// "Album · 1975", or just "Album" when the server has no year.
fn album_line(album: &Album) -> String {
    match album.year.filter(|year| *year > 0) {
        Some(year) => format!("Album · {year}"),
        None => "Album".to_string(),
    }
}

fn playlist_count(playlist: &Playlist) -> u64 {
    playlist.entry.len().max(playlist.song_count.unwrap_or(0) as usize) as u64
}

fn track_count_line(n: u64) -> String {
    format!("{n} track{}", if n == 1 { "" } else { "s" })
}

/// Subsonic reports seconds; the client wants "m:ss", or "h:mm:ss" past an hour.
fn fmt_duration(seconds: u32) -> String {
    let (hours, minutes, seconds) = (seconds / 3600, (seconds % 3600) / 60, seconds % 60);
    if hours > 0 {
        format!("{hours}:{minutes:02}:{seconds:02}")
    } else {
        format!("{minutes}:{seconds:02}")
    }
}

/// Apply the OpenSubsonic offset and clamp the result to the start of the track.
fn shifted(time_ms: Option<i64>, offset: i64) -> Option<u64> {
    time_ms.map(|time| (time - offset).max(0) as u64)
}

#[cfg(test)]
mod tests {
    use super::*;
    use ryotunes_navidrome::{LyricsList, NavidromeAuth};

    fn covers() -> Covers {
        Covers::new(&NavidromeAuth {
            base_url: "https://music.example.com".into(),
            username: "admin".into(),
            salt: "abc".into(),
            token: "c42b93a1ac3b382e88071510296a96ce".into(),
        })
        .expect("the base url is valid")
    }

    fn song() -> Song {
        serde_json::from_str(
            r#"{"id":"so-1","title":"Bohemian Rhapsody","artist":"Queen",
                "albumArtist":"Queen","album":"A Night at the Opera","albumId":"al-1",
                "artistId":"ar-1","coverArt":"al-1","duration":355,"playCount":7,
                "starred":"2021-01-01T00:00:00Z"}"#,
        )
        .unwrap()
    }

    #[test]
    fn a_song_row_carries_prefixed_ids_and_the_servers_star() {
        let row = track_to_song(&song(), &covers());
        assert_eq!(row.video_id, "nd:track:so-1");
        assert_eq!(row.artist_id.as_deref(), Some("nd:artist:ar-1"));
        assert_eq!(row.album_id.as_deref(), Some("nd:album:al-1"));
        assert_eq!(row.artists, "Queen");
        assert_eq!(row.album.as_deref(), Some("A Night at the Opera"));
        assert_eq!(row.duration.as_deref(), Some("5:55"));
        assert_eq!(row.play_count.as_deref(), Some("7"));
        assert_eq!(row.rating, Some(Rating::Like));
        assert_eq!(row.artist_runs.len(), 1);
        assert_eq!(row.artist_runs[0].id.as_deref(), Some("nd:artist:ar-1"));
        assert!(row.thumbnail.as_deref().unwrap().contains("getCoverArt"));
        assert!(row.thumbnail.as_deref().unwrap().contains("size=320"));
        assert!(row.set_video_id.is_none(), "an album row has no playlist position");
    }

    #[test]
    fn a_track_with_no_album_cover_or_duration_degrades() {
        let bare: Song = serde_json::from_str(r#"{"id":"so-2","title":"Untitled"}"#).unwrap();
        let row = track_to_song(&bare, &covers());
        assert_eq!(row.video_id, "nd:track:so-2");
        assert_eq!(row.artists, "");
        assert_eq!(row.album, None);
        assert_eq!(row.album_id, None);
        assert_eq!(row.artist_id, None);
        assert_eq!(row.duration, None);
        assert_eq!(row.thumbnail, None);
        assert_eq!(row.rating, None, "an unstarred track is not a like");
        assert_eq!(row.play_count, None);
        assert!(row.artist_runs.is_empty());
    }

    #[test]
    fn a_collaboration_line_links_to_the_one_artist_the_server_named() {
        let mut song = song();
        song.artist = Some("Queen & David Bowie".into());
        let row = track_to_song(&song, &covers());
        assert_eq!(row.artists, "Queen & David Bowie");
        assert_eq!(row.artist_runs.len(), 1);
        assert_eq!(row.artist_runs[0].text, "Queen & David Bowie");
        assert_eq!(row.artist_runs[0].id.as_deref(), Some("nd:artist:ar-1"));
    }

    #[test]
    fn a_playlist_card_counts_tracks_and_prefixes_its_id() {
        let playlist: Playlist =
            serde_json::from_str(r#"{"id":"pl-1","name":"Mix","songCount":3}"#).unwrap();
        let card = playlist_to_card(&playlist, &covers());
        assert_eq!(card.id, "nd:playlist:pl-1");
        assert_eq!(card.title, "Mix");
        assert_eq!(card.subtitle.as_deref(), Some("3 tracks"));
    }

    #[test]
    fn album_cards_show_the_artist_and_fall_back_to_the_release_line() {
        let with_artist: Album = serde_json::from_str(
            r#"{"id":"al-1","title":"A Night at the Opera","artist":"Queen","year":1975}"#,
        )
        .unwrap();
        let card = album_to_card(&with_artist, &covers());
        assert_eq!(card.id, "nd:album:al-1");
        assert_eq!(card.title, "A Night at the Opera");
        assert_eq!(card.subtitle.as_deref(), Some("Queen"));

        let undated: Album = serde_json::from_str(r#"{"id":"al-2","title":"Jazz"}"#).unwrap();
        assert_eq!(album_to_card(&undated, &covers()).subtitle.as_deref(), Some("Album"));
        let dated: Album =
            serde_json::from_str(r#"{"id":"al-3","name":"Rumours","year":1977}"#).unwrap();
        assert_eq!(album_to_card(&dated, &covers()).subtitle.as_deref(), Some("Album · 1977"));
    }

    #[test]
    fn the_album_page_carries_the_servers_star_and_its_songs() {
        let album: Album = serde_json::from_str(
            r#"{"id":"al-1","name":"A Night at the Opera","artist":"Queen","artistId":"ar-1",
                "coverArt":"al-1","year":1975,"songCount":2,"starred":"2021-01-01T00:00:00Z",
                "song":[{"id":"so-1","title":"Death on Two Legs","track":1},
                        {"id":"so-2","title":"Lazing on a Sunday Afternoon","track":2}]}"#,
        )
        .unwrap();
        let page = album_page(&album, &covers());
        assert_eq!(page.title.as_deref(), Some("A Night at the Opera"));
        assert_eq!(page.artist.as_deref(), Some("Queen"));
        assert_eq!(page.artist_id.as_deref(), Some("nd:artist:ar-1"));
        assert_eq!(page.subtitle.as_deref(), Some("Album · 1975"));
        assert_eq!(page.second_subtitle.as_deref(), Some("2 tracks"));
        assert_eq!(page.playlist_id.as_deref(), Some("nd:album:al-1"));
        assert!(page.in_library, "the server star is the library save");
        assert_eq!(page.items.len(), 2);
        assert_eq!(page.items[0].video_id, "nd:track:so-1");
        assert!(page.continuation.is_none());
    }

    #[test]
    fn a_playlist_page_indexes_duplicate_songs_apart() {
        let playlist: Playlist = serde_json::from_str(
            r#"{"id":"pl-1","name":"Mix","songCount":3,
                "entry":[{"id":"so-1","title":"a"},{"id":"so-2","title":"b"},{"id":"so-1","title":"a"}]}"#,
        )
        .unwrap();
        let page = playlist_page(&playlist, &covers());
        assert!(page.owned);
        assert!(page.continuation.is_none());
        assert_eq!(page.items.len(), 3);
        assert_eq!(page.items[0].set_video_id.as_deref(), Some("nd:idx:0"));
        assert_eq!(page.items[1].set_video_id.as_deref(), Some("nd:idx:1"));
        assert_eq!(page.items[2].set_video_id.as_deref(), Some("nd:idx:2"));
        assert_eq!(
            page.items[0].video_id, page.items[2].video_id,
            "the same song appears twice under one id"
        );
        assert_ne!(
            page.items[0].set_video_id, page.items[2].set_video_id,
            "the positions are what separate them"
        );
        assert_eq!(page.subtitle.as_deref(), Some("3 tracks"));
    }

    #[test]
    fn an_artist_page_shows_albums_and_no_top_songs() {
        let artist: Artist = serde_json::from_str(
            r#"{"id":"ar-1","name":"Queen","coverArt":"ar-1","albumCount":1,
                "album":[{"id":"al-1","name":"A Night at the Opera","artist":"Queen"}]}"#,
        )
        .unwrap();
        let page = artist_page(&artist, &covers());
        assert_eq!(page.name.as_deref(), Some("Queen"));
        assert_eq!(page.channel_id, "nd:artist:ar-1");
        assert!(page.top_songs.is_empty());
        assert_eq!(page.sections.len(), 1);
        assert_eq!(page.sections[0].title, "Albums");
        assert_eq!(page.sections[0].items[0].id, "nd:album:al-1");
        assert!(page.thumbnail.as_deref().unwrap().contains("ar-1"));
    }

    #[test]
    fn an_artist_without_albums_has_no_carousel() {
        let artist: Artist = serde_json::from_str(r#"{"id":"ar-2","name":"New"}"#).unwrap();
        assert!(artist_page(&artist, &covers()).sections.is_empty());
    }

    #[test]
    fn search_splits_into_three_card_lists_and_no_top_result() {
        let results: NdResults = serde_json::from_str(
            r#"{"song":[{"id":"so-1","title":"t"}],
                "album":[{"id":"al-1","title":"a"}],
                "artist":[{"id":"ar-1","name":"r"}]}"#,
        )
        .unwrap();
        let all = search_all(&results, &covers());
        assert!(all.top.is_empty(), "navidrome names no top result");
        assert_eq!(all.songs[0].id, "nd:track:so-1");
        assert_eq!(all.albums[0].id, "nd:album:al-1");
        assert_eq!(all.artists[0].id, "nd:artist:ar-1");
        assert!(all.playlists.is_empty(), "search3 has no playlist category");
        assert!(all.continuation.is_none());

        assert_eq!(search_cards(&results, "songs", &covers())[0].id, "nd:track:so-1");
        assert!(search_cards(&results, "playlists", &covers()).is_empty());
        assert!(search_cards(&results, "nonsense", &covers()).is_empty());
        assert_eq!(search_songs(&results, &covers())[0].video_id, "nd:track:so-1");
    }

    #[test]
    fn home_shelves_keep_their_order_and_drop_empty_rows() {
        let recent: Vec<Album> =
            serde_json::from_str(r#"[{"id":"al-1","title":"Recent"}]"#).unwrap();
        let playlists: Vec<Playlist> =
            serde_json::from_str(r#"[{"id":"pl-1","name":"Mix"}]"#).unwrap();
        let songs: Vec<Song> = serde_json::from_str(r#"[{"id":"so-1","title":"Pick"}]"#).unwrap();
        let starred: Starred = serde_json::from_str(
            r#"{"album":[{"id":"al-2","title":"Favourite"}],
                "song":[{"id":"so-2","title":"Also favourite"}]}"#,
        )
        .unwrap();

        let sections = home_sections(&recent, &[], &starred, &playlists, &songs, &covers());
        let titles: Vec<_> = sections.iter().map(|s| s.title.as_str()).collect();
        assert_eq!(titles, ["Recently played", "Starred", "Playlists", "Random picks"]);
        assert_eq!(sections[1].items.len(), 1);
        assert_eq!(sections[1].items[0].id, "nd:album:al-2", "albums win over songs");

        let song_starred: Starred =
            serde_json::from_str(r#"{"album":[],"song":[{"id":"so-2","title":"Also favourite"}]}"#)
                .unwrap();
        let sections = home_sections(&[], &[], &song_starred, &[], &[], &covers());
        assert_eq!(sections.len(), 1);
        assert_eq!(sections[0].items[0].id, "nd:track:so-2", "the fallback keeps the shelf");

        assert!(home_sections(&[], &[], &Starred::default(), &[], &[], &covers()).is_empty());
    }

    #[test]
    fn the_library_playlist_tab_is_cards() {
        let playlists: Vec<Playlist> =
            serde_json::from_str(r#"[{"id":"pl-1","name":"Mix"},{"id":"pl-2","name":"Other"}]"#)
                .unwrap();
        let cards = library_playlists(&playlists, &covers());
        assert_eq!(cards.len(), 2);
        assert_eq!(cards[1].id, "nd:playlist:pl-2");
    }

    #[test]
    fn synced_lyrics_keep_their_timing_and_apply_the_servers_offset() {
        let lyrics: StructuredLyrics = serde_json::from_str(
            r#"{"lang":"eng","synced":true,"offset":100,"line":[
                {"start":0,"value":"first"},
                {"start":2000,"end":3000,"value":"second"}]}"#,
        )
        .unwrap();
        let mapped = lyrics_to_lyrics(&lyrics, "Navidrome");
        assert_eq!(mapped.source, "Navidrome");
        assert!(mapped.synced);
        assert!(!mapped.instrumental);
        assert_eq!(mapped.lines.len(), 2);
        assert_eq!(mapped.lines[0].time_ms, Some(0), "a negative result clamps to the start");
        assert_eq!(mapped.lines[1].time_ms, Some(1900), "positive offset means sooner");
        assert_eq!(mapped.lines[1].end_time_ms, Some(2900));
        assert_eq!(mapped.lines[1].text, "second");
        assert!(mapped.lines[1].words.is_none());
    }

    #[test]
    fn unsynced_lyrics_carry_text_and_no_times() {
        let lyrics: StructuredLyrics =
            serde_json::from_str(r#"{"synced":false,"line":[{"value":"a"},{"value":"b"}]}"#)
                .unwrap();
        let mapped = lyrics_to_lyrics(&lyrics, "Navidrome");
        assert!(!mapped.synced);
        assert_eq!(mapped.lines.len(), 2);
        assert_eq!(mapped.lines[0].time_ms, None);
        assert_eq!(mapped.lines[1].end_time_ms, None);
        assert_eq!(mapped.lines[1].text, "b");
    }

    #[test]
    fn a_lyrics_list_picks_the_synced_entry_before_mapping() {
        let list: LyricsList = serde_json::from_str(
            r#"{"structuredLyrics":[
                {"synced":false,"line":[{"value":"plain"}]},
                {"synced":true,"line":[{"start":500,"value":"timed"}]}]}"#,
        )
        .unwrap();
        let mapped = lyrics_to_lyrics(list.best().unwrap(), "Navidrome");
        assert!(mapped.synced);
        assert_eq!(mapped.lines[0].time_ms, Some(500));
        assert_eq!(mapped.lines[0].text, "timed");
    }

    #[test]
    fn durations_grow_an_hour_field_only_when_needed() {
        assert_eq!(fmt_duration(59), "0:59");
        assert_eq!(fmt_duration(185), "3:05");
        assert_eq!(fmt_duration(3600), "1:00:00");
        assert_eq!(fmt_duration(3725), "1:02:05");
    }

    #[test]
    fn one_track_reads_singular() {
        assert_eq!(track_count_line(1), "1 track");
        assert_eq!(track_count_line(0), "0 tracks");
    }
}
