# Security and privacy

Ryotunes handles account sessions, local media paths and playback URLs. Do not attach its data directory, database, credentials or raw logs to a public report.

## Stored data

| Purpose | Default path |
|---|---|
| Library, settings, history, provider sessions and downloads | `~/.local/share/dev.ryoku.ryotunes/` |
| Playback cache | `~/.cache/dev.ryoku.ryotunes/audio/` |
| User palette files | `~/.config/ryotunes/` |
| State and diagnostics | `~/.local/state/ryotunes/` |
| Runtime socket | `$XDG_RUNTIME_DIR/ryotunes/` |

The corresponding absolute `XDG_DATA_HOME`, `XDG_CACHE_HOME`, `XDG_CONFIG_HOME` and `XDG_STATE_HOME` values take precedence. Ryotunes ignores relative XDG paths.

Application directories are created with mode `0700`. The SQLite database and atomically written private files use mode `0600`.

Provider credentials include:

- `spotify/credentials.json` and `spotify/device.id` under the data directory;
- YouTube session cookies and delegated identity values in private SQLite settings rows;
- SoundCloud and Tidal session data in private SQLite settings rows;
- the Last.fm session key in a private SQLite settings row.

Sign out of an affected provider and revoke its session before sharing a report about credential exposure.

## Browser cookie import

YouTube Music and SoundCloud sign-in can read eligible Firefox-family and Chromium-family cookie stores. Ryotunes does not open the browser's live SQLite database for queries. It copies the database and its SQLite sidecars into a temporary `0700` directory, creates the copies with mode `0600`, opens the copy in read-only mode and removes the temporary directory when the read ends.

The import selects only cookies for the requested service. It does not write to a browser profile.

## Reports and logs

`ryotunes-diagnostics` reports package, system, runtime and dependency state without including logs, account data, tokens, local paths, hostnames or network endpoints.

The daemon log can contain provider errors and local context. Review and sanitize it before sharing. The packaged data-directory log is:

```text
~/.local/share/dev.ryoku.ryotunes/logs/ryotunesd.log
```

## Reporting a vulnerability

Do not open a public issue containing a credential, session token, private media path or unredacted log. Use GitHub's private vulnerability reporting for `ryoku-dev/ryotunes` when available. Include the affected version, a minimal reproduction, the expected boundary and the observed result. Revoke exposed credentials first.
