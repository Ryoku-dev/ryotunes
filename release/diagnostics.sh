#!/usr/bin/env bash
set -u

line() { printf '%-24s %s\n' "$1" "$2"; }
first_line() {
  local output
  output=$("$@" 2>/dev/null | head -n1)
  printf '%s' "${output:-unknown}"
}

printf '%s\n' 'Ryotunes diagnostics (sanitized)'
printf '%s\n' '--------------------------------'
if command -v xbps-query >/dev/null 2>&1; then
  package=$(xbps-query -p pkgver ryotunes 2>/dev/null || printf 'source/unpackaged')
elif command -v pacman >/dev/null 2>&1; then
  package=$(pacman -Q ryotunes 2>/dev/null | awk '{print $2}' || printf 'source/unpackaged')
else
  package='source/unpackaged'
fi
line 'Ryotunes package:' "$package"
line 'Kernel:' "$(uname -sr 2>/dev/null || printf 'unknown')"
line 'Architecture:' "$(uname -m 2>/dev/null || printf 'unknown')"
line 'Session:' "${XDG_SESSION_TYPE:-unknown}"
line 'Desktop:' "${XDG_CURRENT_DESKTOP:-unknown}"
line 'Wayland display:' "${WAYLAND_DISPLAY:+present}"
line 'mpv:' "$(first_line mpv --version)"
line 'yt-dlp:' "$(first_line yt-dlp --version)"
line 'FFmpeg:' "$(first_line ffmpeg -version)"
line 'Daemon process:' "$(pgrep -x ryotunesd >/dev/null 2>&1 && printf running || printf stopped)"
line 'Client process:' "$(pgrep -x ryotunes >/dev/null 2>&1 && printf running || printf stopped)"
line 'User socket:' "$(systemctl --user is-active ryotunesd.socket 2>/dev/null || printf unavailable)"
line 'GPU classes:' "$(lspci 2>/dev/null | grep -Ei 'VGA|3D controller' | sed -E 's/^[0-9a-f:.]+ //' | paste -sd ';' - || printf 'unknown')"

printf '\nRuntime packages:\n'
if command -v xbps-query >/dev/null 2>&1; then
  packages=(mpv yt-dlp ffmpeg6 wayland libxkbcommon libX11 libXcursor libXfixes mesa-dri vulkan-loader)
  present() { xbps-query "$1" >/dev/null 2>&1; }
else
  packages=(mpv yt-dlp ffmpeg wayland libxkbcommon libx11 libxcursor libxfixes mesa vulkan-icd-loader)
  present() { pacman -Q "$1" >/dev/null 2>&1; }
fi
for package in "${packages[@]}"; do
  if present "$package"; then
    printf '  [ok] %s\n' "$package"
  else
    printf '  [!!] %s missing\n' "$package"
  fi
done

printf '\nThis report excludes logs, account data, tokens, local paths, hostnames, and network endpoints.\n'
