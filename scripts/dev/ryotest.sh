#!/bin/bash
# Isolated visual test rig: a nested headless weston (own Wayland socket, invisible, cannot touch
# the desktop shell) hosting one Ryotunes client per NAME.
#   scripts/dev/ryotest.sh up                      start the compositor (socket "ryotest")
#   scripts/dev/ryotest.sh run NAME [client-dir]   launch a client; prints its pid
#   scripts/dev/ryotest.sh ctl NAME 'cmd args'     send a control command (see shell.qml devCtl)
#   scripts/dev/ryotest.sh shot NAME out.png       capture the compositor (1920x1080, upright)
#   scripts/dev/ryotest.sh down                    stop everything
# NEVER add outputs to the live Hyprland session for this (it crashes the shell).
set -u
export XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-/run/user/$(id -u)}
SOCK=ryotest
case $1 in
  up)
    [ -S "$XDG_RUNTIME_DIR/$SOCK" ] && { echo "already up"; exit 0; }
    setsid weston --backend=headless --renderer=gl --width=1920 --height=1080 --socket=$SOCK --idle-time=0 --no-config --debug > /tmp/weston-$SOCK.log 2>&1 &
    for _ in $(seq 1 40); do [ -S "$XDG_RUNTIME_DIR/$SOCK" ] && break; sleep 0.25; done
    echo "weston on $SOCK";;
  run)
    NAME=$2; DIR=${3:-$(cd "$(dirname "$0")/../.." && pwd)/client}
    mkdir -p "$XDG_RUNTIME_DIR/ryotunes"
    export WAYLAND_DISPLAY=$SOCK RYOTUNES_WINDOW_TITLE="RyoTest-$NAME" RYOTUNES_CTL="$XDG_RUNTIME_DIR/ryotunes/ctl-$NAME.sock"
    rm -f "$RYOTUNES_CTL"
    setsid qs -p "$DIR" > "/tmp/ryotest-$NAME.log" 2>&1 &
    echo $!;;
  ctl)
    printf "%s\n" "$3" | socat -t 1 - "UNIX-CONNECT:$XDG_RUNTIME_DIR/ryotunes/ctl-$2.sock";;
  shot)
    OUT=$3; T=$(mktemp -d)
    (cd "$T" && WAYLAND_DISPLAY=$SOCK weston-screenshooter >/dev/null 2>&1)
    F=$(ls "$T"/*.png | head -1)
    python3 -c "from PIL import Image,ImageOps; ImageOps.flip(Image.open('$F').convert('RGB')).save('$OUT')"
    rm -rf "$T"; echo "$OUT";;
  down)
    pkill -f "qs -p .*client" ; pkill -f "weston --backend=headless.*--socket=$SOCK"; echo down;;
esac
