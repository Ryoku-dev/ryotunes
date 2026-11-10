# Persistence

Ryotunes keeps the 1.x data location and schema so rebuilding the application does not discard a library, device playlist, account, history row, or cache entry. SQLite migrations are additive. The current schema version is `PRAGMA user_version=1`.

## Directories

| Purpose | XDG path | Fallback |
|---|---|---|
| Data | `$XDG_DATA_HOME/dev.ryoku.ryotunes` | `$HOME/.local/share/dev.ryoku.ryotunes` |
| Playback cache | `$XDG_CACHE_HOME/dev.ryoku.ryotunes/audio` | `$HOME/.cache/dev.ryoku.ryotunes/audio` |
| Cover art cache (client) | `$XDG_CACHE_HOME/ryotunes/art` | `$HOME/.cache/ryotunes/art`; covers stay in memory only when neither resolves |
| User configuration | `$XDG_CONFIG_HOME/ryotunes` | `$HOME/.config/ryotunes` |
| State and diagnostics | `$XDG_STATE_HOME/ryotunes` | `$HOME/.local/state/ryotunes` |
| Runtime socket, FIFOs and short-lived browser cookie store copies | `$XDG_RUNTIME_DIR/ryotunes` | `/tmp/ryotunes-<uid>` |

A relative XDG value is ignored. Ryotunes creates these private directories with mode `0700`. The database and atomically written private files use mode `0600`.

## SQLite database

The database is `<data>/ryotunes.sqlite`. If it is absent and exactly one other sibling name ends in `.sqlite`, that file is renamed to `ryotunes.sqlite` before opening. More than one sibling is ambiguous and none is adopted. The connection uses WAL, `synchronous=NORMAL`, a five-second busy timeout, and one connection. Startup deletes expired stream URLs and obsolete `LOCAL:%` history rows.

### Tables

| Table | Columns | Keys and indexes | Owner |
|---|---|---|---|
| `settings` | `key TEXT`, `value TEXT` | primary key `key` | String settings, JSON state, provider tokens, and small session records. |
| `stream_url_cache` | `video_id TEXT`, `url TEXT`, `itag INTEGER`, `expires_at INTEGER`, `loudness_db REAL`, `is_video INTEGER`, `ping_url TEXT`, `ping_client TEXT` | primary key `video_id` | Expiring provider stream URLs and playback-ping metadata. The four nullable/trailing fields are additive 1.x migrations. |
| `lyrics_cache` | `video_id TEXT`, `lyrics TEXT`, `fetched_at INTEGER` | primary key `video_id` | `lyrics` is source `model.Lyrics` JSON; SQL `NULL` is a negative result. Positive rows do not expire; negative rows expire after the caller's TTL, normally 24 hours. |
| `plays` | `id INTEGER`, `video_id TEXT`, `played_at INTEGER`, `song_json TEXT` | primary key `id`; `plays_played_at(played_at)` | Eligible YouTube play history. `song_json` is the exact `model.Track` wire shape. |
| `local_tracks` | `path TEXT`, `title TEXT`, `artist TEXT`, `album TEXT`, `album_key TEXT`, `track_no INTEGER`, `duration_secs INTEGER`, `cover TEXT`, `mtime INTEGER` | primary key `path`; `local_tracks_album(album_key)` | Local scanner index and mtime cache. |
| `playlist_track` | `playlist_id TEXT`, `video_id TEXT` | primary key `(playlist_id,video_id)` without rowid; `playlist_track_video(video_id)` | Account-scoped YouTube playlist membership index. |
| `local_playlists` | `id TEXT`, `title TEXT`, `created_at INTEGER`, `updated_at INTEGER` | primary key `id` | Device playlists, including `RYOTUNES_LOCAL_PLAYLIST:*`. |
| `local_playlist_tracks` | `playlist_id TEXT`, `video_id TEXT`, `position INTEGER`, `song_json TEXT` | primary key `(playlist_id,video_id)` without rowid; `local_playlist_tracks_position(playlist_id,position)` | Ordered device-playlist members. |
| `radio_station_cache` | `station_id TEXT`, `station_json TEXT`, `updated_at INTEGER` | primary key `station_id` | Restore metadata for live-radio queues; only the 64 newest rows remain. |

`expires_at`, `fetched_at`, `played_at`, `created_at`, and `updated_at` are Unix seconds. Radio cache ordering uses Unix nanoseconds in its existing integer column.

## Settings rows

Every value is a string, even when its logical type is boolean, integer, JSON, or time. Renderer-visible setters validate values before writing. Other rows are accessed only through the daemon/provider KV boundary.

### Renderer-visible settings

| Key | Type / allowed values | Effective default | Consumer |
|---|---|---|---|
| `volume` | integer string `0..100` | `100` | Player and volume controls. |
| `proxy` | empty or unauthenticated `http://`/`https://` scheme, host, and optional port; at most 2048 bytes | empty | Provider and update HTTP clients. |
| `quality` | `LOW`, `AUTO`, or `HIGH` | `HIGH` | Stream selection. |
| `enable_history` | `true` or `false` | `true` | YouTube remote watch-history registration; does not gate local `plays`. |
| `discord_rpc` | `true` or `false` | `false` | Discord RPC lifecycle. |
| `close_to_tray` | `true` or `false` | `false` | Client close behavior. |
| `autostart` | `true` or `false` | `false` | Client preference; the daemon does not register an OS launcher. |
| `autoplay` | `true` or `false` | `true` | Queue radio extension. |
| `prevent_duplicates` | `true` or `false` | `false` | Manual queue insertion policy. |
| `lyrics_boidu` | `true` or `false` | `false` | Boidu lyrics source; changing it invalidates source lyrics. |
| `lyrics_romanize` | `true` or `false` | `false` | Output-only lyric readings; readings are never cached. |
| `low_resource_mode` | `true` or `false` | `false` | Daemon cadence and client rendering policy. |
| `disabled_stream_clients` | sorted, deduplicated comma list of known stream-client names | empty | YouTube stream fallback selection. |
| `ui_scale` | `80`, `90`, `100`, `110`, `120`, `130`, or `140` | `100` | Client interface scale. |
| `discord_presence_name` | trimmed empty value or 2–128 Unicode characters | `Ryotunes` | Discord activity title. |

`SettingsSnapshot` returns only persisted rows from this table, not synthesized defaults and not daemon-only keys.

### Daemon and provider settings

| Key or pattern | Logical type / default | Consumer |
|---|---|---|
| `provider` | `youtube`, `spotify`, `soundcloud`, or `tidal`; YouTube when absent | Active catalogue provider. |
| `queue_json` | queue-state JSON; absent means empty queue | Queue restore. |
| `queue_index` | queue fingerprint/index JSON; absent means no projection | Queue restore validation. |
| `queue_position` | non-negative seconds string; absent means zero | One-shot paused position restore. |
| `personal_json` | JSON object, at most 1 MiB; canonical empty personal object when absent/invalid | Personal picks, saved items, pins, recents, artists, dismissed seeds, and home layout. |
| `equalizer_json` | Versioned JSON `{version,active}`; version 1. `active` is the complete normalized `model.Equalizer`. Absent data uses the bypassed five-band Flat default. Corrupt or unsupported-version data is left untouched and blocks replacement. | Active parametric equalizer only, so live edits never rewrite presets. |
| `equalizer_presets_json` | Versioned JSON `{version,presets}`; version 1. Contains user presets only. Built-in presets are generated by `internal/eq`. Corrupt or unsupported-version data is left untouched and blocks preset writes. | Saved listener presets. Version 1 combined `equalizer_json` data is migrated without losing its preset array. |
| `local_folders` | JSON string array; empty array when absent | Local scanner roots. |
| `local_scan_version` | scanner schema string; current scanner writes `5` | Local metadata reparse gate. |
| `visitor_data` | YouTube visitor token; absent triggers bootstrap | InnerTube anonymous/login session. |
| `last_visitor_try` | Unix-seconds string; absent means no cooldown | Visitor-data bot-gate retry cooldown. |
| `session_cookie` | complete YouTube cookie header; absent means signed out | Authenticated InnerTube calls. |
| `session_cookie_source` | browser store JSON `{kind,path,container}`, `kind` is `gecko` or `chromium` (the integers 0 and 1 from earlier builds still read); absent means the session is not followed | YouTube re-adoption from the browser at start and on rejection. |
| `account_identity` | digest of the signed-in Google identity, kept only for an account with no channel id, email or handle | Proves the same account on re-adoption. |
| `selected_identity_json` | selected YouTube identity projection JSON; absent means none | Account restore. |
| `account_identities_json` | `model.AccountIdentities` JSON array; empty when absent | Account identity picker restore. |
| `data_sync_id` | delegated YouTube identity token; absent means primary identity | InnerTube `onBehalfOfUser`. |
| `account_json` | renderer-safe `model.Account` JSON; signed out when absent | Account snapshot restore. |
| `account_selection_pending` | JSON/presence marker; absent means no pending selection | Multi-channel first-login flow. |
| `playlist_index_synced_at` | Unix-seconds string; absent means never | Six-hour YouTube playlist-index TTL. |
| `potoken_session` | JSON `{session_id,token,expires_at}`; absent means mint on demand | YouTube BotGuard session. |
| `soundcloud_auth` | SoundCloud auth/session JSON with its remembered browser store; absent means signed out | SoundCloud provider and its re-adoption from the browser. |
| `tidal_auth` | rotating TIDAL token JSON; absent means signed out | TIDAL provider. |
| `tidal_credentials` | TIDAL device/PKCE override JSON; shipped defaults when absent | TIDAL sign-in. |
| `tidal_quality` | TIDAL quality name; `LOSSLESS` when absent | TIDAL playback ceiling below the account entitlement. |
| `lt_server_url` | empty or `ws://`/`wss://` URL | Listen Together; there is no default server. |
| `lastfm_session_key` | Last.fm session secret; absent means disconnected | Last.fm scrobbling. |
| `lastfm_username` | Last.fm display name; absent means disconnected | Last.fm status. |
| `playlist_cover:<id>` | absolute local cover path; absent means automatic art | Device/YouTube playlist custom artwork. `VL` is stripped from YouTube IDs. |
| `playlist_cover:<id>:synced` | provider synchronization marker; absent means unsynced | YouTube custom-cover synchronization. |
| `playlist_sort:<id>` | `model.PlaylistSort` string; absent means provider/default order | Local playlist presentation sort. |

Provider packages may add an auth/session key only through `provider.KV`; such a key remains daemon-only and must be added to this table when introduced.

## Files under the data directory

| Path | Mode / lifetime | Contents |
|---|---|---|
| `ryotunes.sqlite` | `0600`, durable | Database above. |
| `ryotunes.sqlite-wal`, `ryotunes.sqlite-shm` | SQLite-managed, transient | WAL data and shared-memory index; never copy one without the database during a live backup. |
| `downloads.json` | `0600`, durable, atomic temp-and-rename | Download settings plus active jobs and latest 200 terminal jobs. |
| `downloads.json.tmp` | transient | Removed on startup after an interrupted write. |
| `logs/ryotunesd.log` | private, rotating | Daemon log where a data-directory diagnostics deployment is used. |
| `logs/ryotunesd.log.1` | private, rotating | Previous 2 MiB log generation. |
| `covers/playlists/*.{jpg,jpeg,png}` | `0600`, durable | Validated custom playlist artwork, named from a playlist-ID digest. |
| `covers/*` | private, durable | Local scanner sidecar or extracted artwork. Cache clearing never removes it. |
| `spotify/credentials.json` | `0600`, durable | Librespot reusable credentials. |
| `spotify/device.id` | `0600`, durable | Stable Spotify device identity, 40 hex characters; a 1.x UUID is replaced by its SHA-1. |
| `spotify/1x-migrated` | durable marker | Written once 1.x's `spotify/spotify/credentials.json` was read, so a sign out is never undone by migrating again. 2.x never writes the 1.x files; sign out removes 1.x's credentials too. |
| `tidal/*.mpd` | private, replaceable | Temporary decoded TIDAL DASH manifests. |

Playback bytes live under the cache directory's `audio/`, not the data directory. `ClearCaches` empties `stream_url_cache`, `lyrics_cache`, `potoken_session`, and every child of `audio/`; it preserves local and playlist artwork, history, accounts, and radio metadata. The client's Force clear caches also empties its cover art cache. Cached covers are JPEG or PNG files named by a digest of URL and size, bounded at 256 MB by least recent use, and fetched again in the background once they are 30 days old.

All application-owned file replacement uses a temporary file in the destination directory, `fsync`, and rename. Custom playlist covers are accepted only from regular JPEG/PNG files no larger than 8 MiB. Automatic playlist artwork uses the first four distinct nonempty track thumbnails in order: zero produces no art, one to three use the first cover, and exactly four produce `ryotunes-collage:` followed by a JSON array of those four URLs.
