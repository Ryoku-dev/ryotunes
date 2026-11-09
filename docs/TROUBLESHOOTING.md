# Troubleshooting

Start with the sanitized system report:

```sh
ryotunes-diagnostics
```

It reports the installed package, session, processes and runtime dependencies without including account details, tokens, local paths, hostnames or network endpoints.

## YouTube rejects every track

**Symptom:** Tracks are skipped as unavailable, or the error says that YouTube rejected the stream link or bot-gated every playback client.

**Fix:** Open Settings, then Storage, and choose **Force clear caches**. This removes stale stream and lyric caches, mpv audio cache data and the stored playback token so Ryotunes can obtain a new playback identity. It preserves accounts, library data, history and artwork. Try the track again after the action finishes.

If the account session has expired, sign out of YouTube Music and sign in again.

## Spotify asks for sign-in or skips every track

Spotify playback requires Premium. Confirm that the account has Premium, then sign out and sign in again. This replaces revoked credentials and restores the saved device session. A free account can be identified but cannot provide audio playback.

If the problem returns after a daemon restart, capture `ryotunes-cli get_logs` before another sign-in so the session restore failure remains visible.

## SoundCloud sign-in does not complete

Ryotunes imports the SoundCloud session from a supported browser profile. Sign in at `soundcloud.com` in Firefox, Chromium, Chrome or a related browser, leave that profile open, then press the SoundCloud connect action again.

The importer reads temporary copies of the browser cookie database and does not modify the browser profile. If the browser uses an unsupported profile location, complete sign-in in a standard profile and retry. A guest session can still browse and play public SoundCloud tracks.

## The daemon does not start

On a systemd user session, inspect both units:

```sh
systemctl --user status ryotunesd
systemctl --user status ryotunesd.socket
journalctl --user -u ryotunesd.service -n 100 --no-pager
```

Confirm that `mpv`, `yt-dlp`, `ffmpeg` and `libvorbis` are installed. Then reload and restart socket activation:

```sh
systemctl --user daemon-reload
systemctl --user restart ryotunesd.socket
ryotunes
```

On Void, run `ryotunes` once to start the daemon directly, then check `pgrep -a ryotunesd` and `ryotunes-diagnostics`.

## Collect logs

The packaged data-directory log is:

```text
~/.local/share/dev.ryoku.ryotunes/logs/ryotunesd.log
```

Ask the daemon for its bounded log ring with:

```sh
ryotunes-cli get_logs
```

Run the sanitized diagnostics helper with:

```sh
ryotunes-diagnostics
```

Logs may contain local media paths, provider responses or account context. Review and remove private values before sharing them. Do not upload the SQLite database, browser profile or provider credential files.
