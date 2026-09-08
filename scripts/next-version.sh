#!/usr/bin/env bash
# Print the next Ryotunes release version, computed purely from the published
# git tags. This is the single source of truth for the version ledger; the
# release workflow calls it to reserve the next version, and it is safe to run
# locally to preview the next number.
#
#   scripts/next-version.sh            # read tags from git
#   RYOTUNES_TAGS="v1.0.9" scripts/next-version.sh   # smoke a rollover, no git
#
# The scheme (matches crates/core::update::version): the major is fixed at 1,
# the patch is a single digit 0..9, the minor is unbounded. The next version is
# the highest existing v1 tag advanced by one step, rolling 1.0.9 -> 1.1.0. When
# no v1 tag exists yet the first baseline is 1.0.0, regardless of any legacy v2
# tags (those are a different line and never enter this ledger).
set -euo pipefail

if [[ -n "${RYOTUNES_TAGS+x}" ]]; then
  tags="$RYOTUNES_TAGS"
else
  tags="$(git tag -l 'v1.*')"
fi

best_minor=-1
best_patch=-1
for t in $tags; do
  # major is literally 1; minor has no leading zero; patch is one digit 0..9.
  if [[ "$t" =~ ^v1\.(0|[1-9][0-9]*)\.([0-9])$ ]]; then
    minor="${BASH_REMATCH[1]}"
    patch="${BASH_REMATCH[2]}"
    if (( minor > best_minor || (minor == best_minor && patch > best_patch) )); then
      best_minor="$minor"
      best_patch="$patch"
    fi
  fi
done

if (( best_minor < 0 )); then
  echo "1.0.0"
  exit 0
fi

minor="$best_minor"
patch=$(( best_patch + 1 ))
if (( patch > 9 )); then
  patch=0
  minor=$(( minor + 1 ))
fi
echo "1.$minor.$patch"
