//! Catalogue types.
//!
//! Navidrome omits empty lists, blank strings and whole objects on a whim, and its ids are opaque
//! strings rather than numbers, so everything but the id is optional here and a missing field
//! degrades a label instead of failing a page. The JSON is camelCase throughout; the fields are
//! snake_case, and the renames live on the container.
//!
//! Two payloads describe an album: `getAlbumList2` is a list of `Child` items keyed `title` (with
//! the album's name repeated under `album`), while `getAlbum`, `search3` and `getArtist` use an
//! `AlbumID3` keyed `name`. [`Album::name`] accepts either spelling and [`Album::title`] settles
//! the order, so one struct serves every endpoint instead of a second type that could drift.
//!
//! Ids stay bare here. The `nd:` prefixes the rest of Ryotunes uses are the bridge's business.

use serde::Deserialize;

/// A track. `getAlbum`, `search3`, `getPlaylist` and the scrobble calls all use this shape.
#[derive(Clone, Debug, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Song {
    pub id: String,
    /// The directory the file lives in. Not shown, but it is what a folder browse would key on.
    #[serde(default)]
    pub parent: Option<String>,
    #[serde(default)]
    pub title: String,
    /// The album title as a label. The album itself is [`Song::album_id`].
    #[serde(default)]
    pub album: Option<String>,
    #[serde(default)]
    pub artist: Option<String>,
    #[serde(default)]
    pub album_artist: Option<String>,
    #[serde(default)]
    pub track: Option<u32>,
    #[serde(default)]
    pub disc_number: Option<u32>,
    #[serde(default)]
    pub year: Option<u32>,
    #[serde(default)]
    pub genre: Option<String>,
    /// The id cover art is requested by, which is not always this song's id.
    #[serde(default)]
    pub cover_art: Option<String>,
    /// Seconds, as Subsonic reports it.
    #[serde(default)]
    pub duration: Option<u32>,
    #[serde(default)]
    pub play_count: Option<u64>,
    /// The date the user starred this song, or `None` when they have not.
    #[serde(default)]
    pub starred: Option<String>,
    #[serde(default)]
    pub album_id: Option<String>,
    #[serde(default)]
    pub artist_id: Option<String>,
    #[serde(default)]
    pub user_rating: Option<u32>,
    #[serde(default)]
    pub is_video: Option<bool>,
    #[serde(default)]
    pub suffix: Option<String>,
    #[serde(default)]
    pub content_type: Option<String>,
    #[serde(default)]
    pub size: Option<u64>,
    #[serde(default)]
    pub created: Option<String>,
}

impl Song {
    /// Whether the song is in the user's starred list. The date is the flag: Subsonic sends the
    /// field only when the answer is yes.
    pub fn is_starred(&self) -> bool {
        self.starred.as_deref().is_some_and(|date| !date.trim().is_empty())
    }

    /// The artist line a row or a player bar shows, falling back to the album artist the way the
    /// other providers do.
    pub fn artist_line(&self) -> String {
        for name in [&self.artist, &self.album_artist] {
            if let Some(name) = name.as_deref().map(str::trim).filter(|name| !name.is_empty()) {
                return name.to_owned();
            }
        }
        String::new()
    }
}

/// An album, from either payload shape. See the module note about `name` and `title`.
#[derive(Clone, Debug, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Album {
    pub id: String,
    /// The album title. `AlbumID3` spells it `name`, a `Child` spells it `title`; both land here.
    #[serde(default, alias = "title")]
    pub name: String,
    /// The `album` key a `Child` also carries, normally the same string as `title`. Kept apart
    /// from `name` rather than aliased, because real list items send both and one field cannot
    /// accept two sources without failing on the duplicate.
    #[serde(default)]
    pub album: Option<String>,
    #[serde(default)]
    pub artist: Option<String>,
    #[serde(default)]
    pub artist_id: Option<String>,
    #[serde(default)]
    pub cover_art: Option<String>,
    #[serde(default)]
    pub song_count: Option<u32>,
    /// Seconds, for the whole album.
    #[serde(default)]
    pub duration: Option<u32>,
    #[serde(default)]
    pub year: Option<u32>,
    #[serde(default)]
    pub genre: Option<String>,
    #[serde(default)]
    pub play_count: Option<u64>,
    #[serde(default)]
    pub starred: Option<String>,
    #[serde(default)]
    pub created: Option<String>,
    /// Populated by `getAlbum`, empty in a list.
    #[serde(default)]
    pub song: Vec<Song>,
}

impl Album {
    /// The title to show: the `name`/`title` field when the server sent one, else the `album` key
    /// a folder listing carries instead.
    pub fn title(&self) -> &str {
        if self.name.trim().is_empty() {
            self.album.as_deref().unwrap_or("")
        } else {
            &self.name
        }
    }

    pub fn is_starred(&self) -> bool {
        self.starred.as_deref().is_some_and(|date| !date.trim().is_empty())
    }
}

/// An artist. `getArtists` and `search3` carry the same shape; `getArtist` adds its albums.
#[derive(Clone, Debug, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Artist {
    pub id: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub cover_art: Option<String>,
    #[serde(default)]
    pub album_count: Option<u32>,
    #[serde(default)]
    pub starred: Option<String>,
    #[serde(default)]
    pub user_rating: Option<u32>,
    /// Populated by `getArtist`, empty in an index.
    #[serde(default, alias = "album")]
    pub albums: Vec<Album>,
}

impl Artist {
    pub fn is_starred(&self) -> bool {
        self.starred.as_deref().is_some_and(|date| !date.trim().is_empty())
    }
}

/// One lettered block of `getArtists`. Navidrome letters them `A`–`Z` and `#`, and may send an
/// empty block for a letter nothing starts with.
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize)]
pub struct ArtistIndex {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub artist: Vec<Artist>,
}

/// The payload of `getArtists`, flattened into one ordered list by [`Artists::flatten`].
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Artists {
    /// Articles the server ignores when it files an artist, so the client does not sort again.
    #[serde(default)]
    pub ignored_articles: Option<String>,
    #[serde(default)]
    pub index: Vec<ArtistIndex>,
}

impl Artists {
    /// Every artist, in the order the server filed them. Cloned rather than borrowed because the
    /// daemon hands the list to a page that outlives the payload.
    pub fn flatten(&self) -> Vec<Artist> {
        self.index.iter().flat_map(|index| index.artist.iter().cloned()).collect()
    }

    pub fn is_empty(&self) -> bool {
        self.index.iter().all(|index| index.artist.is_empty())
    }
}

/// A playlist. `getPlaylists` leaves `entry` empty; `getPlaylist` fills it.
#[derive(Clone, Debug, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Playlist {
    pub id: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub comment: Option<String>,
    #[serde(default)]
    pub owner: Option<String>,
    #[serde(default)]
    pub public: Option<bool>,
    #[serde(default)]
    pub song_count: Option<u32>,
    /// Seconds, for the whole playlist.
    #[serde(default)]
    pub duration: Option<u32>,
    #[serde(default)]
    pub cover_art: Option<String>,
    #[serde(default)]
    pub created: Option<String>,
    #[serde(default)]
    pub changed: Option<String>,
    #[serde(default)]
    pub entry: Vec<Song>,
}

impl Playlist {
    /// The entries' server ids in play order. Duplicates are kept: the same song twice is two
    /// rows, and a reorder has to write both.
    pub fn song_ids(&self) -> Vec<&str> {
        self.entry.iter().map(|song| song.id.as_str()).collect()
    }

    /// The server index of the last entry, for a "remove the track I am looking at" call.
    pub fn last_index(&self) -> Option<u32> {
        self.entry.len().checked_sub(1).map(|index| index as u32)
    }
}

/// The payload of `search3`, and the shape `getStarred2` uses as well.
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SearchResults {
    #[serde(default, alias = "song")]
    pub songs: Vec<Song>,
    #[serde(default, alias = "album")]
    pub albums: Vec<Album>,
    #[serde(default, alias = "artist")]
    pub artists: Vec<Artist>,
}

impl SearchResults {
    pub fn is_empty(&self) -> bool {
        self.songs.is_empty() && self.albums.is_empty() && self.artists.is_empty()
    }
}

/// The payload of `getStarred2`. The same three lists as a search, with no query behind them.
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Starred {
    #[serde(default, alias = "song")]
    pub songs: Vec<Song>,
    #[serde(default, alias = "album")]
    pub albums: Vec<Album>,
    #[serde(default, alias = "artist")]
    pub artists: Vec<Artist>,
}

impl Starred {
    pub fn is_empty(&self) -> bool {
        self.songs.is_empty() && self.albums.is_empty() && self.artists.is_empty()
    }
}

/// The account behind `getUser`. Only the identity the client acts on is kept; the rest of the
/// response is personal data this client has no business holding.
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct User {
    #[serde(default)]
    pub username: String,
    #[serde(default)]
    pub admin_role: bool,
    #[serde(default)]
    pub playlist_role: bool,
    #[serde(default)]
    pub scrobbling_enabled: bool,
}

/// One OpenSubsonic extension and the versions the server implements.
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize)]
pub struct Extension {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub versions: Vec<u32>,
}

/// One timed line of a structured lyric.
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize)]
pub struct LyricLine {
    /// Milliseconds from the start of the track. Absent on an unsynced lyric.
    #[serde(default)]
    pub start: Option<i64>,
    /// Where the line stops being highlighted, when the server knows.
    #[serde(default)]
    pub end: Option<i64>,
    #[serde(default)]
    pub value: String,
}

/// The payload of `getLyricsBySongId`, one entry per language or version the server holds.
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct LyricsList {
    #[serde(default)]
    pub structured_lyrics: Vec<StructuredLyrics>,
}

impl LyricsList {
    /// The entry a panel should show: the first synced one, since a synced lyric also renders when
    /// the player does not highlight it. Falls back to the first entry, which is what an unsynced
    /// server sends.
    pub fn best(&self) -> Option<&StructuredLyrics> {
        self.structured_lyrics
            .iter()
            .find(|lyrics| lyrics.synced && !lyrics.line.is_empty())
            .or_else(|| self.structured_lyrics.first())
    }
}

/// One lyric in one language.
#[derive(Clone, Debug, Default, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StructuredLyrics {
    #[serde(default)]
    pub display_artist: Option<String>,
    #[serde(default)]
    pub display_title: Option<String>,
    #[serde(default)]
    pub lang: Option<String>,
    /// Whether [`LyricLine::start`] is meaningful. A server that only has plain text says no.
    #[serde(default)]
    pub synced: bool,
    /// Milliseconds the whole lyric is shifted by, when the server says so.
    #[serde(default)]
    pub offset: Option<i64>,
    #[serde(default)]
    pub line: Vec<LyricLine>,
}

impl StructuredLyrics {
    /// The lyric as plain lines, for a panel that shows text before it follows the playhead.
    pub fn plain_lines(&self) -> Vec<&str> {
        self.line.iter().map(|line| line.value.as_str()).collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The example payloads are the ones the OpenSubsonic documentation publishes, wrapped in the
    /// Subsonic envelope the server actually sends. Parsing the payload out of the envelope here
    /// keeps the fixture honest without pulling the client funnel into Task 1.
    fn payload(raw: &str) -> serde_json::Value {
        let envelope: serde_json::Value = serde_json::from_str(raw).expect("the fixture is json");
        envelope["subsonic-response"].clone()
    }

    const GET_ARTISTS: &str = r#"{
        "subsonic-response": {
            "status": "ok",
            "version": "1.16.1",
            "artists": {
                "ignoredArticles": "The El La Los Las Le Les",
                "index": [
                    {"name": "A", "artist": [
                        {"id": "ar-1", "name": "ABBA", "coverArt": "ar-1", "albumCount": 18,
                         "starred": "2012-04-02T20:00:00.000Z", "userRating": 5}
                    ]},
                    {"name": "B", "artist": []},
                    {"name": "Q", "artist": [
                        {"id": "ar-2", "name": "Queen", "albumCount": 15}
                    ]}
                ]
            }
        }
    }"#;

    const GET_ALBUM_LIST_2: &str = r#"{
        "subsonic-response": {
            "status": "ok",
            "version": "1.16.1",
            "albumList2": {"album": [
                {"id": "al-1", "parent": "ar-2", "title": "A Night at the Opera",
                 "album": "A Night at the Opera",
                 "isDir": true, "coverArt": "al-1", "artist": "Queen", "year": 1975,
                 "genre": "Rock", "songCount": 12, "duration": 2587,
                 "created": "2021-02-23T04:34:22.19Z", "starred": "2021-03-01T10:00:00Z"},
                {"id": "al-2", "title": "Jazz", "artist": "Queen", "songCount": 13}
            ]}
        }
    }"#;

    const SEARCH_3: &str = r#"{
        "subsonic-response": {
            "status": "ok",
            "version": "1.16.1",
            "searchResult3": {
                "artist": [{"id": "ar-2", "name": "Queen", "albumCount": 15}],
                "album": [{"id": "al-1", "name": "A Night at the Opera", "artist": "Queen",
                           "songCount": 12}],
                "song": [{"id": "so-1", "parent": "al-1", "title": "Bohemian Rhapsody",
                          "album": "A Night at the Opera", "artist": "Queen", "track": 11,
                          "duration": 355, "coverArt": "al-1", "albumId": "al-1",
                          "artistId": "ar-2", "playCount": 7, "userRating": 5,
                          "starred": "2021-01-01T00:00:00Z", "suffix": "flac"}]
            }
        }
    }"#;

    const GET_ALBUM: &str = r#"{
        "subsonic-response": {
            "status": "ok",
            "version": "1.16.1",
            "album": {
                "id": "al-1", "name": "A Night at the Opera", "artist": "Queen",
                "artistId": "ar-2", "coverArt": "al-1", "songCount": 2, "duration": 700,
                "created": "2021-02-23T04:34:22.19Z",
                "song": [
                    {"id": "so-1", "title": "Death on Two Legs", "track": 1, "discNumber": 1,
                     "duration": 223, "albumId": "al-1", "artistId": "ar-2"},
                    {"id": "so-2", "title": "Lazing on a Sunday Afternoon", "track": 2,
                     "duration": 65, "albumId": "al-1", "starred": "2021-01-01T00:00:00Z"}
                ]
            }
        }
    }"#;

    const LYRICS: &str = r#"{
        "subsonic-response": {
            "status": "ok",
            "version": "1.16.1",
            "lyricsList": {"structuredLyrics": [
                {"displayArtist": "Queen", "displayTitle": "Bohemian Rhapsody", "lang": "eng",
                 "synced": false, "line": [{"value": "Is this the real life?"}]},
                {"displayArtist": "Queen", "displayTitle": "Bohemian Rhapsody", "lang": "eng",
                 "synced": true, "offset": 0, "line": [
                    {"start": 0, "value": "Is this the real life?"},
                    {"start": 7000, "end": 9000, "value": "Is this just fantasy?"}
                 ]}
            ]}
        }
    }"#;

    #[test]
    fn the_artists_index_flattens_in_the_servers_order() {
        let artists: Artists =
            serde_json::from_value(payload(GET_ARTISTS)["artists"].clone()).unwrap();
        let flat = artists.flatten();
        assert_eq!(flat.len(), 2, "the empty letter block contributes nothing");
        assert_eq!(flat[0].name, "ABBA");
        assert_eq!(flat[1].name, "Queen");
        assert!(flat[0].is_starred());
        assert!(!flat[1].is_starred());
        assert_eq!(flat[0].album_count, Some(18));
        assert_eq!(artists.ignored_articles.as_deref(), Some("The El La Los Las Le Les"));
        assert!(!artists.is_empty());
    }

    #[test]
    fn an_album_list_item_parses_and_keeps_its_title() {
        let albums: Vec<Album> =
            serde_json::from_value(payload(GET_ALBUM_LIST_2)["albumList2"]["album"].clone())
                .unwrap();
        assert_eq!(albums.len(), 2);
        assert_eq!(albums[0].name, "A Night at the Opera");
        assert_eq!(albums[0].album.as_deref(), Some("A Night at the Opera"));
        assert_eq!(albums[0].title(), "A Night at the Opera");
        assert_eq!(albums[0].artist.as_deref(), Some("Queen"));
        assert_eq!(albums[0].song_count, Some(12));
        assert!(albums[0].is_starred());
        assert!(albums[0].song.is_empty(), "a list item carries no songs");
        assert!(!albums[1].is_starred());
    }

    #[test]
    fn one_album_struct_reads_both_payload_shapes() {
        let from_name: Album = serde_json::from_value(payload(GET_ALBUM)["album"].clone()).unwrap();
        assert_eq!(from_name.name, "A Night at the Opera");
        assert_eq!(from_name.song.len(), 2);
        assert_eq!(from_name.song[0].disc_number, Some(1));
        assert!(from_name.song[1].is_starred());

        let titled: Album =
            serde_json::from_str(r#"{"id":"al-9","title":"Only a title"}"#).unwrap();
        assert_eq!(titled.title(), "Only a title");
        let legacy: Album =
            serde_json::from_str(r#"{"id":"al-9","album":"Only an album key"}"#).unwrap();
        assert_eq!(legacy.name, "", "a folder listing has no name field");
        assert_eq!(legacy.title(), "Only an album key");
    }

    #[test]
    fn search3_parses_all_three_categories() {
        let results: SearchResults =
            serde_json::from_value(payload(SEARCH_3)["searchResult3"].clone()).unwrap();
        assert_eq!(results.songs.len(), 1);
        assert_eq!(results.albums.len(), 1);
        assert_eq!(results.artists.len(), 1);
        let song = &results.songs[0];
        assert_eq!(song.title, "Bohemian Rhapsody");
        assert_eq!(song.album_id.as_deref(), Some("al-1"));
        assert_eq!(song.artist_id.as_deref(), Some("ar-2"));
        assert_eq!(song.cover_art.as_deref(), Some("al-1"));
        assert_eq!(song.play_count, Some(7));
        assert_eq!(song.user_rating, Some(5));
        assert_eq!(song.duration, Some(355));
        assert_eq!(song.suffix.as_deref(), Some("flac"));
        assert!(song.is_starred());
    }

    #[test]
    fn starred2_uses_the_same_three_lists() {
        let starred: Starred = serde_json::from_str(
            r#"{"song":[{"id":"so-1","title":"One"}],
                "album":[{"id":"al-1","name":"An album"}],
                "artist":[{"id":"ar-1","name":"An artist"}]}"#,
        )
        .unwrap();
        assert_eq!(starred.songs.len(), 1);
        assert_eq!(starred.albums[0].name, "An album");
        assert_eq!(starred.artists[0].name, "An artist");
        assert!(!starred.is_empty());
        assert!(Starred::default().is_empty());
    }

    #[test]
    fn a_song_with_only_an_id_parses() {
        let song: Song = serde_json::from_str(r#"{"id":"so-1"}"#).unwrap();
        assert_eq!(song.id, "so-1");
        assert_eq!(song.title, "");
        assert_eq!(song.artist_line(), "");
        assert!(!song.is_starred());
        assert_eq!(song.duration, None);
    }

    #[test]
    fn the_artist_line_falls_back_to_the_album_artist() {
        let mut song: Song =
            serde_json::from_str(r#"{"id":"so-1","albumArtist":"Various Artists"}"#).unwrap();
        assert_eq!(song.artist_line(), "Various Artists");
        song.artist = Some("  ".into());
        assert_eq!(song.artist_line(), "Various Artists", "a blank artist is not a name");
        song.artist = Some("Queen".into());
        assert_eq!(song.artist_line(), "Queen");
    }

    #[test]
    fn an_unknown_field_does_not_break_a_payload() {
        let song: Song =
            serde_json::from_str(r#"{"id":"so-1","title":"x","somethingNew":true}"#).unwrap();
        assert_eq!(song.title, "x");
    }

    #[test]
    fn structured_lyrics_keep_their_timing() {
        let list: LyricsList =
            serde_json::from_value(payload(LYRICS)["lyricsList"].clone()).unwrap();
        let best = list.best().expect("there is a lyric");
        assert!(best.synced, "a synced entry wins over the plain one");
        assert_eq!(best.line.len(), 2);
        assert_eq!(best.line[0].start, Some(0));
        assert_eq!(best.line[0].end, None);
        assert_eq!(best.line[1].start, Some(7000));
        assert_eq!(best.line[1].end, Some(9000));
        assert_eq!(best.plain_lines(), ["Is this the real life?", "Is this just fantasy?"]);
        assert_eq!(best.lang.as_deref(), Some("eng"));
    }

    #[test]
    fn the_first_entry_is_used_when_nothing_is_synced() {
        let list: LyricsList = serde_json::from_str(
            r#"{"structuredLyrics":[{"synced":false,"line":[{"value":"a"}]},
                                    {"synced":false,"line":[{"value":"b"}]}]}"#,
        )
        .unwrap();
        assert_eq!(list.best().unwrap().plain_lines(), ["a"]);
        assert!(LyricsList::default().best().is_none());
    }

    #[test]
    fn a_playlists_entries_expose_their_ids_and_indexes_in_order() {
        let playlist: Playlist = serde_json::from_str(
            r#"{"id":"pl-1","name":"Mix","songCount":3,
                "entry":[{"id":"so-1"},{"id":"so-2"},{"id":"so-1"}]}"#,
        )
        .unwrap();
        assert_eq!(playlist.song_ids(), ["so-1", "so-2", "so-1"]);
        assert_eq!(playlist.last_index(), Some(2));
        let empty: Playlist = serde_json::from_str(r#"{"id":"pl-2"}"#).unwrap();
        assert_eq!(empty.last_index(), None);
    }

    #[test]
    fn an_extension_parses_with_or_without_versions() {
        let ext: Extension =
            serde_json::from_str(r#"{"name":"songLyrics","versions":[1]}"#).unwrap();
        assert_eq!(ext.name, "songLyrics");
        assert_eq!(ext.versions, [1]);
        let bare: Extension = serde_json::from_str(r#"{"name":"formPost"}"#).unwrap();
        assert!(bare.versions.is_empty());
    }
}
