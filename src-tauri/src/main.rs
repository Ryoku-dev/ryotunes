// Prevents additional console window on Windows in release, DO NOT REMOVE!!
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    // Linux ships `ryotunes` as a thin launcher for the native Quickshell client; it never runs the
    // Tauri app. macOS and Windows still run the bundled Tauri player.
    #[cfg(target_os = "linux")]
    {
        native::launch();
    }
    #[cfg(not(target_os = "linux"))]
    {
        app_lib::run();
    }
}

/// On Linux the installed `/usr/bin/ryotunes` (the desktop's keybind, dock and launcher) hands
/// `ryotunesd` a `show`, which raises the connected client or opens the native `ryotunes-qml`
/// client. Connecting to the socket is what starts the daemon — systemd socket activation, or a
/// direct spawn on a box without systemd — so a cold boot lands in the native client too. There is
/// deliberately no Tauri route on Linux: `--tauri`/`RYOTUNES_TAURI` are gone, and a machine that
/// cannot reach the daemon gets a real error rather than a second player on the same audio device.
#[cfg(target_os = "linux")]
mod native;
