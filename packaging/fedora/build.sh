#!/usr/bin/env bash
# Execute inside Fedora 44 after installing the spec's BuildRequires.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
out=$(realpath -m "${1:-$root/fedora-out}")
"$root/packaging/fedora/prepare-source.sh" "$out"
export SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-$(git -C "$root" log -1 --format=%ct)}
rpmbuild -ba --define "_topdir $out" --define '_buildhost fedora44-builder' --define 'use_source_date_epoch_as_buildtime 1' \
    --define "_smp_build_ncpus ${CARGO_BUILD_JOBS:-4}" "$out/SPECS/ryotunes.spec"
