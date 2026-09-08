# Ryotunes architecture

Ryotunes is split so that playback survives independently of the visible interface. This is especially important on Ryoku laptops, where keeping a WebKit view active for a song that is already playing wastes CPU, memory and power.

## Process model

The Tauri/Rust process is authoritative for playback and desktop integration. The SvelteKit WebView is a client of that state rather than the clock that drives it.

```text
Ryoku / Hyprland
       │
       ├── MPRIS + media keys + tray
       │             │
       ▼             ▼
  Tauri / Rust ── player crate ── libmpv
       │
       ├── Innertube / library / integrations
       │
       └── visible WebKit UI (SvelteKit)
                    │
                    ├── Home / Search / Library
                    ├── Queue / Lyrics / Now Playing
                    └── Mini player
```

## Crates and binaries

The workspace separates the playback core from whoever renders it. Phase 1 of the native-client plan moved everything in `src-tauri/src` that is not a window, a webview or a Tauri command into `crates/core`, behind three host traits; the daemon, CLI and QML rows below are the targets later phases add.

| Path | Role | Origin |
|---|---|---|
| `crates/innertube`, `crates/player`, `crates/listen-protocol`, `crates/sync-server` | unchanged | existing |
| `crates/core` | `AppState`, orchestrator, db, lyrics, local, discord, lastfm, media (MPRIS), radio, listentogether, cipher, potoken, session (login), settings. No Tauri. Talks to the outside through three traits (4.2) | moved from `src-tauri/src` |
| `crates/protocol` | request/response/event types, `serde` only, shared by daemon, CLI, tests | new |
| `crates/ryotunesd` | binary: socket server, single-instance lock, systemd-friendly lifecycle, GTK thread hosting the hidden WebKitGTK views, tray | new; absorbs `tray.rs`, `webview.rs`, the login part of `session.rs` |
| `crates/ryotunes-cli` | `ryotunes-cli <method> [json]` and `ryotunes-cli events`; the shell's and scripts' entry point | new |
| `client/` | the QML client run by Quickshell: `qs -c ryotunes` (shipped as `/usr/share/ryotunes/client`), plus `client/mini/` for the mini player window | new, replaces `ui/` |
| `src-tauri/` | deleted at the end of phase 4 | removed |

## Daemon

`ryotunesd` hosts `crates/core` behind a unix socket so the interface can be killed without stopping playback. It is a systemd user service, socket-activated by `ryotunesd.socket` on the first client connection, and it exits on a bounded idle (nothing playing and no subscriber) after five minutes.

- Socket: `$XDG_RUNTIME_DIR/ryotunes/ryotunesd.sock`, in a `0700` directory, created under `umask 077`. A single instance is enforced with an `flock` on `ryotunesd.sock.lock`; a second launch connects to the incumbent, asks it to `show`, and exits 0.
- Framing: newline-delimited JSON, one object per line, both directions. Request `{"id":12,"method":"play","params":{"videoId":"…"}}`; response `{"id":12,"result":…}` or `{"id":12,"error":{"code":"…","message":"…"}}`; event `{"event":"position","data":{"position":12.3}}`.
- Methods: every `#[tauri::command]` name and JSON shape, plus the control methods `hello` (returns `{"protocol":1,"daemon":"2.4.1"}`), `subscribe` (opts into events and replies with the current `playback`/`queue`/`settings`/`auth` snapshot), `show` (raise a subscribed client's window, or launch the client when none is listening), `quit` (stop playback, unregister MPRIS, exit), and `sign_in` (open the Google login window).
- `ryotunes-cli <method> [json]` runs one method and prints its `result`; `ryotunes-cli events [name…]` subscribes and prints one event per line until interrupted.

The Tauri app and the daemon share the same data directory (`$XDG_DATA_HOME/dev.ryoku.ryotunes`); run one at a time until the cutover.

### Native downloads

`crates/ryotunesd/src/downloads.rs` owns a persistent download queue independently of the client. `client/Downloads.qml` mirrors the `downloads` subscription snapshot and `downloads-changed` events; the player button, queue/history page and settings use dedicated `enqueue_download`, `get_downloads`, `cancel_download`, `retry_download`, `clear_download_history`, `open_download`, `open_download_folder`, `get_download_settings` and `set_download_settings` socket methods.

- Settings and the latest 200 terminal records live in `downloads.json` under the daemon's data directory. Writes are serialized with queue mutations and use a synced temporary file plus atomic rename. Each job captures its file preferences at enqueue time.
- Worker admission is bounded to 1–4 jobs (default 1), with at most 200 active/queued tracks. A reserved slot is held through cancellation until the subprocess is reaped; retries get a fresh attempt identity so an older task cannot complete a newer attempt.
- yt-dlp runs via direct argv in a separate, lower-priority process group, with one fragment downloader and constrained FFmpeg threading. Config/plugin execution is disabled, output/error buffers are bounded, and a job has a finite timeout. Quit and SIGTERM stop admission, terminate whole process groups and await teardown. Queued/active downloads inhibit daemon idle exit even without a client.
- Downloads go to private staging directories beneath the destination. A successful exit and a nonempty completed audio file are required before a same-filesystem hard link atomically publishes the final file without replacing an existing file. Generated artist directories cannot resolve outside the chosen folder. Clearing history never removes music files.
- With metadata embedding on, `crates/ryotunesd/src/download_media.rs` enriches staged audio via `lofty` and saves image and lyric companions before publication. Artwork uses a trusted thumbnail or a strictly matched iTunes fallback; lyrics reuse the core cache/providers with a bounded lookup. Providers receive track details including title, artist, duration and provider identifiers. For local playback, core lyrics checks `.lrc`, embedded lyrics, then `.txt` before network/cache, enabling offline lyrics. The worker retains its slot throughout enrichment; cancelled jobs never publish. Audio and companions share the final filename stem and collision suffix, using no-overwrite hard links and validated regular staging files. Missing metadata produces non-fatal per-job warnings.
- Restart recovers interrupted jobs as retryable failures, not automatic new network activity. Spotify downloads are explicitly labelled YouTube search matches; they do not export the Spotify stream or promise the same recording. SoundCloud links come from the existing provider client; protected/unavailable content produces a visible job error.
- `client/Daemon.qml` recreates its socket while disconnected: Quickshell retains a failed initial native socket, so setting `connected = true` repeatedly on that object is insufficient. The reconnect timer stops entirely while connected.

## Background playback

Closing the main window is not the same operation as quitting the application.

When playback is active, Ryotunes can destroy/hibernate the expensive user-facing WebView while keeping the native backend alive. MPRIS, tray actions and Ryoku shell media controls therefore continue without retaining the full renderer. Reopening reconstructs the WebView and resynchronises it from native state.

A tray-only session with no playback has a bounded idle lifetime and exits automatically. Explicit Quit stops the playback session, unregisters MPRIS and shuts integrations down immediately.

## UI update model

Playback state is event-driven. Ryotunes avoids a permanent high-frequency frontend transport clock and avoids heavy requestAnimationFrame/FFT loops for ordinary idle playback.

The primary QML Home uses a reusable `ListView` with an asynchronous header. `client/components/HomeFeed.qml` pins `contentY` to `originY` until interaction, with `Binding.RestoreNone`: restoring the old pre-layout value (usually zero) when the binding is released skips a tall header whose origin is negative. The QML regression exercises a real wheel event after header growth.

Home's personal state is a mirror of the existing daemon `personal_json` store, not another database. `client/Personal.qml` hydrates before applying local mutations, serializes immediate saves, and ignores its own outstanding write echoes. Songs are recorded from genuine `now-playing` events; album/playlist controls capture their context before playback and record it after the command succeeds. Subscription snapshots restore the UI without recording another listen.

`client/lib/recommendations.js` builds a provider-scoped candidate pool from the existing Home response. A lightweight relevance score combines provider order, artist affinity, recency, shortcut affinity and recent-play fatigue; a maximum-marginal-relevance pass adds variety. Candidate count is capped at 300 and output at 12. The diversity pass caches each candidate's maximum similarity as picks are added, so its work is O(candidates × picks), with no training, new network endpoint or timer. Header/feed and personal-data changes trigger recomputation; additional card artwork uses the existing image pipeline.

We considered [implicit](https://github.com/benfred/implicit) (MIT) and [LightFM](https://github.com/lyst/lightfm) (Apache-2.0), but their trained models add machinery that one user's sparse local history does not justify. Neither is a dependency and no implementation was copied. The local diversity pass follows the standard [MMR relevance/diversity principle](https://www.elastic.co/search-labs/blog/maximum-marginal-relevance-diversify-results).

Search loads results incrementally in bounded pages, deduplicates them and preserves query, selection and scroll state when navigating back.

## Artwork

Large artwork paths reuse already available thumbnails, prepare higher-resolution images before swapping them into view, reject stale track requests, and keep the cache bounded. The goal is to avoid blank artwork and decode spikes during queue/Now Playing changes.

Device playlists carry no provider thumbnail, so the daemon derives one from their songs (`crates/ryotunesd/src/methods.rs`): the first four *distinct* non-empty song covers in playlist order. Four distinct covers become one thumbnail string — the literal `ryotunes-collage:` prefix followed by a JSON array of exactly four URLs; one to three collapse to the first cover; an empty playlist carries none. That marker is rendered only by the shared native `Artwork.qml` as a 2x2 grid — it is never a real URL, never sent to a provider, and download artwork resolution rejects it as an untrusted host. A user's custom cover overrides the automatic art, and clearing it returns the automatic art without contacting YouTube Music. The art is recomputed on every fetch, so add/remove/reorder shows up on the next `get_library`/`get_playlist`; a `library-changed` event refreshes cards, the page header and the sidebar live.

## Linux / Ryoku lifecycle

The main window uses the stable application id `dev.ryoku.ryotunes`. On Ryoku, a compositor rule floats and centres the main surface before it becomes visible. Native geometry remains a fallback rather than the primary source of a tiled-to-floating transition.

Cold launch, second-instance launch, tray reopen and mini-player-to-main restoration all converge on the same native visibility lifecycle. The UI sends a mounted/ready signal, with a native failsafe so a hidden WebView cannot deadlock the application in a tray-only state.

## Packaging invariants

The Ryoku replacement package is intentionally conservative:

- never uninstall the `ryoku-desktop` package;
- preserve genuine stock entry points for rollback;
- expose one active `/usr/bin/ryotunes` route and one desktop launcher;
- migrate old custom Ryotunes generations only after the new generation is active;
- install/remove only the Ryotunes-managed Hyprland rule;
- verify binary ownership and active routes after installation.

## Release gates

`scripts/release-check.sh` runs source-level structural and regression checks. A distributable build must additionally pass Svelte/TypeScript semantic checking, locked Cargo fetch/test/check, the native Tauri release build and final pacman package ownership validation on an Arch/Ryoku build machine.
