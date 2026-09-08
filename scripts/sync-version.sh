#!/usr/bin/env bash
# Make the working tree carry one coherent version everywhere. This is the only
# code that writes the version into the manifests; the release workflow and the
# local preview (scripts/release.sh) both call it so a release is always
# internally consistent.
#
#   scripts/sync-version.sh 1.0.3             # sync every manifest to 1.0.3
#   scripts/sync-version.sh 1.0.3 --release   # ...and roll the changelog
#
# Version numbers written: the Cargo workspace, the workspace members in
# Cargo.lock (never the coincidentally-named registry crates), the Tauri config,
# the web UI package, the native QML client's version file, and the Arch PKGBUILD
# (pkgver, pkgrel pinned to 1, and the
# permanent epoch=1 that lets pacman upgrade a machine off the legacy 2.x line
# onto 1.x). It is idempotent: running it twice is a no-op, and the changelog is
# only rolled once per version.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

ver="${1:-}"
release=0
[[ "${2:-}" == "--release" ]] && release=1
[[ "$ver" =~ ^1\.(0|[1-9][0-9]*)\.[0-9]$ ]] || {
  echo "sync-version: '$ver' is not a v1 release version (1.<minor>.<digit>)" >&2
  exit 2
}

# Cargo workspace version (the first `version = "X.Y.Z"`, under [workspace.package]).
sed -i '0,/^version = "[0-9][0-9.]*"$/s//version = "'"$ver"'"/' Cargo.toml
# Tauri and the web UI each carry exactly one "version" key.
sed -i 's/"version": "[0-9][0-9.]*"/"version": "'"$ver"'"/' src-tauri/tauri.conf.json
sed -i 's/"version": "[0-9][0-9.]*"/"version": "'"$ver"'"/' ui/package.json
# The native QML client reads its own APP version from this file at runtime
# (Quickshell.shellDir + '/version'); it ships inside /usr/share/ryotunes/client.
printf '%s\n' "$ver" > client/version
# Arch package: pkgver tracks the version, pkgrel is the release-asset contract's
# fixed 1, epoch=1 is permanent so 1:X.Y.Z outranks the installed 2.x.
sed -i 's/^pkgver=.*/pkgver='"$ver"'/; s/^pkgrel=.*/pkgrel=1/' packaging/arch/PKGBUILD
if grep -q '^epoch=' packaging/arch/PKGBUILD; then
  sed -i 's/^epoch=.*/epoch=1/' packaging/arch/PKGBUILD
else
  sed -i '/^pkgrel=/a epoch=1' packaging/arch/PKGBUILD
fi

# Cargo.lock: bump only the workspace members (path crates), matched by the names
# the workspace actually declares, so registry crates that happen to be at the
# same version (tauri-plugin-*) are left untouched.
python3 - "$ver" <<'PY'
import datetime, re, sys, tomllib
from pathlib import Path

ver = sys.argv[1]
root = Path.cwd()
cargo = tomllib.loads((root / "Cargo.toml").read_text())
names = set()
for member in cargo["workspace"]["members"]:
    names.add(tomllib.loads((root / member / "Cargo.toml").read_text())["package"]["name"])

lock_path = root / "Cargo.lock"
lock = lock_path.read_text()
for name in names:
    # In Cargo.lock the version always immediately follows the name line.
    lock = re.sub(
        rf'(?m)^(name = "{re.escape(name)}"\nversion = ")[^"]*(")',
        rf'\g<1>{ver}\g<2>',
        lock,
    )
lock_path.write_text(lock)

metadata = root / "packaging/linux/dev.ryoku.ryotunes.metainfo.xml"
text = metadata.read_text()
current = re.search(r'<releases>\s*<release version="([^"]+)"', text)
if current is None:
    raise SystemExit("AppStream metadata has no release list")
if current.group(1) != ver:
    text = text.replace("<releases>", f'<releases><release version="{ver}" date="{datetime.date.today().isoformat()}"/>', 1)
    metadata.write_text(text)
PY

if (( release )); then
  # The Unreleased section becomes this version's, dated today. Idempotent: if a
  # "## vX.Y.Z" heading already exists (a local preview or a re-run cut it), the
  # changelog is left alone.
  python3 - "$ver" <<'PY'
import datetime, sys
from pathlib import Path

ver = sys.argv[1]
p = Path("CHANGELOG.md")
s = p.read_text()
if f"## v{ver}" in s:
    raise SystemExit(0)
if "## Unreleased" not in s:
    raise SystemExit("CHANGELOG.md has no '## Unreleased' section")
today = datetime.date.today().isoformat()
p.write_text(s.replace("## Unreleased", f"## Unreleased\n\n## v{ver} - {today}", 1))
PY
fi

echo "sync-version: $ver"
