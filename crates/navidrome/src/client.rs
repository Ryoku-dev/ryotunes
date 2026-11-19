//! One request funnel, the sign-in handshake, and the catalogue calls.
//!
//! Every call is a `GET {base}/rest/{method}.view` answered with the
//! `{"subsonic-response": {…}}` envelope. The funnel checks `status`, maps an `error` object once
//! through [`Error::from_api`], and only then deserialises the payload the caller named. A call
//! that needs a session and has none returns [`Error::SignedOut`] without touching the network.
//!
//! Nothing here polls, caches or retries: the daemon asks for a page when the user opens one, and a
//! failure is surfaced rather than papered over. The two exceptions are deliberate — a sign-in
//! tolerates a server that does not know the OpenSubsonic extension call, and a lyrics request
//! treats "this server has no lyrics" as an empty answer at the call site.

use std::time::Duration;

use serde::de::DeserializeOwned;
use serde::Deserialize;
use serde_json::{Map, Value};

use crate::auth::{normalise_server, Capabilities, NavidromeAuth};
use crate::models::{
    Album, Artist, Artists, Extension, LyricsList, Playlist, SearchResults, Song, Starred,
    StructuredLyrics, User,
};
use crate::stream::Covers;
use crate::{Error, Result};

const USER_AGENT: &str = concat!("Ryotunes/", env!("CARGO_PKG_VERSION"));
/// Catalogue calls are small JSON documents. A hung LAN server would otherwise hold a daemon
/// method open forever; a slow scan is not a slow API.
const REQUEST_TIMEOUT: Duration = Duration::from_secs(30);
const CONNECT_TIMEOUT: Duration = Duration::from_secs(10);
/// The server's own maximum for a list call. Asking for more is a protocol error, not a bigger
/// page.
const PAGE_LIMIT: u32 = 500;

/// Which `getAlbumList2` list a caller wants. The server's spellings are the wire format; this
/// enum keeps a typo out of a query string.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum AlbumList {
    Newest,
    Recent,
    Frequent,
    Random,
    AlphabeticalByName,
    AlphabeticalByArtist,
    Starred,
    ByYear,
    ByGenre,
}

impl AlbumList {
    pub fn as_str(self) -> &'static str {
        match self {
            AlbumList::Newest => "newest",
            AlbumList::Recent => "recent",
            AlbumList::Frequent => "frequent",
            AlbumList::Random => "random",
            AlbumList::AlphabeticalByName => "alphabeticalByName",
            AlbumList::AlphabeticalByArtist => "alphabeticalByArtist",
            AlbumList::Starred => "starred",
            AlbumList::ByYear => "byYear",
            AlbumList::ByGenre => "byGenre",
        }
    }
}

/// What a star is attached to. Subsonic spells the parameter differently for each, and uses `id`
/// for a song rather than `songId`, which is exactly the kind of detail to name once.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum StarKind {
    Song,
    Album,
    Artist,
}

impl StarKind {
    fn param(self) -> &'static str {
        match self {
            StarKind::Song => "id",
            StarKind::Album => "albumId",
            StarKind::Artist => "artistId",
        }
    }
}

/// The provider. Holds the HTTP client and, in memory only, the session.
pub struct Navidrome {
    http: reqwest::Client,
    auth: parking_lot::RwLock<Option<NavidromeAuth>>,
    /// `None` until a sign-in negotiates extensions. A restored blob knows nothing about them, so
    /// an empty list is not evidence that the server lacks a feature.
    capabilities: parking_lot::RwLock<Option<Capabilities>>,
}

impl Default for Navidrome {
    fn default() -> Self {
        Self::new()
    }
}

impl Navidrome {
    pub fn new() -> Self {
        let http = reqwest::Client::builder()
            .user_agent(USER_AGENT)
            .timeout(REQUEST_TIMEOUT)
            .connect_timeout(CONNECT_TIMEOUT)
            .build()
            .unwrap_or_default();
        Navidrome {
            http,
            auth: parking_lot::RwLock::new(None),
            capabilities: parking_lot::RwLock::new(None),
        }
    }

    // ---- session -------------------------------------------------------------------------

    /// Install a session restored from the settings blob. Clears what was negotiated before: the
    /// blob does not carry extension support, so the next lyrics call asks rather than assumes.
    pub fn set_auth(&self, auth: Option<NavidromeAuth>) {
        *self.auth.write() = auth;
        *self.capabilities.write() = None;
    }

    pub fn auth(&self) -> Option<NavidromeAuth> {
        self.auth.read().clone()
    }

    pub fn signed_in(&self) -> bool {
        self.auth.read().is_some()
    }

    /// What the server said it supports. Empty until a sign-in has negotiated it.
    pub fn capabilities(&self) -> Capabilities {
        self.capabilities.read().clone().unwrap_or_default()
    }

    /// Sign in with a server address, user and password.
    ///
    /// The token is derived locally, so a wrong password is only discovered by the ping. Nothing is
    /// installed until the ping and `getUser` succeed; a failure leaves any previous session in
    /// place. The salt is reused when the address and user are unchanged, which keeps artwork that
    /// is already on screen valid.
    pub async fn sign_in(
        &self,
        server: &str,
        username: &str,
        password: &str,
    ) -> Result<NavidromeAuth> {
        let base_url = normalise_server(server)?;
        let username = username.trim();
        let reuse = {
            let current = self.auth.read();
            current
                .as_ref()
                .filter(|auth| auth.base_url == base_url && auth.username == username)
                .map(|auth| auth.salt.clone())
        };
        let candidate = NavidromeAuth::with_salt(server, username, password, reuse.as_deref())?;
        let capabilities = self.establish(&candidate).await?;
        self.set_auth(Some(candidate.clone()));
        *self.capabilities.write() = Some(capabilities);
        Ok(candidate)
    }

    /// Drop the in-memory session. The host owns deleting the settings blob; this never calls out.
    pub fn sign_out(&self) {
        self.set_auth(None);
    }

    /// Confirm the current session with the server.
    pub async fn ping(&self) -> Result<()> {
        self.call::<Value>("ping", &[], None).await.map(|_| ())
    }

    /// The URL builder for the current session, or `None` when signed out.
    pub fn covers(&self) -> Option<Covers> {
        self.auth.read().as_ref().and_then(|auth| auth.covers().ok())
    }

    /// The stream URL mpv opens. Always resolved fresh: it embeds the current token.
    pub fn stream_url(&self, song_id: &str) -> Result<String> {
        let auth = self.auth.read().clone().ok_or(Error::SignedOut)?;
        auth.covers()?.stream_url(song_id).ok_or(Error::Unsupported("this track has no server id"))
    }

    /// Ping, read the account, and note which OpenSubsonic extensions the server implements. A
    /// server that does not know the extension call is still signed in; only the ping decides.
    async fn establish(&self, auth: &NavidromeAuth) -> Result<Capabilities> {
        self.call_with::<Value>(auth, "ping", &[], None).await?;
        // `getUser` names the account it returns; Navidrome refuses it with
        // "missing parameter: 'username'" rather than defaulting to the caller.
        let user: User = self
            .call_with(auth, "getUser", &[("username", auth.username.as_str())], Some("user"))
            .await?;
        tracing::debug!(user = %user.username, "navidrome accepted the sign-in");

        let extensions = match self
            .call_with::<Vec<Extension>>(
                auth,
                "getOpenSubsonicExtensions",
                &[],
                Some("openSubsonicExtensions"),
            )
            .await
        {
            Ok(extensions) => extensions,
            Err(err) => {
                // A plain Subsonic server may not have this endpoint at all. Extensions decide
                // optional features; they are not a condition of signing in.
                tracing::debug!(error = %err, "navidrome did not list open subsonic extensions");
                Vec::new()
            }
        };

        Ok(Capabilities {
            extensions: extensions
                .into_iter()
                .map(|extension| extension.name)
                .filter(|name| !name.is_empty())
                .collect(),
        })
    }

    // ---- catalogue -----------------------------------------------------------------------

    pub async fn artists(&self) -> Result<Vec<Artist>> {
        let artists: Artists = self.call("getArtists", &[], Some("artists")).await?;
        Ok(artists.flatten())
    }

    pub async fn artist(&self, id: &str) -> Result<Artist> {
        self.call("getArtist", &[("id", id)], Some("artist")).await
    }

    pub async fn album(&self, id: &str) -> Result<Album> {
        self.call("getAlbum", &[("id", id)], Some("album")).await
    }

    /// One page of a server-side album list. `size` is clamped to the protocol maximum of 500.
    pub async fn albums(&self, kind: AlbumList, offset: u32, size: u32) -> Result<Vec<Album>> {
        let offset = offset.to_string();
        let size = size.clamp(1, PAGE_LIMIT).to_string();
        let list: AlbumListPayload = self
            .call(
                "getAlbumList2",
                &[("type", kind.as_str()), ("offset", &offset), ("size", &size)],
                Some("albumList2"),
            )
            .await?;
        Ok(list.album)
    }

    pub async fn playlists(&self) -> Result<Vec<Playlist>> {
        let list: PlaylistPayload = self.call("getPlaylists", &[], Some("playlists")).await?;
        Ok(list.playlist)
    }

    pub async fn playlist(&self, id: &str) -> Result<Playlist> {
        self.call("getPlaylist", &[("id", id)], Some("playlist")).await
    }

    pub async fn starred(&self) -> Result<Starred> {
        self.call("getStarred2", &[], Some("starred2")).await
    }

    pub async fn random_songs(&self, size: u32) -> Result<Vec<Song>> {
        let size = size.clamp(1, PAGE_LIMIT).to_string();
        let list: SongPayload =
            self.call("getRandomSongs", &[("size", &size)], Some("randomSongs")).await?;
        Ok(list.song)
    }

    /// One search across artists, albums and songs. Subsonic has no playlist result here.
    pub async fn search(&self, query: &str, limit: u32) -> Result<SearchResults> {
        let limit = limit.clamp(1, PAGE_LIMIT).to_string();
        self.call(
            "search3",
            &[
                ("query", query),
                ("artistCount", &limit),
                ("albumCount", &limit),
                ("songCount", &limit),
            ],
            Some("searchResult3"),
        )
        .await
    }

    // ---- annotations ---------------------------------------------------------------------

    /// Star or unstar one song, album or artist.
    pub async fn star(&self, kind: StarKind, id: &str, on: bool) -> Result<()> {
        let method = if on { "star" } else { "unstar" };
        self.call::<Value>(method, &[(kind.param(), id)], None).await.map(|_| ())
    }

    /// Report playback. `submission = false` is the "now playing" notification and never touches
    /// the play count; `submission = true` submits the listen at `time_ms` (Unix milliseconds).
    pub async fn scrobble(&self, id: &str, submission: bool, time_ms: i64) -> Result<()> {
        let time = time_ms.to_string();
        let params: &[(&str, &str)] = if submission {
            &[("id", id), ("submission", "true"), ("time", &time)]
        } else {
            &[("id", id), ("submission", "false")]
        };
        self.call::<Value>("scrobble", params, None).await.map(|_| ())
    }

    /// Server lyrics for a song, or `None` when the server has none.
    ///
    /// The `songLyrics` extension is the only source worth asking: Navidrome's legacy `getLyrics`
    /// needs an external lyrics agent. When a sign-in has established that the extension is absent
    /// this returns immediately; when extension support is unknown (a restored session) it asks and
    /// treats "unknown endpoint" as an empty answer, because the daemon falls back to LRCLIB.
    pub async fn lyrics(&self, song_id: &str) -> Result<Option<StructuredLyrics>> {
        if !self.signed_in() {
            return Err(Error::SignedOut);
        }
        let known = self.capabilities.read().clone();
        if known.as_ref().is_some_and(|capabilities| !capabilities.lyrics()) {
            return Ok(None);
        }
        match self
            .call::<LyricsList>("getLyricsBySongId", &[("id", song_id)], Some("lyricsList"))
            .await
        {
            Ok(list) => Ok(list.best().cloned()),
            // HTTP 404 is how a server without the endpoint answers; 70 is "no such item".
            Err(Error::Api { code: 404, .. }) | Err(Error::NotFound) => Ok(None),
            Err(err) => Err(err),
        }
    }

    // ---- playlist write-back -------------------------------------------------------------

    /// Create a playlist and return what the server made. Servers before 1.14 return no playlist
    /// object, so the list is read back and the newest playlist with that name is taken.
    pub async fn create_playlist(&self, name: &str) -> Result<Playlist> {
        let response: Value = self.call("createPlaylist", &[("name", name)], None).await?;
        if let Some(playlist) = response.get("playlist") {
            return Ok(serde_json::from_value(playlist.clone())?);
        }
        self.playlists().await?.into_iter().rev().find(|playlist| playlist.name == name).ok_or(
            Error::Api { code: 0, message: "navidrome did not return the created playlist".into() },
        )
    }

    /// Rename, add and remove in one update. Removal is by position, which is what the server
    /// exposes; a duplicated song therefore has two distinct positions and no ambiguity.
    pub async fn update_playlist(
        &self,
        id: &str,
        name: Option<&str>,
        add: &[String],
        remove_indices: &[u32],
    ) -> Result<()> {
        let mut params: Vec<(&str, String)> = vec![("playlistId", id.to_owned())];
        if let Some(name) = name.map(str::trim).filter(|name| !name.is_empty()) {
            params.push(("name", name.to_owned()));
        }
        for song in add {
            params.push(("songIdToAdd", song.clone()));
        }
        for index in remove_indices {
            params.push(("songIndexToRemove", index.to_string()));
        }
        let owned = as_borrowed(&params);
        self.call::<Value>("updatePlaylist", &owned, None).await.map(|_| ())
    }

    pub async fn delete_playlist(&self, id: &str) -> Result<()> {
        self.call::<Value>("deletePlaylist", &[("id", id)], None).await.map(|_| ())
    }

    /// Remove one row of a playlist. `index` is the row's position when the caller has one; a
    /// duplicated song has two positions, so falling back to "the first entry holding this id" is
    /// only right when the caller could not say which row it meant.
    pub async fn remove_playlist_entry(
        &self,
        playlist_id: &str,
        song_id: &str,
        index: Option<u32>,
    ) -> Result<()> {
        let index = match index {
            Some(index) => index,
            None => {
                let playlist = self.playlist(playlist_id).await?;
                let position = playlist
                    .entry
                    .iter()
                    .position(|song| song.id == song_id)
                    .ok_or(Error::NotFound)?;
                position as u32
            }
        };
        self.update_playlist(playlist_id, None, &[], &[index]).await
    }

    /// Rewrite a playlist's songs in one call. Subsonic documents this as an update through
    /// `createPlaylist` with an existing `playlistId`, which is also its only reorder path.
    pub async fn replace_playlist(&self, id: &str, song_ids: &[String]) -> Result<()> {
        let mut params: Vec<(&str, String)> = vec![("playlistId", id.to_owned())];
        for song in song_ids {
            params.push(("songId", song.clone()));
        }
        let owned = as_borrowed(&params);
        self.call::<Value>("createPlaylist", &owned, None).await.map(|_| ())
    }

    // ---- the funnel ----------------------------------------------------------------------

    async fn call<T: DeserializeOwned>(
        &self,
        method: &str,
        params: &[(&str, &str)],
        payload: Option<&str>,
    ) -> Result<T> {
        let auth = self.auth.read().clone().ok_or(Error::SignedOut)?;
        self.call_with(&auth, method, params, payload).await
    }

    async fn call_with<T: DeserializeOwned>(
        &self,
        auth: &NavidromeAuth,
        method: &str,
        params: &[(&str, &str)],
        payload: Option<&str>,
    ) -> Result<T> {
        let url = auth.covers()?.endpoint(method, params)?;
        // `reqwest::Error` quotes the URL it failed on, and that URL carries the auth query — the
        // token is a password equivalent, so the url is dropped before the error reaches a log, a
        // toast or the diagnostics page.
        let response = self.http.get(url).send().await.map_err(|e| Error::Http(e.without_url()))?;
        let status = response.status();
        let body = response.text().await.map_err(|e| Error::Http(e.without_url()))?;
        if !status.is_success() {
            // A reverse proxy in front of the server answers 401/403 itself, and that body is
            // not a Subsonic envelope. A dead session is the actionable reading either way.
            if matches!(status.as_u16(), 401 | 403) {
                return Err(Error::AuthFailed);
            }
            return Err(Error::Api {
                code: status.as_u16() as u32,
                message: format!("http {status}"),
            });
        }
        decode(serde_json::from_str(&body)?, payload)
    }
}

/// Turn a prepared parameter list into the borrowed pairs the funnel takes.
fn as_borrowed<'a>(params: &'a [(&'a str, String)]) -> Vec<(&'a str, &'a str)> {
    params.iter().map(|(key, value)| (*key, value.as_str())).collect()
}

/// The wire envelope. Every field but the response object is optional: a failure carries an
/// `error` object, a success carries one named payload, and either way `status` decides.
#[derive(Debug, Deserialize)]
struct Envelope {
    #[serde(rename = "subsonic-response")]
    response: ResponseBody,
}

#[derive(Debug, Deserialize)]
struct ResponseBody {
    #[serde(default)]
    status: String,
    #[serde(default)]
    error: Option<ApiError>,
    #[serde(flatten)]
    payload: Map<String, Value>,
}

#[derive(Debug, Deserialize)]
struct ApiError {
    #[serde(default)]
    code: u32,
    #[serde(default)]
    message: String,
}

/// Check the envelope and deserialise the payload the caller named. Kept synchronous and separate
/// so the error table is unit-tested without a socket.
fn decode<T: DeserializeOwned>(envelope: Envelope, payload: Option<&str>) -> Result<T> {
    let body = envelope.response;
    if body.status != "ok" {
        return Err(match body.error {
            Some(error) => Error::from_api(error.code, error.message),
            None => Error::Api {
                code: 0,
                message: format!("navidrome answered status `{}`", body.status),
            },
        });
    }
    match payload {
        Some(key) => {
            let value = body.payload.get(key).cloned().ok_or(Error::Api {
                code: 0,
                message: format!("navidrome answered without `{key}`"),
            })?;
            Ok(serde_json::from_value(value)?)
        }
        None => Ok(serde_json::from_value(Value::Object(body.payload))?),
    }
}

#[derive(Debug, Deserialize)]
struct AlbumListPayload {
    #[serde(default)]
    album: Vec<Album>,
}

#[derive(Debug, Deserialize)]
struct PlaylistPayload {
    #[serde(default)]
    playlist: Vec<Playlist>,
}

#[derive(Debug, Deserialize)]
struct SongPayload {
    #[serde(default)]
    song: Vec<Song>,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn decode_str<T: DeserializeOwned>(body: &str, payload: Option<&str>) -> Result<T> {
        decode(serde_json::from_str(body).expect("the fixture is json"), payload)
    }

    #[test]
    fn an_ok_envelope_hands_back_the_named_payload() {
        let body = r#"{"subsonic-response":{"status":"ok","version":"1.16.1",
            "artist":{"id":"ar-1","name":"Queen"}}}"#;
        let artist: Artist = decode_str(body, Some("artist")).unwrap();
        assert_eq!(artist.id, "ar-1");
        assert_eq!(artist.name, "Queen");
    }

    #[test]
    fn a_failed_envelope_maps_the_subsonic_code() {
        let body = r#"{"subsonic-response":{"status":"failed","version":"1.16.1",
            "error":{"code":40,"message":"Wrong username or password"}}}"#;
        let err = decode_str::<Value>(body, None).unwrap_err();
        assert!(matches!(err, Error::AuthFailed), "{err}");

        let body = r#"{"subsonic-response":{"status":"failed",
            "error":{"code":70,"message":"The requested data was not found."}}}"#;
        assert!(matches!(decode_str::<Value>(body, None), Err(Error::NotFound)));
    }

    #[test]
    fn an_unknown_error_code_keeps_the_servers_own_words() {
        let body = r#"{"subsonic-response":{"status":"failed",
            "error":{"code":10,"message":"Required parameter is missing"}}}"#;
        assert_eq!(
            decode_str::<Value>(body, None).unwrap_err().to_string(),
            "navidrome answered error 10: Required parameter is missing"
        );
    }

    #[test]
    fn a_failed_envelope_without_an_error_object_is_not_read_as_success() {
        let body = r#"{"subsonic-response":{"status":"failed"}}"#;
        let err = decode_str::<Value>(body, None).unwrap_err();
        assert!(matches!(err, Error::Api { code: 0, .. }), "{err}");
    }

    #[test]
    fn a_missing_payload_is_an_error_not_a_default() {
        let body = r#"{"subsonic-response":{"status":"ok","version":"1.16.1"}}"#;
        let err = decode_str::<Album>(body, Some("album")).unwrap_err();
        assert!(err.to_string().contains("`album`"), "{err}");
    }

    #[test]
    fn a_signed_out_client_refuses_to_stream_without_a_request() {
        let navidrome = Navidrome::new();
        assert!(!navidrome.signed_in());
        assert!(matches!(navidrome.stream_url("so-1"), Err(Error::SignedOut)));
        assert!(navidrome.covers().is_none());
    }

    #[test]
    fn an_installed_session_builds_its_urls() {
        let navidrome = Navidrome::new();
        navidrome.set_auth(Some(NavidromeAuth {
            base_url: "https://music.example.com".into(),
            username: "admin".into(),
            salt: "abc".into(),
            token: "c42b93a1ac3b382e88071510296a96ce".into(),
        }));
        assert!(navidrome.signed_in());
        let stream = navidrome.stream_url("so-1").expect("a session builds a url");
        assert!(stream.contains("/rest/stream.view"));
        assert!(navidrome.covers().unwrap().url("al-1", 320).unwrap().contains("getCoverArt"));
        navidrome.sign_out();
        assert!(!navidrome.signed_in());
        assert!(navidrome.covers().is_none());
    }

    #[tokio::test]
    async fn lyrics_are_skipped_when_the_sign_in_saw_no_extension() {
        let navidrome = Navidrome::new();
        navidrome.set_auth(Some(NavidromeAuth {
            base_url: "https://music.example.com".into(),
            username: "admin".into(),
            salt: "abc".into(),
            token: "t".into(),
        }));
        *navidrome.capabilities.write() =
            Some(Capabilities { extensions: vec!["formPost".into()] });
        assert_eq!(navidrome.lyrics("so-1").await.unwrap(), None);
        assert!(matches!(Navidrome::new().lyrics("so-1").await, Err(Error::SignedOut)));
    }

    #[test]
    fn album_lists_use_the_servers_own_spellings() {
        assert_eq!(AlbumList::Recent.as_str(), "recent");
        assert_eq!(AlbumList::Newest.as_str(), "newest");
        assert_eq!(AlbumList::AlphabeticalByArtist.as_str(), "alphabeticalByArtist");
    }

    #[test]
    fn a_star_names_the_parameter_the_protocol_documents() {
        assert_eq!(StarKind::Song.param(), "id");
        assert_eq!(StarKind::Album.param(), "albumId");
        assert_eq!(StarKind::Artist.param(), "artistId");
    }

    /// Navidrome's `getUser` names the account it returns; without the parameter the sign-in dies
    /// after a successful ping with "missing parameter: 'username'". A tiny local server records
    /// the request line, so the contract is pinned on the wire rather than in a helper.
    #[tokio::test]
    async fn sign_in_names_the_account_it_reads() {
        use std::io::{Read, Write};
        use std::net::TcpListener;
        use std::time::{Duration, Instant};

        let listener = TcpListener::bind("127.0.0.1:0").expect("a free loopback port");
        listener.set_nonblocking(true).expect("the listener can be polled");
        let addr = listener.local_addr().expect("a bound address");
        let server = std::thread::spawn(move || {
            let deadline = Instant::now() + Duration::from_secs(5);
            let mut seen: Vec<String> = Vec::new();
            while Instant::now() < deadline {
                let Ok((mut socket, _)) = listener.accept() else {
                    std::thread::sleep(Duration::from_millis(5));
                    continue;
                };
                socket.set_nonblocking(false).expect("a blocking fixture socket");
                let mut buf = [0u8; 4096];
                let n = socket.read(&mut buf).unwrap_or(0);
                let request = String::from_utf8_lossy(&buf[..n]).into_owned();
                let body = if request.contains("/rest/ping.view") {
                    r#"{"subsonic-response":{"status":"ok","version":"1.16.1"}}"#
                } else if request.contains("/rest/getUser.view") {
                    r#"{"subsonic-response":{"status":"ok","version":"1.16.1","user":{"username":"aleft"}}}"#
                } else {
                    r#"{"subsonic-response":{"status":"ok","version":"1.16.1","openSubsonicExtensions":[]}}"#
                };
                let _ = write!(
                    socket,
                    "HTTP/1.1 200 OK\r\ncontent-type: application/json\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{}",
                    body.len(),
                    body
                );
                let asked_for_user = request.contains("/rest/getUser.view");
                seen.push(request);
                if asked_for_user {
                    break;
                }
            }
            seen
        });

        let navidrome = Navidrome::new();
        navidrome
            .sign_in(&format!("http://{addr}"), "aleft", "secret")
            .await
            .expect("the fixture server accepts the sign-in");
        let requests = server.join().expect("the fixture thread finishes");
        let user = requests
            .iter()
            .find(|request| request.contains("/rest/getUser.view"))
            .expect("getUser was asked");
        assert!(user.contains("username=aleft"), "the account must be named: {user}");
    }

    /// The token is a password equivalent, so a transport failure must not quote the URL it was
    /// carrying. Port 1 on loopback is closed, so this fails at connect without a network.
    #[tokio::test]
    async fn a_transport_failure_never_echoes_the_auth_query() {
        let navidrome = Navidrome::new();
        let err = navidrome
            .sign_in("http://127.0.0.1:1", "admin", "secret")
            .await
            .expect_err("nothing listens on port 1");
        let text = err.to_string();
        assert!(!text.contains("admin"), "the username leaked: {text}");
        assert!(!text.contains("secret"), "the password leaked: {text}");
        assert!(!text.contains("t="), "the auth query leaked: {text}");
        assert!(!navidrome.signed_in(), "a failed sign-in installs nothing");
    }
}
