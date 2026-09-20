# A vendored, offline build of the primary QML client and native daemon.
%global debug_package %{nil}
%global _lto_cflags %{nil}
%global source_date_epoch_from_changelog 0
%global clamp_mtime_to_source_date_epoch 1
Name:           ryotunes
Version:        1.1.6
Release:        %{?ryotunes_release}%{!?ryotunes_release:1}%{?dist}
Summary:        Native QML music player with libmpv playback
License:        GPL-3.0-or-later
URL:            https://github.com/ryoku-dev/ryotunes
Source0:        %{name}-%{version}.tar.gz
ExclusiveArch:  x86_64
BuildRequires:  cargo
BuildRequires:  rust
BuildRequires:  gcc
BuildRequires:  gcc-c++
BuildRequires:  pkgconf-pkg-config
BuildRequires:  openssl-devel
BuildRequires:  sqlite-devel
BuildRequires:  mpv-devel
BuildRequires:  webkit2gtk4.1-devel
BuildRequires:  systemd-rpm-macros
BuildRequires:  desktop-file-utils
BuildRequires:  python3
Requires:       quickshell >= 0.2.1
Requires:       ryoku-ui
Requires:       qt6-qtdeclarative
Requires:       qt6-qtimageformats
Requires:       zenity
Recommends:     cava
Requires:       qt6-qtsvg
Requires:       qt6-qtwayland
# Fedora's parallel Node streams expose this versioned capability for /usr/bin/node.
# yt-dlp EJS supports Node >= 22; do not replace an installed compatible stream.
Requires:       alternative-for(nodejs-bin) >= 1:22.0.0
Requires:       yt-dlp >= 2026.03.17
Requires:       /usr/bin/ffmpeg
Requires:       /usr/bin/ffprobe
Requires:       xdg-utils
Requires:       hicolor-icon-theme
Requires:       dejavu-sans-fonts
Requires:       dejavu-sans-mono-fonts
%{?systemd_ordering}

%description
The primary Quickshell/QML RyoTunes application, playback daemon and command-line
client. Uses Fedora's libmpv and WebKitGTK (provider login), with DNF-managed
updates. The shared Ryoku QML runtime is supplied by the separate ryoku-ui RPM.
The legacy Tauri frontend is not built or installed.

%prep
%setup -q

%build
export CARGO_HOME="$PWD/.cargo-home"
export CARGO_NET_OFFLINE=true
export LIBSQLITE3_SYS_USE_PKG_CONFIG=1
export CARGO_BUILD_JOBS=%{_smp_build_ncpus}
export RUSTFLAGS="%{build_rustflags} --remap-path-prefix=$PWD=/usr/src/ryotunes"
cargo build --frozen --release -p ryotunesd -p ryotunes-cli --features ryotunesd/fedora

%install
install -Dm755 ${CARGO_TARGET_DIR:-target}/release/ryotunes-native %{buildroot}%{_bindir}/ryotunes
for name in ryotunesd ryotunes-cli; do
    install -Dm755 ${CARGO_TARGET_DIR:-target}/release/$name %{buildroot}%{_bindir}/$name
done
install -Dm755 packaging/linux/ryotunes-qml %{buildroot}%{_bindir}/ryotunes-qml
install -d %{buildroot}%{_datadir}/ryotunes
cp -a client skins matugen %{buildroot}%{_datadir}/ryotunes/
rm -rf %{buildroot}%{_datadir}/ryotunes/client/tests
find %{buildroot}%{_datadir}/ryotunes -type d -exec chmod 755 {} +
find %{buildroot}%{_datadir}/ryotunes -type f -exec chmod 644 {} +
install -Dm644 packaging/linux/ryotunes.desktop %{buildroot}%{_datadir}/applications/ryotunes.desktop
install -Dm644 packaging/linux/dev.ryoku.ryotunes.metainfo.xml %{buildroot}%{_datadir}/metainfo/dev.ryoku.ryotunes.metainfo.xml
for size in 32 64 128; do
    install -Dm644 src-tauri/icons/${size}x${size}.png %{buildroot}%{_datadir}/icons/hicolor/${size}x${size}/apps/ryotunes.png
done
install -Dm644 src-tauri/icons/128x128@2x.png %{buildroot}%{_datadir}/icons/hicolor/256x256/apps/ryotunes.png
install -Dm644 src-tauri/icons/icon.png %{buildroot}%{_datadir}/icons/hicolor/512x512/apps/ryotunes.png
install -Dm644 packaging/linux/ryotunesd.service %{buildroot}%{_userunitdir}/ryotunesd.service
install -Dm644 packaging/linux/ryotunesd.socket %{buildroot}%{_userunitdir}/ryotunesd.socket
# Fedora preset policy belongs to the administrator; the launcher starts the socket on demand.

%check
export CARGO_HOME="$PWD/.cargo-home" CARGO_NET_OFFLINE=true LIBSQLITE3_SYS_USE_PKG_CONFIG=1
export CARGO_BUILD_JOBS=%{_smp_build_ncpus}
export RUSTFLAGS="%{build_rustflags} --remap-path-prefix=$PWD=/usr/src/ryotunes"
cargo test --frozen --workspace --exclude ryotunes --features ryotunesd/fedora -- --skip mpv_keeps_the_gain_through_pitch_changes_and_failures
for skin in skins/*/skin.json; do ${CARGO_TARGET_DIR:-target}/release/ryotunes-cli skin check "${skin%/skin.json}"; done
desktop-file-validate %{buildroot}%{_datadir}/applications/ryotunes.desktop

%post
%systemd_user_post ryotunesd.socket ryotunesd.service

%preun
%systemd_user_preun ryotunesd.socket ryotunesd.service

%postun
%systemd_user_postun ryotunesd.socket ryotunesd.service

%files
%license LICENSE
%doc README.md UPSTREAM.md docs/INSTALL-FEDORA.md
%{_bindir}/ryotunes
%{_bindir}/ryotunesd
%{_bindir}/ryotunes-cli
%{_bindir}/ryotunes-qml
%{_datadir}/ryotunes/
%{_datadir}/applications/ryotunes.desktop
%{_datadir}/metainfo/dev.ryoku.ryotunes.metainfo.xml
%{_datadir}/icons/hicolor/*/apps/ryotunes.png
%{_userunitdir}/ryotunesd.service
%{_userunitdir}/ryotunesd.socket
