# Ryotunes architecture

Ryotunes is a Linux music player for Ryoku. It plays YouTube Music, Spotify,
SoundCloud, Tidal, internet radio and local files through one native player,
and it is drawn in Lithic, the Ryoku UI language. This document is the
contract between its parts. Read it before touching any package.

Everything is Go. The desktop is a Gio application; playback and every
provider live in a daemon that survives the window. The repository is the
source of truth and the packages under `release/` are the only way it reaches
a machine.

## Processes

```text
Ryoku / Hyprland
   │  Super+J, MPRIS, media keys, tray, live palette
   ▼
ryotunes (Gio client, one process, two windows: main and "Ryotunes Mini")
   │  unix socket, newline-delimited JSON, requests + events
   ▼
ryotunesd (daemon)
   ├── player        libmpv through purego, audio only
   ├── providers     youtube · spotify · soundcloud · tidal · local · radio
   ├── library       sqlite: settings, personal, device playlists, history, caches
   ├── downloads     yt-dlp workers, history, metadata and companions
   └── integrations  mpris · tray · discord · last.fm · listen together · self-update

ryotunes-cli         one method per call, or an event tail; what scripts and the shell use
ryotunes-sync        the optional Listen Together room relay (a server, not installed by default)
```

| Binary | Package | cgo | Why |
|---|---|---|---|
| `ryotunesd` | `cmd/ryotunesd` | no | libmpv is loaded with purego, sqlite is modernc, the YouTube player JS runs in goja |
| `ryotunes` | `cmd/ryotunes` | yes | Gio needs Wayland, X11 and EGL |
| `ryotunes-cli` | `cmd/ryotunes-cli` | no | |
| `ryotunes-sync` | `cmd/ryotunes-sync` | no | |

The daemon never draws, the client never talks to a provider. The CLI is a
thin socket client. Keep it that way: a feature that needs both ends is a
method and an event, not a shared package.

## Lifecycle

- Socket `$XDG_RUNTIME_DIR/ryotunes/ryotunesd.sock` in a `0700` directory
  created under `umask 077`. One daemon per user, held by an `flock` on
  `ryotunesd.sock.lock`. A second `ryotunesd` connects to the incumbent,
  calls `show`, exits 0.
- `ryotunesd.socket` (systemd user unit) activates the daemon on the first
  connection; the daemon also runs unactivated, and `ryotunes` starts the
  socket unit or the daemon directly on a cold launch.
- The daemon exits after a bounded idle (nothing playing, no subscriber, no
  queued or active download, no Listen Together room) of five minutes.
  Explicit `quit` stops playback, unregisters MPRIS, tears integrations down
  and exits.
- `ryotunes` is single instance: when a client is already subscribed, a
  second launch asks the daemon to `show` and exits; the daemon raises the
  subscribed client through the `show` event. Closing the window never stops
  playback.

## Wire protocol

`internal/protocol` is the one definition of the wire. Both binaries import
it; nothing else defines a method name, a param shape or an event payload.

- Request `{"id":12,"method":"play","params":{...}}`, response
  `{"id":12,"result":...}` or `{"id":12,"error":{"code":"...","message":"..."}}`,
  event `{"event":"position","data":{...}}`. One JSON object per line.
- Method names and payload field names are those of Ryotunes 1.x, so
  `ryotunes-cli` scripts, the Ryoku shell and RyoStore keep working. New
  methods follow the same style: `snake_case` names, `camelCase` or
  `snake_case` fields exactly as `internal/protocol` declares them.
- `protocol.Methods` is the typed registry: each method has a params type, a
  result type and a doc string. The daemon's dispatcher and the client's
  typed calls are generated from it, so adding a method is one entry plus
  one handler.
- Events go only to connections that called `subscribe`. The `subscribe`
  result is the full snapshot (playback, queue, settings, auth for every
  provider, downloads, personal, Listen Together state) so a client restores
  without a second round trip.

## The daemon

`internal/daemon` wires the parts; no logic of its own beyond the method
handlers, which are thin: validate params, call the owning package, emit.

```text
internal/
  protocol/     wire types, method registry, event names (shared)
  model/        Track, Album, Artist, Playlist, Shelf, Lyrics, Stream, ids (shared)
  bus/          EventSink: Emit(name, payload) fan-out to subscribers
  store/        sqlite (modernc) and the json files: settings, personal, device playlists,
                history, caches; every table and key documented in docs/PERSISTENCE.md
  player/       libmpv: load, pause, seek, volume, gapless prefetch, audio fx, the
                equalizer chain, params, events to the bus, friendly error mapping
  eq/           the parametric equalizer: limits, presets, Equalizer APO and AutoEQ
                text, the mpv filter chain and the response maths the client draws
  queue/        the queue: manual items, continuation, shuffle, repeat, stop after current,
                restore; plays a completed download from its file before asking a
                Resolver for streams, and a Continuer for more
  provider/     the Provider interface and Registry
    youtube/    InnerTube clients, visitorData, cipher (goja), player resolve chain,
                yt-dlp fallback, browser-cookie sign-in, browse parsers
    spotify/    go-librespot session, loopback stream server for mpv, web api browse
    soundcloud/ client_id discovery, api v2, stream transcodings, cookie sign-in
    tidal/      OAuth, api, playbackinfo manifests
    local/      folder scan, tags, local playlists, watch
    radio/      radio-browser directory, live streams
  lyrics/       providers, lrc parsing, cache, romanize/
  downloads/    queue, workers, yt-dlp argv, staging and publication, metadata
  integrations/ mpris/ tray/ discord/ lastfm/ listentogether/ (ltproto/ is the shared room protocol)
  diag/         the log ring and file, get_logs, client error intake
  update/       release lookup, verification, pacman / xbps / dnf paths
  spawn/        starts the window and browser handoffs in their own systemd scope, so
                stopping ryotunesd.service never takes them with it
  daemon/       App, Server, Methods, Lifecycle, Diagnostics, Logs
```

### Provider contract

```go
type Provider interface {
    ID() model.Provider
    Capabilities() Capabilities            // what the client may offer for this source
    Home(ctx, more string) (model.Feed, error)
    Search(ctx, q string, kind model.SearchKind, page string) (model.SearchPage, error)
    Suggest(ctx, q string) ([]string, error)
    Album(ctx, id string) (model.Album, error)
    Artist(ctx, id string) (model.ArtistPage, error)
    Playlist(ctx, id string, more string) (model.Playlist, error)
    Library(ctx) (model.Library, error)
    Resolve(ctx, trackID string, q model.Quality) (model.Stream, error)
    Next(ctx, trackID string, played []string) ([]model.Track, error)
    Account(ctx) (model.Account, error)
}
```

Optional interfaces (`Rater`, `PlaylistEditor`, `AlbumSaver`, `Lyricist`,
`SignIn`) are asserted by the daemon, and `Capabilities` tells the client
which controls to show. A provider never touches the player, the store or
the bus; it returns model values and the daemon does the rest. Provider
errors are typed (`provider.ErrSignInRequired`, `ErrUnavailable`,
`ErrBotGated`, `ErrNotFound`) so the queue can decide between halting and
skipping.

### Streams

`model.Stream` is what mpv plays: a URL, headers, a user agent, an optional
expiry and the codec/bitrate for the UI. YouTube, SoundCloud, Tidal and
radio return a remote URL. Spotify returns a loopback URL: the Spotify
package serves the decrypted Ogg Vorbis over `127.0.0.1` with `Range`
support so mpv seeks like any file. Local returns a `file://` URL. The
player never knows which provider it is playing.

### Sessions

Each provider's session survives the daemon being off for any length of time;
only the provider rejecting it, or the user signing out, ends it.

- YouTube Music and SoundCloud sessions belong to the user's browser. The
  daemon stores the cookies it imported and which store and container they
  came from (the store's kind is persisted by name, and the path must be a
  regular `cookies.sqlite` or `Cookies` file under a root `Discover` walks),
  and follows that browser: once in the background at start, with a
  deadline, and whenever the provider rejects the session, it re-reads the
  remembered store and adopts the current cookies if they prove the same
  account. Proof is the channel id, email or handle, whichever both sides
  carry, else the identity of the account the server marks selected at sign
  in, compared with the one it marks selected for the browser's session; a
  display name never proves an account. A sign out during a proof wins: the
  session keys are written under one short lock and re-checked before a
  re-adoption commits. Subscribers are answered from memory and a change
  arrives as an `auth-changed` or `soundcloud-auth` event (`restored` for a
  quiet re-adoption). When the store offers nothing newer, YouTube asks the
  account menu before giving up the session, so a 403 that is not about the
  session keeps it, and a 403 for a remembered channel that the account menu
  refuses while it accepts the account itself drops only that channel. A
  SoundCloud token that merely expired is kept with its source until the
  browser offers the next one: the client hears `stale` once (signed out, no
  toast), public requests go without the token, and only a 401 or 403 ends
  it, as `session_expired`. Only the SoundCloud access token is stored, never
  the browser's refresh token or session cookie. It never refreshes or rotates
  a browser's tokens itself, because both copies share one session and the
  browser would be signed out. Store copies live in `$XDG_RUNTIME_DIR/ryotunes`
  when that directory is this user's and mode 0700 (else a private temporary
  directory), and copies older than 15 s are swept at start.
- Spotify (go-librespot reusable credentials, with a persistent 40 hex device
  id) and TIDAL (OAuth device flow, refresh token) are the daemon's own
  sessions. Refreshes are single-flight and persisted before use, and a
  restore is bounded by a deadline and announced as an event when it finishes
  after a client subscribed. A failed refresh keeps the session unless the
  provider says the grant is invalid: for TIDAL that is `invalid_grant` or
  `access_denied`, while a refused client id or secret keeps the refresh
  token, and a refresh answer with a blank refresh token keeps the old one.
  An API 401 refreshes the TIDAL token only when its `subStatus` names the
  token, and not again for a renewed token TIDAL refused, until it expires.
  A dropped Spotify connection is recovered from the stored credentials:
  once per session when Spotify refuses them, and again on the next request
  when the network was unreachable or timed out, as after a resume. When the
  Premium check after a Spotify login is rate limited, fails with a 5xx or
  answers unreadably, the session is kept with the product unknown and the
  check runs again at the next Spotify request after `Retry-After` (a minute
  by default, an hour at most); only a non-Premium answer then refuses playback.
- 1.x's Spotify session (`spotify/spotify/credentials.json` and `device.id`
  under the data directory) is migrated once at start and never written by
  2.x; its UUID device id becomes the SHA-1 of its text. An unreadable 1.x
  file is logged as a warning and skipped. Sign out removes both credential
  copies, and a marker keeps a later start from migrating the 1.x copy again.

YouTube learns what to recommend from plays reported to the signed-in
account, so the daemon reports a YouTube play the way the web player does,
unless Watch history is off in Settings. The report goes out once per play,
when mpv has opened the file and it is actually playing: a track skipped
before it loads, a prefetch, a seek or a resume never counts, and a Listen
Together sync of the same track does not reload it. It is the player
response's `videostatsPlaybackUrl`, sent only to an https `youtube.com` host
with the session's cookies and SAPISIDHASH. The ping comes from the resolve
that just ran when that is under 15 minutes old, and otherwise from one
lightweight `player` request, so a cached stream or a downloaded file counts
too, and an old response is never replayed under another account. Rows
queued by hand or by autoplay carry no playlist context.

### Ids on the wire

Every track id tells its provider: `spotify:track:…` and `spotify:…` are
Spotify, `sc:…` is SoundCloud, `tidal:…` is Tidal, `LOCAL…` is a local file,
`RYOTUNES_RADIO:…` is a radio station, `RYOTUNES_*` are Ryotunes' own
collections, and anything else is a YouTube id. `model.ProviderOf(id)` is
the one function that decides.

### Concurrency

The daemon is goroutine-per-request with one `App` mutex around queue and
playback state; provider calls run outside the lock. The player's event loop
is one goroutine that publishes to the bus. Long work (downloads, library
scans, Listen Together) runs in its own package with its own locks and
publishes events; it never calls back into `App` under a lock.

### Errors and logging

`log/slog`, JSON to `$XDG_DATA_HOME/dev.ryoku.ryotunes/logs/ryotunesd.log` with
size rotation, level from `RYOTUNES_LOG` (default `warn`). Every repairable
playback wedge logs one `info` line that names the repair. User-visible
errors are plain sentences in `internal/daemon/copy.go`; packages return
typed errors and the handler picks the sentence.

## The client

`internal/client` is a Gio application drawn with the shared Lithic library
(`github.com/ryoku-dev/ryoku/docs/lithic/library/go`).

```text
internal/client/
  conn/        socket client: typed calls, event stream, reconnect, snapshot replay
  state/       reducers: playback, queue, settings, auth, downloads, personal,
               lyrics, listen together; one Store with Invalidate hooks; the
               client lock and preferences
  art/         image loading: http, decode off the UI goroutine, downscale, LRU,
               collage for device playlists, and each cover's palette (vibrant and
               dominant) behind the accent and the window tint, with the blurred
               sample behind the cover backdrop; network covers and palettes
               persist in a 256 MB disk cache under the user cache dir
               (ryotunes/art), refetched in the background after 30 days
  spectrum/    the visualizer feeds: cava bands and pw-record stereo samples,
               each running only while a visible visualizer wants it
  ui/
    look/      the app theme on the library: accent, cover tint and source colours,
               pane corners, the Ryoku mark, provider glyphs, icons, frame geometry,
               overlay host, tips, wrapping choice chips, exact scrolling, scrubbers,
               artwork and its blurred backdrop, visualizer styles and beat, karaoke lyrics, equalizer graph
               and faders, playback motion
    env/       what every surface receives: theme, store, connection, router,
               artwork, feeds, overlays, toasts
    act/       daemon actions shared by every surface
    widgets/   track table, cards, shelves, grids, quick tiles, hero, menus,
               playlist dialogs, page states, transport, seek row, playing art
    chrome/    top bar, provider switcher, sidebar, bottom bar, player bar,
               sound, side panel (queue, lyrics, together), now playing, mini
    pages/     one package per page (equalizer included)
    shell/     windows and whether the main one opens floating or tiled, frame,
               router, overlays and keys, theme reconciling, single instance
```

- One `Store` holds the daemon snapshot plus client-only state. Reducers are
  pure functions of (state, event); the frame loop drains a channel and
  invalidates.
- Pages are retained structs with their own widget state, composed from the
  library's `kit` and `widgets`; they never draw a colour of their own.
- The client holds an exclusive lock on `$XDG_RUNTIME_DIR/ryotunes/client.lock`
  while it runs. A second launch asks the daemon to `show` and exits; the
  daemon's `Show` spawns `ryotunes` when nobody holds the lock.
- The mini player is a second `app.Window` in the same process titled
  `Ryotunes Mini`; the main window is titled `Ryotunes`. Both read the same
  `Store`.
- `ryotunes --png out.png [--demo]` renders one frame headlessly, for review
  and for the docs.

### Lithic

The look is specified in the Ryoku repository, `docs/lithic/`, and drawn by
its Go library. The library fills roles from the live palette (`shell.json`
`themePalette`, then `colors.json` while following the wallpaper, then the
signature fallback) and watches it with inotify. The client adds the provider
brand colours and the Ryoku orange in `look/brand.go`, the only colour
literals it has. Appearance offers Follow Ryoku, Dark and Light, density,
reduced motion and an optional palette file (a Material role set, the format
`colors.json` uses). `docs/DESIGN.md` explains how the language maps to the
music player.

## Persistence

`$XDG_DATA_HOME/dev.ryoku.ryotunes/` is the data directory, unchanged from
Ryotunes 1.x so a user's library, device playlists, settings and sign-ins
survive the rebuild. It holds `ryotunes.sqlite` (settings, personal, device
playlists, history, play counts, stream and lyrics caches, local index; the
1.x schema with additive migrations only), `downloads.json`, `logs/`,
`covers/playlists/`, and the provider credentials (`spotify/credentials.json`,
`spotify/device.id`, the YouTube, SoundCloud and Tidal tokens as settings
rows). `$XDG_CACHE_HOME/dev.ryoku.ryotunes/audio` is the mpv disk cache.
`$XDG_CONFIG_HOME/ryotunes/` holds user palette files. The schema and every
settings key are in `docs/PERSISTENCE.md`. Files are written to a temporary
name and renamed.

## Packaging

`release/` builds the Arch package (`ryotunes 1:<version>-1`, epoch kept),
the Void depends and installer, the AUR `-bin` recipe. The package installs
the four binaries, the systemd user units and preset, the desktop entry,
icons, the metainfo, the matugen template, and `LICENSE`, `NOTICE` and
`THIRD_PARTY_NOTICES` under `/usr/share/licenses/ryotunes/`. It never removes
`ryoku-desktop`. `release/generate-third-party-notices.go` writes the
third-party notices from the modules the binaries link. CI runs `gofmt`, `go vet`, `go test ./...`, `go build` with
`CGO_ENABLED=0` for the three pure binaries and the Gio client with cgo, the
release gates in `release/check.sh` (the notices must be current), and the glibc ceiling check.

## Development

```sh
go build ./...            # everything
go test ./...             # unit tests, no network
go run ./cmd/ryotunesd    # a daemon against $XDG_RUNTIME_DIR; set XDG_* to sandbox it
go run ./cmd/ryotunes     # the client; --png out.png --demo renders offline
go run ./cmd/ryotunes-cli hello
```

Sandbox a daemon with `XDG_RUNTIME_DIR=/tmp/rt/run XDG_DATA_HOME=/tmp/rt/data`
so it never touches the user's library or socket. Headless renders need no
display.
