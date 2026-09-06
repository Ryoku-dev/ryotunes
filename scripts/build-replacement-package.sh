#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${1:-$ROOT/dist}"
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"

VERSION="$(sed -n 's/^version = "\([0-9.]*\)"/\1/p' "$(dirname "$0")/../Cargo.toml" | head -1)"
PKGREL="1"
PKGNAME="ryotunes-v2.4"
BINNAME="ryotunes-v2.4"
SYNCNAME="ryotunes-v2.4-sync"
ICONNAME="ryotunes-v2.4"
LIBDIR="/usr/lib/ryotunes-v2.4"
SHAREDIR="/usr/share/ryotunes-v2.4"
HOOKNAME="99-ryotunes-v2.4-replacement.hook"
INSTALLSCRIPT="ryotunes-v2.4.install"

RYO="$ROOT/target/release/ryotunes"
SYNC="$ROOT/target/release/ryotunes-sync"
[[ -x "$RYO" ]] || { echo "missing release binary: $RYO" >&2; exit 1; }
[[ -x "$SYNC" ]] || { echo "missing sync binary: $SYNC" >&2; exit 1; }
for cmd in makepkg bsdtar zstd zip sha256sum; do
  command -v "$cmd" >/dev/null || { echo "missing command: $cmd" >&2; exit 1; }
done

WORK="$(mktemp -d -t ryotunes-v2.4-package-XXXXXX)"
trap 'rm -rf "$WORK"' EXIT
PKGWORK="$WORK/package"
mkdir -p "$PKGWORK"

install -Dm755 "$RYO" "$PKGWORK/ryotunes"
cat > "$PKGWORK/$BINNAME" <<'WRAP'
#!/usr/bin/env bash
set -e
exec /usr/lib/ryotunes-v2.4/ryotunes "$@"
WRAP
chmod 755 "$PKGWORK/$BINNAME"
install -Dm755 "$SYNC" "$PKGWORK/$SYNCNAME"

cp "$ROOT/packaging/linux/ryotunes.desktop" "$PKGWORK/ryotunes.desktop"
sed -i -e 's|^Exec=.*|Exec=ryotunes|' -e "s|^Icon=.*|Icon=$ICONNAME|" "$PKGWORK/ryotunes.desktop"
grep -q '^X-Ryotunes-Replacement=true$' "$PKGWORK/ryotunes.desktop" || printf '%s
' 'X-Ryotunes-Replacement=true' >> "$PKGWORK/ryotunes.desktop"

cp "$ROOT/LICENSE" "$PKGWORK/LICENSE"
cp "$ROOT/README.md" "$PKGWORK/README.md"
cp "$ROOT/RELEASE_NOTES.md" "$PKGWORK/RELEASE_NOTES.md"
cp "$ROOT/UPSTREAM.md" "$PKGWORK/UPSTREAM.md"
cp "$ROOT/integrations/quickshell/RyotunesBarWidget.qml" "$PKGWORK/RyotunesBarWidget.qml"
cp "$ROOT/integrations/quickshell/README.md" "$PKGWORK/QUICKSHELL_README.md"
cp "$ROOT/packaging/ryoku/ryotunes-window-rule.lua" "$PKGWORK/ryotunes-window-rule.lua"
cp "$ROOT/src-tauri/icons/32x32.png" "$PKGWORK/icon32.png"
cp "$ROOT/src-tauri/icons/64x64.png" "$PKGWORK/icon64.png"
cp "$ROOT/src-tauri/icons/128x128.png" "$PKGWORK/icon128.png"
cp "$ROOT/src-tauri/icons/128x128@2x.png" "$PKGWORK/icon256.png"
cp "$ROOT/src-tauri/icons/icon.png" "$PKGWORK/icon512.png"

cat > "$PKGWORK/activate-replacement" <<'ACTIVATE'
#!/usr/bin/env bash
set -euo pipefail
LIBDIR=/usr/lib/ryotunes-v2.4
REALBIN="$LIBDIR/ryotunes"
TEMPLATE=/usr/share/ryotunes-v2.4/ryotunes.desktop
TARGET_BIN=/usr/bin/ryotunes
TARGET_DESKTOP=/usr/share/applications/ryotunes.desktop
STATE=/var/lib/ryotunes-v2.4
BACKUP="$STATE/stock"

[[ -x "$REALBIN" ]] || { echo "Ryotunes replacement binary is missing: $REALBIN" >&2; exit 1; }
[[ -f "$TEMPLATE" ]] || { echo "Ryotunes desktop template is missing: $TEMPLATE" >&2; exit 1; }
mkdir -p "$BACKUP"

# When v2.4 is installed on top of an older custom package, the live /usr/bin and desktop entry
# are custom too, so they must never be mistaken for stock. Adopt the genuine stock backup that
# the previous replacement package already captured before that package is removed.
adopt_previous_backup() {
  local prev
  for prev in     /var/lib/ryotunes-v2.3/stock     /var/lib/ryotunes-v2.2/stock     /var/lib/ryotunes-v2.1/stock     /var/lib/ryotunes-v2.0/stock     /var/lib/ryotunes-v1.9/stock     /var/lib/ryotunes-v1.8/stock     /var/lib/ryotunes-v1.7/stock     /var/lib/ryotunes-v1.6/stock     /var/lib/ryotunes-v1.5/stock     /var/lib/ryotunes-v1.4/stock; do
    if [[ ! -e "$BACKUP/ryotunes" && ! -L "$BACKUP/ryotunes" ]]        && [[ -e "$prev/ryotunes" || -L "$prev/ryotunes" ]]; then
      cp -a "$prev/ryotunes" "$BACKUP/ryotunes"
    fi
    if [[ ! -f "$BACKUP/ryotunes.desktop" && -f "$prev/ryotunes.desktop" ]]; then
      cp -a "$prev/ryotunes.desktop" "$BACKUP/ryotunes.desktop"
    fi
    if [[ ( -e "$BACKUP/ryotunes" || -L "$BACKUP/ryotunes" )        && -f "$BACKUP/ryotunes.desktop" ]]; then
      break
    fi
  done
}
adopt_previous_backup

is_known_custom_bin() {
  local resolved
  resolved="$(readlink -f "$TARGET_BIN" 2>/dev/null || true)"
  [[ "$resolved" == "$REALBIN" ]] || [[ "$resolved" =~ ^/usr/lib/ryotunes-v(1\.[4-9]|2\.[0-3]|2[0-4])/ryotunes$ ]]
}
is_custom_desktop() {
  [[ -f "$TARGET_DESKTOP" ]] && grep -q '^X-Ryotunes-Replacement=true$' "$TARGET_DESKTOP"
}

if [[ -e "$TARGET_BIN" || -L "$TARGET_BIN" ]] && ! is_known_custom_bin; then
  rm -f "$BACKUP/ryotunes"
  cp -a "$TARGET_BIN" "$BACKUP/ryotunes"
fi
if [[ -f "$TARGET_DESKTOP" ]] && ! is_custom_desktop; then
  cp -a --remove-destination "$TARGET_DESKTOP" "$BACKUP/ryotunes.desktop"
fi

rm -f "$TARGET_BIN"
ln -s "$REALBIN" "$TARGET_BIN"
install -Dm644 "$TEMPLATE" "$TARGET_DESKTOP"
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database /usr/share/applications >/dev/null 2>&1 || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor >/dev/null 2>&1 || true
ACTIVATE
chmod 755 "$PKGWORK/activate-replacement"

cat > "$PKGWORK/deactivate-replacement" <<'DEACTIVATE'
#!/usr/bin/env bash
set -euo pipefail
REALBIN=/usr/lib/ryotunes-v2.4/ryotunes
TARGET_BIN=/usr/bin/ryotunes
TARGET_DESKTOP=/usr/share/applications/ryotunes.desktop
BACKUP=/var/lib/ryotunes-v2.4/stock

is_our_bin() {
  [[ -L "$TARGET_BIN" ]] && [[ "$(readlink -f "$TARGET_BIN" 2>/dev/null || true)" == "$REALBIN" ]]
}
is_our_desktop() {
  [[ -f "$TARGET_DESKTOP" ]]     && grep -q '^X-Ryotunes-Replacement=true$' "$TARGET_DESKTOP"     && grep -q '^Icon=ryotunes-v2.4$' "$TARGET_DESKTOP"
}

if is_our_bin; then
  rm -f "$TARGET_BIN"
  [[ -e "$BACKUP/ryotunes" || -L "$BACKUP/ryotunes" ]] && cp -a "$BACKUP/ryotunes" "$TARGET_BIN"
elif [[ ! -e "$TARGET_BIN" && ! -L "$TARGET_BIN" && ( -e "$BACKUP/ryotunes" || -L "$BACKUP/ryotunes" ) ]]; then
  cp -a "$BACKUP/ryotunes" "$TARGET_BIN"
fi

if is_our_desktop; then
  rm -f "$TARGET_DESKTOP"
  [[ -f "$BACKUP/ryotunes.desktop" ]] && cp -a "$BACKUP/ryotunes.desktop" "$TARGET_DESKTOP"
elif [[ ! -f "$TARGET_DESKTOP" && -f "$BACKUP/ryotunes.desktop" ]]; then
  cp -a "$BACKUP/ryotunes.desktop" "$TARGET_DESKTOP"
fi
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database /usr/share/applications >/dev/null 2>&1 || true
DEACTIVATE
chmod 755 "$PKGWORK/deactivate-replacement"

cat > "$PKGWORK/$HOOKNAME" <<'HOOK'
[Trigger]
Operation = Install
Operation = Upgrade
Type = Path
Target = usr/bin/ryotunes
Target = usr/share/applications/ryotunes.desktop

[Action]
Description = Re-activating Ryotunes v2.4 after a Ryoku package update
When = PostTransaction
Exec = /usr/lib/ryotunes-v2.4/activate-replacement
HOOK

cat > "$PKGWORK/$INSTALLSCRIPT" <<'INSTALL'
post_install() {
  /usr/lib/ryotunes-v2.4/activate-replacement || return 1
}
post_upgrade() {
  /usr/lib/ryotunes-v2.4/activate-replacement || return 1
}
pre_remove() {
  /usr/lib/ryotunes-v2.4/deactivate-replacement || true
}
post_remove() {
  rm -rf /var/lib/ryotunes-v2.4
  command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database /usr/share/applications >/dev/null 2>&1 || true
}
INSTALL

cat > "$PKGWORK/PKGBUILD" <<'PKG'
pkgname=ryotunes-v2.4
pkgver=$VERSION
pkgrel=1
pkgdesc='Ryotunes v2.4 - Ryoku replacement Linux desktop music client'
arch=('x86_64')
url='https://github.com/ashmitvoid/RYOTUNES'
license=('GPL-3.0-or-later')
depends=('webkit2gtk-4.1' 'libappindicator-gtk3' 'mpv' 'openssl' 'librsvg' 'desktop-file-utils' 'hicolor-icon-theme' 'xdg-utils')
provides=('ryotunes=$VERSION')
conflicts=('ryotunes-v2.3' 'ryotunes-v2.2' 'ryotunes-v2.1' 'ryotunes-v2.0')
install='ryotunes-v2.4.install'
source=('ryotunes-v2.4' 'ryotunes' 'ryotunes-v2.4-sync' 'ryotunes.desktop' 'activate-replacement' 'deactivate-replacement' '99-ryotunes-v2.4-replacement.hook' 'ryotunes-v2.4.install' 'LICENSE' 'README.md' 'RELEASE_NOTES.md' 'UPSTREAM.md' 'RyotunesBarWidget.qml' 'QUICKSHELL_README.md' 'icon32.png' 'icon64.png' 'icon128.png' 'icon256.png' 'icon512.png' 'ryotunes-window-rule.lua')
sha256sums=('SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP' 'SKIP')
package() {
  install -Dm755 ryotunes-v2.4 "$pkgdir/usr/bin/ryotunes-v2.4"
  install -Dm755 ryotunes "$pkgdir/usr/lib/ryotunes-v2.4/ryotunes"
  install -Dm755 ryotunes-v2.4-sync "$pkgdir/usr/bin/ryotunes-v2.4-sync"
  install -Dm755 activate-replacement "$pkgdir/usr/lib/ryotunes-v2.4/activate-replacement"
  install -Dm755 deactivate-replacement "$pkgdir/usr/lib/ryotunes-v2.4/deactivate-replacement"
  install -Dm644 ryotunes.desktop "$pkgdir/usr/share/ryotunes-v2.4/ryotunes.desktop"
  install -Dm644 99-ryotunes-v2.4-replacement.hook "$pkgdir/usr/share/libalpm/hooks/99-ryotunes-v2.4-replacement.hook"
  install -Dm644 RyotunesBarWidget.qml "$pkgdir/usr/share/ryotunes-v2.4/quickshell/RyotunesBarWidget.qml"
  install -Dm644 QUICKSHELL_README.md "$pkgdir/usr/share/doc/ryotunes-v2.4/QUICKSHELL.md"
  install -Dm644 ryotunes-window-rule.lua "$pkgdir/usr/share/ryotunes-v2.4/ryotunes-window-rule.lua"
  install -Dm644 icon32.png "$pkgdir/usr/share/icons/hicolor/32x32/apps/ryotunes-v2.4.png"
  install -Dm644 icon64.png "$pkgdir/usr/share/icons/hicolor/64x64/apps/ryotunes-v2.4.png"
  install -Dm644 icon128.png "$pkgdir/usr/share/icons/hicolor/128x128/apps/ryotunes-v2.4.png"
  install -Dm644 icon256.png "$pkgdir/usr/share/icons/hicolor/256x256/apps/ryotunes-v2.4.png"
  install -Dm644 icon512.png "$pkgdir/usr/share/icons/hicolor/512x512/apps/ryotunes-v2.4.png"
  install -Dm644 LICENSE "$pkgdir/usr/share/licenses/ryotunes-v2.4/LICENSE"
  install -Dm644 README.md "$pkgdir/usr/share/doc/ryotunes-v2.4/README.md"
  install -Dm644 RELEASE_NOTES.md "$pkgdir/usr/share/doc/ryotunes-v2.4/RELEASE_NOTES.md"
  install -Dm644 UPSTREAM.md "$pkgdir/usr/share/doc/ryotunes-v2.4/UPSTREAM.md"
}
PKG
(cd "$PKGWORK" && makepkg --clean --cleanbuild --noconfirm)
PKGFILE="$(find "$PKGWORK" -maxdepth 1 -name "$PKGNAME-$VERSION-$PKGREL-*.pkg.tar.zst" -print -quit)"
[[ -n "$PKGFILE" ]] || { echo "package creation failed" >&2; exit 1; }

LIST="$WORK/package-contents.txt"
bsdtar -tf "$PKGFILE" > "$LIST"
for path in   'usr/bin/ryotunes-v2.4'   'usr/lib/ryotunes-v2.4/ryotunes'   'usr/share/ryotunes-v2.4/ryotunes.desktop'   'usr/share/ryotunes-v2.4/ryotunes-window-rule.lua'   'usr/share/libalpm/hooks/99-ryotunes-v2.4-replacement.hook'; do
  grep -qx "$path" "$LIST" || { echo "missing package path: $path" >&2; exit 1; }
done
if grep -Eq '^usr/bin/ryotunes$|^usr/share/applications/ryotunes(-v[^/]*)?\.desktop$' "$LIST"; then
  echo "replacement package claims a Ryoku-owned launcher path" >&2
  exit 1
fi

cp "$PKGFILE" "$OUT/"
FINAL_PKG="$OUT/$(basename "$PKGFILE")"

# AUR/prebuilt payload: package-owned usr/ tree only, no pacman metadata.
PAYLOAD_DIR="$WORK/payload"
mkdir -p "$PAYLOAD_DIR"
bsdtar -xf "$FINAL_PKG" -C "$PAYLOAD_DIR" usr
tar --zstd -C "$PAYLOAD_DIR" -cf "$OUT/ryotunes-v$VERSION-linux-x86_64.tar.zst" usr

# One-click-ish end-user folder. install.sh keeps old custom packages out of the way and reasserts
# v2.4 after their uninstall hooks restore stock.
BUNDLE="$WORK/Ryotunes-v2.4-Ryoku-x86_64"
mkdir -p "$BUNDLE"
cp "$FINAL_PKG" "$BUNDLE/"
cp "$ROOT/scripts/ryoku-window-rule.sh" "$BUNDLE/ryoku-window-rule.sh"
cat > "$BUNDLE/install.sh" <<'BINSTALL'
#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
PKG="$(find "$HERE" -maxdepth 1 -name 'ryotunes-v2.4-$VERSION-1-x86_64.pkg.tar.zst' -print -quit)"
[[ -n "$PKG" ]] || { echo "Ryotunes v2.4 package not found beside install.sh" >&2; exit 1; }

sudo pacman -U --needed "$PKG"
for old in ryotunes-v2.3 ryotunes-v2.2 ryotunes-v2.1 ryotunes-v2.0 ryotunes-v20 ryotunes-v21 ryotunes-v22 ryotunes-v23 ryotunes-v24 ryotunes-v1.4 ryotunes-v1.5 ryotunes-v1.6 ryotunes-v1.7 ryotunes-v1.8 ryotunes-v1.9; do
  if pacman -Q "$old" >/dev/null 2>&1; then
    sudo pacman -R --noconfirm "$old"
  fi
done
sudo /usr/lib/ryotunes-v2.4/activate-replacement
"$HERE/ryoku-window-rule.sh" install
command -v update-desktop-database >/dev/null 2>&1 && sudo update-desktop-database /usr/share/applications >/dev/null 2>&1 || true
echo "Ryotunes v2.4 installed. Launch it with: ryotunes"
BINSTALL
chmod 755 "$BUNDLE/install.sh" "$BUNDLE/ryoku-window-rule.sh"

cat > "$BUNDLE/uninstall.sh" <<'BUNINSTALL'
#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
"$HERE/ryoku-window-rule.sh" remove || true
sudo pacman -Rns ryotunes-v2.4
BUNINSTALL
chmod 755 "$BUNDLE/uninstall.sh"

cat > "$BUNDLE/README.txt" <<'BREADME'
Ryotunes v2.4 for Ryoku / CachyOS / Arch x86_64

Recommended:
  ./install.sh

Manual package install:
  sudo pacman -U ./ryotunes-v2.4-$VERSION-1-x86_64.pkg.tar.zst

The installer keeps ryoku-desktop installed and replaces only the stock Ryotunes entry points.
BREADME

(
  cd "$BUNDLE"
  sha256sum "$(basename "$FINAL_PKG")" > SHA256SUMS
)
(cd "$WORK" && zip -qr "$OUT/Ryotunes-v2.4-Ryoku-x86_64.zip" "$(basename "$BUNDLE")")

(
  cd "$OUT"
  sha256sum     "$(basename "$FINAL_PKG")"     ryotunes-v$VERSION-linux-x86_64.tar.zst     Ryotunes-v2.4-Ryoku-x86_64.zip     > SHA256SUMS-v$VERSION.txt
)

echo "release assets written to $OUT"
cat "$OUT/SHA256SUMS-v$VERSION.txt"
