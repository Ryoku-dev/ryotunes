#!/usr/bin/env python3
"""Check release identity and shipped capability policy, not source spelling.

Builds, runtime smoke checks and behavioral tests cover implementation behavior;
matching comments, UI copy or function bodies cannot prove those contracts.
"""
from pathlib import Path
import json
import re
import tomllib
import xml.etree.ElementTree as ET

root = Path(__file__).resolve().parents[1]


def read(rel):
    return (root / rel).read_text()


def req(ok, msg):
    if not ok:
        raise SystemExit(f'release invariant failed: {msg}')


cargo = tomllib.loads(read('Cargo.toml'))
lock = tomllib.loads(read('Cargo.lock'))
version = cargo['workspace']['package']['version']
req(re.fullmatch(r'1\.(0|[1-9][0-9]*)\.[0-9]', version),
    'workspace version is not a v1 release (1.<minor>.<single-digit patch>)')
locked = {package['name']: package['version'] for package in lock['package']}
for member in cargo['workspace']['members']:
    manifest = tomllib.loads(read(f'{member}/Cargo.toml'))['package']
    member_version = manifest.get('version')
    if isinstance(member_version, dict) and member_version.get('workspace'):
        req(locked.get(manifest['name']) == version,
            f'Cargo.lock {manifest["name"]} version does not match {version}')

config = json.loads(read('src-tauri/tauri.conf.json'))
req(config.get('version') == version, 'Tauri version does not match workspace')
req(config.get('identifier') == 'dev.ryoku.ryotunes', 'Tauri application identity changed')
req(json.loads(read('ui/package.json')).get('version') == version,
    'UI version does not match workspace')
req(read('client/version').strip() == version,
    'native QML client version file does not match workspace')
appstream = ET.fromstring(read('packaging/linux/dev.ryoku.ryotunes.metainfo.xml'))
req(appstream.find('releases/release').get('version') == version,
    'AppStream release version does not match workspace')
arch = read('packaging/arch/PKGBUILD')
req(re.search(rf'^pkgver={re.escape(version)}$', arch, re.M),
    'Arch package version does not match workspace')
req(re.search(r'^pkgrel=1$', arch, re.M), 'release asset contract requires pkgrel=1')
req(re.search(r'^epoch=1$', arch, re.M),
    'release asset contract requires epoch=1 so pacman upgrades off the legacy 2.x line')

tauri_version = tuple(map(int, locked['tauri'].split('.')[:3]))
req(tauri_version >= (2, 11, 1),
    'Tauri regressed into the GHSA-7gmj-67g7-phm9 affected range')

for filename, window in [('default', 'main'), ('mini', 'mini')]:
    capability = json.loads(read(f'src-tauri/capabilities/{filename}.json'))
    req(capability.get('windows') == [window],
        f'{window} capability applies to unintended WebViews')
    permissions = set(capability.get('permissions', []))
    req({'allow-ui-commands', 'core:app:allow-version', 'core:event:default'} <= permissions,
        f'{window} capability is missing required application/event permissions')
    forbidden = {'dialog:allow-open', 'core:default', 'core:image:default',
                 'core:path:default', 'core:tray:default', 'core:menu:default',
                 'core:resources:default'}
    req(not permissions.intersection(forbidden),
        f'{window} capability exposes unnecessary native permissions')

print(f'Release invariants v{version}: OK')
