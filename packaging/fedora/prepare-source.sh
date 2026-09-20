#!/usr/bin/env bash
# Run with Fedora's cargo available. Network is used only to assemble the SRPM input.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
out=$(realpath -m "${1:-$root/fedora-out}")
version=$(python3 -c 'import tomllib,sys; print(tomllib.load(open(sys.argv[1],"rb"))["workspace"]["package"]["version"])' "$root/Cargo.toml")
export SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH:-$(git -C "$root" log -1 --format=%ct)}
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/ryotunes-$version" "$out/SOURCES" "$out/SPECS"
# Include local implementation changes, but never build output or local credentials.
git -C "$root" ls-files --cached --others --exclude-standard -z |
    tar -C "$root" --null -T - -cf - | tar -C "$stage/ryotunes-$version" -xf -
cd "$stage/ryotunes-$version"
mkdir -p .cargo
cargo vendor --locked vendor > .cargo/config.toml
# cargo vendor prints an absolute directory; source RPMs must remain relocatable.
sed -i 's|^directory = .*|directory = "vendor"|' .cargo/config.toml
printf '%s\n' "$SOURCE_DATE_EPOCH" > SOURCE_DATE_EPOCH
cp packaging/fedora/ryotunes.spec "$out/SPECS/"
tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner \
    -C "$stage" -cf - "ryotunes-$version" | gzip -n > "$out/SOURCES/ryotunes-$version.tar.gz"
sha256sum "$out/SOURCES/ryotunes-$version.tar.gz" > "$out/source.sha256"
