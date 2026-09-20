#!/usr/bin/env bash
# Optional compositor check, inside the disposable test container after installing sway/grim.
set -euo pipefail
[[ -f /run/.containerenv && $(id -u) == 0 ]] || exit 1
uid=$(id -u ryotest)
out=/work/fedora-out/validation
printf 'output HEADLESS-1 mode 1440x1000\n' > /tmp/ryotunes-sway.conf
session=(runuser -u ryotest -- env XDG_RUNTIME_DIR=/run/user/$uid DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$uid/bus)
"${session[@]}" systemctl --user stop ryotunesd.service
pkill -u ryotest -x sway || true
"${session[@]}" env WLR_BACKENDS=headless WLR_RENDERER=pixman WLR_LIBINPUT_NO_DEVICES=1 sway -c /tmp/ryotunes-sway.conf > "$out/sway.log" 2>&1 &
sway_pid=$!
trap 'kill "$sway_pid" 2>/dev/null || true' EXIT
for attempt in {1..50}; do
    [[ -S /run/user/$uid/wayland-1 ]] && break
    sleep 0.2
done
[[ -S /run/user/$uid/wayland-1 ]]
wayland=("${session[@]}" env WAYLAND_DISPLAY=wayland-1 XDG_CURRENT_DESKTOP=sway QT_QPA_PLATFORM=wayland QT_QUICK_BACKEND=software)
"${wayland[@]}" systemctl --user import-environment WAYLAND_DISPLAY XDG_CURRENT_DESKTOP QT_QPA_PLATFORM QT_QUICK_BACKEND
"${wayland[@]}" ryotunes
sleep 3
"${wayland[@]}" qs -p /usr/share/ryotunes/client ipc call -- window ctl 'mini on'
sleep 2
"${wayland[@]}" grim "$out/wayland-mini.png"
[[ $(magick identify -format %k "$out/wayland-mini.png") -gt 16 ]]
"${wayland[@]}" qs -p /usr/share/ryotunes/client ipc call -- window show
sleep 1
"${wayland[@]}" grim "$out/wayland-main.png"
"${wayland[@]}" journalctl --user -u ryotunesd.service --since '-1 minute' --no-pager > "$out/wayland-daemon.log"
! grep -E 'Failed to load configuration|Type .* unavailable|ReferenceError|Cannot assign' "$out/wayland-daemon.log"
echo 'PASS: QML main and layer-shell mini-player load in headless Sway'
"${wayland[@]}" systemctl --user stop ryotunesd.service
"${session[@]}" systemctl --user unset-environment WAYLAND_DISPLAY QT_QPA_PLATFORM
