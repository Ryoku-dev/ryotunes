#!/usr/bin/env bash
# Build the prebuilt Ryotunes pacman package from *this* checkout with an
# unprivileged makepkg, exactly as the Release workflow does, and drop the
# package plus its sha256 sidecar into dist/ (or the directory given as $1).
#
# The published asset name is the client contract that Ryoku's `update`/`doctor`
# parse from the GitHub release:
#
#   ryotunes-<pkgver>-1-x86_64.pkg.tar.zst        # pkgrel is pinned to 1
#   ryotunes-<pkgver>-1-x86_64.pkg.tar.zst.sha256 # "HEX␠␠filename"
#
# packaging/arch/PKGBUILD::prepare() tars the surrounding tree, so the package is
# built from the exact source of the checkout this script runs in (the tagged
# tree in CI), never from a moving branch.
#
# Local smoke on any machine with Docker (no Arch host required):
#   docker run --rm -v "$PWD:/src" -w /src archlinux:latest scripts/build-arch-package.sh
#
# On an Arch host, run it directly as a normal (non-root) user:
#   scripts/build-arch-package.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${1:-$ROOT/dist}"

# A fresh Arch container starts as root, and makepkg refuses to run as root. Make
# the container buildable (base-devel + git + sudo) and re-exec unprivileged, so
# the single documented `docker run ... archlinux:latest` command just works.
if [[ "$(id -u)" -eq 0 ]]; then
  pacman -Syu --noconfirm --needed base-devel git sudo
  id builder >/dev/null 2>&1 || useradd -m builder
  printf 'builder ALL=(ALL) NOPASSWD: /usr/bin/pacman\n' > /etc/sudoers.d/builder
  chmod 440 /etc/sudoers.d/builder
  chown -R builder:builder "$ROOT"
  install -d -o builder -g builder "$OUT"
  exec sudo -u builder -H bash "${BASH_SOURCE[0]}" "$OUT"
fi

command -v makepkg >/dev/null || { echo "makepkg not found (run on Arch or in archlinux:latest)" >&2; exit 1; }

VERSION="$(sed -n 's/^version = "\([0-9.]*\)"/\1/p' "$ROOT/Cargo.toml" | head -1)"
[[ -n "$VERSION" ]] || { echo "could not read version from Cargo.toml" >&2; exit 1; }
WANT="ryotunes-${VERSION}-1-x86_64.pkg.tar.zst"

cd "$ROOT/packaging/arch"

# --packagelist honours PKGDEST/PKGEXT and resolves pkgver-pkgrel-arch, so it is
# the authoritative output path regardless of a host's makepkg.conf. Select the
# contract asset by exact name (never a stray ryotunes-debug-* entry).
built=""
while IFS= read -r p; do
  [[ "$(basename "$p")" == "$WANT" ]] && { built="$p"; break; }
done < <(makepkg --packagelist)
if [[ -z "$built" ]]; then
  echo "makepkg will not produce the client contract asset '$WANT'" >&2
  echo "(pkgrel must be 1 and pkgver must equal Cargo.toml's $VERSION)" >&2
  exit 1
fi
name="$WANT"

# -s installs makedepends/depends via sudo pacman; --cleanbuild re-extracts the
# tree; --nocheck skips the in-PKGBUILD test suite (the workflow's verify job
# already builds + tests the tagged tree on its own).
makepkg --clean --cleanbuild --syncdeps --noconfirm --nocheck

[[ -f "$built" ]] || { echo "expected package not produced: $built" >&2; exit 1; }

mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"
cp -f "$built" "$OUT/$name"
( cd "$OUT" && sha256sum "$name" > "$name.sha256" )

echo "built $OUT/$name"
cat "$OUT/$name.sha256"
