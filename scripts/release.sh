#!/usr/bin/env bash
# Preview the next version suggested by the historical release tags.
# scripts/release.sh --write syncs that version into the working tree only.
# Review the suggested version; Fedora releases no longer create version tags.
# To choose a specific version, use scripts/sync-version.sh VERSION --release.
# The release workflow builds the checked-in Fedora packages and submits their
# SRPMs to Copr on default-branch pushes or manual dispatch.
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
echo "A push to the default branch builds the checked-in Fedora version and"
echo "submits its SRPMs to Copr. Nothing is committed, tagged or pushed here."

if (( write )); then
  echo
  echo "applying v$next to the working tree for review (not committing)..."
  scripts/sync-version.sh "$next" --release
  echo "review the version and diff before committing and pushing to release."
fi
