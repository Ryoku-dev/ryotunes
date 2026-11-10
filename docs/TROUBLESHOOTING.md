# Troubleshooting

Start with the sanitized system report:

```sh
ryotunes-diagnostics
```

It reports the installed package, session, processes and runtime dependencies without including account details, tokens, local paths, hostnames or network endpoints.

## YouTube rejects every track

**Symptom:** Tracks are skipped as unavailable, or the error says that YouTube rejected the stream link or bot-gated every playback client.

**Fix:** Open Settings, then Storage, and choose **Force clear caches**. This removes stale stream and lyric caches, mpv audio cache data, saved covers and the stored playback token so Ryotunes can obtain a new playback identity. It preserves accounts, library data and history; covers load again from the network. Try the track again after the action finishes.

If YouTube Music shows signed out, press Sign in. Ryotunes reads the session from the browser it came from again; when the browser is still signed in to the same account, nothing else is asked.

## Spotify asks for sign-in or skips every track

Spotify playback requires Premium. Confirm that the account has Premium, then sign out and sign in again. This replaces revoked credentials and restores the saved device session. A free account can be identified but cannot provide audio playback.

If the problem returns after a daemon restart, capture `ryotunes-cli get_logs` before another sign-in so the session restore failure remains visible.

## SoundCloud sign-in does not complete

Ryotunes follows the SoundCloud session in the browser profile you signed in with. SoundCloud renews that session only while `soundcloud.com` is open, about once a day, so after a day away SoundCloud can read signed out in Ryotunes. Open `soundcloud.com` in that browser, or press the SoundCloud connect action, and Ryotunes picks the renewed session up without asking again. A first sign-in works the same way: sign in at `soundcloud.com` in Firefox, Chromium, Chrome or a related browser, leave that profile open, then press the connect action.

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
