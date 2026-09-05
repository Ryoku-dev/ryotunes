#!/bin/bash
# Launch an isolated test client on a headless Hyprland output, so visual checks never touch the
# real screen. Usage: scripts/dev/ryotest.sh NAME OUTPUT WORKSPACE [client-dir]
#   NAME       tag for the window title (RyoTest-NAME), log (/tmp/ryotest-NAME.log) and the
#              control socket ($XDG_RUNTIME_DIR/ryotunes/ctl-NAME.sock)
#   OUTPUT     a headless output created with `hyprctl output create headless OUTPUT`
#   WORKSPACE  the workspace parked on that output (see docs/superpowers/plans, "rig")
# Prints the qs pid. Drive it with one command per line on the socket:
#   printf 'nav search {"q":"joji"}\n' | socat - UNIX-CONNECT:$XDG_RUNTIME_DIR/ryotunes/ctl-NAME.sock
#   (nav <page> [json] | show | mini on|off | np queue|lyrics|off | panel on|off | decor rich|calm | theme system|light|dark)
# Capture with: grim -o OUTPUT shot.png
set -u
NAME=$1; OUT=$2; WS=$3; DIR=${4:-$(cd "$(dirname "$0")/../.." && pwd)/client}
export WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-wayland-1} XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-/run/user/$(id -u)}
export RYOTUNES_WINDOW_TITLE="RyoTest-$NAME" RYOTUNES_SCREEN="$OUT" RYOTUNES_CTL="$XDG_RUNTIME_DIR/ryotunes/ctl-$NAME.sock"
mkdir -p "$XDG_RUNTIME_DIR/ryotunes"; rm -f "$RYOTUNES_CTL"
setsid qs -p "$DIR" > "/tmp/ryotest-$NAME.log" 2>&1 &
PID=$!
ADDR=""
for _ in $(seq 1 40); do
  ADDR=$(hyprctl -j clients | jq -r ".[] | select(.title==\"RyoTest-$NAME\") | .address")
  [ -n "$ADDR" ] && break; sleep 0.25
done
[ -n "$ADDR" ] && hyprctl dispatch "hl.dsp.window.move({ workspace = $WS, window = \"address:$ADDR\", silent = true })" >/dev/null 2>&1
echo "$PID"
