# Install on Arch Linux, CachyOS and Ryoku

Ryotunes releases provide a verified x86_64 pacman package. Building from source is for development and package work.

## Ryoku

Ryotunes is included with Ryoku. Restore it after removal with:

```sh
ryoku-host pkg add ryotunes
```

## Release package

Download these files from the [latest release](https://github.com/ryoku-dev/ryotunes/releases/latest):

```text
ryotunes-X.Y.Z-1-x86_64.pkg.tar.zst
ryotunes-X.Y.Z-1-x86_64.pkg.tar.zst.sha256
```

Verify and install the package:

```sh
sha256sum -c ryotunes-X.Y.Z-1-x86_64.pkg.tar.zst.sha256
sudo pacman -U ./ryotunes-X.Y.Z-1-x86_64.pkg.tar.zst
```

The package carries permanent pacman epoch `1`, so `pacman -Q ryotunes` reports `1:X.Y.Z-1`. The asset filename remains epochless.

Quit Ryotunes and open it again after an update so the client and daemon use the same release:

```sh
ryotunes-cli quit
ryotunes
```

## Runtime dependencies

The package declares the complete dependency set in `release/arch/PKGBUILD`. Playback and downloads depend on:

| Package | Use |
|---|---|
| `mpv` | Audio playback through libmpv |
| `yt-dlp` | Download and provider fallback work |
| `ffmpeg` | Download conversion and metadata |
| `libvorbis` | Spotify Ogg Vorbis decoding support |

`libogg`, the Gio display libraries, icon support, `xdg-utils` and polkit are also package dependencies. Go is needed only to build from source.

## User units

The package installs:

```text
/usr/lib/systemd/user/ryotunesd.socket
/usr/lib/systemd/user/ryotunesd.service
```

The socket unit listens at `%t/ryotunes/ryotunesd.sock` with mode `0600` in a `0700` directory. The package enables socket activation for users. Inspect or restart it with:

```sh
systemctl --user status ryotunesd.socket ryotunesd.service
systemctl --user restart ryotunesd.socket
```

An update does not restart the daemon during the pacman transaction. Quit and reopen Ryotunes after the package is installed.

## Build from source

Install the `base-devel` and `go` build tools, plus the runtime and display dependencies listed in `release/arch/PKGBUILD`. From the repository root, use the same build settings as the package:

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

To build the package instead:

```sh
./release/build-arch-package.sh dist
```

## Sandboxed development

Use separate XDG directories so a development daemon cannot open the installed socket or modify the user's library:

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

Use the same environment in another terminal for `go run ./cmd/ryotunes` or `go run ./cmd/ryotunes-cli hello`.
