//! Idle exit: with nothing playing and no client subscribed, the daemon keeps the lightweight
//! backend alive for a bounded grace and then leaves cleanly, exactly as the Tauri tray-only build
//! does (`src-tauri/src/main_window.rs` `schedule_idle_exit` / `IDLE_EXIT_GRACE`). The deadline is
//! armed when playback stops or the last subscriber disconnects, and cancelled when playback starts
//! or a client subscribes.
//!
//! The decision is a pure state machine ([`IdleTimer`]) so it can be driven by a fake clock in
//! tests; [`Lifecycle`] wraps it for the running daemon, spawning a single tokio timer per arm and
//! invalidating stale timers through an epoch counter (the same trick `main_window.rs` uses).

use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use tokio::sync::mpsc::UnboundedSender;

/// Idle grace before a tray-only daemon exits: one minute after the last client is gone with
/// nothing playing (the client quits itself a minute into a pause when its window is off screen,
/// so a paused, closed Ryotunes is fully out of memory ~2 minutes after the pause). Socket
/// activation brings the daemon back on the next `ryotunes`, media key or MPRIS call. The
/// `RYOTUNESD_IDLE_EXIT_SECS` environment variable overrides it (manual verification uses 10).
const DEFAULT_IDLE_EXIT_SECS: u64 = 60;

fn idle_exit_grace() -> Duration {
    let secs = std::env::var("RYOTUNESD_IDLE_EXIT_SECS")
        .ok()
        .and_then(|v| v.parse::<u64>().ok())
        .filter(|&s| s > 0)
        .unwrap_or(DEFAULT_IDLE_EXIT_SECS);
    Duration::from_secs(secs)
}

/// The pure arming logic: eligible for idle exit when nothing is playing and no client is
/// subscribed. `refresh` arms the deadline on the first idle moment and clears it the instant the
/// daemon becomes busy again; every event routes through it so arm/cancel/re-arm can never drift.
struct IdleTimer {
    grace: Duration,
    deadline: Option<Instant>,
    playing: bool,
    clients: usize,
    /// A queued or active download holds the daemon open exactly like playback or a subscriber:
    /// closing the UI must never abandon a download in flight. Driven by [`Lifecycle::
    /// downloads_busy_changed`] off the downloads manager's own active/queued count, so it never
    /// borrows the subscriber count for this.
    downloading: bool,
}

impl IdleTimer {
    fn new(grace: Duration) -> Self {
        IdleTimer { grace, deadline: None, playing: false, clients: 0, downloading: false }
    }

    fn eligible(&self) -> bool {
        self.clients == 0 && !self.playing && !self.downloading
    }

    /// Arm the deadline when newly idle (keeping an already-running deadline so the grace is not
    /// pushed out by redundant events), clear it the moment the daemon is busy again.
    fn refresh(&mut self, now: Instant) {
        if self.eligible() {
            if self.deadline.is_none() {
                self.deadline = Some(now + self.grace);
            }
        } else {
            self.deadline = None;
        }
    }

    fn set_playing(&mut self, now: Instant, playing: bool) {
        self.playing = playing;
        self.refresh(now);
    }

    fn set_downloading(&mut self, now: Instant, downloading: bool) {
        self.downloading = downloading;
        self.refresh(now);
    }

    fn client_connected(&mut self, now: Instant) {
        self.clients += 1;
        self.refresh(now);
    }

    fn client_gone(&mut self, now: Instant) {
        self.clients = self.clients.saturating_sub(1);
        self.refresh(now);
    }

    /// The armed deadline has passed and the daemon is still idle.
    fn expired(&self, now: Instant) -> bool {
        matches!(self.deadline, Some(d) if now >= d)
    }
}

/// Runtime wrapper around [`IdleTimer`]. Each transition into the armed state spawns one timer that
/// sleeps to the deadline and, if it is still the current arm (epoch unchanged) and the daemon is
/// still idle, sends on the quit channel — the daemon then runs `shutdown_for_quit` and exits 0.
pub struct Lifecycle {
    inner: Mutex<IdleTimer>,
    epoch: AtomicU64,
    quit: UnboundedSender<()>,
}

impl Lifecycle {
    /// Build the lifecycle and arm the startup idle deadline: a freshly activated daemon with no
    /// subscriber and nothing playing is already idle and must not linger forever.
    pub fn new(quit: UnboundedSender<()>) -> Arc<Self> {
        let this = Arc::new(Lifecycle {
            inner: Mutex::new(IdleTimer::new(idle_exit_grace())),
            epoch: AtomicU64::new(0),
            quit,
        });
        this.apply(IdleTimer::refresh);
        this
    }

    /// Playback started or stopped. A paused track counts as not playing, exactly as the Tauri
    /// policy treats it.
    pub fn playing_changed(self: &Arc<Self>, playing: bool) {
        self.apply(|t, now| t.set_playing(now, playing));
    }

    /// A client subscribed to events (the daemon's equivalent of a visible UI).
    pub fn client_connected(self: &Arc<Self>) {
        self.apply(IdleTimer::client_connected);
    }

    /// A subscribed client's connection ended.
    pub fn client_gone(self: &Arc<Self>) {
        self.apply(IdleTimer::client_gone);
    }

    /// The downloads manager has queued/active work, or has just drained to none. A busy manager
    /// pins the daemon open across a UI close (a download that outlives its window), and clearing
    /// it re-arms the idle grace like any other transition back to idle.
    pub fn downloads_busy_changed(self: &Arc<Self>, busy: bool) {
        self.apply(|t, now| t.set_downloading(now, busy));
    }

    /// Apply a transition under the lock and reconcile the timer: on a fresh arm (no deadline ->
    /// deadline) schedule one waiter; on a cancel (deadline -> none) bump the epoch so any pending
    /// waiter no-ops when it wakes.
    fn apply(self: &Arc<Self>, f: impl FnOnce(&mut IdleTimer, Instant)) {
        let (before, after) = {
            let mut timer = self.inner.lock().unwrap();
            let before = timer.deadline;
            f(&mut timer, Instant::now());
            (before, timer.deadline)
        };
        match (before, after) {
            (None, Some(deadline)) => {
                let epoch = self.epoch.fetch_add(1, Ordering::AcqRel) + 1;
                self.schedule(epoch, deadline);
            }
            (Some(_), None) => {
                self.epoch.fetch_add(1, Ordering::AcqRel);
            }
            _ => {}
        }
    }

    fn schedule(self: &Arc<Self>, epoch: u64, deadline: Instant) {
        let this = self.clone();
        tokio::spawn(async move {
            tokio::time::sleep(deadline.saturating_duration_since(Instant::now())).await;
            if this.epoch.load(Ordering::Acquire) != epoch {
                return; // a later arm/cancel superseded this deadline
            }
            let expired = this.inner.lock().unwrap().expired(Instant::now());
            if expired {
                tracing::info!(
                    "idle grace elapsed with no subscriber and nothing playing; exiting"
                );
                let _ = this.quit.send(());
            }
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn arms_cancels_expires_and_rearms() {
        let grace = Duration::from_secs(300);
        let t0 = Instant::now();
        let mut t = IdleTimer::new(grace);

        // Startup idle: the first refresh arms a deadline one grace out.
        t.refresh(t0);
        let d0 = t.deadline.expect("armed when idle at startup");
        assert_eq!(d0, t0 + grace);
        assert!(!t.expired(t0));
        assert!(!t.expired(t0 + grace - Duration::from_secs(1)));

        // A client subscribes: cancelled, and no amount of elapsed time expires it.
        t.client_connected(t0 + Duration::from_secs(10));
        assert!(t.deadline.is_none());
        assert!(!t.expired(t0 + grace * 2));

        // Playback also holds it open once the client leaves.
        t.set_playing(t0 + Duration::from_secs(20), true);
        t.client_gone(t0 + Duration::from_secs(30));
        assert!(t.deadline.is_none(), "playing keeps it alive with no client");

        // Re-arm from a fresh instant when playback stops with nobody listening.
        let t1 = t0 + Duration::from_secs(40);
        t.set_playing(t1, false);
        let d1 = t.deadline.expect("re-armed when idle again");
        assert_eq!(d1, t1 + grace);

        // Expire past the new deadline.
        assert!(!t.expired(d1 - Duration::from_secs(1)));
        assert!(t.expired(d1));
        assert!(t.expired(d1 + Duration::from_secs(5)));
    }

    #[test]
    fn a_download_in_flight_holds_the_daemon_open() {
        let grace = Duration::from_secs(300);
        let t0 = Instant::now();
        let mut t = IdleTimer::new(grace);

        // Idle at startup with a deadline armed, then a download is enqueued: the daemon must not
        // idle-exit while it runs, even with no subscriber and nothing playing (closing the UI
        // must never abandon a download).
        t.refresh(t0);
        assert!(t.deadline.is_some());
        t.set_downloading(t0 + Duration::from_secs(5), true);
        assert!(t.deadline.is_none(), "an active download cancels the idle deadline");
        assert!(!t.expired(t0 + grace * 2), "no elapsed time expires it while downloading");

        // The queue drains: idle again, so the grace re-arms from that instant.
        let t1 = t0 + Duration::from_secs(30);
        t.set_downloading(t1, false);
        assert_eq!(t.deadline.expect("re-armed once downloads drain"), t1 + grace);
    }

    #[test]
    fn redundant_idle_events_do_not_push_out_the_deadline() {
        let grace = Duration::from_secs(300);
        let t0 = Instant::now();
        let mut t = IdleTimer::new(grace);
        t.refresh(t0);
        let armed = t.deadline.unwrap();
        // A second "still not playing" a minute later must not reset the grace.
        t.set_playing(t0 + Duration::from_secs(60), false);
        assert_eq!(t.deadline.unwrap(), armed);
    }
}
