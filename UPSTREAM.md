# Project history

Ryotunes 2.x is an original Ryoku codebase written in Go. It replaced the 1.x Rust, QML, Svelte and JavaScript application rather than translating those files. The app icon, Linux package metadata, user-facing behaviour, data paths and wire contract carried forward so existing installations keep their libraries, settings, account sessions and desktop integration. The YouTube client profile values were also carried from the GPL-3.0 1.x and LiMusic data table.

Ryotunes 1.x began as a modified GPL-3.0-or-later work based on LiMusic by SimoHypers and contributors:

- https://github.com/SimoHypers/limusic

The lyric romanization in 2.x retains a GPL-3.0 port from PeekMusic, itself a Metrolist fork. Its source and pinned revision are recorded in `internal/lyrics/romanize` and the project `NOTICE`.

Ryotunes 2.x source is distributed under GPL-3.0-or-later. The client and daemon binaries include GPL-3.0-only components and are distributed under GPLv3 only. Copyright, trademark and third-party terms are recorded in [NOTICE](NOTICE) and [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES).
