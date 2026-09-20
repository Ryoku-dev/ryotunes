#!/usr/bin/env bash
# Same payload and scriptlets with an older release number, for DNF lifecycle tests.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
out=$(realpath -m "${1:-$root/fedora-out}")
export SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-$(git -C "$root" log -1 --format=%ct)}
rpmbuild -bb --nocheck --define "_topdir $out" --define "_rpmdir $out/upgrade" \
    --define 'ryotunes_release 0' --define '_buildhost fedora44-builder' \
    --define 'use_source_date_epoch_as_buildtime 1' \
    --define "_smp_build_ncpus ${CARGO_BUILD_JOBS:-4}" "$out/SPECS/ryotunes.spec"
