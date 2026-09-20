# Fedora 44 (x86_64)

This port packages the primary Quickshell/QML client, `ryotunesd`, the native
launcher and `ryotunes-cli`. The legacy Tauri frontend and optional sync relay
are not included. Fedora 44 is the validated build target; other releases and
architectures need their own build and desktop validation.

## Dependencies and external sources

Fedora's official `fedora` and `updates` repositories supply Quickshell 0.2.1,
Qt 6, libmpv (`mpv-libs`), WebKitGTK 4.1, SQLite, yt-dlp, FFmpeg-free, Zenity,
fonts and XDG helpers. A Fedora Node.js runtime version 22 or newer supplies
JavaScript for yt-dlp. The RPM requires the versioned
`alternative-for(nodejs-bin)` capability, so an installed compatible stream
(including `nodejs22-bin`) satisfies it without replacement by Node 24.
Because Fedora does not package yt-dlp-ejs, the Fedora daemon enables
`--js-runtimes node --remote-components ejs:github`: yt-dlp downloads its
version-matched challenge solver from the upstream yt-dlp/ejs GitHub releases
when YouTube needs it. This requires network access and uses yt-dlp’s cache;
it is not a package self-update. See the [upstream EJS guide](https://github.com/yt-dlp/yt-dlp/wiki/EJS). WebKitGTK is still required by the daemon for provider
login and JavaScript, even though the interface is QML. Cava is recommended
for the optional spectrum; no Cava means no live spectrum. Qt supplies font
substitution when Ryoku's preferred fonts are absent.

The shared `Ryoku.Ui.Singletons` module comes from **`ryoku-ui`**, owned by
ryoku-arch. Existing Fedora work put it inside `ryoku-desktop`. The companion
patches in [packaging/fedora/companion](../packaging/fedora/companion/README.md)
split that payload and make the newer WindowManager API optional. They are
prepared for upstream coordination; they have not been submitted or merged.
Until that happens, build the pinned companion RPM below. No public Ryoku RPM
repository or signing key is assumed. RyoTunes contains no copied shared QML.

FFmpeg-free is enough for the tested formats. Some codecs may require RPM
Fusion's free repository and its FFmpeg/libavcodec packages; those are optional,
not enabled by these scripts. Follow [RPM Fusion's configuration guide](https://rpmfusion.org/Configuration)
if you need them. Online providers and yt-dlp can change independently of the
application; their successful extraction is a separate runtime check.

## Install, update and remove

Install the two locally built RPMs in one DNF transaction:

```sh
sudo dnf install ./fedora-out/companion/RPMS/x86_64/ryoku-ui-*.rpm \
  ./fedora-out/RPMS/x86_64/ryotunes-*.rpm
ryotunes
```

These local build artifacts are unsigned. For distribution, sign RPMs and
repository metadata and configure verified keys; the scripts do not publish.
Do not use `rpm --nodeps`. With a Ryoku desktop package, the companion split
requires a matching rebuilt `ryoku-desktop` in the same transaction so files
have one owner. The companion's 0.1 version is for standalone validation,
not an upgrade over an existing Ryoku desktop release.

The launcher starts `ryotunesd.socket` on demand. Optionally enable it for later
sessions with `systemctl --user enable --now ryotunesd.socket`. Fedora systemd
RPM macros handle user-manager reload/preset/removal. Upgrades do not restart
playback. No Arch preset or pacman hook is installed. After an upgrade, **Quit**
and reopen to load the new daemon and QML:

```sh
sudo dnf upgrade ryotunes                     # when a provider repository exists
sudo dnf install ./ryotunes-NEW-1.fc44.x86_64.rpm  # local RPM update
systemctl --user stop ryotunesd.service
ryotunes
sudo dnf remove ryotunes
```

Removal stops/disables the service and socket for running user managers; music,
configuration and library data remain in your home directory. Settings → About
can discover upstream versions and changelogs, but Fedora builds refuse the
install RPC before any network download or pacman invocation, even if pacman
is installed. An upstream tag does not imply an RPM is available yet.

## Release workflow and Copr

`.github/workflows/release.yml` builds Fedora 44 binary RPMs and vendored SRPMs
on pushes to the default branch or through **Run workflow**. It retains both
application and companion packages as the `fedora-packages` Actions artifact,
then submits the companion and application SRPMs to Copr and waits for each
build to succeed. The separate Fedora RPM workflow continues to run desktop
validation on pushes and pull requests.

Configure these repository settings before running a release:

- Secret `COPR_CONFIG`: the complete configuration from the
  [Copr API page](https://copr.fedorainfracloud.org/api/).
- Variable `COPR_PROJECT`: the existing destination in `owner/project` form.
  Enable its `fedora-44-x86_64` chroot and grant the API account build access.

The workflow uses the checked-in package version and release. Update them
before publishing a new version (`scripts/sync-version.sh VERSION --release`
keeps the version manifests aligned). It does not create commits, tags, Arch
packages or GitHub releases. To retry a failed submission, rerun the failed
Copr job to reuse the existing package artifact.

After a successful Copr build, install with `sudo dnf copr enable OWNER/PROJECT`
and `sudo dnf install ryotunes`. The companion `ryoku-ui` is built into the same
repository.

## Clean container build

On a host with rootless Podman and Git:

```sh
podman run -d --name ryotunes-build --network=host \
  -v "$PWD:/work:Z" registry.fedoraproject.org/fedora:44 sleep infinity
podman exec ryotunes-build dnf -y install rpm-build rust cargo rustfmt \
  gcc gcc-c++ pkgconf-pkg-config openssl-devel sqlite-devel mpv-devel \
  webkit2gtk4.1-devel systemd-rpm-macros desktop-file-utils python3 git patch
podman exec -w /work ryotunes-build packaging/fedora/build-companion.sh
podman exec -w /work ryotunes-build packaging/fedora/build.sh
```

Outputs are binary RPMs and SRPMs in `fedora-out/{RPMS,SRPMS}` and
`fedora-out/companion/{RPMS,SRPMS}`. `prepare-source.sh` includes current
unignored source changes, vendors the complete locked Cargo graph (including
its pinned librespot Git revision), and normalizes archive ordering, owners and
timestamps. Only source preparation accesses the network. `%build` and `%check`
use `--frozen`/offline Cargo and Fedora libraries; SQLite explicitly uses the
system library rather than its bundled C source. No Node/pnpm frontend build or rustup is required. `CARGO_BUILD_JOBS` defaults to 4; reduce it on memory-limited builders.

Rebuild the SRPM in an offline Fedora buildroot with the declared BuildRequires
installed, or with `mock -r fedora-44-x86_64 --rebuild ...src.rpm`. Reproducibility
requires the same source archive and Fedora toolchain/package versions: the
mutable Fedora image tag is not a frozen dependency snapshot. Record the image
digest and `rpm -qa` for each build. `SOURCE_DATE_EPOCH` defaults to the source
commit timestamp; package timestamps and build host are fixed. The build runs
native tests and skin/desktop validation. The existing environment-sensitive
libmpv pitch test is run separately with a timeout during validation.

## GNOME, KDE and Ryoku

The normal application is a floating Qt window. GNOME and X11 get a floating
mini-player; layer-shell desktops retain the existing overlay. Set
`RYOTUNES_PORTABLE_MINI=1` for another Wayland compositor without layer-shell.
Copy-link actions use Qt's clipboard on both display protocols. With Qt's
software renderer, artwork and the mini-player omit shader clipping to stay visible. Native file
pickers use Zenity. Without Ryoku theme files the shared runtime supplies its
built-in palette; shipped Paper, Ember and Mist skins and explicit light/dark
modes remain available. Follow System refers to Ryoku's theme, not automatic
GNOME/Plasma palette synchronization. GNOME requires an extension to display
StatusNotifier tray icons; MPRIS and reopening from the application launcher
do not depend on a tray icon.

A graphical session must be available to the user manager: the daemon uses GTK
for provider login. Standard GNOME/KDE sessions import this environment. For a
custom session, import DISPLAY/WAYLAND_DISPLAY and XDG_CURRENT_DESKTOP into
`systemctl --user import-environment` before starting the daemon. Never start
an audio daemon as root in your real desktop session.

See [FEDORA-VALIDATION.md](FEDORA-VALIDATION.md) for recorded evidence and limits.

## Reproduce desktop validation

```sh
podman build --target desktop-test -t ryotunes-test -f packaging/fedora/Containerfile .
podman run -d --name ryotunes-test --systemd=always -v "$PWD:/work:z" ryotunes-test
podman exec -w /work ryotunes-test packaging/fedora/validate-container.sh
```

Run `packaging/fedora/build-upgrade-fixture.sh` in the builder first to include
an actual DNF release-0 → release-1 transaction. Set `CARGO_TARGET_DIR` to the
same persistent directory for both builds to avoid recompiling. Optional live
provider check: pass `-e RYOTUNES_ONLINE_TEST=1` to the validation `podman exec`.
For the additional Wayland check, install `sway grim` in the disposable test
container, reinstall the RPM after the removal test, then run
`packaging/fedora/wayland-smoke.sh`. Rootless containers may require removing
Sway's `cap_sys_nice` file capability **inside that container** with
`setcap -r /usr/bin/sway`; the headless renderer does not need realtime priority.
Stop these test containers when done. They do not access the host display or
sound server.
