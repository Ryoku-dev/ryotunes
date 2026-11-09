#!/usr/bin/env bash
# Fail when a cgo binary (the client or the daemon) requires a glibc newer
# than the supported ceiling; Void ships 2.41.
set -euo pipefail

package=${1:?usage: check-glibc.sh <package> [max-glibc]}
maximum=${2:-2.41}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
bsdtar -xf "$package" -C "$work" usr/bin/ryotunes usr/bin/ryotunesd

newer() {
  [[ $1 != "$2" && $(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -n1) == "$1" ]]
}

failed=0
for name in ryotunes ryotunesd; do
  binary=$work/usr/bin/$name
  if [[ $(od -An -N4 -tx1 "$binary" | tr -d ' \n') != 7f454c46 ]]; then
    printf 'usr/bin/%s is not an ELF binary.\n' "$name" >&2
    failed=1
    continue
  fi
  while read -r symbol_version symbol; do
    [[ -n ${symbol_version:-} ]] || continue
    symbol_version=${symbol_version#GLIBC_}
    symbol_version=${symbol_version%)}
    if newer "$symbol_version" "$maximum"; then
      printf 'usr/bin/%s: %s needs glibc %s (max %s)\n' "$name" "$symbol" "$symbol_version" "$maximum" >&2
      failed=1
    fi
  done < <(objdump -T "$binary" | grep -o 'GLIBC_[0-9.]*) *[^ ]*$' || true)
done

((failed == 0)) && printf 'glibc: ryotunes and ryotunesd run on %s\n' "$maximum"
exit "$failed"
