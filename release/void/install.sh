#!/usr/bin/env bash
# Install or update Ryotunes on Void by repacking the verified Arch release asset.
set -euo pipefail

repo=ryoku-dev/ryotunes
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

command -v xbps-create >/dev/null || { echo 'This installer requires Void Linux.' >&2; exit 2; }
command -v bsdtar >/dev/null || sudo xbps-install -Sy bsdtar

depends=$(
  while IFS= read -r dependency; do
    [[ -z $dependency || $dependency == \#* ]] && continue
    printf '%s>=0 ' "$dependency"
  done < "$root/release/void/depends"
)

tag=$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" |
  sed -n 's/.*"tag_name": *"\(v[0-9][0-9.]*\)".*/\1/p' | head -n1)
[[ -n $tag ]] || { echo 'Could not find the latest release.' >&2; exit 1; }
version=${tag#v}
package="ryotunes-$version-1-x86_64.pkg.tar.zst"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
curl -fsSLO "https://github.com/$repo/releases/download/$tag/$package"
curl -fsSLO "https://github.com/$repo/releases/download/$tag/$package.sha256"
sha256sum -c "$package.sha256"

mkdir root repository
bsdtar -xf "$package" -C root usr
rm -rf root/usr/lib/systemd
libs=$(ldd root/usr/bin/ryotunes 2>&1 || true)
if grep -q 'version `GLIBC_.*not found' <<<"$libs"; then
  echo "$tag requires a newer glibc than this system provides." >&2
  exit 1
fi

(
  cd repository
  xbps-create -q -A x86_64 -n "ryotunes-${version}_1" \
    -l GPL-3.0-or-later \
    -H "https://github.com/$repo" \
    -s 'Ryoku native music client, playback daemon, CLI, and sync server' \
    -D "$depends" ../root
)
xbps-rindex -a repository/*.xbps

upgrade=()
xbps-query ryotunes >/dev/null 2>&1 && upgrade=(-u)
sudo xbps-install -Sy "${upgrade[@]}" --repository="$work/repository" ryotunes
printf 'Installed Ryotunes %s. Quit Ryotunes and open it again to restart ryotunesd.\n' "$version"
