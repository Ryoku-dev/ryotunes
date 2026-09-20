#!/usr/bin/env python3
"""Cheap structural guardrails for release builds when the Rust toolchain is unavailable.

This is not a Rust parser. It only checks a few model initializers that have historically broken
when upstream added required fields. Cargo remains the authoritative check on a development host.
"""
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
SKIP = {'.git', 'target', 'node_modules', 'build', '.svelte-kit', 'fedora-out', '.fedora-build', 'vendor'}


def rust_files():
    for path in ROOT.rglob('*.rs'):
        if not any(part in SKIP for part in path.parts):
            yield path


def balanced_block(text: str, brace: int) -> tuple[str, int]:
    depth = 0
    i = brace
    state = 'code'
    block_depth = 0
    while i < len(text):
        c = text[i]
        n = text[i + 1] if i + 1 < len(text) else ''
        if state == 'line':
            if c == '\n':
                state = 'code'
        elif state == 'block':
            if c == '/' and n == '*':
                block_depth += 1
                i += 1
            elif c == '*' and n == '/':
                block_depth -= 1
                i += 1
                if block_depth == 0:
                    state = 'code'
        elif state == 'string':
            if c == '\\':
                i += 1
            elif c == '"':
                state = 'code'
        elif state == 'raw':
            # Handle ordinary r#"..."# strings well enough for model initializers.
            end = text.find('"', i)
            if end == -1:
                return text[brace + 1 :], len(text)
            i = end
            state = 'code'
        else:
            if c == '/' and n == '/':
                state = 'line'
                i += 1
            elif c == '/' and n == '*':
                state = 'block'
                block_depth = 1
                i += 1
            elif c == 'r' and n in {'"', '#'}:
                state = 'raw'
            elif c == '"':
                state = 'string'
            elif c == '{':
                depth += 1
            elif c == '}':
                depth -= 1
                if depth == 0:
                    return text[brace + 1 : i], i
        i += 1
    raise ValueError('unclosed initializer')


def initializers(name: str):
    pat = re.compile(rf'\b{re.escape(name)}\s*\{{')
    for path in rust_files():
        text = path.read_text(errors='replace')
        for match in pat.finditer(text):
            line_start = text.rfind('\n', 0, match.start()) + 1
            prefix = text[line_start : match.start()]
            if re.search(r'\bstruct\s*$', prefix) or '->' in prefix:
                continue
            brace = text.find('{', match.start(), match.end() + 1)
            body, _ = balanced_block(text, brace)
            line = text.count('\n', 0, match.start()) + 1
            yield path, line, body


def top_level_fields(body: str) -> tuple[set[str], bool]:
    fields: set[str] = set()
    spread = False
    depth = 0
    state = 'code'
    block_depth = 0
    token_start = 0
    tokens: list[str] = []
    i = 0
    while i < len(body):
        c = body[i]
        n = body[i + 1] if i + 1 < len(body) else ''
        if state == 'line':
            if c == '\n':
                state = 'code'
        elif state == 'block':
            if c == '/' and n == '*':
                block_depth += 1
                i += 1
            elif c == '*' and n == '/':
                block_depth -= 1
                i += 1
                if block_depth == 0:
                    state = 'code'
        elif state == 'string':
            if c == '\\':
                i += 1
            elif c == '"':
                state = 'code'
        else:
            if c == '/' and n == '/':
                state = 'line'
                i += 1
            elif c == '/' and n == '*':
                state = 'block'
                block_depth = 1
                i += 1
            elif c == '"':
                state = 'string'
            elif c in '({[':
                depth += 1
            elif c in ')}]':
                depth -= 1
            elif c == ',' and depth == 0:
                tokens.append(body[token_start:i])
                token_start = i + 1
        i += 1
    tokens.append(body[token_start:])

    for token in tokens:
        token = re.sub(r'(?m)^\s*//.*$', '', token).strip()
        if not token:
            continue
        if token.startswith('..'):
            spread = True
            continue
        match = re.match(r'([A-Za-z_][A-Za-z0-9_]*)\s*(?::|$)', token)
        if match:
            fields.add(match.group(1))
    return fields, spread


checks = {
    'SongItem': {'added_by', 'added_by_avatar', 'is_upload'},
    'BrowseItem': {'is_upload'},
    'PlaylistPage': {'collaborative'},
    'CachedStream': {'ping_url', 'ping_client'},
    'PlaybackData': {'playback_ping'},
}
failures: list[str] = []
for name, required in checks.items():
    for path, line, body in initializers(name):
        fields, spread = top_level_fields(body)
        if spread:
            continue
        missing = required - fields
        if missing:
            rel = path.relative_to(ROOT)
            failures.append(f'{rel}:{line}: {name} missing {", ".join(sorted(missing))}')

# UI_SETTINGS is the single source of the renderer-visible setting keys; it lives in the core so the
# Tauri host and the daemon share one list.
state_src = (ROOT / 'crates/core/src/state.rs').read_text()
match = re.search(r'pub const UI_SETTINGS:\s*\[&str;\s*(\d+)\]\s*=\s*\[(.*?)\];', state_src, re.S)
if not match:
    failures.append('crates/core/src/state.rs: could not parse UI_SETTINGS')
else:
    declared = int(match.group(1))
    actual = len(re.findall(r'"[^"\\]*(?:\\.[^"\\]*)*"', match.group(2)))
    if declared != actual:
        failures.append(f'crates/core/src/state.rs: UI_SETTINGS declares {declared}, contains {actual}')

if failures:
    print('\n'.join(failures), file=sys.stderr)
    sys.exit(1)
print('Rust model shapes: OK')
