# Companion change owned by ryoku-arch

`0001-split-ryoku-ui.patch` targets the existing Fedora port at
`itsKontra/ryoku-arch`, commit `f4a932df3bf731ab385f145060067a48945c3cbe`
(`feat/fedora-support`). The public `ryoku-dev/ryoku-arch` main branch did not
have `release/rpm` when queried on 2026-09-20 (GitHub API HTTP 404).

The patch splits the existing `Ryoku.Ui` payload from `ryoku-desktop` into
`ryoku-ui`. The desktop requires the matching runtime; its RPM staging excludes
the moved directory. Arch payload ownership stays unchanged. Both RPMs must be
updated together. The new spec is picked up by the existing RPM build loop.
No shared QML implementation is maintained in RyoTunes.

Apply in the Ryoku Fedora branch and build through its normal RPM process.
For this fork's isolated validation, apply the patch in a temporary checkout
and build only `ryoku-ui` against that checkout's source archive. Keep the
resulting RPM and SRPM alongside the RyoTunes test artifacts.

This companion patch is prepared for review, not submitted or merged. No
publisher repository or signing key is invented here. Production standalone
installation depends on the Ryoku maintainer delivering this split package.

`0002-optional-window-manager.patch` fixes a verified import failure: Fedora's
Quickshell 0.2.1 snapshot has no `Quickshell.WindowManager`. QML resolves all
singletons in the module, so even a music player using only Tokens failed to
load because Wm imported that optional module. Move the protocol import into a
separately loaded adapter. Wm retains its native workspace data on versions
that provide the module; on older versions its protocol list is empty. The
Ryoku desktop's workspace UI on that older Quickshell still needs separate
validation. This compatibility fix also belongs in the shared runtime.
