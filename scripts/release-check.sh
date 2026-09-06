#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

fail=0
say() { printf '[%s] %s\n' "$1" "$2"; }

say check 'release identity'
# One version everywhere: Cargo.toml is the source (scripts/release.sh bumps it).
ver="$(sed -n 's/^version = "\([0-9.]*\)"/\1/p' Cargo.toml | head -1)"
[[ -n "$ver" ]] || { say FAIL 'workspace version missing from Cargo.toml'; fail=1; }
grep -q "\"version\": \"$ver\"" src-tauri/tauri.conf.json || { say FAIL "Tauri version is not $ver"; fail=1; }
grep -q "\"version\": \"$ver\"" ui/package.json || { say FAIL "UI version is not $ver"; fail=1; }
grep -q '"identifier": "dev.ryoku.ryotunes"' src-tauri/tauri.conf.json || { say FAIL 'unexpected application identifier'; fail=1; }

say check 'private-machine and secret patterns'
# Do not scan license/upstream attribution for project names. This scan is for actual release data.
if grep -RniE --exclude-dir=.git --exclude='.git' --exclude-dir=target --exclude-dir=node_modules --exclude-dir=.pnpm-store --exclude='UPSTREAM.md' --exclude='release-check.sh' \
  '(/home/[A-Za-z0-9._-]+/|/Users/[A-Za-z0-9._-]+/|[A-Z]:\\Users\\[^\\]+\\|BEGIN (RSA|OPENSSH|EC) PRIVATE KEY|[A-Za-z0-9-]+\.ts\.net)' .; then
  say FAIL 'machine-specific path, endpoint, or secret-like value found'
  fail=1
fi

say check 'legacy product strings in active source'
if grep -RniE --exclude-dir=.git --exclude-dir=target --exclude-dir=node_modules --exclude='UPSTREAM.md' --exclude='LICENSE' \
  --exclude='release-check.sh' 'limusic|LIMUSIC|SimoHypers' src-tauri crates ui packaging scripts 2>/dev/null; then
  say FAIL 'legacy branding remains in active source'
  fail=1
fi

say check 'Rust model invariants'
python scripts/check-source-shapes.py
python scripts/check-rust-structure.py

say check 'release interface invariants'
python3 scripts/check-release-invariants.py || fail=1

say check 'project structure and local links'
python scripts/check-project-links.py

say check 'JSON/TOML parse'
python - <<'PY'
import json, tomllib
from pathlib import Path
root=Path('.')
for p in root.rglob('*.json'):
    if any(x in p.parts for x in ('.git','target','node_modules','.svelte-kit','build')): continue
    if p.name == 'tsconfig.json': continue  # JSONC, parsed by TypeScript/Svelte tooling
    json.loads(p.read_text())
for p in root.rglob('*.toml'):
    if any(x in p.parts for x in ('.git','target','node_modules','.svelte-kit','build')): continue
    tomllib.loads(p.read_text())
print('JSON/TOML: OK')
PY

say check 'every shipped skin passes ryotunes-cli skin check'
if command -v cargo >/dev/null 2>&1; then
  cargo build -q -p ryotunes-cli
  for d in skins/*/; do
    [ -f "${d}skin.json" ] || continue
    ./target/debug/ryotunes-cli skin check "$d" >/dev/null \
      || { say FAIL "skin ${d} fails validation"; fail=1; }
  done
else
  say skip 'cargo unavailable; skin validation not run'
fi

if command -v node >/dev/null 2>&1; then
  say check 'TypeScript syntax'
  node scripts/check-ts-syntax.mjs
  say check 'pure frontend behaviour regressions'
  for check in dnd localsearch menu personal queue rows shortcut-match sort ytlink; do
    node --no-warnings --experimental-strip-types "ui/src/lib/${check}.check.ts"
  done
fi

if [[ "${RYOTUNES_STATIC_ONLY:-0}" == "1" ]]; then
  # Package builders run the semantic frontend check and the real Rust/Tauri compile exactly once
  # afterwards.  Keeping this pass structural avoids three duplicate frontend builds and an
  # unnecessary full cargo-test compile on an end-user machine.
  say skip 'static-only package preflight; compiler checks run by build-package.sh'
else
  if command -v pnpm >/dev/null 2>&1 && [[ -d ui/node_modules ]]; then
    say check 'Svelte frontend'
    (cd ui && pnpm check && pnpm build)
  else
    say skip 'pnpm/node_modules unavailable; frontend semantic check not run'
  fi

  if command -v cargo >/dev/null 2>&1; then
    say check 'Rust format and tests'
    cargo fmt --all -- --check
    cargo test --workspace
  else
    say skip 'cargo unavailable; Rust compiler checks not run'
  fi
fi

if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git diff --check
fi

(( fail == 0 )) || exit 1
say ok 'release checks completed'
