//! Diagnostics: one rolling record of everything that went wrong, for both halves of the app.
//!
//! Every `warn`/`error` the daemon logs (tracing), every RPC that answered an error (the server
//! records it at the one chokepoint all responses pass), every panic (the hook below), and every
//! failure the *client* saw (the `log_client_error` RPC — including failures the client buffers
//! locally while the daemon is down) land in one ring buffer and one rotating log file under
//! `$XDG_DATA_HOME/dev.ryoku.ryotunes/logs/`. The Settings ▸ Diagnostics page reads them back
//! through `get_logs`, so "some users can't play anything" becomes a list of lines instead of a
//! guess. The file survives restarts (that is the point: a crash loop still leaves evidence);
//! the ring is per-process and always available even if the disk is the thing that is broken.

use std::collections::VecDeque;
use std::io::Write;
use std::path::{Path, PathBuf};
use std::sync::OnceLock;
use std::time::{SystemTime, UNIX_EPOCH};

use parking_lot::Mutex;

use serde_json::{json, Value};
use tracing::field::{Field, Visit};
use tracing::{Event, Level, Subscriber};
use tracing_subscriber::layer::Context;
use tracing_subscriber::Layer;

/// Ring-buffer cap. Two thousand lines is far more than anyone reads at once but small enough to
/// snapshot into every `get_logs` without thinking.
const RING_MAX: usize = 2000;
/// Rotate the log file at 2 MiB, keeping one previous generation (≈4 MiB worst case).
const FILE_MAX_BYTES: u64 = 2 * 1024 * 1024;
const FILE_NAME: &str = "ryotunesd.log";
const ROTATED_NAME: &str = "ryotunesd.log.1";
/// Bound one logged message so a pathological error (a giant upstream body) cannot poison the
/// ring or the file.
const MESSAGE_MAX_CHARS: usize = 2000;

#[derive(Clone, Debug)]
pub struct Entry {
    pub ts_ms: i64,
    pub level: &'static str,
    pub source: &'static str,
    pub target: String,
    pub message: String,
}

impl Entry {
    fn to_json(&self) -> Value {
        json!({
            "ts": self.ts_ms,
            "level": self.level,
            "source": self.source,
            "target": self.target,
            "message": self.message,
        })
    }

    fn to_line(&self) -> String {
        format!(
            "{} [{}] {} {}: {}\n",
            format_utc(self.ts_ms),
            self.level.to_uppercase(),
            self.source,
            self.target,
            self.message,
        )
    }
}

struct Shared {
    entries: VecDeque<Entry>,
    path: PathBuf,
    bytes_written: u64,
    errors: u64,
    warnings: u64,
}

/// The rolling record. Cheap to clone an `Arc` of; installed once at startup.
pub struct Diagnostics {
    shared: Mutex<Shared>,
}

static GLOBAL: OnceLock<std::sync::Arc<Diagnostics>> = OnceLock::new();

impl Diagnostics {
    /// Open (or create) the log file under `logs_dir`, install this as the global recorder,
    /// and seed the ring from the tail of the existing file. A daemon that died (or was
    /// upgraded) leaves its evidence behind: the first `get_logs` after a crash already shows
    /// what the previous process recorded, without the client having to read files.
    pub fn install(logs_dir: &Path) -> std::sync::Arc<Diagnostics> {
        let _ = std::fs::create_dir_all(logs_dir);
        let path = logs_dir.join(FILE_NAME);
        let bytes_written = std::fs::metadata(&path).map(|m| m.len()).unwrap_or(0);
        let mut entries = VecDeque::new();
        let mut errors = 0u64;
        let mut warnings = 0u64;
        for entry in read_tail(&path, RING_MAX) {
            match entry.level {
                "error" => errors += 1,
                "warn" => warnings += 1,
                _ => {}
            }
            entries.push_back(entry);
        }
        let di = std::sync::Arc::new(Diagnostics {
            shared: Mutex::new(Shared { entries, path, bytes_written, errors, warnings }),
        });
        let _ = GLOBAL.set(di.clone());
        di
    }

    pub fn record(&self, level: &'static str, source: &'static str, target: &str, message: &str) {
        let message = truncate(message, MESSAGE_MAX_CHARS);
        let entry =
            Entry { ts_ms: now_ms(), level, source, target: truncate(target, 120), message };
        let mut shared = self.shared.lock();
        match level {
            "error" => shared.errors += 1,
            "warn" => shared.warnings += 1,
            _ => {}
        }
        if shared.entries.len() >= RING_MAX {
            shared.entries.pop_front();
        }
        let line = entry.to_line();
        shared.append_to_file(&line);
        shared.entries.push_back(entry);
    }

    /// Filtered, newest-first snapshot for the client. `level`: "all" | "warn" | "error";
    /// `source`: "all" | "daemon" | "client"; `query`: case-insensitive substring.
    pub fn snapshot(&self, level: &str, source: &str, query: &str, limit: usize) -> Value {
        let shared = self.shared.lock();
        let needle = query.to_lowercase();
        let mut out = Vec::new();
        for entry in shared.entries.iter().rev() {
            if level == "warn" && entry.level == "info" {
                continue;
            }
            if level == "error" && entry.level != "error" {
                continue;
            }
            if source != "all" && entry.source != source {
                continue;
            }
            if !needle.is_empty()
                && !entry.message.to_lowercase().contains(&needle)
                && !entry.target.to_lowercase().contains(&needle)
            {
                continue;
            }
            out.push(entry.to_json());
            if out.len() >= limit {
                break;
            }
        }
        json!({
            "entries": out,
            "total": shared.entries.len(),
            "errors": shared.errors,
            "warnings": shared.warnings,
            "path": shared.path.to_string_lossy(),
            "fileBytes": shared.bytes_written,
        })
    }

    /// Everything currently in the ring, oldest-first, as log-file lines (what `open_logs_folder`
    /// complements: the file may be older than the ring after a rotation).
    pub fn path(&self) -> PathBuf {
        self.shared.lock().path.clone()
    }
}

impl Shared {
    fn append_to_file(&mut self, line: &str) {
        // Rotate first if this append would cross the cap: rename the current file to .1
        // (replacing any previous .1) and start fresh. A rename failure just keeps appending;
        // losing a rotation is never a reason to lose the line.
        if self.bytes_written >= FILE_MAX_BYTES {
            let rotated = self.path.with_file_name(ROTATED_NAME);
            if std::fs::rename(&self.path, &rotated).is_ok() {
                self.bytes_written = 0;
            }
        }
        match std::fs::OpenOptions::new().create(true).append(true).open(&self.path) {
            Ok(mut file) => {
                if file.write_all(line.as_bytes()).is_ok() {
                    let _ = file.flush();
                    self.bytes_written += line.len() as u64;
                }
            }
            // The disk being unwritable is exactly the kind of failure worth keeping in the
            // ring; drop the file half silently and carry on serving from memory.
            Err(_) => {}
        }
    }
}

/// A tracing layer that feeds warn+ events into the diagnostics ring and file. Installed
/// alongside the stderr formatter so `journalctl` keeps working unchanged.
pub struct CaptureLayer;

impl<S: Subscriber> Layer<S> for CaptureLayer {
    fn on_event(&self, event: &Event<'_>, _ctx: Context<'_, S>) {
        let meta = event.metadata();
        if meta.level() > &Level::WARN {
            return;
        }
        let mut visitor = MessageVisitor::default();
        event.record(&mut visitor);
        let level = if meta.level() == &Level::ERROR { "error" } else { "warn" };
        record(level, "daemon", meta.target(), &visitor.render());
    }
}

#[derive(Default)]
struct MessageVisitor {
    message: String,
    error: Option<String>,
}

impl MessageVisitor {
    fn render(self) -> String {
        match (self.message.is_empty(), self.error) {
            (true, Some(e)) => e,
            (true, None) => String::new(),
            (false, None) => self.message,
            (false, Some(e)) => format!("{} (error: {e})", self.message),
        }
    }
}

impl Visit for MessageVisitor {
    fn record_str(&mut self, field: &Field, value: &str) {
        if field.name() == "message" {
            self.message = value.to_owned();
        } else if field.name() == "error" {
            self.error = Some(value.to_owned());
        }
    }
    fn record_debug(&mut self, field: &Field, value: &dyn std::fmt::Debug) {
        if field.name() == "message" {
            self.message = format!("{value:?}");
        } else if field.name() == "error" {
            self.error = Some(format!("{value:?}"));
        }
    }
}

/// The global entry point: everything (tracing layer, server chokepoint, panic hook, client RPC)
/// records through here. Before `install` runs there is nowhere to record; those early failures
/// still reach stderr via the formatter.
pub fn record(level: &'static str, source: &'static str, target: &str, message: &str) {
    if let Some(di) = GLOBAL.get() {
        di.record(level, source, target, message);
    }
}

pub fn snapshot(level: &str, source: &str, query: &str, limit: usize) -> Value {
    GLOBAL.get().map(|di| di.snapshot(level, source, query, limit)).unwrap_or_else(|| {
        json!({
            "entries": [], "total": 0, "errors": 0, "warnings": 0,
            "path": "", "fileBytes": 0,
        })
    })
}

pub fn logs_path() -> Option<PathBuf> {
    GLOBAL.get().map(|di| di.path())
}

/// Reveal the log folder in the user's file manager. The Settings page offers this beside the
/// viewer so "send me the logs" is a click, not a path hunt.
pub fn open_logs_folder() -> Result<(), String> {
    let path = logs_path().ok_or("Logging is not initialised yet.")?;
    let dir = path.parent().ok_or("The log file has no folder.")?;
    let mut child = std::process::Command::new("xdg-open")
        .arg(dir)
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .spawn()
        .map_err(|error| format!("Could not open the logs folder: {error}"))?;
    // A file manager may stay open for hours; reap it on a blocking thread so a tokio worker
    // never parks inside `wait`.
    std::thread::spawn(move || {
        let _ = child.wait();
    });
    Ok(())
}

/// Route panics into the log before the default hook prints them. A daemon that dies on launch
/// (the worst support case) leaves the panic line in the file for the next session's UI to show.
pub fn install_panic_hook() {
    let previous = std::panic::take_hook();
    std::panic::set_hook(Box::new(move |info| {
        let location = info
            .location()
            .map(|l| format!("{}:{}", l.file(), l.line()))
            .unwrap_or_else(|| "unknown".into());
        let payload = info
            .payload()
            .downcast_ref::<&str>()
            .map(|s| s.to_string())
            .or_else(|| info.payload().downcast_ref::<String>().cloned())
            .unwrap_or_else(|| "unknown panic payload".into());
        record("error", "daemon", &format!("panic at {location}"), &payload);
        previous(info);
    }));
}

fn truncate(input: &str, max: usize) -> String {
    if input.chars().count() <= max {
        return input.to_owned();
    }
    let mut out: String = input.chars().take(max).collect();
    out.push('…');
    out
}

fn now_ms() -> i64 {
    SystemTime::now().duration_since(UNIX_EPOCH).unwrap_or_default().as_millis() as i64
}

/// `2026-09-24T19:12:03Z` from Unix millis (civil-from-days; no chrono for one formatter).
fn format_utc(ts_ms: i64) -> String {
    let secs = ts_ms.div_euclid(1000);
    let millis = ts_ms.rem_euclid(1000);
    let days = secs.div_euclid(86_400);
    let rem = secs.rem_euclid(86_400);
    let (y, m, d) = civil_from_days(days);
    format!(
        "{y:04}-{m:02}-{d:02}T{:02}:{:02}:{:02}.{millis:03}Z",
        rem / 3600,
        (rem % 3600) / 60,
        rem % 60,
    )
}

fn civil_from_days(days: i64) -> (i64, u32, u32) {
    let z = days + 719_468;
    let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
    let doe = z - era * 146_097;
    let yoe = (doe - doe / 1460 + doe / 36_524 - doe / 146_096) / 365;
    let y = yoe + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = (doy - (153 * mp + 2) / 5 + 1) as u32;
    let m = if mp < 10 { mp + 3 } else { mp - 9 } as u32;
    (if m <= 2 { y + 1 } else { y }, m, d)
}

/// The last `max` lines of the log file, in file order (oldest first). A missing or
/// unreadable file seeds nothing (the ring simply starts empty); a line that does not parse
/// is skipped, never fatal — the file is evidence, not a contract.
fn read_tail(path: &Path, max: usize) -> Vec<Entry> {
    let Ok(text) = std::fs::read_to_string(path) else { return Vec::new() };
    let mut out: Vec<Entry> = text.lines().rev().filter_map(parse_line).collect();
    out.reverse();
    if out.len() > max {
        out.drain(0..out.len() - max);
    }
    out
}

/// `2026-09-24T19:12:03.456Z [ERROR] daemon rpc:play: message…` back into an entry.
fn parse_line(line: &str) -> Option<Entry> {
    let (stamp, rest) = line.split_once(" [")?;
    let ts_ms = parse_utc(stamp)?;
    let (level, rest) = rest.split_once("] ")?;
    let level = match level.to_ascii_lowercase().as_str() {
        "error" => "error",
        "warn" => "warn",
        "info" => "info",
        _ => return None,
    };
    let (origin, message) = rest.split_once(": ")?;
    let (source, target) = origin.split_once(' ')?;
    let source = match source {
        "daemon" => "daemon",
        "client" => "client",
        _ => return None,
    };
    Some(Entry { ts_ms, level, source, target: target.to_owned(), message: message.to_owned() })
}

/// The inverse of [`format_utc`] (same civil algorithm, reversed).
fn parse_utc(stamp: &str) -> Option<i64> {
    // YYYY-MM-DDThh:mm:ss.mmmZ
    if stamp.len() != 24 {
        return None;
    }
    let b = stamp.as_bytes();
    if b[4] != b'-'
        || b[7] != b'-'
        || b[10] != b'T'
        || b[13] != b':'
        || b[16] != b':'
        || b[19] != b'.'
        || b[23] != b'Z'
    {
        return None;
    }
    let num = |s: &str| s.parse::<i64>().ok();
    let (y, mo, d) = (num(&stamp[0..4])?, num(&stamp[5..7])?, num(&stamp[8..10])?);
    let h = num(&stamp[11..13])?;
    let mi = num(&stamp[14..16])?;
    let s = num(&stamp[17..19])?;
    let ms = num(&stamp[20..23])?;
    let days = days_from_civil(y, mo as u32, d as u32);
    Some(((days * 86_400 + h * 3600 + mi * 60 + s) * 1000) + ms)
}

fn days_from_civil(y: i64, m: u32, d: u32) -> i64 {
    let y = if m <= 2 { y - 1 } else { y };
    let era = if y >= 0 { y } else { y - 399 } / 400;
    let yoe = y - era * 400;
    let mp = if m > 2 { m - 3 } else { m + 9 } as i64;
    let doy = (153 * mp + 2) / 5 + d as i64 - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    era * 146_097 + doe - 719_468
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Dir(PathBuf);
    impl Drop for Dir {
        fn drop(&mut self) {
            let _ = std::fs::remove_dir_all(&self.0);
        }
    }
    fn temp_dir(tag: &str) -> Dir {
        static SEQ: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0);
        let path = std::env::temp_dir().join(format!(
            "ryotunes-diag-{tag}-{}-{}",
            std::process::id(),
            SEQ.fetch_add(1, std::sync::atomic::Ordering::Relaxed),
        ));
        std::fs::create_dir_all(&path).unwrap();
        Dir(path)
    }

    #[test]
    fn records_filter_and_count_by_level_and_source() {
        let dir = temp_dir("filter");
        let di = Diagnostics::install(&dir.0);
        di.record("error", "daemon", "resolve", "bot-gated stream");
        di.record("warn", "daemon", "spotify", "session dead");
        di.record("info", "daemon", "boot", "started");
        di.record("error", "client", "rpc:play", "daemon not connected");
        let all = di.snapshot("all", "all", "", 100);
        assert_eq!(all["total"], 4);
        assert_eq!(all["errors"], 2);
        assert_eq!(all["warnings"], 1);
        // newest first
        assert_eq!(all["entries"][0]["source"], "client");

        let errors = di.snapshot("error", "all", "", 100);
        assert_eq!(errors["entries"].as_array().unwrap().len(), 2);
        let daemon = di.snapshot("all", "daemon", "", 100);
        assert_eq!(daemon["entries"].as_array().unwrap().len(), 3);
        let search = di.snapshot("all", "all", "bot", 100);
        assert_eq!(search["entries"].as_array().unwrap().len(), 1);
        assert_eq!(search["entries"][0]["level"], "error");
    }

    #[test]
    fn log_lines_round_trip_through_the_tail_reader() {
        let dir = temp_dir("tail");
        {
            let di = Diagnostics::install(&dir.0);
            di.record("error", "daemon", "rpc:play", "bot-gated stream");
            di.record("warn", "client", "daemon", "connection lost");
        }
        // A fresh install over the same file re-seeds the ring (the crash-restart case).
        let di = Diagnostics::install(&dir.0);
        let snap = di.snapshot("all", "all", "", 100);
        assert_eq!(snap["total"], 2, "both previous entries survived the restart");
        assert_eq!(snap["errors"], 1);
        assert_eq!(snap["warnings"], 1);
        // snapshot is newest-first: the warn came second.
        assert_eq!(snap["entries"][0]["target"], "daemon");
        assert_eq!(snap["entries"][0]["message"], "connection lost");
        assert_eq!(snap["entries"][1]["message"], "bot-gated stream");
        // Unparsable lines are skipped rather than poisoning the seed.
        std::fs::write(
            dir.0.join(FILE_NAME),
            b"garbage line\n2026-09-24T19:12:03.456Z [ERROR] daemon x: hello\n",
        )
        .unwrap();
        let di = Diagnostics::install(&dir.0);
        let snap = di.snapshot("all", "all", "", 100);
        assert_eq!(snap["total"], 1, "unparsable lines are skipped");
        assert_eq!(snap["entries"][0]["ts"], 1_790_277_123_456i64, "timestamp round-trips");
    }

    #[test]
    fn entries_land_in_the_log_file_and_rotate_once() {
        let dir = temp_dir("file");
        let di = Diagnostics::install(&dir.0);
        for i in 0..20 {
            di.record("error", "daemon", "test", &format!("failure number {i}"));
        }
        let log = dir.0.join(FILE_NAME);
        let text = std::fs::read_to_string(&log).unwrap();
        assert_eq!(text.lines().count(), 20);
        // Each line leads with the UTC timestamp: `YYYY-MM-DDThh:mm:ss.mmmZ [LEVEL] …`.
        assert!(
            text.lines().all(|l| l.as_bytes()[4] == b'-' && l.contains("Z [ERROR]")),
            "timestamps are UTC ISO-like"
        );

        // Force a rotation by claiming the file is at the cap.
        {
            let mut shared = di.shared.lock();
            shared.bytes_written = FILE_MAX_BYTES;
        }
        di.record("error", "daemon", "test", "after rotation");
        assert!(dir.0.join(ROTATED_NAME).exists());
        let fresh = std::fs::read_to_string(&log).unwrap();
        assert_eq!(fresh.lines().count(), 1);
        assert!(fresh.contains("after rotation"));
    }

    #[test]
    fn ring_drops_oldest_at_capacity() {
        let dir = temp_dir("ring");
        let di = Diagnostics::install(&dir.0);
        for i in 0..(RING_MAX + 50) {
            di.record("warn", "daemon", "test", &format!("line {i}"));
        }
        let snap = di.snapshot("all", "all", "", RING_MAX + 10);
        assert_eq!(snap["total"], RING_MAX);
        assert_eq!(snap["entries"][0]["message"], format!("line {}", RING_MAX + 49));
    }

    #[test]
    fn utc_formatting_matches_known_instants() {
        // 2000-01-01T00:00:00Z = 946_684_800 s; the others check hour/day rollover.
        assert_eq!(format_utc(946_684_800_000), "2000-01-01T00:00:00.000Z");
        assert_eq!(format_utc(1_709_635_200_123), "2024-03-05T10:40:00.123Z");
        assert_eq!(format_utc(1_709_268_800_000), "2024-03-01T04:53:20.000Z");
    }
}
