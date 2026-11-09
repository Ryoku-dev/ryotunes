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
  player/       libmpv: load, pause, seek, volume, gapless prefetch, audio fx, params,
                events to the bus, friendly error mapping
  queue/        the queue: manual items, continuation, shuffle, repeat, stop after current,
                restore; asks a Resolver for streams and a Continuer for more
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

`log/slog`, JSON to `$XDG_STATE_HOME/ryotunes/ryotunesd.log` with size
rotation, level from `RYOTUNES_LOG` (default `warn`). Every repairable
playback wedge logs one `info` line that names the repair. User-visible
errors are plain sentences in `internal/daemon/copy.go`; packages return
typed errors and the handler picks the sentence.

## The client

`internal/client` is a Gio application in Lithic.

```text
internal/client/
  lithic/      the Lithic kit for Gio: roles from the Ryoku palette, cut primitives,
               type, plaque, tablet, field, search, toggle, slider, tabs, filters,
               menu, popover, dialog, placard, tooltip, chart, flake, blade, list
  conn/        socket client: typed calls, event stream, reconnect, snapshot replay
  state/       reducers: playback, queue, settings, auth, downloads, personal,
               lyrics, listen together; one Store with Invalidate hooks
  art/         image loading: http, decode off the UI goroutine, downscale, LRU,
               collage for device playlists
  ui/          the window: rail, panes, router, pages, chrome, mini window
```

- One `Store` holds the daemon snapshot plus client-only state (route stack,
  panel modes, zoom). Reducers are pure functions of (state, event); the
  frame loop drains a channel and invalidates.
- Pages are retained structs with their own widget state (`lithic.List`,
  clicks, tweens), composed from the kit; they never draw a colour or a
  radius of their own.
- The mini player is a second `app.Window` in the same process titled
  `Ryotunes Mini`; the main window is titled `Ryotunes`. Both read the same
  `Store`.
- `ryotunes --png out.png [--demo]` renders one frame headlessly, for review
  and for the docs.

### Lithic

The look is specified in the Ryoku repository, `docs/lithic/`. The client's
`lithic` package implements it: roles are filled from the live palette
(`shell.json` `themePalette`, then `colors.json` while `theme.json` follows
the wallpaper, then the signature fallback), watched with inotify. No colour
value exists in the client outside the signature fallback and the three
fixed minerals. Appearance offers Follow desktop, Dark and Light, density,
reduced motion and an optional palette file (a Material role set, the format
`colors.json` uses), which is what a Ryotunes skin is now.

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
icons, the metainfo and the matugen template. It never removes
`ryoku-desktop`. CI runs `gofmt`, `go vet`, `go test ./...`, `go build` with
`CGO_ENABLED=0` for the three pure binaries and the Gio client with cgo, the
release gates in `release/check.sh`, and the glibc ceiling check.

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
