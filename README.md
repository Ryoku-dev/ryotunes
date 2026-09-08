
<div align="center">

<img src="docs/assets/ryotunes-hero.svg" alt="Ryotunes — Music, shaped for Ryoku" width="100%" />

<br />

<a href="https://github.com/ashmitvoid/ryotunes/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/ashmitvoid/ryotunes?style=flat-square&label=release&color=9a604b"></a>
<a href="LICENSE"><img alt="GPL-3.0-or-later" src="https://img.shields.io/github/license/ashmitvoid/ryotunes?style=flat-square&color=7f9b8d"></a>
<img alt="Linux x86_64" src="https://img.shields.io/badge/Linux-x86__64-8996a8?style=flat-square&logo=linux&logoColor=white">
<img alt="Ryoku native" src="https://img.shields.io/badge/Ryoku-native-6f7f76?style=flat-square">
<img alt="Audio only" src="https://img.shields.io/badge/playback-audio%20only-a86d58?style=flat-square">
<br />
<img alt="Rust" src="https://img.shields.io/badge/Rust-native%20core-2d3136?style=flat-square&logo=rust&logoColor=white">
<img alt="Tauri 2" src="https://img.shields.io/badge/Tauri-2-2d3136?style=flat-square&logo=tauri&logoColor=white">
<img alt="Svelte 5" src="https://img.shields.io/badge/Svelte-5-2d3136?style=flat-square&logo=svelte&logoColor=white">
<img alt="libmpv" src="https://img.shields.io/badge/audio-libmpv-2d3136?style=flat-square">

<br /><br />

**A Ryoku-native desktop music player with native audio playback, live shell theming, and a UI that knows when to disappear.**

[Download](https://github.com/ashmitvoid/ryotunes/releases/latest) · [Architecture](docs/ARCHITECTURE.md) · [Install](docs/INSTALL-ARCH.md) · [Troubleshooting](docs/TROUBLESHOOTING.md) · [Ryoku](https://github.com/neur0map/ryoku-arch)

</div>

<br />

<img width="3369" height="2017" alt="2026_08_27_02_44_21_screenshot" src="https://github.com/user-attachments/assets/44aaf6cc-f88e-4c1d-97ff-c44e45d4c72b" width="100%"/>

<p align="center"><sub>Repository artwork based on the live v2 Home layout and Ryoku visual language.</sub></p>

---

## Built for Ryoku, not merely compatible with it

Ryotunes is a Linux-first desktop music application shaped around the way **Ryoku + Hyprland** actually behave.

The visible interface is Svelte/WebKitGTK. Playback is not. Rust and libmpv own the audio session, desktop integration, lifecycle, and media state. That separation lets Ryotunes stay visually rich while open and become dramatically quieter when the main interface is no longer needed.

<table>
<tr>
<td width="33%" valign="top">

### Native where it matters
Audio stays in **libmpv**, outside the frontend renderer. MPRIS, media keys, tray controls, gapless playback and the active session remain native.

</td>
<td width="33%" valign="top">

### Ryoku in real time
**Follow System** consumes Ryoku's live Material-role palette. Named themes and wallpaper-derived colours retint both the main window and mini-player immediately.

</td>
<td width="33%" valign="top">

### Quiet in the background
During background playback, the expensive visible WebKit surface can be **destroyed / hibernated** while the native playback session continues.

</td>
</tr>
</table>

> [!NOTE]
> Ryotunes is intentionally **audio-only**. It does not provide music-video playback.

---

## The experience

| Surface | What Ryotunes does |
|---|---|
| **Home** | Stable, non-virtualized sections, listening console, recommendations and progressive loading without scroll-jitter regressions |
| **Search** | Songs, albums, artists and playlists with bounded incremental loading and preserved navigation state |
| **Library** | Liked music, account playlists, persistent device playlists and local music in the same desktop flow |
| **Radio** | Demand-driven Internet Radio directory with native libmpv live-stream playback |
| **Now Playing** | Artwork-first playback surface with queue, metadata and lyrics access |
| **Lyrics** | Synced lyrics with click-to-seek and mini-player follow mode |
| **Queue** | Manual queue control plus radio / continuation behaviour |
| **Downloads (native client)** | One-click audio downloads beside the heart, a bounded background queue, persistent history and configurable file preferences |
| **Mini-player** | A separate compact Ryoku surface with its own exact Hyprland title and independent geometry |
| **Integrations** | MPRIS, hardware media keys, tray, Last.fm, configurable Discord Rich Presence and optional Listen Together |

---
## Download music in the native client

Click the **download arrow beside the heart** in the player to start saving the current track immediately. The icon shows progress, then a check mark when the file is saved. Clicking an active or completed download opens **Downloads** in the sidebar; failed and cancelled downloads can be retried.

To save an entire album, open its **⋯ menu → Download album**. This queues all downloadable tracks in album order, including any additional pages, regardless of the track filter. It uses the same saved preferences and worker limit as single-track downloads. The confirmation reports how many tracks are in Downloads and any unavailable tracks; a queue or connection error reports partial progress rather than claiming the whole album was added. Spotify albums still use labelled YouTube matches, not Spotify audio exports.

**Settings › Downloads** controls the destination folder, Original / MP3 / Opus format, artist subfolders, embedded cover art & lyrics, and simultaneous workers. The default destination is your XDG Music folder's `Ryotunes/` directory (usually `~/Music/Ryotunes`), with original audio and **one worker**. You can choose up to four workers; additional tracks wait in a queue capped at 200. Save preferences explicitly. File preferences are captured when a track is queued; changing the worker limit changes admission immediately without interrupting running jobs.

The Downloads page separates the live queue from the latest 200 history records, with cancel, retry and open-folder actions. Closing the window leaves downloads running. Quitting Ryotunes terminates and reaps its downloader workers; interrupted jobs remain in history for an explicit retry after restart. Clearing history never deletes music, and existing files are never overwritten.

With **Embed cover art & lyrics** on (the default), downloads embed available artwork and plain lyrics where the audio format supports them, and save matching `.jpg`/`.png` artwork plus synced `.lrc` or plain `.txt` lyrics alongside the song. Ryotunes reads local `.lrc`, embedded lyrics, then `.txt` before consulting its network cache/providers, so saved lyrics work offline. Artwork uses the track thumbnail with a name-and-artist iTunes lookup fallback; lyrics reuse the existing providers, including LRCLIB. These services receive track metadata such as title, artist, duration and provider identifiers. Missing matches produce a non-fatal history note rather than failing the audio download. Existing songs are not backfilled or overwritten; download them again to add metadata.

Downloads require **yt-dlp and FFmpeg** (included in the native source package's dependencies). YouTube and public SoundCloud tracks use their source links. **Spotify uses a labelled YouTube search match**, not an export of the Spotify stream, and the matched recording may differ. Local files, live radio, DRM-protected and unavailable tracks are not downloadable. Only download music you have permission to save.


## Ryoku integration

Ryotunes does not implement a disconnected theme layer and then approximate Ryoku on top of it. The Linux build integrates with the shell directly.

- **Live palette bridge** — Rust resolves Ryoku Material roles and watches Ryoku theme sources with inotify.
- **Immediate theme updates** — no 60-second frontend palette polling loop.
- **Compositor-owned startup geometry** — the main surface is floated, sized and centred by the Ryoku/Hyprland rule before it becomes visible.
- **Mini-player isolation** — the main rule matches the exact title `^(Ryotunes)$`; `Ryotunes Mini` remains independent.
- **MPRIS continuity** — Ryoku media controls remain available when the visible main UI is closed.
- **Rollback-safe replacement packaging** — custom Ryotunes replaces only the stock Ryotunes entry points, never `ryoku-desktop`.

### Skins

Every colour, type face, radius and duration the chrome reads comes from one **skin**. The default, `system`, follows the Ryoku desktop live (wallpaper palette, named scheme, motion scale, decor). The `matugen` skin is the same manifest written by a matugen template from your wallpaper — on Ryoku it is one of the *Theme apps*, regenerated on every palette change. Any folder with a `skin.json` is a skin, and editing it re-paints the running app:

```
/usr/share/ryotunes/skins/<id>/        shipped: paper, ember, mist
~/.config/ryotunes/skins/<id>/         yours (shadows a shipped id); skins/matugen is the generated one
/usr/share/ryotunes/matugen/ryotunes.json   the matugen template
```

Pick one in *Settings › Appearance*, or **Get more skins** there — that opens [RyoStore](https://github.com/neur0map/ryostore) on its *Ryotunes skins* category, and an install lands in `~/.local/share/ryoku/ryotunes-skins/<id>/`. To make your own: fork the painted palette with **New skin from current**, validate with `ryotunes-cli skin check <dir>`, and submit it to the store catalogue as `ryotunes-skins/<id>/` — the format and the checklist are in **[docs/SKINS.md](docs/SKINS.md)**.

For the shell itself:

**[Ryoku Arch](https://github.com/neur0map/ryoku-arch)** · **[Ryoku Discord](https://discord.gg/8KjBmUEyKA)**

---

## A small lifecycle contract with a big payoff

| Situation | Behaviour |
|---|---|
| Main window open | Full Svelte/WebKit UI is active |
| Close while music is playing | Main WebKit can hibernate; native playback + MPRIS + tray remain alive |
| Tray-only, nothing playing | Application exits after the bounded 5-minute idle period |
| Reopen | Main WebView is reconstructed and resynchronized from native state |
| Explicit **Quit** | Playback stops, MPRIS unregisters, media state clears and backend integrations shut down |

This is why the UI is a **client of playback state**, not the transport clock that owns it.

---

## Architecture

```mermaid
flowchart LR
    R["Ryoku / Hyprland"] --> M["MPRIS · media keys · tray"]
    R --> T["live palette + window policy"]

    M --> N["Tauri / Rust host"]
    T --> N

    N --> P["native player crate"]
    P --> MPV["libmpv · audio only"]

    N --> I["Innertube · library · integrations"]
    N --> W["Svelte 5 / WebKitGTK"]

    W --> U["Home · Search · Library"]
    W --> Q["Queue · Lyrics · Now Playing"]
    W --> MINI["Ryotunes Mini"]

    W -. "hibernate while background playback continues" .-> N
```

More detail: **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)**

---

## Performance is part of the product

Ryotunes treats background efficiency as an architectural requirement, not a cleanup pass.

- event-driven playback state instead of a permanent 100 ms global frontend transport timer;
- no always-on FFT / requestAnimationFrame loop for ordinary idle playback;
- main WebKit hibernation during background playback;
- stable Home DOM — physical Home virtualization is deliberately avoided;
- bounded artwork decode/cache paths;
- native playback state remains authoritative across close, tray, mini-player and reopen transitions;
- Internet Radio discovery is demand-driven — no startup fetch or permanent station polling loop.

The design rules behind the interface are documented in **[docs/DESIGN.md](docs/DESIGN.md)**.

---

## Install

Ryotunes v2 targets **x86_64 Ryoku, CachyOS and Arch-based systems**.

The published upstream package is **`ryotunes-v2.4 2.4.1-1`**; the current source build reports **2.5.1**.

The normal user path is a **prebuilt package**. End users do not need Node, pnpm, Rust, Cargo or a local Tauri build.

1. Open **[GitHub Releases](https://github.com/ashmitvoid/ryotunes/releases/latest)**.
2. Download `ryotunes-v2.4-2.4.1-1-x86_64.pkg.tar.zst`.
3. Install:

```bash
sudo pacman -U ./ryotunes-v2.4-2.4.1-1-x86_64.pkg.tar.zst
```

The active route is:

```text
/usr/bin/ryotunes
  -> /usr/lib/ryotunes-v2.4/ryotunes
```

The replacement package preserves genuine stock Ryotunes entry points for rollback and restores them when the custom package is removed.

> [!TIP]
> An AUR `-bin` recipe lives in [`aur/`](aur/). Public AUR publication is pending; the repository does not pretend the package is available there before it actually is.

Source/development setup: **[docs/INSTALL-ARCH.md](docs/INSTALL-ARCH.md)**

---

<details>
<summary><b>Repository map</b></summary>

```text
crates/
  innertube/          YouTube Music request/model layer
  player/             native playback core
  listen-protocol/    shared Listen Together protocol
  sync-server/        optional room relay

src-tauri/             Tauri host, lifecycle, MPRIS, tray, integrations
ui/                    SvelteKit / Svelte 5 interface
integrations/          Ryoku / shell integration assets
packaging/             Arch / Ryoku replacement packaging
scripts/               diagnostics, release gates and packaging tools
docs/                  architecture, install, design and troubleshooting
aur/                   binary AUR recipe sources
```

</details>

<details>
<summary><b>Development & validation</b></summary>

The dependency graphs are locked. Release work is expected to pass both frontend and native gates.

```bash
cd ui
pnpm install --frozen-lockfile
pnpm check
pnpm build

cd ..
cargo fmt --all -- --check
cargo test --workspace --locked
cargo check --workspace --locked
cargo tauri build --no-bundle
```

Release checklist: **[docs/RELEASE-CHECKLIST.md](docs/RELEASE-CHECKLIST.md)**

</details>

<details>
<summary><b>Troubleshooting</b></summary>

Start with **[docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md)**.

For a useful bug report, include:

- Ryoku / Hyprland version
- distro and kernel
- Wayland/display-server context
- GPU setup
- steps to reproduce
- bundled diagnostics output when relevant

</details>

---

## Contributing

Contributions are welcome when they preserve the product contracts that make Ryotunes feel native on Ryoku.

Start with **[CONTRIBUTING.md](CONTRIBUTING.md)**. Changes touching playback lifecycle, theme integration, Home rendering, background behaviour or replacement packaging deserve explicit regression testing.

---

## Upstream, license & independence

Ryotunes is a **GPL-3.0-or-later modified work derived from [SimoHypers/LiMusic](https://github.com/SimoHypers/limusic)**.

The project retains transparent upstream attribution while pursuing its own Ryoku-specific product direction, interface, lifecycle, performance architecture and packaging model. See **[UPSTREAM.md](UPSTREAM.md)** for the detailed note.

Ryotunes is independently developed and is **not affiliated with, authorized by, sponsored by, or endorsed by YouTube or Google**. YouTube and YouTube Music are trademarks of Google LLC.

Distributed under **[GPL-3.0-or-later](LICENSE)**.

---

<div align="center">

### Music, shaped for the Ryoku desktop.

<sub>Rust · Tauri 2 · Svelte 5 · WebKitGTK · libmpv · Hyprland</sub>

</div>
