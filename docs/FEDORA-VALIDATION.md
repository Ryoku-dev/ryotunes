# Fedora validation — 2026-09-20

Target: Fedora 44 x86_64. Source fork baseline: `eb60975a2f9078bafd04fdd8c578e89ad3040656`;
implementation is uncommitted. No releases, pushes, host RPM installs or live desktop changes were
performed. All installations used disposable rootless Podman containers.

Base image: `registry.fedoraproject.org/fedora:44`, digest
`sha256:a43233b829403f8f21d0b0f3e20836ba3c386c95cfa572e9e240009e6a75bf8c`.
Toolchain: Fedora rustc/cargo 1.98.1, RPM 6.0.2, Qt 6.11.2, Quickshell
0.2.1 revision `dacfa9de829ac7cb173825f593236bf2c21f637e`.

The shared runtime was built from the pinned Ryoku companion source and the two
reviewable patches in `packaging/fedora/companion/`. It is a real separate RPM,
not a QML stub or a copy in RyoTunes. The test version is `ryoku-ui-0.1-1.fc44`.

## Completed checks

- Node stream regression: rebuilt the RPM with the versioned
  `alternative-for(nodejs-bin) >= 1:22.0.0` dependency. Installation and the
  release-0 → release-1 upgrade retain preinstalled Fedora Node 22.23.1.
  Desktop/playback checks passed again; a separate fresh yt-dlp download with
  the daemon's Node/EJS options produced Opus. CI now preinstalls Node 22 and
  asserts that its package survives installation and upgrade.
- Native RPM and self-contained vendored SRPM built on Fedora libraries.
  `ldd` confirms Fedora `libmpv.so.2`, `libsqlite3.so.0`, GTK3/WebKitGTK 4.1;
  no unresolved shared libraries. DNF resolved the runtime dependencies from
  Fedora repositories plus the local companion RPM.
- 313 native tests passed across the workspace excluding the legacy Tauri
  host. Four pre-existing tests remained ignored. The environment-sensitive
  libmpv pitch/gain/filter test passed separately in 1.07 seconds (30s timeout).
- All 51 QML tests passed on Fedora Qt. Rust formatting, source/release/link
  guards, frontend pure-behavior checks, shipped skin validation and desktop
  file validation passed.
- Real systemd user socket activation: service inactive until connection,
  installed launcher starts QML, daemon answers protocol handshake. Fedora's
  install-update RPC refuses immediately with DNF guidance.
- Xvfb/Openbox with a private session bus and PulseAudio null sink: local WAV
  decoding and advancing playback position; MPRIS Pause/Play; compositor close
  while playing and launcher reopen; floating mini-player; explicit Quit.
  Light/dark/system controls update saved preferences and render successfully.
- Headless Sway with the Wayland Qt platform: primary window and layer-shell
  mini-player render, captured and visually inspected. Software-renderer
  testing caught the mini-player's unsupported shader mask; it now renders
  without that mask on software Qt. Artwork uses the same fallback.
- Real DNF release-0 → release-1 upgrade: the daemon's MainPID stays unchanged
  during the transaction; package verification succeeds afterwards. Quit/reopen
  then starts the upgraded application. The fixture uses the same source with
  an older RPM release number.
- Full public YouTube download through the installed daemon, using Fedora
  yt-dlp/Node and FFmpeg-free: completed at 100%; `ffprobe` verified the saved
  audio codec as Opus. Cover/lyrics enrichment was disabled for this test.
  This is one live-provider check, not a guarantee for all YouTube content.
- DNF removal stops both running user units and preserves the test user's music,
  including the downloaded Opus file. Socket reactivation after explicit Quit
  passed before removal.

## Evidence and reproduction

Build logs, package inventories, shared-library resolution, screenshots and
session logs are saved under `fedora-out/validation/` and the checkout's ignored
`*build*.log` files. The build scripts and `validate-container.sh` reproduce the
checks; `.github/workflows/fedora.yml` runs the build and deterministic desktop
checks on Fedora 44 without publishing. The workflow itself has not run on
GitHub in this session.

Use `build-upgrade-fixture.sh` after a normal build to create release 0 of the
same source, then validate in a fresh systemd container. The fixture tests RPM
lifecycle semantics, not migrations from a previously released Fedora version.
Set `RYOTUNES_ONLINE_TEST=1` when running `validate-container.sh` to additionally
exercise the public YouTube test download and FFmpeg conversion. CI leaves that
external-service-dependent check off.

## Limits

These are virtual display/audio tests. Physical audio output, GPU drivers,
portal file opening, provider account login, Spotify streaming, hardware media
keys and a full Ryoku/GNOME/Plasma desktop session have not been validated.
The test setup hit the host's per-user inotify limit while two systemd
containers ran at once. Stopping this task's older container and restarting the
test container's D-Bus/login manager resolved it; host limits were not changed.
Sway's realtime scheduling file capability was removed only inside its
disposable container to run headlessly under rootless Podman.

The minimal session has no StatusNotifier tray host or desktop portal and logs
that absence; it does not prevent playback or reopening. No claim is made of
SELinux-enforcing VM validation, full Ryoku desktop compatibility with Fedora's
older Quickshell workspace APIs, or byte-for-byte RPM reproducibility across
different Fedora dependency versions. The legacy Tauri frontend was not rebuilt;
its Linux launcher code is shared with the tested native executable.

The companion package split and optional WindowManager adapter still need to
land in Ryoku's Fedora branch. No public signed RPM repository is configured or
published. Until a publisher delivers it, install the local companion and
application RPMs together as described in INSTALL-FEDORA.md.
