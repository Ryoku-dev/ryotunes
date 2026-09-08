#!/usr/bin/env bash
# Build the reproducible source tarball a release attaches, locally. This is the
# same artifact the release workflow publishes: ryotunes-<version>.tar.gz with a
# ryotunes-<version>/ prefix, produced by `git archive` so a tag yields identical
# bytes every time. Run from a committed checkout (uncommitted changes are not
# included, by design).
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

if [[ -n "$(git status --porcelain --untracked-files=normal)" ]]; then
  echo "source release: commit the source first; refusing to label an older HEAD with the working-tree version" >&2
  exit 1
fi

ver="$(sed -n 's/^version = "\([0-9.]*\)"/\1/p' Cargo.toml | head -1)"
out="${1:-$root/ryotunes-$ver.tar.gz}"

"$root/scripts/release-check.sh"

git archive --format=tar --prefix="ryotunes-$ver/" HEAD | gzip -n > "$out"
( cd "$(dirname "$out")" && sha256sum "$(basename "$out")" > "$(basename "$out").sha256" )
printf 'Created %s (+ .sha256)\n' "$out"
