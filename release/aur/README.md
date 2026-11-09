# ryotunes-bin AUR packaging

This directory contains the AUR recipe for the precompiled x86_64 Ryotunes release. The recipe downloads the matching Arch package and its SHA-256 sidecar from the official GitHub release, verifies the payload, and installs its `usr/` tree.

Before publishing an update:

1. Confirm that both release assets exist for `pkgver`.
2. Run `makepkg --verifysource` and `makepkg -si` on Arch Linux.
3. Regenerate `.SRCINFO` with `makepkg --printsrcinfo > .SRCINFO` in the AUR checkout.
4. Publish `PKGBUILD`, `.SRCINFO`, `ryotunes-bin.install`, and `LICENSE` together.

The application is GPL-3.0-or-later. These packaging instructions are offered under 0BSD.
