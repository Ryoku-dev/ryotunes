#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
version=${1:-$(tr -d '[:space:]' < "$root/VERSION")}
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Expected a semantic version.' >&2; exit 2; }

if grep -Eq "^## v${version}( |$)" "$root/CHANGELOG.md"; then
  section="## v$version"
else
  section='## Unreleased'
fi

printf '# Ryotunes %s\n\n' "$version"
awk -v section="$section" '
  /^## / {
    if (printing) exit
    printing = $0 == section || index($0, section " ") == 1
    next
  }
  printing { print }
' "$root/CHANGELOG.md"
cat <<EOF

## Install or update on Arch Linux

Download \`ryotunes-$version-1-x86_64.pkg.tar.zst\` and its \`.sha256\` sidecar, then run:

\`\`\`sh
sha256sum -c ryotunes-$version-1-x86_64.pkg.tar.zst.sha256
sudo pacman -U ./ryotunes-$version-1-x86_64.pkg.tar.zst
\`\`\`

Quit Ryotunes and reopen it after the transaction so ryotunesd starts the new binary.
EOF
