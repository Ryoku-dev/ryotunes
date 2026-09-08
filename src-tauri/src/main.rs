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
mod native {
    use std::io::{BufRead, BufReader, Write};
    use std::os::unix::net::UnixStream;
    use std::path::Path;
    use std::process::{Command, Stdio};
    use std::time::{Duration, Instant};

    pub fn launch() {
        let sock = ryotunes_protocol::socket_path();
        // Fast path: a live daemon (its client already up) or an idle systemd-activated socket
        // answers the first `show` straight away.
        if show(&sock).is_ok() {
            log("asked ryotunesd to show the native client");
            return;
        }
        // Nothing is listening: bring the daemon up ourselves, then raise the client. Never Tauri.
        if let Err(e) = ensure_daemon() {
            log(&format!("could not start ryotunesd: {e}"));
            std::process::exit(1);
        }
        match show_until_ready(&sock) {
            Ok(()) => log("asked ryotunesd to show the native client"),
            Err(e) => {
                eprintln!(
                    "ryotunes: could not reach the Ryotunes daemon (ryotunesd): {e}\n\
                     Start it with `systemctl --user start ryotunesd.socket`, or run `ryotunesd`."
                );
                std::process::exit(1);
            }
        }
    }

    /// A successful protocol reply acknowledges `show`; EOF and daemon errors are failures.
    fn show(sock: &Path) -> std::io::Result<()> {
        let mut stream = UnixStream::connect(sock)?;
        stream.set_read_timeout(Some(Duration::from_secs(5)))?;
        stream.write_all(b"{\"id\":1,\"method\":\"show\"}\n")?;
        let mut line = String::new();
        BufReader::new(stream).read_line(&mut line)?;
        let response: ryotunes_protocol::Response = serde_json::from_str(&line)
            .map_err(|e| std::io::Error::new(std::io::ErrorKind::InvalidData, e))?;
        if response.id != 1 {
            return Err(std::io::Error::new(
                std::io::ErrorKind::InvalidData,
                "unexpected response ID",
            ));
        }
        if let Some(error) = response.error {
            return Err(std::io::Error::other(error.message));
        }
        Ok(())
    }

    /// Retry `show` while the daemon we just started binds and serves its socket. Bounded, so an
    /// unreachable daemon surfaces as an error instead of hanging forever.
    fn show_until_ready(sock: &Path) -> std::io::Result<()> {
        let deadline = Instant::now() + Duration::from_secs(20);
        loop {
            let error = match show(sock) {
                Ok(()) => return Ok(()),
                Err(e) => e,
            };
            if Instant::now() >= deadline {
                return Err(error);
            }
            std::thread::sleep(Duration::from_millis(100));
        }
    }

    /// Bring the daemon up without ever touching the Tauri app. Prefer systemd socket activation so
    /// the daemon lands under the user manager (tray, MPRIS, idle-exit); fall back to spawning
    /// `ryotunesd` directly on a box without a working systemd `--user` instance, where the daemon's
    /// own bind clears any stale socket first.
    fn ensure_daemon() -> std::io::Result<()> {
        if start_socket_unit() {
            return Ok(());
        }
        spawn_daemon()
    }

    /// `systemctl --user start ryotunesd.socket`: true when systemd accepted it, so the socket is now
    /// listening and the next connect activates the service. False when there is no working user
    /// systemd or the unit is absent — the caller then spawns the daemon directly.
    fn start_socket_unit() -> bool {
        Command::new("systemctl")
            .args(["--user", "--no-block", "start", "ryotunesd.socket"])
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status()
            .map(|s| s.success())
            .unwrap_or(false)
    }

    /// Spawn `ryotunesd` detached. Its single-instance handshake either becomes the daemon (binding
    /// the socket after clearing a stale file) or forwards `show` to a live one and exits.
    fn spawn_daemon() -> std::io::Result<()> {
        Command::new("ryotunesd")
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .map(|_| ())
    }

    // The tracing subscriber lives in the daemon; this launcher only prints to stderr.
    fn log(msg: &str) {
        eprintln!("ryotunes: {msg}");
    }
}
