mod app;
mod download_media;
mod downloads;
mod gtk_thread;
mod js;
mod lifecycle;
mod login;
mod methods;
mod server;
mod sink;
mod tray;

use std::io::{BufRead, BufReader, Write};
use std::os::unix::io::AsRawFd;
use std::os::unix::net::UnixStream;
use std::path::Path;
use std::sync::Arc;

fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| "warn".into()),
        )
        .init();

    let path = ryotunes_protocol::socket_path();
    // Single instance: hold the lock and run, or hand a `show` to the incumbent and exit 0.
    let lock = match acquire_or_show(&path)? {
        Some(lock) => lock,
        None => return Ok(()),
    };

    let rt = tokio::runtime::Builder::new_multi_thread().enable_all().build()?;
    let gtk = gtk_thread::Gtk::start();
    let sink = Arc::new(sink::SocketSink::default());
    let js: Arc<dyn ryotunes_core::host::JsBridge> = Arc::new(js::GtkJs::new(gtk.clone()));
    let login: Arc<dyn ryotunes_core::host::LoginFlow> =
        Arc::new(login::GtkLogin::new(gtk.clone()));

    rt.block_on(async {
        let (quit_tx, mut quit_rx) = tokio::sync::mpsc::unbounded_channel();
        // The idle-exit lifecycle arms itself immediately: an activated daemon with no subscriber
        // and nothing playing must not linger past the grace.
        let lifecycle = lifecycle::Lifecycle::new(quit_tx.clone());

        let paths = app::paths();
        let data_dir = paths.data_dir.clone();
        let (state, events, media_rx, lt_rx) =
            app::build(paths, sink.clone(), js, login, &tokio::runtime::Handle::current())?;
        app::spawn_pumps(state.clone(), events, media_rx, lt_rx, sink.clone(), lifecycle.clone());
        tray::spawn(state.clone(), quit_tx.clone(), sink.clone());

        // Restore a cached Spotify session in the background so browsing and playback are ready
        // without blocking startup. Logged, never fatal: no cached credentials is the common case.
        {
            let state = state.clone();
            tokio::spawn(async move {
                match state.spotify.restore().await {
                    Ok(true) => tracing::info!("spotify: restored a cached session"),
                    Ok(false) => tracing::debug!("spotify: no cached session to restore"),
                    Err(e) => tracing::warn!("spotify: restore failed: {:#}", e),
                }
            });
        }

        // The downloads subsystem shares the app data dir for its durable queue/history, reuses the
        // core SoundCloud client to resolve `sc:track:` permalinks, and reports queued/active work
        // to the lifecycle so closing the UI never abandons a download in flight.
        let downloads = downloads::Downloads::new(
            sink.clone(),
            state.soundcloud.clone(),
            data_dir,
            lifecycle.clone(),
            Some(state.clone()),
        )?;

        let server = server::Server::bind(&path, sink.clone(), lifecycle)?;
        let methods = Arc::new(methods::Methods {
            state: state.clone(),
            quit: quit_tx,
            downloads: downloads.clone(),
        });
        // A `systemctl stop` (or any `kill`) sends SIGTERM: handle it exactly like ctrl_c / an
        // explicit quit so the awaited teardown below runs — cancelling and reaping every download
        // process group — rather than leaving that to the runtime `Drop`.
        let mut sigterm = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .expect("install SIGTERM handler");
        tokio::select! {
            _ = server.run(methods) => {}
            _ = quit_rx.recv() => {}
            _ = tokio::signal::ctrl_c() => {}
            _ = sigterm.recv() => {}
        }
        // Explicit teardown: stop mpv, flush the resume position, unregister MPRIS, drop Discord and
        // leave any Listen Together room — exactly what `main_window::request_quit` runs today.
        // Reap downloads alongside playback teardown; neither subsystem waits for the other before
        // it stops accepting work.
        tokio::join!(downloads.shutdown(), state.shutdown_for_quit());
        Ok::<(), anyhow::Error>(())
    })?;

    drop(lock);
    // systemd owns the socket file for an activated instance; only a self-bound socket is ours to
    // remove.
    if !server::socket_activated() {
        let _ = std::fs::remove_file(&path);
    }
    Ok(())
}

/// Single-instance handshake, mirroring `tauri-plugin-single-instance`: hold `ryotunesd.sock.lock`
/// under `flock(LOCK_EX | LOCK_NB)` for the process lifetime, so a second daemon cannot fight this
/// one over the shared SQLite/mpv state. `Some(lock)` — we are the instance, keep the handle open.
/// `None` — an incumbent holds the lock; we asked it to `show`, printed its reply, and should exit 0.
fn acquire_or_show(path: &Path) -> anyhow::Result<Option<std::fs::File>> {
    let dir = path.parent().expect("socket path has a parent");
    std::fs::create_dir_all(dir)?;
    use std::os::unix::fs::PermissionsExt;
    let _ = std::fs::set_permissions(dir, std::fs::Permissions::from_mode(0o700));
    let lock_path = dir.join("ryotunesd.sock.lock");
    let file =
        std::fs::OpenOptions::new().create(true).truncate(false).write(true).open(&lock_path)?;
    // Safe: a plain advisory-lock syscall on a fd we own.
    let rc = unsafe { libc::flock(file.as_raw_fd(), libc::LOCK_EX | libc::LOCK_NB) };
    if rc == 0 {
        return Ok(Some(file));
    }
    let err = std::io::Error::last_os_error();
    if err.raw_os_error() != Some(libc::EWOULDBLOCK) {
        return Err(anyhow::anyhow!("locking {}: {err}", lock_path.display()));
    }
    // Another ryotunesd is live: a second launch means "show the window" (or launch the client).
    match forward_show(path) {
        Ok(response) => println!("{response}"),
        Err(e) => eprintln!("ryotunesd already running; `show` request failed: {e}"),
    }
    Ok(None)
}

/// Connect to the incumbent's socket, request `show`, and return its one-line response.
fn forward_show(path: &Path) -> std::io::Result<String> {
    let mut stream = UnixStream::connect(path)?;
    stream.write_all(b"{\"id\":1,\"method\":\"show\"}\n")?;
    let mut reader = BufReader::new(stream);
    let mut line = String::new();
    reader.read_line(&mut line)?;
    Ok(line.trim_end().to_string())
}
