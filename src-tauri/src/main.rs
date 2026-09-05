// Prevents additional console window on Windows in release, DO NOT REMOVE!!
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    #[cfg(target_os = "linux")]
    if daemon::defer_to_native() {
        return;
    }
    app_lib::run();
}

/// The native client is the default: `/usr/bin/ryotunes` (the desktop's keybind, dock and launcher)
/// asks `ryotunesd` to `show`, which raises the connected client or opens `ryotunes-qml`. Connecting
/// to the socket is what starts the daemon when systemd holds it idle (socket activation), so a
/// cold boot lands in the native client too. The Tauri app, with its own player, runs only on
/// `ryotunes --tauri` (or `RYOTUNES_TAURI=1`) or when no daemon socket exists at all, so a box
/// without the daemon installed keeps working.
#[cfg(target_os = "linux")]
mod daemon {
    use std::io::{BufRead, BufReader, Write};
    use std::os::unix::net::UnixStream;

    pub fn defer_to_native() -> bool {
        if std::env::args().any(|a| a == "--tauri") || std::env::var_os("RYOTUNES_TAURI").is_some()
        {
            return false;
        }
        let sock = ryotunes_protocol::socket_path();
        if !sock.exists() {
            tracing_lite("no ryotunesd socket; starting the Tauri app");
            return false;
        }
        match show(&sock) {
            Ok(reply) => {
                tracing_lite(&format!("asked ryotunesd to show ({reply})"));
                true
            }
            Err(e) => {
                tracing_lite(&format!("ryotunesd `show` failed: {e}; starting the Tauri app"));
                false
            }
        }
    }

    fn show(sock: &std::path::Path) -> std::io::Result<String> {
        let mut stream = UnixStream::connect(sock)?;
        stream.set_read_timeout(Some(std::time::Duration::from_secs(5)))?;
        stream.write_all(b"{\"id\":1,\"method\":\"show\"}\n")?;
        let mut line = String::new();
        BufReader::new(stream).read_line(&mut line)?;
        Ok(line.trim_end().to_string())
    }

    // The tracing subscriber is installed inside `app_lib::run`, which this path never reaches.
    fn tracing_lite(msg: &str) {
        eprintln!("ryotunes: {msg}");
    }
}
