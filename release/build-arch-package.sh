#!/usr/bin/env bash
# Build the exact checkout and publish the epochless package filename contract.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
out=${1:-$root/dist}

if [[ $(id -u) -eq 0 ]]; then
  pacman -Syu --noconfirm --needed base-devel git go sudo
  id builder >/dev/null 2>&1 || useradd -m builder
  printf 'builder ALL=(ALL) NOPASSWD: /usr/bin/pacman\n' > /etc/sudoers.d/builder
  chmod 440 /etc/sudoers.d/builder
  chown -R builder:builder "$root"
  install -d -o builder -g builder "$out"
  exec sudo -u builder -H bash "${BASH_SOURCE[0]}" "$out"
fi

command -v makepkg >/dev/null || { echo 'makepkg is required.' >&2; exit 1; }
version=$(tr -d '[:space:]' < "$root/VERSION")
epoch=$(sed -n 's/^epoch=\([0-9][0-9]*\)$/\1/p' "$root/release/arch/PKGBUILD")
[[ $epoch == 1 ]] || { echo 'release/arch/PKGBUILD must keep epoch=1.' >&2; exit 1; }

wanted="ryotunes-${epoch}:${version}-1-x86_64.pkg.tar.zst"
contract="ryotunes-${version}-1-x86_64.pkg.tar.zst"
cd "$root/release/arch"

built=
while IFS= read -r candidate; do
  if [[ $(basename "$candidate") == "$wanted" ]]; then
    built=$candidate
    break
  fi
done < <(makepkg --packagelist)
[[ -n $built ]] || { echo "makepkg will not produce $contract" >&2; exit 1; }

makepkg --clean --cleanbuild --syncdeps --noconfirm --nocheck
[[ -f $built ]] || { echo "Expected package was not produced: $built" >&2; exit 1; }
"$root/release/check-glibc.sh" "$built"

mkdir -p "$out"
out=$(cd "$out" && pwd)
cp -f "$built" "$out/$contract"
(
  cd "$out"
  sha256sum "$contract" > "$contract.sha256"
)
printf 'Built %s/%s\n' "$out" "$contract"
cat "$out/$contract.sha256"
