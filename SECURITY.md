# Security and privacy

Ryotunes is a Tauri desktop application: the UI is rendered by the operating system WebView
(WebKitGTK on Linux), while playback, account state, filesystem access and network integrations live
in Rust. Treat both sides as part of the security boundary.

## Keep the system WebView current

Ryotunes does **not** bundle its own WebKit engine. On Arch/CachyOS/Ryoku, WebKitGTK is supplied and
patched by the system package manager. Keep the machine fully updated with `pacman -Syu`.

At the time of the v2.4.1 hardening pass (September 2026), WebKitGTK **2.52.6** is the stable
security baseline. WebKitGTK advisory WSA-2026-0005 lists vulnerabilities affecting releases before
2.52.6. `./scripts/diagnostics.sh` reports the installed WebKitGTK version without exposing private
paths or account information.

The Rust Tauri dependency is also kept above **2.11.1**, which contains the fix for
GHSA-7gmj-67g7-phm9 (origin confusion allowing some remote pages to be mistaken for trusted local
origins on affected platforms).

## WebView / IPC design

- The normal `main` and `mini` surfaces load the bundled Ryotunes frontend, not a remote web app.
- All 93 application commands are registered with Tauri's runtime authority through an explicit
  `AppManifest` and one `allow-ui-commands` permission granted only to the `main` and `mini`
  surfaces. The remote Google login page and hidden cipher/PoToken JavaScript runtimes therefore
  cannot invoke Ryotunes application commands even though they exist inside the same process.
- The Google sign-in WebView has a separate `login` label and is not included in the main/mini
  capability files.
- The bundled surfaces do not use `core:default`; they receive only the Tauri app/event APIs and
  explicit window/WebView operations Ryotunes actually uses. In particular, renderer-side core image/path,
  tray, menu and resource defaults are not exposed.
- Neither bundled renderer has file-dialog permission. Playlist import/export, artwork selection
  and local-folder selection open native pickers from Rust commands, so WebKit cannot silently
  choose arbitrary filesystem paths.
- Portable playlist files accept YouTube Music track metadata only. Local-file identifiers contain
  filesystem paths for native local playback, so they are deliberately refused by portable
  export/import rather than leaking machine paths into a shareable JSON file.
- Renderer-writable settings, external URLs, Listen Together endpoints and media parameters are
  validated again in Rust. The frontend is never treated as an authorization boundary.
- Authenticated proxy URLs are rejected, and an invalid legacy proxy setting is discarded before
  the networking stack starts, so proxy credentials are not returned through renderer-visible
  settings.
- Internet Radio playback accepts only an opaque station id from WebKit. Native code resolves the
  cached/Radio Browser station record and rejects literal localhost/private/link-local stream
  addresses rather than accepting a renderer-supplied URL.
- Radio Browser discovery accepts only official `*.api.radio-browser.info` mirrors and bounds
  each directory response before parsing it.
- Persisted radio-station records are re-normalized through the same URL policy when a queue is
  restored, so records written by an older release cannot bypass current stream validation.
- The Google sign-in WebView can navigate only to HTTPS Google/YouTube hosts and has no main/mini
  capability set.
- External links are opened by passing a validated HTTP(S) URL directly to the OS opener. They are
  never interpolated into a shell command.
- The Tauri asset protocol has an empty static scope. Local cover art is copied into Ryotunes-owned
  storage; watched music directories are not recursively exposed to the renderer.
- Local track ids contain a native path for offline playback, but Rust verifies that exact path is
  present in the native-scanned `local_tracks` database before it can be handed to mpv. A forged
  `LOCAL:` id from WebKit therefore cannot open an arbitrary file.
- Account cookies, delegated YouTube identity values, visitor data, queue internals and stream URLs
  are deliberately excluded from the settings IPC API.
- The optional Listen Together relay bounds WebSocket frame/message sizes, room count, queue length,
  pending suggestions and each client's outbound queue. It binds to localhost by default; use a TLS
  reverse proxy when deliberately exposing it as a public `wss://` endpoint.

### CSP note

The global Tauri CSP remains unset because the cipher and PoToken extraction stack uses isolated,
hidden `data:` WebViews that require inline/dynamic JavaScript. Tauri injects a configured global
CSP into those data documents as well, which breaks the extraction harness. Those hidden WebViews are not granted the main/mini capability set or the application-command
permission, so their dynamic JavaScript cannot cross into Ryotunes commands. A future move of the harness to a dedicated custom
protocol can allow a strict per-application CSP without weakening playback.

## Local secrets and files

Session state is stored in the Tauri application-data directory. On Unix, Ryotunes sets its app data
and cache directories to mode `0700` and its SQLite state database to `0600`.

Do **not** attach the application-data directory, SQLite database, browser session data, or raw
application logs to public bug reports. They may contain account/session material or local media
paths.

For support, prefer:

```sh
./scripts/diagnostics.sh
```

The diagnostics script intentionally excludes account details, cookies, tokens, local media paths,
hostnames and configured network endpoints.

## Dependency monitoring

GitHub Dependabot monitors Rust, frontend and GitHub Actions dependencies weekly. The
`Security audit` workflow also runs RustSec (`cargo audit`) and a production frontend dependency
audit (`pnpm audit`) on the hardening branch/main and on a weekly schedule.

### Reviewed RustSec exceptions (2026-09-20)

`.cargo/audit.toml` records three advisories whose affected operations are not
used by the pinned librespot revision `e7eb953d5848fd97bacd00e6c0e33500765eaa63`:

- [RUSTSEC-2023-0071](https://rustsec.org/advisories/RUSTSEC-2023-0071):
  `librespot-core` is the only consumer of `rsa`. Its
  `core/src/connection/handshake.rs` verifies Spotify's signature using
  `RsaPublicKey::verify`. It performs no private-key operation, so the private-key
  timing attack does not apply. There is currently no patched RSA release.
- [RUSTSEC-2026-0194](https://rustsec.org/advisories/RUSTSEC-2026-0194) and
  [RUSTSEC-2026-0195](https://rustsec.org/advisories/RUSTSEC-2026-0195):
  `librespot-core` is the only consumer of `quick-xml` 0.38. Its ProductInfo
  parser in `core/src/session.rs` uses plain `Reader` events and text decoding.
  It never iterates XML attributes, uses `NsReader`, or calls
  `NamespaceResolver`, which are the affected paths. The other `quick-xml`
  dependency is already on patched version 0.41.

These are reachability exceptions, not claims that the dependencies are patched.
Re-review them whenever the librespot pin changes or new consumers of these
crates are introduced. Remove the XML exceptions when the fork accepts
`quick-xml` 0.41 or later. Other RustSec vulnerabilities still fail the audit.

Librespot now uses platform TLS (OpenSSL on Linux) because its pinned proxy
backend otherwise pulls in obsolete rustls 0.22 and rustls-webpki 0.102.
The application's remaining rustls dependency is updated to 0.23.45.

## Reporting a vulnerability

If you discover a bug that exposes credentials or session data, do not post the secret publicly.
Revoke or sign out the affected session first, then report the issue with sanitized reproduction
steps through GitHub's private vulnerability-reporting channel when available.
