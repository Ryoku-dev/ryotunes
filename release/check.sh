#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

unformatted=$(gofmt -l .)
if [[ -n $unformatted ]]; then
  printf 'gofmt is required for:\n%s\n' "$unformatted" >&2
  exit 1
fi

go vet ./...
go test ./...
CGO_ENABLED=0 go build -trimpath ./cmd/ryotunes-cli ./cmd/ryotunes-sync

version=$(tr -d '[:space:]' < VERSION)
release_version=$(awk '/^## v[0-9]/{ value=$2; sub(/^v/, "", value); print value; exit }' CHANGELOG.md)
if [[ -z $release_version || $version != "$release_version" ]]; then
  printf 'VERSION is %s but the top CHANGELOG release is %s.\n' "$version" "${release_version:-missing}" >&2
  exit 1
fi

if git grep -nF $'\xe2\x80\x94' -- .; then
  echo 'Tracked text must not contain an em dash.' >&2
  exit 1
fi

if git grep -nEI '#[0-9a-f]{6}([0-9a-f]{2})?' -- internal/client ':(exclude)internal/client/ui/look/brand.go'; then
  echo 'Client colour literals belong only in internal/client/ui/look/brand.go.' >&2
  exit 1
fi

if command -v desktop-file-validate >/dev/null 2>&1; then
  desktop-file-validate release/linux/ryotunes.desktop
fi

printf 'release checks passed for %s\n' "$version"
