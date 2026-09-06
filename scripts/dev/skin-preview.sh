#!/usr/bin/env bash
# Render one skin in the ryotest rig and write an 800x500 preview.png.
#   scripts/dev/skin-preview.sh <id>            an installed skin id
#   scripts/dev/skin-preview.sh <dir>           a skin folder (its parent becomes RYOTUNES_SKIN_DIRS)
#   scripts/dev/skin-preview.sh <id-or-dir> out.png   write somewhere else
# Default output is skins/<id>/preview.png. Brings the rig up, launches one client pinned to the
# skin, navigates home, screenshots the 1920x1080 compositor, crops to the floating window (its box
# against weston's flat background) and fits it, padded with the window's own paper, to exactly
# 800x500. Reads the rig through scripts/dev/ryotest.sh; that passes RYOTUNES_* through.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/../.." && pwd)"
ryotest="$here/ryotest.sh"

arg=${1:?"usage: skin-preview.sh <id-or-dir> [out.png]"}
skinjson=""
if [ -d "$arg" ] && [ -f "$arg/skin.json" ]; then
  dir="$(cd "$arg" && pwd)"
  id="$(basename "$dir")"
  export RYOTUNES_SKIN_DIRS="$(dirname "$dir")"
  skinjson="$dir/skin.json"
elif [ -d "$root/skins/$arg" ] && [ -f "$root/skins/$arg/skin.json" ]; then
  id="$arg"
  export RYOTUNES_SKIN_DIRS="$root/skins"
  skinjson="$root/skins/$id/skin.json"
else
  id="$arg"
  [ -f "$root/skins/$id/skin.json" ] && skinjson="$root/skins/$id/skin.json"
fi
out=${2:-"$root/skins/$id/preview.png"}
mkdir -p "$(dirname "$out")"

name="skinprev-$id"
raw="$(mktemp --suffix=.png)"
mypid=""
cleanup() { [ -n "$mypid" ] && kill "$mypid" 2>/dev/null || true; rm -f "$raw"; }
trap cleanup EXIT

export RYOTUNES_SKIN="$id" RYOTUNES_SKIN_MODE=system   # the skin renders in its own default mode
"$ryotest" up >/dev/null
# Start one client and identify exactly the qs process we spawned, so we leave any sibling rig
# clients (another agent's) untouched and never shoot two windows into one frame.
before="$(pgrep -f 'qs -p .*/client' | sort || true)"
"$ryotest" run "$name" >/dev/null
sleep 1
after="$(pgrep -f 'qs -p .*/client' | sort || true)"
mypid="$(comm -13 <(printf '%s\n' "$before") <(printf '%s\n' "$after") | head -1)"
sleep 4
"$ryotest" ctl "$name" 'nav home' >/dev/null 2>&1 || true
sleep 3
"$ryotest" shot "$name" "$raw" >/dev/null

python3 - "$raw" "$out" "$skinjson" <<'PY'
import json
import os
import sys
import numpy as np
from PIL import Image

raw, out = sys.argv[1], sys.argv[2]
skinjson = sys.argv[3] if len(sys.argv) > 3 else ""
im = Image.open(raw).convert("RGB")
a = np.asarray(im).astype(int)
H, W = a.shape[:2]

# The client floats, centred, over weston's flat grey desktop and a thin full-width top panel.
# Sample the desktop from the left margin (mid-height, clear of the window and the panel), mask
# everything unlike it, then read the window as the wide central column band and, within it, the
# tallest contiguous row run - so the thin panel and corner widgets never enlarge the crop.
bg = a[H // 2, 10]
mask = np.abs(a - bg).max(axis=2) > 40
xs = np.where(mask.sum(axis=0) > H * 0.4)[0]
bbox = None
if len(xs):
    x0, x1 = int(xs.min()), int(xs.max()) + 1
    rows = mask[:, x0:x1].sum(axis=1) > (x1 - x0) * 0.5
    best = (0, 0)
    start = None
    for y in range(H + 1):
        on = y < H and rows[y]
        if on and start is None:
            start = y
        elif not on and start is not None:
            if y - start > best[1] - best[0]:
                best = (start, y)
            start = None
    y0, y1 = best
    if y1 > y0:
        # inset 1px to drop the window's drop-shadow/border seam
        bbox = (x0 + 1, y0 + 1, x1 - 1, y1 - 1)
win = im.crop(bbox) if bbox else im

# Pad colour: the skin's paper if known, else the window's own corner, so the frame is one surface.
pad = win.getpixel((1, 1))
if skinjson and os.path.exists(skinjson):
    m = json.load(open(skinjson))
    modes = m.get("modes", {})
    md = modes.get(m.get("default", "dark")) or (next(iter(modes.values()), {}) if modes else {})
    h = (md.get("paper") or "").lstrip("#")
    if len(h) == 6:
        pad = (int(h[0:2], 16), int(h[2:4], 16), int(h[4:6], 16))

# Fit inside 800x500 preserving aspect, centred on the pad.
canvas = Image.new("RGB", (800, 500), pad)
w, h = win.size
s = min(800 / w, 500 / h)
nw, nh = max(1, round(w * s)), max(1, round(h * s))
canvas.paste(win.resize((nw, nh), Image.LANCZOS), ((800 - nw) // 2, (500 - nh) // 2))
canvas.save(out)
print(f"{out}  (window {w}x{h} -> 800x500)")
PY
echo "$out"
