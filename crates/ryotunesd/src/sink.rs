//! `EventSink` over the socket: every event goes to every connection that subscribed.

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Mutex;

use ryotunes_core::host::EventSink;
use ryotunes_protocol::{Event, Outgoing};
use serde_json::Value;
use tokio::sync::mpsc::UnboundedSender;

#[derive(Default)]
pub struct SocketSink {
    subscribers: Mutex<Vec<UnboundedSender<String>>>,
    count: AtomicUsize,
}

impl SocketSink {
    pub fn subscribe(&self, tx: UnboundedSender<String>) {
        self.subscribers.lock().unwrap().push(tx);
        self.count.fetch_add(1, Ordering::Relaxed);
    }

    /// Live subscribers, after dropping the ones whose connection has gone. A client killed
    /// outright leaves its sender open until the next emit would find it closed; `show` reads
    /// this to decide between raising a client and launching one, so it must not count ghosts
    /// (a stale count kept the daemon emitting `show` to nobody instead of spawning the client).
    pub fn subscriber_count(&self) -> usize {
        let mut subs = self.subscribers.lock().unwrap();
        self.prune(&mut subs);
        self.count.load(Ordering::Relaxed)
    }

    fn prune(&self, subs: &mut Vec<UnboundedSender<String>>) {
        let before = subs.len();
        subs.retain(|tx| !tx.is_closed());
        let dropped = before - subs.len();
        if dropped > 0 {
            self.count.fetch_sub(dropped, Ordering::Relaxed);
        }
    }
}

impl EventSink for SocketSink {
    fn emit(&self, event: &'static str, payload: Value) {
        let line = Outgoing::Event(Event { event: event.to_owned(), data: payload }).to_line();
        let mut subs = self.subscribers.lock().unwrap();
        self.prune(&mut subs);
        for tx in subs.iter() {
            let _ = tx.send(line.clone());
        }
    }
}
