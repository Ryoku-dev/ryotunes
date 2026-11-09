# Install on Void Linux

Ryotunes publishes an x86_64 Arch package. The Void installer verifies that release, repacks its `usr/` tree as an xbps package and installs it through xbps.

## Install or update

Run the installer from a clone or release source archive:

```sh
./release/void/install.sh
```

The script:

1. reads the latest Ryotunes release tag;
2. downloads the package and its SHA-256 sidecar;
3. verifies the checksum;
4. removes the systemd files;
5. checks that the client runs with the installed glibc;
6. creates `ryotunes-X.Y.Z_1.x86_64.xbps` with `release/void/depends`;
7. installs or upgrades it with `xbps-install`.

Quit and reopen Ryotunes after the update:

```sh
ryotunes-cli quit
ryotunes
```

## Runtime dependencies

Void package names differ in a few places:

| Package | Use |
|---|---|
| `mpv` | Audio playback through libmpv |
| `yt-dlp` | Download and provider fallback work |
| `ffmpeg6` | Download conversion and metadata |
| `libvorbis` | Spotify Ogg Vorbis decoding support |

`release/void/depends` is the complete list, including `libogg`, display libraries, icon support, `xdg-utils` and polkit.

## Daemon startup

Void does not use the packaged systemd user units. The installer removes them while repacking. `ryotunes` starts `ryotunesd` directly when it cannot connect to the per-user socket. The same close, idle and Quit lifecycle applies.

Check the daemon process and the sanitized diagnostics report with:

```sh
pgrep -a ryotunesd
ryotunes-diagnostics
```

## Build from source

Install Go, the compiler toolchain and the development packages corresponding to `release/void/depends`. Use the same build settings as the Arch release recipe:

```sh
version=$(cat VERSION)
ldflags="-s -w -X github.com/ryoku-dev/ryotunes/internal/version.Version=$version"
mkdir -p build
CGO_ENABLED=1 go build -trimpath -buildmode=pie -ldflags "$ldflags" -o build/ryotunes ./cmd/ryotunes
CGO_ENABLED=1 go build -trimpath -buildmode=pie -ldflags "$ldflags" -o build/ryotunesd ./cmd/ryotunesd
CGO_ENABLED=0 go build -trimpath -buildmode=pie -ldflags "$ldflags" -o build/ryotunes-cli ./cmd/ryotunes-cli
CGO_ENABLED=0 go build -trimpath -buildmode=pie -ldflags "$ldflags" -o build/ryotunes-sync ./cmd/ryotunes-sync
go test ./...
```

The release package is checked against a glibc 2.41 ceiling because that is the supported Void baseline.

## Sandboxed development

Keep a development daemon away from the installed socket and data:

```sh
root=$(mktemp -d)
mkdir -p "$root/run" "$root/data" "$root/cache" "$root/config" "$root/state"
env \
  XDG_RUNTIME_DIR="$root/run" \
  XDG_DATA_HOME="$root/data" \
  XDG_CACHE_HOME="$root/cache" \
  XDG_CONFIG_HOME="$root/config" \
  XDG_STATE_HOME="$root/state" \
  go run ./cmd/ryotunesd
```

Use the same environment in another terminal for the client or CLI.
