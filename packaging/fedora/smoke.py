#!/usr/bin/env python3
"""Installed-package exercise. Run as a disposable session user with DISPLAY and D-Bus.

The caller supplies a PulseAudio/PipeWire null sink and optional systemd user manager.
No network provider is required for this deterministic desktop/playback test.
"""
import json
import math
import os
from pathlib import Path
import socket
import struct
import subprocess
import time
import wave


def run(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT).strip()


def eventually(fn, timeout=25):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        try:
            value = fn()
            if value:
                return value
        except (OSError, AssertionError, subprocess.CalledProcessError) as exc:
            last = exc
        time.sleep(0.25)
    raise AssertionError(f"timed out: {last}")


def rpc(method, params=None):
    with socket.socket(socket.AF_UNIX) as sock:
        sock.settimeout(20)
        sock.connect(str(Path(os.environ['XDG_RUNTIME_DIR']) / 'ryotunes/ryotunesd.sock'))
        sock.sendall((json.dumps(dict(id=1, method=method, params=params or {})) + '\n').encode())
        reply = json.loads(sock.makefile().readline())
        if reply.get('error'):
            raise AssertionError(reply['error'])
        return reply.get('result')


def window(title):
    return run('xdotool', 'search', '--onlyvisible', '--name', '^' + title + '$').splitlines()[0]


def ctl(command):
    return run('qs', '-p', '/usr/share/ryotunes/client', 'ipc', 'call', '--', 'window', 'ctl', command)


run('ryotunes')
assert eventually(lambda: rpc('hello'))['daemon']
main = eventually(lambda: window('Ryotunes'))
print('PASS: installed launcher, daemon RPC and visible QML window', flush=True)

try:
    rpc('install_update', dict(version='1.0.9'))
    raise AssertionError('Fedora allowed the install RPC')
except AssertionError as exc:
    assert 'DNF' in str(exc), exc
print('PASS: Fedora rejects Arch update installation', flush=True)

music = Path.home() / 'Music' / 'rpm-smoke'
music.mkdir(parents=True, exist_ok=True)
with wave.open(str(music / 'tone.wav'), 'w') as audio:
    audio.setparams((1, 2, 44100, 0, 'NONE', 'not compressed'))
    audio.writeframes(b''.join(struct.pack('<h', int(2000 * math.sin(2 * math.pi * 440 * i / 44100))) for i in range(44100 * 45)))
library = rpc('add_local_folder', dict(path=str(music)))
song = next(s for s in library['songs'] if 'tone.wav' in s['video_id'])
rpc('play', dict(item=song))
eventually(lambda: rpc('get_playback')['duration'] > 0)
rpc('seek', dict(position=0.0))
print('PLAYBACK', json.dumps(eventually(lambda: rpc('get_playback'))), flush=True)
time.sleep(3)
state = rpc('get_playback')
assert state['position'] > 0, state
print('PASS: libmpv decodes local WAV and advances playback through the null sink', flush=True)

bus = ('gdbus', 'call', '--session', '--dest', 'org.mpris.MediaPlayer2.ryotunes', '--object-path', '/org/mpris/MediaPlayer2')
run(*bus, '--method', 'org.mpris.MediaPlayer2.Player.Pause')
eventually(lambda: rpc('get_playback')['paused'])
run(*bus, '--method', 'org.mpris.MediaPlayer2.Player.Play')
eventually(lambda: not rpc('get_playback')['paused'])
print('PASS: MPRIS pause/play controls daemon playback', flush=True)

run('xdotool', 'windowactivate', '--sync', main)
run('xdotool', 'key', '--clearmodifiers', 'alt+F4')
time.sleep(1)
before = rpc('get_playback')['position']
time.sleep(1)
assert rpc('get_playback')['position'] > before
run('ryotunes')
eventually(lambda: window('Ryotunes'))
print('PASS: close keeps playback alive; launcher reopens the window', flush=True)

ctl('mini on')
eventually(lambda: window('Ryotunes Mini'))
ctl('show')
eventually(lambda: window('Ryotunes'))
for mode in ('light', 'dark', 'system'):
    ctl('theme ' + mode)
    time.sleep(0.5)
    prefs = json.loads((Path.home() / '.config/ryotunes/client.json').read_text())
    assert prefs['themeMode'] == mode, prefs
assert rpc('get_downloads') is not None
print('PASS: standalone mini-player, light/dark/system controls and downloads RPC', flush=True)

if os.environ.get('RYOTUNES_ONLINE_TEST') == '1':
    settings = rpc('get_download_settings')
    settings.update(path=str(music / 'downloads'), embedMetadata=False, format='opus')
    rpc('set_download_settings', dict(settings=settings))
    job = rpc('enqueue_download', dict(videoId='jNQXAC9IVRw', title='RPM download test', artists='Test'))
    print('DOWNLOAD', json.dumps(job), flush=True)
    def downloaded():
        snapshot = rpc('get_downloads')
        jobs = snapshot['jobs']
        for item in jobs:
            if item['id'] == job['id']:
                if item['status'] == 'failed':
                    raise RuntimeError(item.get('error'))
                if item['status'] == 'completed':
                    return item
        return None
    completed = eventually(downloaded, timeout=180)
    assert Path(completed['filePath']).is_file(), completed
    codec = run('ffprobe', '-v', 'error', '-select_streams', 'a:0', '-show_entries', 'stream=codec_name', '-of', 'default=nw=1:nk=1', completed['filePath'])
    assert codec == 'opus', codec
    print('PASS: online yt-dlp download and FFmpeg Opus conversion', json.dumps(completed), flush=True)

snapshot = os.environ.get('RYOTUNES_SCREENSHOT')
if snapshot:
    run('magick', 'import', '-window', 'root', snapshot)
rpc('quit')
print('PASS: explicit quit RPC', flush=True)
