#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ver="$(sed -n 's/^version = "\([0-9.]*\)"/\1/p' "$root/Cargo.toml" | head -1)"
out="${1:-$root/ryotunes-v$ver-final-source.tar.gz}"

"$root/scripts/release-check.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
stage="$tmp/ryotunes-v$ver"
mkdir -p "$stage"

tar \
  --exclude='.git' \
  --exclude='target' \
  --exclude='ui/node_modules' \
  --exclude='.pnpm-store' \
  --exclude='ui/build' \
  --exclude='ui/.svelte-kit' \
  --exclude='__pycache__' \
  --exclude='*.pyc' \
  --exclude='*.sqlite' \
  --exclude='*.sqlite-shm' \
  --exclude='*.sqlite-wal' \
  --exclude='*.log' \
  --exclude='src-tauri/lastfm.keys' \
  --exclude='ryotunes-v2.4.1-final-source.tar.gz' \
  -C "$root" -cf - . | tar -C "$stage" -xf -

tar -C "$tmp" -czf "$out" ryotunes-v2.4.1
printf 'Created %s\n' "$out"
