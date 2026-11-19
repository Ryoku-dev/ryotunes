# Navidrome Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Navidrome as a fifth catalogue provider (beside YouTube Music, Spotify, SoundCloud and TIDAL), so a user can browse, search, play, star, scrobble and edit playlists on their own Subsonic/OpenSubsonic music server from the primary QML client.

**Architecture:** A new `crates/navidrome` crate owns the HTTP funnel, salted-token auth, Subsonic response envelope and the catalogue calls. `crates/core/src/navidrome_bridge.rs` maps its models to the existing YouTube-shaped browse JSON, exactly as `tidal_bridge.rs` does. The daemon adds a `Provider::Navidrome` branch per method, a `navidrome-auth` event and a `navidrome` subscribe-snapshot block. Playback hands a self-contained `/rest/stream` URL to mpv (no FIFO, no librespot). The QML client adds a fifth pill, a Settings sign-in row, provider-scoped ids and a sign-in gate.

**Tech Stack:** Rust 2021, `reqwest` (rustls), `serde`, `md-5`; Quickshell/QML for the client. No new system dependency, no new binary, no protocol change (methods/events are additive, `PROTOCOL_VERSION` stays 1).

**API reference:** [OpenSubsonic API](https://opensubsonic.netlify.app/docs/), [Navidrome Subsonic compatibility](https://www.navidrome.org/docs/developers/subsonic-api/). Subsonic version `1.16.1`, JSON output (`f=json`).

**Decisions taken with the product owner:**

| Topic | Decision |
|---|---|
| Scope v1 | Browse, search, stream, star/unstar, scrobble, playlist write-back, server lyrics |
| Auth | Server URL + username + password; requests use `u`/`s`/`t = md5(password + salt)`. The password is **not** persisted — only server, user, salt and derived token. |
| Placement | Fifth catalogue provider, TIDAL arrangement (Home/Library/Search are server-scoped) |
| Servers | One saved server at a time |

**Explicitly out of scope for v1 (keep the seams clean):** downloads, server internet radio, bookmarks / play-queue sync, transcoding or bitrate selection, self-signed TLS opt-out, multiple saved servers, podcasts, browse-by-folder/genre, API-key auth, share-link paste, offline caching.

## Global constraints

- Every gate stays green after every task: `cargo fmt --all -- --check`, `cargo test --workspace --locked`, `cargo check --workspace --locked`, `scripts/release-check.sh` (exit 0).
- No new `tauri` dependency anywhere in `crates/*`. The legacy Tauri host is not touched; `methods.rs`'s `every_tauri_command_has_a_method` test must keep passing without adding Tauri commands.
- Network stays behind the daemon; QML never talks HTTP. No polling loop, no new interval: Home/Library/Search stay demand-driven.
- Cover-art and stream URLs carry the derived token in their query string. They are display/playback URLs only; they are never sent to another service and never used as download sources.
- Bounded work: Home shelves cap at 12 items each; library album paging caps at 2000 albums; search caps at 50 per category.
- Fail-soft: scrobble and star echoes are best-effort (`debug!` on failure), never user-facing errors. Catalogue errors are user-facing and use a `navidrome` error code family.
- Commit subjects: `navidrome: …`, imperative, no trailing period, no attribution trailers.

## Design overview

### Identity

Navidrome ids are opaque strings (MD5 hashes or UUIDs), so mint namespaced ids exactly like the other providers:

```
nd:track:<server song id>
nd:album:<server album id>
nd:artist:<server artist id>
nd:playlist:<server playlist id>
```

`nd:` never collides with a YouTube video id, `spotify:`, `sc:` or `tidal:`. The crate speaks bare server ids; the daemon and bridge add/strip the prefix. Server ids are only ever placed into URLs through `reqwest`'s query pairs, so characters are encoded correctly.

### Auth

`NavidromeAuth` is what persists under the daemon-only settings key `navidrome_auth`:

```rust
pub struct NavidromeAuth {
    pub base_url: String,   // normalised, no trailing slash, may include a path prefix
    pub username: String,
    pub salt: String,       // 16 random alphanumerics, generated once per server+user
    pub token: String,      // lowercase hex md5(password + salt)
}
```

- Sign-in: normalise the URL, `ping` with the derived token, then `getUser` for the display name and `getOpenSubsonicExtensions` for capability flags. A failed ping persists nothing.
- Persistence: `db.set_setting("navidrome_auth", json)`. The password exists only on the stack during sign-in.
- The salt is reused when reconnecting to the same server+user, so already-rendered cover URLs keep working. A password change on the server invalidates old derived-token URLs (they 401 and degrade to the artwork placeholder until re-browsed); stream URLs are always resolved at play time.
- Subsonic error code `40` maps to `AuthFailed`; the daemon then clears the stored auth and asks the client to sign in again.

### Request shape

```
GET {base}/rest/{method}.view?u={user}&s={salt}&t={token}&v=1.16.1&c=Ryotunes&f=json&{params}
```

The response envelope is `{"subsonic-response": {status, version, error?, …}}`. The crate parses the envelope as `Value`, checks `status`/`error`, then deserialises the named payload key. `status == "failed"` becomes `Error::Api { code, message }`.

### Streaming and artwork

- Stream: `{base}/rest/stream?id={id}&format=raw` + auth query. `format=raw` disables server transcoding. Hand the URL to mpv like SoundCloud/TIDAL; do not cache it (`expires_in_seconds: 0`), because it embeds the current token.
- Cover art: `{base}/rest/getCoverArt?id={coverArt}&size={px}` + auth query. `client/lib/style.js`'s `thumb()` only rewrites Google image URLs and local paths, so these URLs pass through untouched.
- `StreamClient` label: `"navidrome"`.

### Endpoint mapping (v1)

| Ryotunes surface | Subsonic calls |
|---|---|
| Home | `getAlbumList2?type=recent`, `?type=newest`, `getStarred2`, `getPlaylists`, `getRandomSongs` |
| Library playlists | `getPlaylists` |
| Library albums | `getAlbumList2?type=alphabeticalByArtist` (paged, 500 at a time, cap 2000) |
| Library artists | `getArtists` (flatten `index[].artist[]`) |
| Album page | `getAlbum?id` |
| Artist page | `getArtist?id` (albums carousel; no top-songs call — `getTopSongs` needs Last.fm) |
| Playlist page | `getPlaylist?id` |
| Search | `search3?query&artistCount=50&albumCount=50&songCount=50` |
| Like / unlike | `star?id=` / `unstar?id=` (song or album) |
| Scrobble | `scrobble?id=&submission=false` on start, `…&submission=true&time=<ms>` past the threshold |
| Lyrics | `getLyricsBySongId?id=` (OpenSubsonic `songLyrics` extension only) |
| Playlist create | `createPlaylist?name=` |
| Playlist rename | `updatePlaylist?playlistId=&name=` |
| Playlist add | `updatePlaylist?playlistId=&songIdToAdd=` |
| Playlist remove | `updatePlaylist?playlistId=&songIndexToRemove=<index>` |
| Playlist reorder | read all ids, then `createPlaylist?playlistId=&songId=…` in the new order |
| Playlist delete | `deletePlaylist?id=` |

---

### Task 1: `crates/navidrome` — models, errors, auth and URL building

**Files:**
- Create: `crates/navidrome/Cargo.toml`, `crates/navidrome/src/lib.rs`, `crates/navidrome/src/models.rs`, `crates/navidrome/src/auth.rs`
- Modify: `Cargo.toml` (workspace member)

**Interfaces:**
- Produces: `ryotunes_navidrome::{Navidrome, NavidromeAuth, Credentials, Covers, Capabilities, Song, Album, Artist, Playlist, SearchResults, Starred, StructuredLyrics, Error, Result}`.

- [ ] **Step 1: Crate manifest.** `md-5 = "0.10"`, `rand`, `serde`, `serde_json`, `thiserror`, `tracing`, `parking_lot`, `tokio` (`sync`), `reqwest` (`rustls-tls`, `gzip`, `brotli`, `json`), plus `[dev-dependencies] tokio` with `rt-multi-thread`/`macros`. Add `crates/navidrome` to `[workspace] members`.
- [ ] **Step 2: Id/URL helpers in `auth.rs`.**
  - `normalise_server(&str) -> Result<String>`: trim, require `http`/`https`, strip a trailing `/` and a trailing `/rest`, reject userinfo/query/fragment. Unit-test a plain host, a path prefix, a pasted `/rest` suffix, a trailing slash, a missing scheme and a `javascript:` URL.
  - `Credentials::{new(server, user, password)}` derives the token; `NavidromeAuth::query()` returns the ordered auth pairs; `Covers::url(cover_id, px)` and `stream_url(song_id)` build absolute URLs.
  - Test the MD5 derivation against a known vector (`md5("secret" + "abc")`), the normalised query order, and that an id containing `&`, space or `#` is percent-encoded (build through `reqwest::Url`, not string concatenation).
- [ ] **Step 3: Models.** `Song`, `Album`, `Artist`, `Playlist`, `SearchResults`, `Starred`, `StructuredLyrics`. Use `#[serde(default)]` widely (Subsonic omits empty lists and blank strings), and `#[serde(alias = "album")]` on `Album::name` so `getAlbum` and `getAlbumList2` parse from one struct. Fixture tests parse the JSON from the OpenSubsonic `getArtists`, `getAlbumList2`, `search3` and `getAlbum` examples.
- [ ] **Step 4: Error enum.** `Http`, `Json`, `Api { code, message }`, `AuthFailed`, `AuthUnsupported`, `SignedOut`, `NotFound`, `Unsupported(&'static str)`. Map Subsonic codes 40 → `AuthFailed`, 41/42/43/44 → `AuthUnsupported`, 70 → `NotFound`.
- [ ] **Step 5: Gate.** `cargo test -p ryotunes-navidrome --locked` passes; `cargo fmt --all -- --check` clean.

### Task 2: `crates/navidrome` — client funnel and catalogue calls

**Files:**
- Create: `crates/navidrome/src/client.rs`, `crates/navidrome/src/stream.rs`
- Modify: `crates/navidrome/src/lib.rs`

**Interfaces:**
- Produces: `Navidrome::{new, set_auth, auth, signed_in, sign_in, sign_out, ping, covers, capabilities, artists, artist, album, albums, playlists, playlist, starred, random_songs, search, star, scrobble, lyrics, create_playlist, update_playlist, delete_playlist, replace_playlist}`.

- [ ] **Step 1: Funnel.** `async fn call<T: DeserializeOwned>(&self, method, params, payload: Option<&str>)`:
  - returns `Error::SignedOut` without a session;
  - builds a `reqwest::Url` from `{base}/rest/{method}.view` and appends auth + params;
  - parses the envelope, maps `status: "failed"` to the error enum;
  - extracts `payload` when given, then `serde_json::from_value`.
- [ ] **Step 2: Sign-in.** `sign_in(server, user, password)`: normalise → `Credentials` → `ping` → `getUser` (name/roles) → `getOpenSubsonicExtensions` (store which extension names are supported in `Capabilities`). Return `NavidromeAuth`. `sign_out()` clears in-memory auth only; the daemon owns deletion of the settings key. Unit-test the envelope error mapping with mocked JSON (no network).
- [ ] **Step 3: Catalogue.** `artists` (flatten `getArtists`), `artist` (`getArtist`), `album` (`getAlbum`), `albums(kind, offset, size)` (`getAlbumList2`), `playlists`, `playlist`, `starred`, `random_songs`, `search` (`search3`). All return typed models; no pagination logic above the call except `albums`.
- [ ] **Step 4: Annotation and lyrics.** `star(kind, id, on)`, `scrobble(id, submission, time_ms)`, `lyrics(song_id) -> Option<StructuredLyrics>` (returns `Ok(None)` when the `songLyrics` extension is absent, rather than an error).
- [ ] **Step 5: Playlist write-back.** `create_playlist(name)`, `update_playlist(id, name: Option<&str>, add: &[String], remove_indices: &[u32])`, `delete_playlist(id)`, `replace_playlist(id, song_ids)` (createPlaylist with an existing `playlistId` replaces the song list — the documented Subsonic reorder path).
- [ ] **Step 6: Stream/cover URLs.** `Covers` and `stream_url` are pure and unit-tested; no network.
- [ ] **Step 7: Live smoke test (ignored by default).** `crates/navidrome/tests/live_smoke.rs` gated on `NAVIDROME_TEST_SERVER`, `NAVIDROME_TEST_USER`, `NAVIDROME_TEST_PASSWORD`; `#[ignore]` like `crates/tidal/tests/live_smoke.rs`.

### Task 3: `navidrome_bridge` — mapping to the YouTube-shaped pages

**Files:**
- Create: `crates/core/src/navidrome_bridge.rs`
- Modify: `crates/core/src/lib.rs` (`pub mod navidrome_bridge;`), `crates/core/Cargo.toml` (`ryotunes-navidrome` path dep)

**Interfaces:**
- Produces: `track_to_song`, `track_to_card`, `album_to_card`, `artist_to_card`, `playlist_to_card`, `album_page`, `playlist_page`, `artist_page`, `search_all`, `search_cards`, `search_songs`, `home_sections`, `library_playlists`, `lyrics_to_lyrics`.

- [ ] **Step 1: Pure mapping.** Mirror `tidal_bridge.rs` and keep it network-free. Functions take `&Covers` so cover URLs are built without the client:
  ```rust
  pub fn track_to_song(track: &Song, covers: &Covers) -> SongItem
  ```
  `video_id: "nd:track:<id>"`, `artist_id: "nd:artist:<artistId>"`, `album_id` likewise, duration from seconds, `thumbnail: covers.url(cover_art, 320)`, `play_count`, and `rating: starred.is_some().then_some(Rating::Like)` so hearts render from server state.
- [ ] **Step 2: Playlist rows carry their position.** Set `set_video_id = Some(format!("nd:idx:{index}"))` in `playlist_page`, so removal is index-exact on the server and repeated tracks are unambiguous. Test that a duplicated song keeps distinct indexes.
- [ ] **Step 3: Pages and search.** `album_page`, `playlist_page` (`owned: true`, `continuation: None`), `artist_page` (albums carousel; `top_songs` empty), `search_all` (`top` empty), `search_cards`, `search_songs`. Match the existing field conventions exactly (see `tidal_bridge.rs:130-227`).
- [ ] **Step 4: Home and library shelves.** `home_sections` produces, in order and only when non-empty: "Recently played" (albums), "Recently added" (albums), "Starred" (albums, falling back to songs when no albums are starred), "Playlists", "Random picks" (songs). `library_playlists` maps playlist cards.
- [ ] **Step 5: Lyrics.** `lyrics_to_lyrics(&StructuredLyrics, source: &str) -> Lyrics` picks the first synced entry, otherwise the first entry; maps `Line { start, end, value }` to `LyricLine { time_ms, end_time_ms, text }`; `synced` from the entry flag. De-duplicate identical line texts the way the lyrics panel expects, if the existing providers do.
- [ ] **Step 6: Unit tests** for every mapping with small fixtures, including a track with no album, no cover, no duration, and a starred track.

### Task 4: Provider enum, ids and `AppState` wiring

**Files:**
- Modify: `crates/core/src/spotify.rs` (`Provider` + id helpers; this module is the de-facto provider module), `crates/core/src/state.rs`, `crates/core/src/discord.rs`
- Modify: `crates/ryotunesd/Cargo.toml` (`ryotunes-navidrome` path dep)

- [ ] **Step 1: `Provider::Navidrome`.** Add the variant, `as_str() == "navidrome"`, `parse("navidrome")`, and `browsing_navidrome()` (selection only; callers check `signed_in()`).
- [ ] **Step 2: Id helpers.** `ND_TRACK_PREFIX`, `ND_ALBUM_PREFIX`, `ND_ARTIST_PREFIX`, `ND_PLAYLIST_PREFIX`, `is_nd_id`, `nd_track_id`, `nd_album_id`, `nd_artist_id`, `nd_playlist_id` returning `&str`. Unit tests, including that `"navidromeish"` is not an id.
- [ ] **Step 3: Settings key and construction.** `pub const NAVIDROME_AUTH_KEY: &str = "navidrome_auth";` near the other provider keys. In `AppState::new` (around `state.rs:461`): construct `Arc<ryotunes_navidrome::Navidrome>`, restore `navidrome_auth` if it parses (warn and ignore a bad blob), and add the field `pub navidrome: Arc<ryotunes_navidrome::Navidrome>`.
- [ ] **Step 4: Resolve.** In `resolve` (before the YouTube latency cache, beside the SoundCloud/TIDAL blocks around `state.rs:1078`):
  ```rust
  if let Some(id) = crate::spotify::nd_track_id(video_id) {
      if !self.navidrome.signed_in() {
          return Err(ResolveError::SignInRequired("Sign in to Navidrome to play this".into()));
      }
      let url = self.navidrome.stream_url(id).map_err(...)?;
      return Ok(PlaybackData {
          video_id: video_id.to_owned(),
          stream_url: url,
          itag: 0,
          mpv_options: Vec::new(),
          headers: Default::default(),
          expires_in_seconds: 0,
          loudness_db: None,
          playback_ping: None,
          title: None,
          artists: None,
          duration: None,
          thumbnail: None,
          is_video: Some(false),
          stream_client: "navidrome".to_owned(),
      });
  }
  ```
  Do not add it to the DB stream cache. `prefetch_stream` must skip `nd:` ids (it only skips Spotify today).
- [ ] **Step 5: Round-trip fixes for provider ids.** `refresh_rating` must not query YouTube for `nd:` ids (add `is_nd_id`, and while here `is_tidal_id`). `now_playing_json` must not let the YouTube liked-list override a Navidrome rating. Add `nd:` to the On Repeat exclusion in the play-history filter (`state.rs:2580-2589`) so a server track does not seed YouTube's On Repeat.
- [ ] **Step 6: Autoplay/radio.** `start_radio` and the autoplay continuation must refuse an `nd:` tail with a clear message ("Navidrome has no radio endpoint") instead of sending it to YouTube `/next`. Add a guard next to the existing local/radio/Spotify guards.
- [ ] **Step 7: Discord labels.** Add Navidrome to `discord.rs::provider_label` and `listen_link` (a link only when the server has a browsable web URL — otherwise no link), plus the existing label tests.

### Task 5: Daemon sign-in, status and snapshot

**Files:**
- Modify: `crates/ryotunesd/src/methods.rs`, `crates/ryotunesd/src/server.rs`

- [ ] **Step 1: Error helpers.** `navidrome_err(e) -> ErrorBody` with codes `navidrome_signed_out`, `navidrome_auth`, `navidrome_unsupported`, `navidrome`; `navidrome_signed_out()` → "Sign in to Navidrome".
- [ ] **Step 2: Status JSON.** `navidrome_status_json(st) -> { signedIn, server, name }` from the restored auth. No network.
- [ ] **Step 3: Snapshot.** Add `"navidrome": navidrome_status_json(st)` to `subscribe` beside `spotify`/`soundcloud`/`tidal`.
- [ ] **Step 4: Methods.**
  - `navidrome_sign_in {server, username, password}` → crate sign-in, persist `NAVIDROME_AUTH_KEY`, emit `navidrome-auth {state:"signed_in", server, name}`, return `{server, name}`.
  - `navidrome_sign_out` → `sign_out()`, `db.delete_setting(NAVIDROME_AUTH_KEY)`, emit `navidrome-auth {state:"signed_out"}`.
  - `navidrome_status` → the status JSON.
  - On an auth failure, clear the stored blob and emit `navidrome-auth {state:"error", message}` so every window shows the sign-in form again. Keep the password out of every message and log line.
- [ ] **Step 5: Diagnostics.** Add `navidrome_signed_out` to the non-error code allowlist in `server.rs:170`.
- [ ] **Step 6: Test** that sign-in with an unreachable server returns `navidrome_auth` and persists nothing (crate mocked, no real socket).

### Task 6: Daemon browse dispatch and annotations

**Files:**
- Modify: `crates/ryotunesd/src/methods.rs`

- [ ] **Step 1: Search.** `search_page`, `search_page_more`, `search_all`, `search_cards` branch on `browsing_navidrome()` before the YouTube path; `*_more` returns `{items: [], continuation: null}` like the other providers. Add helpers `navidrome_search_all/page/cards` beside `tidal_*` at the end of the file.
- [ ] **Step 2: Home.** In `get_home`, return `navidrome_home(st)` when `browsing_navidrome()` (before the chip-filter branch, like TIDAL). Signed out → `{chips: [], sections: [], continuation: null}`. `get_home_more` stays YouTube-only.
- [ ] **Step 3: Library.** `get_library` → playlists; `get_library_albums` → the paged album list; `get_library_artists` → the flattened artist list; all empty when signed out.
- [ ] **Step 4: Detail pages.** Add `nd_playlist_id` / `nd_album_id` / `nd_artist_id` branches to `get_playlist`, `get_album` and `get_artist` helpers, beside the TIDAL ones.
- [ ] **Step 5: Like.** In `rate`, branch on `nd_track_id` first: signed in → `star("song", id, rating == Like)`; signed out → keep the existing `rate_locally` fallback. UX-echo the new rating through the normal `rating` event (reuse `rate_locally`'s emit path or emit directly) and refresh the now-playing rating.
- [ ] **Step 6: Album star.** `set_album_saved` accepts `nd:album:` → `star("album", id, saved)`.
- [ ] **Step 7: Playlist write-back.**
  - `create_playlist` when browsing Navidrome → `create_playlist(name)` → `nd:playlist:<id>`.
  - `edit_playlist_details` on `nd:playlist:` → `update_playlist(id, name, …)`.
  - `delete_playlist` on `nd:playlist:` → `delete_playlist(id)`.
  - `add_to_playlist` on `nd:playlist:` requires an `nd:track:` video id (reject cross-catalogue adds with "This playlist is on your Navidrome server.").
  - `remove_from_playlist` on `nd:playlist:` uses `setVideoId`'s `nd:idx:<n>` when present, else fetches the playlist and removes the first matching song id.
  - `set_playlist_sort` on `nd:playlist:` reads the playlist, reorders the ids, and calls `replace_playlist`.
  - `set_playlist_cover` on `nd:playlist:` returns a clear "Navidrome playlists use server artwork." error rather than a YouTube call.
- [ ] **Step 8: Lyrics.** In `get_lyrics`, before the generic chain: an `nd:track:` id with a signed-in server calls `lyrics(id)`; a hit maps through `navidrome_bridge::lyrics_to_lyrics`; a miss or an unsupported extension falls through to the existing local/LRCLIB chain.
- [ ] **Step 9: Guard unimplemented surfaces.** `get_waveform`, `get_sc_user` and `start_radio` keep their clear errors for `nd:` ids. Confirm no `nd:` id can reach `st.album`/`st.artist`/`st.playlist`.

### Task 7: Scrobbling

**Files:**
- Create: `crates/core/src/navidrome_scrobble.rs` (or a small private module inside `state.rs`)
- Modify: `crates/core/src/state.rs`, `crates/core/src/lib.rs`

- [ ] **Step 1: Threshold state.** Port Last.fm's rule (a track scrobbles at min(duration/2, 4 min) once at least 30 s have played) as a pure `fn crosses_threshold(pos, duration) -> bool` with unit tests, so both scrobblers agree.
- [ ] **Step 2: Wire the three existing choke points.** `set_now_playing` (around `state.rs:2336`, beside `lastfm.set_track`), `on_duration` (beside the lastfm duration push) and the position tick (beside `lastfm.set_position`):
  - track change on an `nd:track:` id → store `{id, started_at, duration, submitted:false}` and spawn `scrobble(id, false, now)` ("now playing");
  - threshold crossed → latch and spawn `scrobble(id, true, started_at * 1000)` once;
  - any other id resets the state.
- [ ] **Step 3: Fail-soft.** Spawned calls only `debug!` on failure; they never toast and never block playback. Signing out mid-track simply drops the scrobble.
- [ ] **Step 4: Unit tests** for the threshold, the latch (no duplicate submissions), and that a non-`nd` track never scrobbles.

### Task 8: QML data model — ids, colour, glyph, state and events

**Files:**
- Modify: `client/lib/ids.js`, `client/lib/glyphs.js`, `client/lib/recommendations.js`, `client/Style.qml`, `client/Playback.qml`, `client/lib/playback.js`

- [ ] **Step 1: Ids.** `providerOf`: `if (id.indexOf("nd:") === 0) return "navidrome";` before the YouTube fallthrough. `sourceGlyph`: `case "navidrome": return "navidrome";`. `providerLabel`: add `"Navidrome"` (and add the missing `"TIDAL"` while here).
- [ ] **Step 2: Colour.** `Style.providerColors.navidrome = "#8b7cf6"` (proposal — validate contrast against the paper tokens during review; it must not read as TIDAL cyan or Spotify green).
- [ ] **Step 3: Glyph.** Add a `"navidrome"` 24×24 stroke glyph to `glyphs.js`: a compact three-bar waveform, drawn in-house (do not copy a third-party logo asset).
- [ ] **Step 4: Playback state.** In `Playback.qml`: update the provider comment, add
  ```qml
  property var navidrome: ({ signedIn: false, server: "", name: null, error: "", busy: false })
  ```
  load it in `loadSnapshot` from `snap.navidrome`, and add `navidromeSignIn(server, user, password)` / `navidromeSignOut()` methods.
- [ ] **Step 5: Event reducer.** In `client/lib/playback.js`, add a `navidrome-auth` case mirroring the SoundCloud shape: `signed_in` sets `signedIn/server/name` and clears `error`; `signed_out` resets the mirror; `error` sets the message and `signedIn = false`; return a toast for `error` only.
- [ ] **Step 6: Recommendations and recents.** Navidrome feeds are already provider-scoped by `Ids.providerOf`; verify `tst_recommendations` still passes and add a Navidrome row. Add a `forgetNavidromeRecents` helper only if sign-out should purge recents (recommended: yes, matching Spotify), wired in `Personal.qml`'s watcher.

### Task 9: QML surfaces

**Files:**
- Modify: `client/chrome/TitleBar.qml`, `client/pages/SettingsPage.qml`, `client/App.qml`, `client/pages/HomePage.qml`, `client/pages/SearchPage.qml`, `client/pages/LibraryPage.qml`, `client/chrome/NowPlaying.qml`, `client/mini/MiniPlayer.qml`, `client/Downloads.qml`, `client/components/Menu.qml`, `client/pages/AlbumPage.qml`, `client/pages/ArtistPage.qml`, `client/pages/PlaylistPage.qml`
- Create: `client/components/NavidromeGate.qml`

- [ ] **Step 1: Provider pill.** Add `{ key: "navidrome", icon: "navidrome" }` to `TitleBar.qml`'s model. Five pills now share `Style.sp(7)` each — verify the title bar at the minimum window width and shrink the pill width only if it clips.
- [ ] **Step 2: Settings → Account.** Add a Navidrome section after TIDAL: heading, icon, status line, and either (a) when signed out, three `TextInput`s (server, username, password with `echoMode: TextInput.Password`) plus a "Connect" `Pill`, or (b) when signed in, the server + username and a "Sign out" `Pill`. Model the row on the TIDAL section (`SettingsPage.qml:1193-1235`). Clear the password field after a successful connect and after an error. Add `onParamsChanged` so a deep link can open the account section while Settings is already mounted.
- [ ] **Step 3: Account popover.** Add the Navidrome identity/action rows to `App.qml`'s `accountMenu` (and fix the already-missing SoundCloud row if it is still absent).
- [ ] **Step 4: Gate.** `NavidromeGate.qml`: icon, "Connect your music server", one sentence, and a button that does `Router.push("settings", { section: "account" })`. Use it in `HomePage`'s empty state (alongside the TIDAL branch), and as a real gate in `SearchPage` and `LibraryPage` (`navidromeGate` mirrors `spotifyGate`, including the early returns in `runSearch`/`load`/`refreshPlaylists`).
- [ ] **Step 5: Library specifics.** The Songs tab and Insights stay YouTube-local; the Albums/Artists/Playlists tabs show the server collections. Ensure the empty-state copy is not "Sign in to Google" when Navidrome is selected.
- [ ] **Step 6: Now-playing labels.** Add Navidrome to the `NowPlaying.qml` eyebrow and replace the Spotify-vs-YouTube ternaries in `mini/MiniPlayer.qml` with `Ids.sourceGlyph`/`Ids.providerOf` so provider ids stop mislabelling.
- [ ] **Step 7: Downloads.** `Downloads.qml`: classify `nd:` ids with an explicit "Navidrome downloads are not supported yet." reason so the button disables with a reason instead of failing a job. Add the same refusal to the album "Download album" action.
- [ ] **Step 8: Share.** These pages hardcode `music.youtube.com` share URLs: `components/Menu.qml`, `AlbumPage.qml`, `ArtistPage.qml`, `PlaylistPage.qml`. Suppress Share for `nd:` ids in v1 rather than emitting a broken YouTube link.

### Task 10: Tests, docs and release hygiene

**Files:**
- Modify: `client/tests/tst_ids.qml`, `client/tests/tst_playback.qml`, `client/tests/tst_recommendations.qml`, `client/tests/tst_personal.qml` (if purging), `CHANGELOG.md`, `README.md`, `docs/ARCHITECTURE.md`
- Modify: `crates/ryotunesd/src/downloads.rs` (refusal test only)

- [ ] **Step 1: QML tests.** Add `nd:` cases to `tst_ids.qml` (providerOf, sourceGlyph, and keep `"navidromeish"`/`"tidalish"` falling through to YouTube). Add a `navidrome-auth` reducer test to `tst_playback.qml` (signed_in, signed_out, error, and that a password is never mirrored). Add a Navidrome provider-scoping case to `tst_recommendations.qml`.
- [ ] **Step 2: Rust tests.** The crate, bridge, id helper, threshold and dispatch guards all have unit tests. Add a `classify()` test in `downloads.rs` asserting an `nd:` id is refused with the intended message.
- [ ] **Step 3: Docs.** `CHANGELOG.md` under Unreleased: one paragraph naming Navidrome as a music-server provider (browse/search/play/star/scrobble/playlists/lyrics), token auth, one server, and that downloads/transcoding are not included. Update `README.md`'s provider list (currently "YouTube, Spotify and SoundCloud", already stale for TIDAL) and the architecture provider row. Add the new crate to `docs/ARCHITECTURE.md`'s crate table.
- [ ] **Step 4: Full gates.**
  ```bash
  cargo fmt --all -- --check
  cargo test --workspace --locked
  cargo check --workspace --locked
  scripts/release-check.sh
  cd client && qs -c ryotunes  # manual smoke: Home, Library, Search, album, playlist, now playing
  ```

## Verification checklist (manual smoke)

1. Sign in to a Navidrome server over `http://` LAN and over `https://`; a wrong password shows an error and stores nothing.
2. Home shows the server's shelves; an empty library shows an empty state, not an error.
3. Search returns songs/albums/artists/playlists; opening each detail page works.
4. Play a track from album, playlist and search; seek; next/previous across the queue.
5. Heart a track → it becomes starred on the server (verify in Navidrome's web UI). Unstar works.
6. Let a track pass the threshold → the server play count increments by one, not two.
7. Lyrics panel shows server synced lyrics when available and falls back to LRCLIB otherwise.
8. New playlist, rename, add a track, remove one of two duplicate tracks (the right index goes), sort, delete.
9. Reload the app → still signed in; cover art and playback still work.
10. Sign out → Home/Library/Search fall to the gate; nothing on the server changes.
11. Select another provider and back; no cross-provider recents or recommendations leak.
12. Download button on a Navidrome track explains it is unsupported; Share is absent.

## Risks and open points

- **Token in artwork URLs.** Cover URLs contain `u/s/t` and can persist inside queue/personal JSON. A server password change invalidates them; covers fall back to the placeholder until re-browsed. Stream URLs are always re-resolved. A daemon-side image proxy would remove this entirely but is out of scope.
- **Non-Navidrome Subsonic servers** (gonic, LMS, Nextcloud Music) vary in optional fields and extension support. `#[serde(default)]` everywhere plus the capability check for lyrics keeps v1 tolerant; a server that requires `p=` or API keys will fail with a clear message.
- **Large libraries.** Album pages are capped at 2000. The Artists tab loads the whole flat list in one call, as `getArtists` does; if a real library proves too large, page it later.
- **TLS.** Self-signed HTTPS is not supported in v1; use a trusted certificate or LAN HTTP. A per-server "allow invalid certificate" toggle is a deliberate non-goal.
- **Transcoding.** `format=raw` asks for the original file. Servers configured to force transcoding still answer with what they have; bitrate/format selection is a follow-up.
- **Cover size.** `getCoverArt?size=` is a hint; servers may return larger art. The QML artwork cache bounds decode, so this is acceptable.
- **Playlist reorder** rewrites the full song list; a very large playlist is one large `createPlaylist` call. Acceptable for v1, note it if it becomes slow.
