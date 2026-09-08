#!/usr/bin/env bash
# Ryotunes releases are automatic. Pushing to the default branch runs
# .github/workflows/release.yml, which computes the next v1 version from the
# tags, syncs every manifest, verifies the tree, then commits + tags + publishes
# the GitHub release (package + source tarball + notes). You do not cut releases
# by hand and you never push a tag locally.
#
# This script is the local preview of that pipeline:
#
#   scripts/release.sh            # show the current and next version + the plan
#   scripts/release.sh --write    # additionally apply the bump to the working
#                                 # tree for review (no commit, tag or push)
#
# To re-publish an existing tag (recover a failed package/publish), use the
# workflow's manual trigger: Actions -> Release -> Run workflow -> the tag.
# To set a specific version out of band (a reset), run scripts/sync-version.sh.
set -euo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
cd "$here"

write=0
[[ "${1:-}" == "--write" ]] && write=1

cur="$(sed -n 's/^version = "\([0-9.]*\)"/\1/p' Cargo.toml | head -1)"
next="$(scripts/next-version.sh)"

echo "current source version : $cur"
echo "next release version   : $next"
echo
echo "A push to the default branch will reserve v$next, build the Arch package"
echo "and source tarball, and publish the GitHub release. Nothing is pushed here."

if (( write )); then
  echo
  echo "applying v$next to the working tree for review (not committing)..."
  scripts/sync-version.sh "$next" --release
  echo "review the diff, then commit and push to release (CI owns the tag)."
fi
