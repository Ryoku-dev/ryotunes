# ryotunes-spotify

A Spotify provider for Ryotunes: OAuth sign-in, rich metadata, and audio streamed into Ryotunes'
single libmpv engine.

## Origin & license

Lifted from **[nolight132/sonora](https://github.com/nolight132/sonora)**, `crates/music/src/spotify`,
commit **695242e**. Sonora is **GPL-3.0-or-later**; so is Ryotunes and so is this crate. librespot
itself is MIT.

Streaming uses **librespot 0.8** with nolight132's librespot-audio fix. The workspace pins the whole
librespot set to that fork via `[patch.crates-io]` in the root `Cargo.toml`
(`github.com/nolight132/librespot`, rev `e7eb953d5848fd97bacd00e6c0e33500765eaa63`).

### Lifted (near-verbatim, only `crate::spotify::` → `crate::` path rewrites)

`pb.rs`, `wire.rs`, `collection.rs`, `collection2.rs`, `albums.rs`, `artists.rs`, `playlists.rs`,
`profiles.rs`, `search.rs`, `radio.rs`, `pathfinder.rs` + `pathfinder/{album,artist,browse,hashes,plays,search}.rs`,
and the domain types in `models.rs`. These carry Sonora's tests (protobuf/pb-codec/wire/pathfinder/
hash-registry parsing), which run offline.

### Rewritten / adapted

- **`models.rs`** — trimmed to the types the Spotify code produces. `Lyrics::plain` no longer
  romanizes (Sonora pulled in kakasi/deunicode; the `romanized` fields stay in the shapes but are
  never populated here).
- **`lyrics.rs`** — synced lines are normalized locally (sort + close open ends) instead of Sonora's
  970-line LRC pipeline. Spotify's color-lyrics payload is already clean, so the heavy pass (and its
  kakasi dependency) bought nothing.
- **`auth.rs`** — Sonora opened a browser or printed the URL to stdout and blocked. A daemon can do
  neither, so the PKCE authorization-code flow is driven here (via `oauth2`, which librespot-oauth
  wraps): the authorization URL is handed to a caller-supplied sink, and the loopback redirect is
  caught by a one-shot listener. The credential cache, the Premium gate, and error classification are
  Sonora's.
- **`client.rs`** — Sonora's `impl MusicApi for LibrespotClient` became inherent async methods on
  `Client`; `search` returns one `SearchResults` bundle; the client owns the streaming engine.
- **`provider.rs`** — Sonora's `impl MusicProvider` became `SpotifyProvider::{new, restore, sign_in,
  sign_out}` returning a `Client`.
- **`stream.rs`** — *replaces* Sonora's `playback.rs` + `sink.rs` + `audio.rs` (a cpal+rodio sink).
  See below. No cpal, no rodio.
- **`credentials.rs`** — a small root-directory holder instead of Sonora's `dirs`-based cache module.

## Audio: pipe → FIFO → mpv

Ryotunes plays everything through one libmpv instance (tempo/pitch/fx on its filter chain), so
Spotify audio must reach mpv, not a soundcard. One librespot `Player` per `Client` decodes and writes
**raw S16LE / 44100 Hz / stereo** through librespot-playback's built-in `pipe` backend into a
per-session FIFO at `$XDG_RUNTIME_DIR/ryotunes/spotify-<pid>.pcm`.

The daemon points mpv at the FIFO with the rawaudio demuxer:

```text
loadfile <StreamHandle::fifo_path()> replace \
  demuxer=rawaudio,demuxer-rawaudio-format=s16le,\
  demuxer-rawaudio-rate=44100,demuxer-rawaudio-channels=2
```

`StreamHandle` exposes `fifo_path()`, `seek(Duration)`, `play()`, `pause()`, and
`next_event() -> Option<StreamEvent>` (`Playing | Paused | Position | EndOfTrack | Unavailable`,
mapped from librespot's `PlayerEvent`).

### Pause caveat (important for the daemon)

librespot **closes the pipe's write end when it pauses** (`handle_pause` → `ensure_sink_stopped`),
which mpv reads as end-of-file. So the daemon's normal pause should **pause mpv**, not call
`StreamHandle::pause()`: with mpv stopped reading, the FIFO fills and librespot's write blocks
(backpressure), keeping the pipe open. Use `StreamHandle::pause()`/`play()` only when a real
teardown/reload is acceptable. Gapless is **off** for v1 (each track is a fresh `loadfile`).

Endianness: librespot emits native-endian S16, i.e. little-endian on Ryotunes' Linux x86-64 target,
matching `s16le`.

## Type mapping → `ryotunes_core::innertube` (`SongItem`)

The provider returns Sonora's richer types (`Track`, `Album`, `Artist`, `Playlist`, `Lyrics`). The
daemon converts a `Track` into the `SongItem`-like shape the existing InnerTube commands already
return, so Spotify rides the same command surface:

| `SongItem` field | from `ryotunes_spotify::Track` |
| --- | --- |
| `video_id` | `"spotify:track:" + track.id` (the id is the Spotify base62; `None` ⇒ unplayable, skip) |
| `title` | `track.name` |
| `artists` | `track.artists` (already joined; `track.artist_refs` has per-artist ids) |
| `artist_id` | first `track.artist_refs[].id` → `"spotify:artist:<id>"` |
| `album` | `track.album` |
| `album_id` | `track.album_id` → `"spotify:album:<id>"` |
| `duration` | `track.duration` (format as `m:ss`) |
| `thumbnail` | `track.cover` |
| `explicit` | `track.explicit` |

Albums/artists/playlists map the same way (`id` → `spotify:album:`/`spotify:artist:`/
`spotify:playlist:`). `SearchResults.artists` is derived from the track hits' `artist_refs` (Spotify's
Pathfinder search has no artist entity), so it lists the artists the result tracks credit.

**Id direction:** this crate *returns* bare base62 ids (`Track.id`, `Album.id`, …) and the daemon
prefixes them to `spotify:` URIs on the wire (above). Going the other way, the client methods
(`album`, `artist`, `playlist`, `track`, `lyrics`, …) *take* bare base62 ids, so the daemon strips the
`spotify:<kind>:` prefix from a stored URI before calling — e.g. `id.rsplit_once(':').map_or(id, |(_, t)| t)`.
`Client::stream` is the exception: it accepts either a bare id or a full `spotify:track:` URI, so a
queued item's id can be played back as-is.

Lyrics: `Lyrics::Synced { lines }` carries per-line `start`/`end` and, when Spotify provides syllable
timing, `words: Vec<LyricsWord { start, end, text }>`.

## Public API

```rust
let provider = SpotifyProvider::new(data_dir); // credentials under <data_dir>/spotify
// restore cached credentials, if any:
if let Some(client) = provider.restore().await? { /* ... */ }
// or sign in; `on_url` receives the URL to open in a browser:
let client = provider.sign_in(|url| send_url_to_client(url)).await?;
provider.sign_out();

let results = client.search("radiohead").await?;      // SearchResults { tracks, albums, artists, playlists }
let detail  = client.album("<id>").await?;            // AlbumDetail
let artist  = client.artist("<id>").await?;           // Artist { info, popular (top_tracks), releases (albums) }
let pl      = client.playlist("<id>").await?;         // PlaylistDetail
let liked   = client.liked(0).await?;                 // page 0 of saved tracks
let home    = client.home().await?;                   // HomeFeed
let lyrics  = client.lyrics("<track_id>").await?;     // Option<Lyrics>
let mut s   = client.stream("<track_id>")?;           // StreamHandle
```

`crates/core::spotify::SpotifyState` wraps the provider + current client behind a `tokio::sync::RwLock`
(`restore` / `sign_in` / `sign_out` / `status` / `client`) for the daemon.

## Daemon wiring (to add in `crates/ryotunesd/src/methods.rs`)

Spotify is an **additional** provider alongside YouTube Music, never a replacement — the InnerTube
path is untouched. The daemon holds a `provider` selector (`"youtube" | "spotify"`, set by a top-bar
switch) and routes the shared search/browse/library commands by it; `SpotifyState` (in
`core::spotify`) supplies the Spotify side. `methods.rs` is owned by another worker right now, so this
crate wires **no** daemon methods.

Two ways to expose Spotify, both fine — pick per the daemon's existing shape:

- **Routed:** the existing `search`/`browse_album`/`liked`/… methods read `provider` and dispatch to
  `SpotifyState::client()` vs the InnerTube path.
- **Namespaced:** dedicated `spotify_*` methods (below), with `provider` still gating which the UI
  calls.

Because both catalogues share one command surface and one queue, **every id the daemon emits for a
Spotify item MUST be a `spotify:` URI** (`spotify:track:…`, `spotify:album:…`, `spotify:artist:…`,
`spotify:playlist:…`), built from the bare base62 ids this crate returns (see the mapping table).
YouTube ids (`videoId`, `UC…`, `MPRE…`) never carry that prefix, so the two catalogues can never
collide, and `spotify_play` / `stream` can tell a Spotify track from a YouTube one by the prefix.
Suggested JSON-RPC method names and shapes:

| method | params | result |
| --- | --- | --- |
| `spotify_status` | `{}` | `{ "signed_in": bool, "stored": bool }` |
| `spotify_sign_in` | `{}` | streams `{ "auth_url": string }` first (from the `on_url` sink), then `{ "signed_in": true }` on success |
| `spotify_sign_out` | `{}` | `{ "signed_in": false }` |
| `spotify_search` | `{ "query": string }` | `{ "tracks": SongItem[], "albums": Album[], "artists": Artist[], "playlists": Playlist[] }` |
| `spotify_album` | `{ "id": string }` | `{ "album": Album, "tracks": SongItem[] }` |
| `spotify_artist` | `{ "id": string }` | `{ "info": Artist, "popular": SongItem[], "releases": Album[] }` |
| `spotify_playlist` | `{ "id": string }` | `{ "playlist": Playlist, "tracks": SongItem[] }` |
| `spotify_liked` | `{ "page": u32 }` | `{ "tracks": SongItem[] }` |
| `spotify_play` | `{ "track_id": string }` | `{ "fifo": string }` — then `loadfile` that FIFO into mpv with the rawaudio options above |

`spotify_sign_in` should push the `auth_url` to the client (e.g. an event/notification) from inside
the `on_url` callback, since `sign_in` only resolves after the browser round-trip completes.
`SongItem`/`Album`/`Artist`/`Playlist` above are the daemon's existing InnerTube shapes, filled via
the mapping table.

## Testing

```sh
CARGO_TARGET_DIR=/home/nero/Work/ryotunes/target cargo test -p ryotunes-spotify
```

All unit tests are offline (protobuf/pb-codec/wire/pathfinder-request/hash-registry parsing, lyric
normalization, event mapping). Nothing here contacts Spotify.

### Manual verification (needs a Spotify **Premium** account)

librespot streaming requires Premium and cannot be exercised in unit tests. To verify end to end:

1. Build a tiny harness (or use the daemon once `spotify_*` methods are wired) that calls
   `SpotifyProvider::new(<tmp dir>)` then `sign_in(|url| println!("{url}"))`.
2. Open the printed URL in a browser, log in, approve. The loopback listener on
   `127.0.0.1:8989/login` catches the redirect; `sign_in` returns a `Client`.
3. `client.profile().await` → your display name. `client.search("...")`, `client.album(id)`,
   `client.liked(0)` → non-empty, well-formed data.
4. `client.lyrics(track_id)` on a track with lyrics → `Some(Lyrics::Synced { .. })` with word timing.
5. `let mut h = client.stream(track_id)?;` then `mpv --demuxer=rawaudio
   --demuxer-rawaudio-format=s16le --demuxer-rawaudio-rate=44100 --demuxer-rawaudio-channels=2
   "$(printf '%s' "$(h.fifo_path())")"` — audio plays. `h.seek(30s)` jumps; watch `h.next_event()`
   emit `Playing`/`Position`/`EndOfTrack`.
6. A non-Premium account must fail `sign_in` with `SignInFailure(SignInProblem::Premium)`.
