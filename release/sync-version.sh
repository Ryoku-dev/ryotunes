#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
version=${1:-}
release=${2:-}
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Usage: sync-version.sh X.Y.Z [--release]' >&2; exit 2; }
[[ -z $release || $release == --release ]] || { echo 'The only optional argument is --release.' >&2; exit 2; }

printf '%s\n' "$version" > VERSION
sed -i "s/^pkgver=.*/pkgver=$version/" release/arch/PKGBUILD release/aur/PKGBUILD
sed -i 's/^pkgrel=.*/pkgrel=1/' release/arch/PKGBUILD release/aur/PKGBUILD

if ! grep -q "<release version=\"$version\"" release/linux/dev.ryoku.ryotunes.metainfo.xml; then
  today=${SOURCE_DATE_EPOCH:+$(date -u -d "@$SOURCE_DATE_EPOCH" +%F)}
  today=${today:-$(date -u +%F)}
  sed -i "/<releases>/a\    <release version=\"$version\" date=\"$today\"/>" release/linux/dev.ryoku.ryotunes.metainfo.xml
fi

if [[ $release == --release ]] && ! grep -q "^## v$version " CHANGELOG.md; then
  today=${today:-$(date -u +%F)}
  sed -i "/^## Unreleased$/a\\
\\
## v$version - $today" CHANGELOG.md
fi

printf 'Synchronized Ryotunes %s.\n' "$version"
