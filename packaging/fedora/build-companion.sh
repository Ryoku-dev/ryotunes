#!/usr/bin/env bash
# Validation bridge until Ryoku publishes the shared runtime. Never edits the input checkout.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
out=$(realpath -m "${1:-$root/fedora-out/companion}")
revision=f4a932df3bf731ab385f145060067a48945c3cbe
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
repo=${2:-$stage/repo}
if [[ $# -lt 2 ]]; then
    git init -q "$repo"
    git -C "$repo" fetch --depth=1 https://github.com/itsKontra/ryoku-arch.git "$revision"
fi
version=0.1
export SOURCE_DATE_EPOCH=$(git -C "$repo" show -s --format=%ct "$revision")
mkdir -p "$stage/ryoku-$version" "$out/SOURCES" "$out/SPECS"
git -C "$repo" archive "$revision" LICENSE ryoku/ui release/rpm/ryoku-desktop.spec release/rpm/stage-package.sh |
    tar -x -C "$stage/ryoku-$version"
cd "$stage/ryoku-$version"
for patchfile in "$root"/packaging/fedora/companion/*.patch; do patch -p1 < "$patchfile"; done
cp release/rpm/ryoku-ui.spec "$out/SPECS/"
tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner -C "$stage" -cf - "ryoku-$version" |
    gzip -n > "$out/SOURCES/ryoku-$version.tar.gz"
rpmbuild -ba --define "_topdir $out" --define '_buildhost fedora44-builder' \
    --define 'source_date_epoch_from_changelog 0' --define 'use_source_date_epoch_as_buildtime 1' \
    --define 'clamp_mtime_to_source_date_epoch 1' "$out/SPECS/ryoku-ui.spec"
