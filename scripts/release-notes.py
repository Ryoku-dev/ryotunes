#!/usr/bin/env python3
"""Print release notes from the release's exact tag, GitHub and its commit range."""
import os
import re
import subprocess
import sys


def git(*args):
    return subprocess.check_output(['git', *args], text=True).strip()


def release_notes(tag):
    if not re.fullmatch(r'v1\.(0|[1-9][0-9]*)\.[0-9]', tag):
        raise ValueError('Expected a canonical v1 release tag')
    current = tuple(map(int, tag[1:].split('.')))
    tags = git('tag', '--list').splitlines()
    earlier = [t for t in tags if re.fullmatch(r'v1\.(0|[1-9][0-9]*)\.[0-9]', t)
               and tuple(map(int, t[1:].split('.'))) < current]
    # The first v1 release explains the transition from the legacy line, not the
    # entire upstream history. Subsequent releases compare only against v1.
    legacy = [t for t in tags if re.fullmatch(r'v2\.[0-9]+\.[0-9]+', t)]
    previous = max(earlier or legacy, key=lambda t: tuple(map(int, t[1:].split('.'))), default=None)
    repo = os.environ.get('GITHUB_REPOSITORY', 'neur0map/ryotunes')
    command = ['gh', 'api', f'repos/{repo}/releases/generate-notes',
               '-f', f'tag_name={tag}', '--jq', '.body']
    if previous:
        command += ['-f', f'previous_tag_name={previous}']
    generated = subprocess.check_output(command, text=True).strip()
    changelog = git('show', f'{tag}:CHANGELOG.md')
    section = re.search(rf'^## {re.escape(tag)}(?:\s[^\n]*)?\n(.*?)(?=^## |\Z)', changelog, re.S | re.M)
    authored = section.group(1).strip() if section else ''
    changes = git('log', '--no-merges', '--format=%s (%h)', f'{previous}..{tag}' if previous else tag)
    commits = [f'- {line}' for line in changes.splitlines() if not line.startswith('release: v')]
    package = f'ryotunes-{tag[1:]}-1-x86_64.pkg.tar.zst'
    sections = [f'# Ryotunes {tag}', authored, generated]
    if commits:
        sections.append('## Changes in this push\n\n' + '\n'.join(commits))
    sections.append(f'''## Install or update (Arch / CachyOS / Ryoku)

Download `{package}` and `{package}.sha256`, then:

```bash
sha256sum -c {package}.sha256
sudo pacman -U ./{package}
```

The package includes Ryotunes, ryotunesd, the CLI and native QML client. Its permanent
pacman epoch makes the new v1 series an upgrade from legacy v2 installations.
`ryoku update` can consume the same package. Independently, use **About → Check for
new version → Update** (administrator approval required).

After installing, **Quit Ryotunes and reopen it**, rather than only closing the
window, to reload the client and background daemon. Source archives and SHA-256
sidecars are attached alongside the binary package.''')
    return '\n\n'.join(s for s in sections if s) + '\n'


if __name__ == '__main__':
    print(release_notes(sys.argv[1]), end='')
