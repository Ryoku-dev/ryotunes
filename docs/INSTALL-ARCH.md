# Install on Arch / CachyOS / Ryoku

Ryotunes is primarily distributed as a precompiled x86_64 pacman package. Normal users should install that package rather than compiling the Tauri application locally.

## Precompiled package

The push-triggered release pipeline publishes a prebuilt x86_64 package and its checksum,
`ryotunes-X.Y.Z-1-x86_64.pkg.tar.zst` and the same filename with `.sha256` appended.
Verify the checksum, then install with pacman (which preserves ownership and pacman's
signature policy):

```bash
package=ryotunes-X.Y.Z-1-x86_64.pkg.tar.zst # Use the filename you downloaded.
sha256sum -c "$package.sha256"
sudo pacman -U "./$package"
```

The file name is epochless, but the package carries `epoch=1`, so this upgrades
cleanly over a machine still on the legacy 2.x line (`pacman -Q ryotunes` then
reports `1:X.Y.Z-1`). After updating, quit and reopen Ryotunes — closing the
window is not enough — so the daemon and client load the new build.

On Ryoku this is automated: `ryoku update` installs a newer published Ryotunes
package, and `ryoku doctor --check` reports its availability without installing.

## Update inside Ryotunes

Open **Settings → About → Check for new version** in either client. About shows the
app version (and the connected daemon version in the native client), the next
available release, **View changelog**, and **Update** when installation is supported.
It credits [ashmitvoid](https://github.com/ashmitvoid) and
[neur0map](https://github.com/neur0map), and links the original
[LiMusic project](https://github.com/SimoHypers/limusic).

Self update does not run `ryoku update`: it downloads the same official x86_64
package, checks SHA-256 and its internal name/version, then asks polkit for
administrator approval to run `pacman -U`. Pacman's dependency and signature
policy still applies. It requires the official `ryotunes` pacman package and a
desktop polkit authentication agent; development builds are not overwritten.
There is no background update polling. Keep the app open during installation,
then **Quit and reopen** it to reload both the interface and background daemon.

The first fresh release is `v1.0.0`. Each default-branch push advances the patch
digit, with `v1.0.9 → v1.1.0` and `v1.9.9 → v1.10.0`; old v2 tags do not affect
the new sequence. Until the first v1 package is published, About reports that no
release is available rather than offering an old v2 source archive as an update.

## AUR binary package

The intended AUR package is `ryotunes-bin`. It downloads the precompiled release payload rather than compiling Rust/Tauri locally.

```bash
paru -S ryotunes-bin
```

This command will become usable after the AUR package is published.

## Native QML client (preview)

Alongside the Tauri application, the package ships a native [Quickshell](https://quickshell.org) client that talks to the same `ryotunesd` daemon. It installs to `/usr/share/ryotunes/client` and is launched by the `ryotunes-qml` wrapper (a `Ryotunes (QML)` desktop entry is also installed). Because Quickshell resolves `qs -c NAME` only from `$XDG_CONFIG_HOME/quickshell/NAME`, the wrapper runs the packaged tree by explicit path:

```bash
ryotunes-qml            # == qs -p /usr/share/ryotunes/client
```

It needs the optional `quickshell` dependency and the Ryoku QML runtime (`Ryoku.Ui.Singletons`).

The native client is the default. `/usr/bin/ryotunes` (the desktop's launcher, keybind and dock) asks `ryotunesd` to `show`: the daemon raises the connected client or opens `ryotunes-qml`, and connecting to the socket is what starts the daemon after a boot (systemd socket activation), so a cold start lands in the native client too. The Tauri app, which carries its own player, runs only on `ryotunes --tauri` (or `RYOTUNES_TAURI=1`), or when the daemon's socket does not exist at all. Only one player ever runs.

## Build from source

Developers can build the repository directly. On Arch-based systems you need the normal Rust/Tauri frontend toolchain plus the runtime dependencies listed in `packaging/arch/PKGBUILD`.

```bash
cd ui
pnpm install --frozen-lockfile
pnpm check
pnpm build

cd ..
cargo fmt --all -- --check
cargo test --workspace --locked
cargo tauri build --no-bundle
```

For a pacman package from the local checkout:

```bash
cd packaging/arch
makepkg -si
```

Before publishing a build, follow `docs/RELEASE-CHECKLIST.md` rather than treating a successful local compile as a release gate.
