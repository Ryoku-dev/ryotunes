//! Unix-socket server: one task per connection, requests dispatched concurrently, responses
//! and events multiplexed through a per-connection writer task.

use std::os::unix::fs::PermissionsExt;
use std::os::unix::io::FromRawFd;
use std::path::Path;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use ryotunes_protocol::{ErrorBody, Incoming, Outgoing, Request, Response};
use serde_json::Value;
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::net::{UnixListener, UnixStream};
use tokio::sync::mpsc;

use crate::lifecycle::Lifecycle;
use crate::sink::SocketSink;

#[async_trait::async_trait]
pub trait Dispatch: Send + Sync + 'static {
    async fn call(
        &self,
        method: &str,
        params: Value,
        conn: &Connection,
    ) -> Result<Value, ErrorBody>;
}

/// What a method handler may do to the connection it arrived on.
pub struct Connection {
    pub tx: mpsc::UnboundedSender<String>,
    pub sink: Arc<SocketSink>,
    lifecycle: Arc<Lifecycle>,
    /// Set once so a connection counts as at most one subscriber for the idle-exit lifecycle, and so
    /// the disconnect in [`handle`] only reports a departure for a connection that actually joined.
    subscribed: AtomicBool,
}

impl Connection {
    /// `subscribe`: from now on every event reaches this connection, and the idle-exit deadline is
    /// cancelled while it stays connected. Idempotent — a second `subscribe` on one connection is a
    /// no-op rather than a double count.
    pub fn subscribe(&self) {
        if self.subscribed.swap(true, Ordering::AcqRel) {
            return;
        }
        self.sink.subscribe(self.tx.clone());
        self.lifecycle.client_connected();
    }
}

pub struct Server {
    listener: UnixListener,
    sink: Arc<SocketSink>,
    lifecycle: Arc<Lifecycle>,
}

impl Server {
    /// Adopt the systemd socket when socket-activated, otherwise bind the path with the directory at
    /// 0700 and the socket at 0700 (a socket's base mode is 0777, so under umask 077 it lands
    /// owner-only, 0700).
    pub fn bind(
        path: &Path,
        sink: Arc<SocketSink>,
        lifecycle: Arc<Lifecycle>,
    ) -> std::io::Result<Server> {
        if let Some(listener) = systemd_listener()? {
            // systemd already created the socket and directory with the unit's SocketMode/
            // DirectoryMode; adopt its fd instead of binding a second one.
            return Ok(Server { listener, sink, lifecycle });
        }
        let dir = path.parent().expect("socket path has a parent");
        std::fs::create_dir_all(dir)?;
        std::fs::set_permissions(dir, std::fs::Permissions::from_mode(0o700))?;
        let _ = std::fs::remove_file(path);
        // Safe: umask is process-global and we restore it immediately; nothing else creates
        // files during this window (the daemon is single-threaded until `run`).
        let old = unsafe { libc::umask(0o077) };
        let listener = UnixListener::bind(path);
        unsafe { libc::umask(old) };
        Ok(Server { listener: listener?, sink, lifecycle })
    }

    pub async fn run(self, dispatch: Arc<dyn Dispatch>) {
        loop {
            match self.listener.accept().await {
                Ok((stream, _)) => {
                    tokio::spawn(handle(
                        stream,
                        dispatch.clone(),
                        self.sink.clone(),
                        self.lifecycle.clone(),
                    ));
                }
                Err(e) => tracing::warn!(error = %e, "accept failed"),
            }
        }
    }
}

/// True when started by systemd socket activation: `LISTEN_FDS >= 1` and `LISTEN_PID` names this
/// process. Both [`Server::bind`] (to adopt the fd) and `main` (to leave the systemd-owned socket
/// file in place on exit) ask this.
pub fn socket_activated() -> bool {
    let fds = std::env::var("LISTEN_FDS").ok().and_then(|v| v.parse::<i32>().ok()).unwrap_or(0);
    let pid = std::env::var("LISTEN_PID").ok().and_then(|v| v.parse::<u32>().ok()).unwrap_or(0);
    fds >= 1 && pid == std::process::id()
}

/// The passed-in listener at `SD_LISTEN_FDS_START` (fd 3) when socket-activated, else `None`.
fn systemd_listener() -> std::io::Result<Option<UnixListener>> {
    if !socket_activated() {
        return Ok(None);
    }
    // Safe: fd 3 is the single socket systemd created and passed for us to own.
    let std_listener = unsafe { std::os::unix::net::UnixListener::from_raw_fd(3) };
    std_listener.set_nonblocking(true)?;
    Ok(Some(UnixListener::from_std(std_listener)?))
}

async fn handle(
    stream: UnixStream,
    dispatch: Arc<dyn Dispatch>,
    sink: Arc<SocketSink>,
    lifecycle: Arc<Lifecycle>,
) {
    let (rd, mut wr) = stream.into_split();
    let (tx, mut rx) = mpsc::unbounded_channel::<String>();
    let writer = tokio::spawn(async move {
        while let Some(line) = rx.recv().await {
            if wr.write_all(line.as_bytes()).await.is_err() {
                break;
            }
        }
    });
    let conn = Arc::new(Connection {
        tx: tx.clone(),
        sink,
        lifecycle: lifecycle.clone(),
        subscribed: AtomicBool::new(false),
    });
    let mut lines = BufReader::new(rd).lines();
    while let Ok(Some(line)) = lines.next_line().await {
        let req = match serde_json::from_str::<Incoming>(&line) {
            Ok(Incoming::Request(r)) => r,
            Err(e) => {
                let _ = tx.send(
                    Outgoing::Response(Response::err(0, "bad_request", e.to_string())).to_line(),
                );
                continue;
            }
        };
        let dispatch = dispatch.clone();
        let conn = conn.clone();
        let tx = tx.clone();
        tokio::spawn(async move {
            let Request { id, method, params } = req;
            let resp = match dispatch.call(&method, params, &conn).await {
                Ok(v) => Response::ok(id, v),
                Err(e) => Response { id, result: None, error: Some(e) },
            };
            let _ = tx.send(Outgoing::Response(resp).to_line());
        });
    }
    // The client is gone: if it had subscribed, its departure may re-arm the idle-exit deadline.
    if conn.subscribed.load(Ordering::Acquire) {
        lifecycle.client_gone();
    }
    drop(tx);
    // The sink still holds this connection's sender, so the writer would idle until the next
    // event failed a write to the dead socket; abort it now so the receiver goes and every clone
    // of the sender reports closed. That is what lets `subscriber_count` see the departure
    // straight away (a `show` right after a client was killed must launch one, not emit into
    // the void).
    writer.abort();
    let _ = writer.await;
}

#[cfg(test)]
mod tests {
    use super::*;
    use ryotunes_core::host::EventSink;

    struct Echo;
    #[async_trait::async_trait]
    impl Dispatch for Echo {
        async fn call(
            &self,
            method: &str,
            params: Value,
            conn: &Connection,
        ) -> Result<Value, ErrorBody> {
            match method {
                "echo" => Ok(params),
                "subscribe" => {
                    conn.subscribe();
                    Ok(Value::Null)
                }
                _ => Err(ErrorBody { code: "unknown_method".into(), message: method.into() }),
            }
        }
    }

    #[tokio::test]
    async fn echo_subscribe_and_event_fan_out() {
        let dir = tempfile_dir();
        let path = dir.join("ryotunesd.sock");
        let sink = Arc::new(SocketSink::default());
        let (quit_tx, _quit_rx) = mpsc::unbounded_channel();
        let lifecycle = Lifecycle::new(quit_tx);
        let server = Server::bind(&path, sink.clone(), lifecycle).unwrap();
        tokio::spawn(server.run(Arc::new(Echo)));

        let stream = UnixStream::connect(&path).await.unwrap();
        let (rd, mut wr) = stream.into_split();
        let mut lines = BufReader::new(rd).lines();

        wr.write_all(b"{\"id\":1,\"method\":\"echo\",\"params\":{\"a\":1}}\n").await.unwrap();
        assert_eq!(lines.next_line().await.unwrap().unwrap(), "{\"id\":1,\"result\":{\"a\":1}}");

        wr.write_all(b"{\"id\":2,\"method\":\"nope\"}\n").await.unwrap();
        let l = lines.next_line().await.unwrap().unwrap();
        assert!(l.contains("\"error\"") && l.contains("unknown_method"));

        assert_eq!(sink.subscriber_count(), 0);
        wr.write_all(b"{\"id\":3,\"method\":\"subscribe\"}\n").await.unwrap();
        assert_eq!(lines.next_line().await.unwrap().unwrap(), "{\"id\":3,\"result\":null}");
        assert_eq!(sink.subscriber_count(), 1);

        sink.emit("position", serde_json::json!({ "position": 2.5 }));
        assert_eq!(
            lines.next_line().await.unwrap().unwrap(),
            "{\"event\":\"position\",\"data\":{\"position\":2.5}}"
        );

        drop(wr);
        drop(lines);
        // The reader loop ends when the client goes and aborts the writer, so the sink's sender
        // reports closed and the count drops on the next read, without an emit in between.
        let mut pruned = false;
        for _ in 0..100 {
            tokio::time::sleep(std::time::Duration::from_millis(10)).await;
            if sink.subscriber_count() == 0 {
                pruned = true;
                break;
            }
        }
        assert!(pruned, "closed subscriber was never pruned");

        // A socket's base mode is 0777, so under umask 077 the file lands at 0700 (owner-only).
        let meta = std::fs::metadata(&path).unwrap();
        assert_eq!(meta.permissions().mode() & 0o777, 0o700);
        assert_eq!(std::fs::metadata(&dir).unwrap().permissions().mode() & 0o777, 0o700);
    }

    fn tempfile_dir() -> std::path::PathBuf {
        let p = std::env::temp_dir().join(format!(
            "ryotunesd-test-{}-{}",
            std::process::id(),
            rand_suffix()
        ));
        p
    }
    fn rand_suffix() -> u64 {
        use std::time::{SystemTime, UNIX_EPOCH};
        SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_nanos() as u64
    }
}
