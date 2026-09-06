#!/usr/bin/env bash
# Cut a Ryotunes release: bump the one version every manifest carries, roll the
# changelog's Unreleased section under the new heading, commit and tag.
#
#   scripts/release.sh patch|minor|major|X.Y.Z [--push]
#
# The tag is what the release pipeline keys on (.github/workflows/release.yml):
# a `v*` tag builds, publishes the GitHub release with a source tarball, and
# tells Ryoku Arch to pick it up. Run it from a clean main checkout.
set -euo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
cd "$here"

bump="${1:-}"
push=0
[[ "${2:-}" == "--push" ]] && push=1
[[ -n "$bump" ]] || { echo "usage: scripts/release.sh patch|minor|major|X.Y.Z [--push]" >&2; exit 2; }

if [[ -n "$(git status --porcelain --untracked-files=no)" ]]; then
  echo "release: the tree has uncommitted changes" >&2
  exit 1
fi

cur="$(sed -n 's/^version = "\([0-9.]*\)"/\1/p' Cargo.toml | head -1)"
IFS=. read -r maj min pat <<<"$cur"
case "$bump" in
  patch) next="$maj.$min.$((pat + 1))" ;;
  minor) next="$maj.$((min + 1)).0" ;;
  major) next="$((maj + 1)).0.0" ;;
  [0-9]*.[0-9]*.[0-9]*) next="$bump" ;;
  *) echo "release: bad bump '$bump'" >&2; exit 2 ;;
esac

if git rev-parse -q --verify "refs/tags/v$next" >/dev/null; then
  echo "release: tag v$next already exists" >&2
  exit 1
fi

echo "release: $cur -> $next"

# Every manifest that carries the version. Cargo.lock follows through `cargo update`
# on the workspace members only (no dependency churn).
sed -i "0,/^version = \"$cur\"/s//version = \"$next\"/" Cargo.toml
sed -i "s/\"version\": \"$cur\"/\"version\": \"$next\"/" src-tauri/tauri.conf.json ui/package.json
cargo update --workspace --offline >/dev/null 2>&1 || cargo update --workspace >/dev/null
sed -i "s/^pkgver=$cur$/pkgver=$next/; s/^pkgrel=[0-9]*$/pkgrel=1/" packaging/arch/PKGBUILD
if grep -q "release invariants v$cur" scripts/check-release-invariants.py 2>/dev/null; then
  sed -i "s/v$cur/v$next/g" scripts/check-release-invariants.py
fi

# Changelog: the Unreleased section becomes this release's, dated.
python3 - "$next" <<'EOF'
import sys, re, datetime
next_ = sys.argv[1]
p = "CHANGELOG.md"
s = open(p).read()
today = datetime.date.today().isoformat()
if "## Unreleased" not in s:
    sys.exit("CHANGELOG.md has no '## Unreleased' section")
s = s.replace("## Unreleased", f"## Unreleased\n\n## v{next_} - {today}", 1)
open(p, "w").write(s)
EOF

git add Cargo.toml Cargo.lock src-tauri/tauri.conf.json ui/package.json packaging/arch/PKGBUILD CHANGELOG.md scripts/check-release-invariants.py 2>/dev/null || true
git commit -q -m "release: v$next"
git tag -a "v$next" -m "Ryotunes v$next"
echo "release: committed and tagged v$next"

if (( push )); then
  git push origin HEAD "refs/tags/v$next"
  echo "release: pushed; the release workflow builds and publishes it"
else
  echo "release: push with: git push origin HEAD refs/tags/v$next"
fi
