#!/usr/bin/env bash
# Intentionally restricted to a disposable, systemd-booted Podman container.
set -euo pipefail
[[ -f /run/.containerenv && $(id -u) == 0 ]] || { echo 'Run in a disposable root-owned Podman container.' >&2; exit 1; }
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/fedora-out/validation"
mkdir -p "$out"
# Exercise coexistence with an already selected Fedora Node stream in CI.
if [[ -n ${RYOTUNES_TEST_NODE_PACKAGE:-} ]]; then
    dnf -y install "$RYOTUNES_TEST_NODE_PACKAGE" > "$out/node-install.log" 2>&1
fi
cat /etc/os-release > "$out/os-release"
rpm -qa | sort > "$out/packages-before.txt"
candidate=("$root"/fedora-out/RPMS/x86_64/ryotunes-*.rpm)
fixture=("$root"/fedora-out/upgrade/x86_64/ryotunes-*.rpm)
if [[ -f ${fixture[0]} ]]; then candidate=("${fixture[@]}"); fi
dnf -y install "$root"/fedora-out/companion/RPMS/x86_64/ryoku-ui-*.rpm \
    "${candidate[@]}" > "$out/install.log" 2>&1
rpm -V ryotunes ryoku-ui
ldd /usr/bin/ryotunesd > "$out/daemon-libraries.txt"
! grep -q 'not found' "$out/daemon-libraries.txt"
grep -q libsqlite3 "$out/daemon-libraries.txt"
grep -q libmpv "$out/daemon-libraries.txt"
id ryotest >/dev/null 2>&1 || useradd -m ryotest
loginctl enable-linger ryotest
uid=$(id -u ryotest)
for attempt in {1..50}; do [[ -S /run/user/$uid/bus ]] && break; sleep 0.2; done
session=(runuser -u ryotest -- env XDG_RUNTIME_DIR=/run/user/$uid DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$uid/bus DISPLAY=:99 QT_QUICK_BACKEND=software XDG_CURRENT_DESKTOP=GNOME)
"${session[@]}" Xvfb :99 -screen 0 1440x1000x24 -ac > "$out/xvfb.log" 2>&1 &
sleep 1
"${session[@]}" openbox > "$out/openbox.log" 2>&1 &
"${session[@]}" pulseaudio --start --exit-idle-time=-1 --load='module-null-sink sink_name=ryotunes_test'
"${session[@]}" systemd-analyze --user verify /usr/lib/systemd/user/ryotunesd.{service,socket}
"${session[@]}" systemctl --user import-environment DISPLAY XDG_CURRENT_DESKTOP QT_QUICK_BACKEND
"${session[@]}" systemctl --user stop ryotunesd.service
"${session[@]}" systemctl --user start ryotunesd.socket
"${session[@]}" systemctl --user is-active ryotunesd.socket
# Activation must be cold until the first RPC/launcher connection.
! "${session[@]}" systemctl --user is-active --quiet ryotunesd.service
if [[ -f ${fixture[0]} ]]; then
    "${session[@]}" ryotunes
    before=$("${session[@]}" systemctl --user show ryotunesd.service -p MainPID --value)
    [[ $before != 0 ]]
    dnf -y upgrade "$root"/fedora-out/RPMS/x86_64/ryotunes-*.rpm > "$out/upgrade.log" 2>&1
    after=$("${session[@]}" systemctl --user show ryotunesd.service -p MainPID --value)
    [[ $before == "$after" ]]
    [[ $(rpm -q --qf '%{RELEASE}' ryotunes) == 1.fc44 ]]
    rpm -V ryotunes
    echo 'PASS: DNF release-0 to release-1 upgrade preserves the running daemon'
    "${session[@]}" systemctl --user stop ryotunesd.service
fi
chown ryotest:ryotest "$out"
if [[ -n ${RYOTUNES_TEST_NODE_PACKAGE:-} ]]; then
    rpm -q "$RYOTUNES_TEST_NODE_PACKAGE" > "$out/node-retained.txt"
    node --version >> "$out/node-retained.txt"
fi
"${session[@]}" env RYOTUNES_ONLINE_TEST="${RYOTUNES_ONLINE_TEST:-0}" RYOTUNES_SCREENSHOT="$out/desktop.png" python3 "$root/packaging/fedora/smoke.py" | tee "$out/smoke.log"
"${session[@]}" journalctl --user -u ryotunesd.service --no-pager > "$out/daemon.log"
"${session[@]}" env QT_QPA_PLATFORM=offscreen /usr/lib64/qt6/bin/qmltestrunner -input "$root/client/tests" > "$out/qml-tests.log" 2>&1
# Quit leaves the systemd-owned socket; the next launch activates a fresh daemon.
"${session[@]}" ryotunes
sleep 3
"${session[@]}" systemctl --user is-active ryotunesd.service
printf 'preserve-me\n' > /home/ryotest/Music/rpm-uninstall-sentinel
rpm -qa | sort > "$out/packages-after.txt"
dnf -y remove ryotunes > "$out/uninstall.log" 2>&1
! "${session[@]}" systemctl --user is-active --quiet ryotunesd.service
! "${session[@]}" systemctl --user is-active --quiet ryotunesd.socket
[[ -f /home/ryotest/Music/rpm-uninstall-sentinel ]]
[[ ! -e /usr/bin/ryotunes && ! -e /usr/share/ryotunes ]]
echo 'PASS: RPM removal stops user units and preserves user files' | tee -a "$out/smoke.log"
