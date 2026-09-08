//! Bounded, daemon-owned downloads. Slots live until child processes are reaped, settings are
//! frozen when a job is queued, and files are atomically published without replacing existing files.
use std::collections::HashMap;
use std::io::{Read, Write};
use std::path::{Path, PathBuf};
use std::process::Stdio;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use parking_lot::Mutex;
use ryotunes_core::db::now_secs;
use ryotunes_core::host::EventSink;
use ryotunes_core::spotify::{sc_track_id, spotify_track_id};
use ryotunes_core::state::AppState;
use ryotunes_core::{local, radio};
use ryotunes_soundcloud::SoundCloud;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tokio::io::{AsyncRead, AsyncReadExt};
use tokio::process::Command;
use tokio::sync::Notify;

use crate::lifecycle::Lifecycle;

const STATE_FILE: &str = "downloads.json";
const MAX_ACTIVE_QUEUED: usize = 200;
const MAX_HISTORY: usize = 200;
const EVENT_THROTTLE: Duration = Duration::from_millis(400);
const STDERR_TAIL_BYTES: usize = 8192;
const MAX_ERROR_LEN: usize = 600;
const MAX_COMPONENT_BYTES: usize = 120;
const STAGING_DIR: &str = ".ryotunes-incomplete";
const INTERRUPTED_MSG: &str =
    "Download interrupted when Ryotunes stopped. Retry to download it again.";
const JOB_TIMEOUT: Duration = Duration::from_secs(30 * 60);

/// Container/codec choice. `Original` keeps the source stream (no re-encode); `Mp3`/`Opus`
/// transcode with ffmpeg.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum DownloadFormat {
    Original,
    Mp3,
    Opus,
}

/// Where a job is in its lifecycle. `queued`/`downloading` are active (cap-counted, lifecycle-busy);
/// the rest are terminal. `failed` and `cancelled` are retryable.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum DownloadStatus {
    Queued,
    Downloading,
    Completed,
    Failed,
    Cancelled,
}

impl DownloadStatus {
    fn is_active(self) -> bool {
        matches!(self, DownloadStatus::Queued | DownloadStatus::Downloading)
    }
}

/// The user-editable download settings. camelCase over the wire.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DownloadSettings {
    pub path: String,
    pub format: DownloadFormat,
    pub workers: u32,
    pub organize_by_artist: bool,
    pub embed_metadata: bool,
}

impl DownloadSettings {
    /// First-run defaults: the XDG Music dir's `Ryotunes/`, original format, one worker, organized
    /// by artist with metadata embedded. Does not touch the filesystem — the folder is created on
    /// the first `set_download_settings` or the first download.
    fn defaults() -> Self {
        DownloadSettings {
            path: default_music_dir().to_string_lossy().into_owned(),
            format: DownloadFormat::Original,
            workers: 1,
            organize_by_artist: true,
            embed_metadata: true,
        }
    }
}

/// One download, exactly as the client renders it. camelCase; timestamps are Unix seconds.
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DownloadJob {
    pub id: String,
    pub video_id: String,
    pub title: String,
    pub artists: String,
    pub thumbnail: String,
    pub status: DownloadStatus,
    pub progress: u32,
    pub file_path: Option<String>,
    pub error: Option<String>,
    pub source: String,
    pub created_at: i64,
    pub finished_at: Option<i64>,
    #[serde(default = "DownloadSettings::defaults")]
    pub settings: DownloadSettings,
    /// Non-fatal metadata notes from the enrichment step (missing cover/lyrics, a tag that could
    /// not be written). Shown in history; their presence never demotes a completed download.
    #[serde(default)]
    pub warnings: Vec<String>,
}

// --- source classification ---------------------------------------------------------------------

/// Which catalogue a `videoId` came from, and therefore how the worker resolves it.
#[derive(Clone, Copy)]
enum Source {
    YouTube,
    SoundCloud,
    Spotify,
}

impl Source {
    /// The display label the client shows; the Spotify one is explicit that it is a search match,
    /// never a claim of the exact Spotify master.
    fn label(self) -> &'static str {
        match self {
            Source::YouTube => "YouTube",
            Source::SoundCloud => "SoundCloud",
            Source::Spotify => "YouTube match for Spotify",
        }
    }
}

/// Classify a `videoId`, rejecting the unsupported kinds with a message fit for a tooltip/toast.
fn classify(video_id: &str) -> Result<Source, String> {
    if local::is_local_song(video_id) {
        return Err("This track is already a local file on your computer.".into());
    }
    if radio::is_radio_id(video_id) {
        return Err("Radio stations are live streams and can't be downloaded.".into());
    }
    if sc_track_id(video_id).is_some() {
        return Ok(Source::SoundCloud);
    }
    if spotify_track_id(video_id).is_some() {
        return Ok(Source::Spotify);
    }
    if is_youtube_id(video_id) {
        return Ok(Source::YouTube);
    }
    Err("This track can't be downloaded.".into())
}

/// A strict YouTube video id: exactly 11 of `[A-Za-z0-9_-]`. Anything else is not a bare id and is
/// refused rather than handed to yt-dlp as an ambiguous URL/search.
fn is_youtube_id(id: &str) -> bool {
    id.len() == 11 && id.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
}

struct Running {
    cancel: Arc<Notify>,
}

struct State {
    settings: DownloadSettings,
    jobs: Vec<DownloadJob>,
    running: HashMap<String, Running>,
    stopping: bool,
}

struct Inner {
    sink: Arc<dyn EventSink>,
    soundcloud: Arc<SoundCloud>,
    lifecycle: Arc<Lifecycle>,
    /// Shared app state, so a finished download can reuse the core lyrics provider/cache and HTTP
    /// client to fetch artwork and lyrics. `None` in unit tests, which never run enrichment.
    app: Option<Arc<AppState>>,
    data_dir: PathBuf,
    state: Mutex<State>,
    last_emit: Mutex<Instant>,
    finished: Notify,
    id_seq: AtomicU64,
}

pub struct Downloads {
    inner: Arc<Inner>,
}

impl Downloads {
    pub fn new(
        sink: Arc<dyn EventSink>,
        soundcloud: Arc<SoundCloud>,
        data_dir: PathBuf,
        lifecycle: Arc<Lifecycle>,
        app: Option<Arc<AppState>>,
    ) -> anyhow::Result<Arc<Self>> {
        let (settings, mut jobs) = load(&data_dir)?;
        recover_interrupted(&mut jobs);
        trim_history(&mut jobs);
        let inner = Arc::new(Inner {
            sink,
            soundcloud,
            lifecycle,
            app,
            data_dir,
            state: Mutex::new(State { settings, jobs, running: HashMap::new(), stopping: false }),
            last_emit: Mutex::new(Instant::now() - EVENT_THROTTLE),
            finished: Notify::new(),
            id_seq: AtomicU64::new(0),
        });
        inner.persist_locked(&inner.state.lock())?;
        Ok(Arc::new(Self { inner }))
    }

    pub fn settings(&self) -> DownloadSettings {
        self.inner.state.lock().settings.clone()
    }
    pub fn snapshot(&self) -> Value {
        self.inner.snapshot()
    }

    pub fn set_settings(&self, incoming: DownloadSettings) -> Result<DownloadSettings, String> {
        let settings = sanitize_settings(incoming)?;
        {
            let mut state = self.inner.state.lock();
            ensure_running(&state)?;
            let previous = std::mem::replace(&mut state.settings, settings.clone());
            if let Err(error) = self.inner.persist_locked(&state) {
                state.settings = previous;
                return Err(format!("Could not save download preferences: {error}"));
            }
        }
        self.inner.pump();
        Ok(settings)
    }

    pub fn enqueue(
        &self,
        video_id: String,
        title: String,
        artists: String,
        thumbnail: String,
    ) -> Result<DownloadJob, String> {
        let source = classify(&video_id)?;
        if video_id.len() > 80 || title.len() > 512 || artists.len() > 512 || thumbnail.len() > 4096
        {
            return Err("The track metadata is too large to download safely.".into());
        }
        if title.trim().is_empty() {
            return Err("This track has no title to use for its download.".into());
        }
        let job = {
            let mut state = self.inner.state.lock();
            ensure_running(&state)?;
            if let Some(job) = state.jobs.iter().rev().find(|job| {
                job.video_id == video_id
                    && (job.status.is_active()
                        || (job.status == DownloadStatus::Completed && file_present(job)))
            }) {
                return Ok(job.clone());
            }
            ensure_capacity(&state)?;
            let job = DownloadJob {
                id: self.inner.next_id(),
                video_id,
                title,
                artists,
                thumbnail,
                status: DownloadStatus::Queued,
                progress: 0,
                file_path: None,
                error: None,
                source: source.label().into(),
                created_at: now_secs(),
                finished_at: None,
                settings: state.settings.clone(),
                warnings: Vec::new(),
            };
            state.jobs.push(job.clone());
            if let Err(error) = self.inner.persist_locked(&state) {
                state.jobs.pop();
                return Err(format!("Could not save the download queue: {error}"));
            }
            job
        };
        self.inner.pump();
        Ok(job)
    }

    pub fn cancel(&self, id: &str) -> Result<(), String> {
        let result = {
            let mut state = self.inner.state.lock();
            let job = state
                .jobs
                .iter_mut()
                .find(|job| job.id == id)
                .ok_or("That download is no longer in the list.")?;
            if !job.status.is_active() {
                return Ok(());
            }
            job.status = DownloadStatus::Cancelled;
            job.finished_at = Some(now_secs());
            // Do NOT release this slot: the worker owns it until its process group has exited.
            if let Some(running) = state.running.get(id) {
                running.cancel.notify_one();
            }
            trim_history(&mut state.jobs);
            self.inner.persist_locked(&state).map_err(|error| {
                format!("Download cancelled, but history could not be saved: {error}")
            })
        };
        self.inner.pump();
        result
    }

    pub fn retry(&self, id: &str) -> Result<DownloadJob, String> {
        let job = {
            let mut state = self.inner.state.lock();
            ensure_running(&state)?;
            ensure_capacity(&state)?;
            let index = state
                .jobs
                .iter()
                .position(|job| job.id == id)
                .ok_or("That download is no longer in the list.")?;
            let old = state.jobs[index].clone();
            if !matches!(old.status, DownloadStatus::Failed | DownloadStatus::Cancelled) {
                return Err("Only failed or cancelled downloads can be retried.".into());
            }
            if let Some(other) = state.jobs.iter().find(|job| {
                job.id != id
                    && job.video_id == old.video_id
                    && (job.status.is_active()
                        || (job.status == DownloadStatus::Completed && file_present(job)))
            }) {
                return Ok(other.clone());
            }
            let mut job = old.clone();
            // A new identity isolates this attempt from a cancelled worker still being reaped.
            job.id = self.inner.next_id();
            job.status = DownloadStatus::Queued;
            job.progress = 0;
            job.error = None;
            job.file_path = None;
            job.created_at = now_secs();
            job.finished_at = None;
            job.settings = state.settings.clone();
            job.warnings = Vec::new();
            state.jobs.remove(index);
            state.jobs.push(job.clone());
            if let Err(error) = self.inner.persist_locked(&state) {
                state.jobs.pop();
                state.jobs.insert(index, old);
                return Err(format!("Could not save the download queue: {error}"));
            }
            job
        };
        self.inner.pump();
        Ok(job)
    }

    pub fn clear_history(&self) -> Result<(), String> {
        {
            let mut state = self.inner.state.lock();
            let previous = state.jobs.clone();
            state.jobs.retain(|job| job.status.is_active());
            if let Err(error) = self.inner.persist_locked(&state) {
                state.jobs = previous;
                return Err(format!("Could not clear download history: {error}"));
            }
        }
        self.inner.emit(true);
        Ok(())
    }

    pub fn open(&self, id: &str) -> Result<(), String> {
        let path = self
            .inner
            .state
            .lock()
            .jobs
            .iter()
            .find(|job| job.id == id)
            .and_then(|job| job.file_path.clone())
            .ok_or("That download has not finished yet.")?;
        let path = Path::new(&path);
        if !path.is_file() {
            return Err("The downloaded file has moved or been deleted.".into());
        }
        open_path(path.parent().ok_or("The download has no parent folder.")?)
    }

    pub fn open_folder(&self) -> Result<(), String> {
        let path = self.inner.state.lock().settings.path.clone();
        std::fs::create_dir_all(&path).map_err(|error| error.to_string())?;
        open_path(Path::new(&path))
    }

    pub async fn shutdown(&self) {
        {
            let mut state = self.inner.state.lock();
            state.stopping = true;
            recover_interrupted(&mut state.jobs);
            trim_history(&mut state.jobs);
            for running in state.running.values() {
                running.cancel.notify_one();
            }
            if let Err(error) = self.inner.persist_locked(&state) {
                tracing::error!(%error, "could not save interrupted download history");
            }
        }
        self.inner.emit(true);
        loop {
            let finished = self.inner.finished.notified();
            tokio::pin!(finished);
            finished.as_mut().enable();
            if self.inner.state.lock().running.is_empty() {
                break;
            }
            finished.await;
        }
    }
}

fn ensure_running(state: &State) -> Result<(), String> {
    if state.stopping {
        Err("Ryotunes is shutting down.".into())
    } else {
        Ok(())
    }
}

fn ensure_capacity(state: &State) -> Result<(), String> {
    let queued = state.jobs.iter().filter(|job| job.status == DownloadStatus::Queued).count();
    if queued + state.running.len() >= MAX_ACTIVE_QUEUED {
        Err(format!(
            "The download queue is full ({MAX_ACTIVE_QUEUED} tracks). Let some finish first."
        ))
    } else {
        Ok(())
    }
}

impl Inner {
    fn pump(self: &Arc<Self>) {
        {
            let mut state = self.state.lock();
            while !state.stopping
                && state.running.len() < state.settings.workers.clamp(1, 4) as usize
            {
                let Some(index) =
                    state.jobs.iter().position(|job| job.status == DownloadStatus::Queued)
                else {
                    break;
                };
                state.jobs[index].status = DownloadStatus::Downloading;
                let job = state.jobs[index].clone();
                let cancel = Arc::new(Notify::new());
                state.running.insert(job.id.clone(), Running { cancel: cancel.clone() });
                let inner = self.clone();
                tokio::spawn(async move {
                    let _slot = WorkerSlot { inner: inner.clone(), id: job.id.clone() };
                    inner.run_job(job, cancel).await;
                });
            }
            // Lifecycle reconciliation stays under the same lock as queue mutations. A stale
            // "idle" notification cannot race a newer enqueue and stop the daemon prematurely.
            self.lifecycle.downloads_busy_changed(
                !state.running.is_empty() || state.jobs.iter().any(|job| job.status.is_active()),
            );
        }
        self.emit(true);
    }

    async fn run_job(self: &Arc<Self>, job: DownloadJob, cancel: Arc<Notify>) {
        let result = tokio::select! {
            biased;
            _ = cancel.notified() => return,
            result = tokio::time::timeout(JOB_TIMEOUT, self.resolve_target(&job)) => {
                match result {
                    Ok(result) => result,
                    Err(_) => Err("The download source took too long to respond.".into()),
                }
            }
        };
        let target = match result {
            Ok(target) => target,
            Err(error) => {
                self.finish(&job.id, Err(error));
                return;
            }
        };
        if !self.is_current(&job.id) {
            return;
        }
        let staging = match create_staging(&job.settings.path, &job.id) {
            Ok(path) => path,
            Err(error) => {
                self.finish(&job.id, Err(error));
                return;
            }
        };
        let result = self.run_downloader(&job, &target, &staging, &cancel).await;
        // After the downloader succeeds, embed artwork/lyrics and write local companion files into
        // staging. The reserved slot stays held across this bounded step; a job cancelled meanwhile
        // is caught by the publication gate below and never reaches the user's folder.
        let mut warnings: Vec<String> = Vec::new();
        let mut sidecars: Vec<PathBuf> = Vec::new();
        if result.is_ok() && job.settings.embed_metadata {
            if let (Some(app), true) = (self.app.as_deref(), self.is_current(&job.id)) {
                if let Ok(audio) = find_output_file(&staging) {
                    let enrichment = crate::download_media::enrich(app, &audio, &job).await;
                    warnings = enrichment.warnings;
                    sidecars = enrichment.sidecars;
                }
            }
        }
        // Publication and cancellation are serialized; a cancelled attempt never publishes a file.
        {
            let mut state = self.state.lock();
            if let Some(current) = state
                .jobs
                .iter_mut()
                .find(|entry| entry.id == job.id && entry.status == DownloadStatus::Downloading)
            {
                let result =
                    result.and_then(|()| publish(&staging, &job, &sidecars, &mut warnings));
                apply_result(current, result);
                if current.status == DownloadStatus::Completed {
                    current.warnings = warnings;
                }
                trim_history(&mut state.jobs);
                self.save_transition(&mut state, &job.id);
            }
        }
        let _ = std::fs::remove_dir_all(staging);
        self.emit(true);
    }

    async fn resolve_target(&self, job: &DownloadJob) -> Result<String, String> {
        match classify(&job.video_id)? {
            Source::YouTube => Ok(format!("https://www.youtube.com/watch?v={}", job.video_id)),
            Source::Spotify => {
                Ok(format!("ytsearch1:{} {} official audio", job.artists.trim(), job.title.trim()))
            }
            Source::SoundCloud => {
                let id = sc_track_id(&job.video_id).ok_or("Invalid SoundCloud track id.")?;
                let track =
                    self.soundcloud.track(id).await.map_err(|error| {
                        format!("Could not look up the SoundCloud track: {error}")
                    })?;
                let url = url::Url::parse(&track.permalink_url)
                    .map_err(|_| "SoundCloud returned an invalid track link.")?;
                if url.scheme() != "https"
                    || !matches!(url.host_str(), Some("soundcloud.com" | "www.soundcloud.com"))
                {
                    return Err("SoundCloud returned an unsupported track link.".into());
                }
                Ok(url.into())
            }
        }
    }

    async fn run_downloader(
        self: &Arc<Self>,
        job: &DownloadJob,
        target: &str,
        staging: &Path,
        cancel: &Notify,
    ) -> Result<(), String> {
        // Holding the lock across spawn closes the cancel-before-register race. No await occurs
        // under this lock, and the slot was reserved before we got here.
        let mut child = {
            let state = self.state.lock();
            if state.stopping
                || !state
                    .jobs
                    .iter()
                    .any(|entry| entry.id == job.id && entry.status == DownloadStatus::Downloading)
            {
                return Err("Download cancelled.".into());
            }
            build_command(target, staging, &job.settings).spawn().map_err(|error| {
                if error.kind() == std::io::ErrorKind::NotFound {
                    "Install yt-dlp and FFmpeg to download music.".into()
                } else {
                    format!("Could not start yt-dlp: {error}")
                }
            })?
        };
        let mut group = ProcessGroup(child.id().map(|pid| pid as i32));
        let stdout = child.stdout.take().expect("piped stdout");
        let stderr = child.stderr.take().expect("piped stderr");
        let streams = async {
            tokio::join!(
                read_stream(self, &job.id, stdout, false),
                read_stream(self, &job.id, stderr, true)
            )
            .1
        };
        tokio::pin!(streams);
        let result = tokio::select! {
            biased;
            _ = cancel.notified() => Err("Download cancelled.".to_string()),
            _ = tokio::time::sleep(JOB_TIMEOUT) => Err("The download exceeded 30 minutes. Retry when the connection improves.".into()),
            result = async { tokio::join!(child.wait(), &mut streams) } => Ok(result),
        };
        match result {
            Ok((status, tail)) => {
                group.0 = None;
                match status {
                    Ok(status) if status.success() => Ok(()),
                    Ok(status) => Err(error_message(&tail, status.code())),
                    Err(error) => Err(format!("Could not wait for yt-dlp: {error}")),
                }
            }
            Err(error) => {
                group.kill();
                let _ = child.wait().await;
                let _ = tokio::time::timeout(Duration::from_secs(2), &mut streams).await;
                Err(error)
            }
        }
    }

    fn is_current(&self, id: &str) -> bool {
        let state = self.state.lock();
        !state.stopping
            && state
                .jobs
                .iter()
                .any(|job| job.id == id && job.status == DownloadStatus::Downloading)
    }

    fn finish(&self, id: &str, result: Result<String, String>) {
        {
            let mut state = self.state.lock();
            if let Some(job) = state
                .jobs
                .iter_mut()
                .find(|job| job.id == id && job.status == DownloadStatus::Downloading)
            {
                apply_result(job, result);
                trim_history(&mut state.jobs);
                self.save_transition(&mut state, id);
            }
        }
        self.emit(true);
    }

    fn save_transition(&self, state: &mut State, id: &str) {
        if let Err(error) = self.persist_locked(state) {
            tracing::error!(%error, "could not save download history");
            if let Some(job) = state.jobs.iter_mut().find(|job| job.id == id) {
                job.status = DownloadStatus::Failed;
                job.error = Some(format!(
                    "{}History could not be saved: {error}",
                    if job.file_path.is_some() { "Audio saved to your folder. " } else { "" }
                ));
            }
        }
    }

    fn progress(&self, id: &str, progress: u32) {
        let changed = {
            let mut state = self.state.lock();
            if let Some(job) = state
                .jobs
                .iter_mut()
                .find(|job| job.id == id && job.status == DownloadStatus::Downloading)
            {
                let next = progress.min(99);
                let changed = job.progress != next;
                job.progress = next;
                changed
            } else {
                false
            }
        };
        if changed {
            self.emit(false);
        }
    }

    fn snapshot(&self) -> Value {
        let state = self.state.lock();
        json!({
            "jobs": state.jobs.iter().rev().collect::<Vec<_>>(),
            "activeCount": state.running.len(),
            "queuedCount": state.jobs.iter().filter(|job| job.status == DownloadStatus::Queued).count(),
        })
    }

    fn emit(&self, force: bool) {
        let mut last = self.last_emit.lock();
        if !force && last.elapsed() < EVENT_THROTTLE {
            return;
        }
        *last = Instant::now();
        self.sink.emit("downloads-changed", self.snapshot());
    }

    // Every caller holds state, so writes cannot race or persist an older snapshot over a newer
    // one. The final rename is atomic; fsync both file and directory before reporting a saved edit.
    fn persist_locked(&self, state: &State) -> std::io::Result<()> {
        std::fs::create_dir_all(&self.data_dir)?;
        let temporary = self.data_dir.join("downloads.json.tmp");
        let result = (|| {
            use std::os::unix::fs::OpenOptionsExt;
            let mut file = std::fs::OpenOptions::new()
                .write(true)
                .create_new(true)
                .mode(0o600)
                .open(&temporary)?;
            serde_json::to_writer(
                &mut file,
                &PersistedRef { settings: &state.settings, jobs: &state.jobs },
            )?;
            file.flush()?;
            file.sync_all()?;
            std::fs::rename(&temporary, self.data_dir.join(STATE_FILE))?;
            std::fs::File::open(&self.data_dir)?.sync_all()
        })();
        let _ = std::fs::remove_file(&temporary);
        result
    }

    fn next_id(&self) -> String {
        let nanos = SystemTime::now().duration_since(UNIX_EPOCH).unwrap_or_default().as_nanos();
        format!("dl-{nanos}-{}", self.id_seq.fetch_add(1, Ordering::Relaxed))
    }
}

// Owns a reserved slot through every return/unwind. Cancellation never releases the slot early.
struct WorkerSlot {
    inner: Arc<Inner>,
    id: String,
}
impl Drop for WorkerSlot {
    fn drop(&mut self) {
        {
            let mut state = self.inner.state.lock();
            state.running.remove(&self.id);
            if let Some(job) = state
                .jobs
                .iter_mut()
                .find(|job| job.id == self.id && job.status == DownloadStatus::Downloading)
            {
                apply_result(
                    job,
                    Err("The download worker stopped unexpectedly. Retry this download.".into()),
                );
                self.inner.save_transition(&mut state, &self.id);
            }
        }
        self.inner.pump();
        self.inner.finished.notify_waiters();
    }
}

struct ProcessGroup(Option<i32>);
impl ProcessGroup {
    fn kill(&mut self) {
        if let Some(pid) = self.0.take() {
            // Safe: this process group was created for our own live child, never a supplied pid.
            unsafe {
                libc::kill(-pid, libc::SIGKILL);
            }
        }
    }
}
impl Drop for ProcessGroup {
    fn drop(&mut self) {
        self.kill();
    }
}

fn apply_result(job: &mut DownloadJob, result: Result<String, String>) {
    job.finished_at = Some(now_secs());
    match result {
        Ok(path) => {
            job.status = DownloadStatus::Completed;
            job.file_path = Some(path);
            job.progress = 100;
        }
        Err(error) => {
            job.status = DownloadStatus::Failed;
            job.error = Some(truncate_chars(&error, MAX_ERROR_LEN));
        }
    }
}

fn build_command(target: &str, staging: &Path, settings: &DownloadSettings) -> Command {
    let template = format!("{}/audio.%(ext)s", staging.to_string_lossy().replace('%', "%%"));
    let mut command = Command::new("yt-dlp");
    command
        .args([
            "--ignore-config",
            "--no-plugin-dirs",
            "--no-playlist",
            "--no-exec",
            "--no-continue",
            "--newline",
            "--no-color",
            "--progress",
            "--progress-delta",
            "0.4",
            "--socket-timeout",
            "20",
            "--retries",
            "2",
            "--fragment-retries",
            "2",
            "--concurrent-fragments",
            "1",
            "--postprocessor-args",
            "ffmpeg:-threads 1",
            "-f",
            "bestaudio/best",
            "-x",
        ])
        .arg("-o")
        .arg(template);
    match settings.format {
        DownloadFormat::Original => {
            command.args(["--audio-format", "best"]);
        }
        DownloadFormat::Mp3 => {
            command.args(["--audio-format", "mp3", "--audio-quality", "320K"]);
        }
        DownloadFormat::Opus => {
            command.args(["--audio-format", "opus", "--audio-quality", "160K"]);
        }
    }
    if settings.embed_metadata {
        command.arg("--embed-metadata");
    }
    command
        .arg("--")
        .arg(target)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    command.process_group(0).kill_on_drop(true);
    // Only async-signal-safe operations execute between fork and exec.
    unsafe {
        command.pre_exec(|| {
            libc::nice(10);
            Ok(())
        });
    }
    command
}

// Read fixed-size chunks instead of lines(): an extractor must not allocate an unbounded line.
async fn read_stream<R: AsyncRead + Unpin>(
    inner: &Inner,
    id: &str,
    mut reader: R,
    collect: bool,
) -> String {
    let mut buffer = [0u8; 4096];
    let mut line = Vec::with_capacity(4096);
    let mut tail = Vec::with_capacity(STDERR_TAIL_BYTES);
    while let Ok(count) = reader.read(&mut buffer).await {
        if count == 0 {
            break;
        }
        if collect {
            if tail.len() + count > STDERR_TAIL_BYTES {
                tail.drain(..tail.len() + count - STDERR_TAIL_BYTES);
            }
            tail.extend_from_slice(&buffer[..count]);
        }
        for byte in &buffer[..count] {
            if *byte == b'\n' || *byte == b'\r' {
                if let Some(progress) = parse_progress(&String::from_utf8_lossy(&line)) {
                    inner.progress(id, progress);
                }
                line.clear();
            } else if line.len() < 4096 {
                line.push(*byte);
            }
        }
    }
    String::from_utf8_lossy(&tail).into_owned()
}

fn create_staging(folder: &str, id: &str) -> Result<PathBuf, String> {
    use std::os::unix::fs::DirBuilderExt;
    let root = Path::new(folder).join(STAGING_DIR);
    std::fs::create_dir_all(folder).map_err(|error| error.to_string())?;
    match std::fs::DirBuilder::new().mode(0o700).create(&root) {
        Ok(()) => {}
        Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => {
            if !std::fs::symlink_metadata(&root).map_err(|error| error.to_string())?.is_dir() {
                return Err("The temporary download folder is not a regular directory.".into());
            }
        }
        Err(error) => {
            return Err(format!("Could not create the temporary download folder: {error}"))
        }
    }
    let path = root.join(sanitize_component(id));
    std::fs::DirBuilder::new()
        .mode(0o700)
        .create(&path)
        .map_err(|error| format!("Could not prepare this download: {error}"))?;
    Ok(path)
}

fn publish(
    staging: &Path,
    job: &DownloadJob,
    sidecars: &[PathBuf],
    warnings: &mut Vec<String>,
) -> Result<String, String> {
    let produced = find_output_file(staging)?;
    let extension = produced
        .extension()
        .and_then(|value| value.to_str())
        .ok_or("The download has no audio extension.")?;
    let path =
        compute_final_path(&job.settings, &job.title, &job.artists, &job.video_id, extension);
    let parent = path.parent().ok_or("The download has no destination folder.")?;
    std::fs::create_dir_all(parent).map_err(|error| error.to_string())?;
    let root = Path::new(&job.settings.path).canonicalize().map_err(|error| error.to_string())?;
    if !parent.canonicalize().map_err(|error| error.to_string())?.starts_with(&root) {
        return Err("The artist folder points outside your download folder.".into());
    }
    std::fs::File::open(&produced)
        .and_then(|file| file.sync_all())
        .map_err(|error| error.to_string())?;
    let stem =
        path.file_stem().and_then(|value| value.to_str()).ok_or("Invalid download filename.")?;
    // Companion artwork/lyrics validated down to regular, non-empty files inside this staging
    // directory with a supported extension; anything else is dropped rather than published.
    let companions = collect_sidecars(staging, sidecars);
    // Flush each validated companion to disk before publishing it, matching the audio's durability
    // above. Best-effort: a flush failure must not block saving the file.
    for (_, source) in &companions {
        let _ = std::fs::File::open(source).and_then(|file| file.sync_all());
    }
    for suffix in 0..1000 {
        let destination = if suffix == 0 {
            path.clone()
        } else {
            parent.join(format!("{stem} ({suffix}).{extension}"))
        };
        // hard_link is an atomic no-replace publication on the destination filesystem. Staging is
        // under the same root. Never fall back to copy/rename, both of which can overwrite files.
        match std::fs::hard_link(&produced, &destination) {
            Ok(()) => {}
            Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => continue,
            Err(error) => return Err(format!("Could not save the audio file: {error}")),
        }
        // Each companion lands under the audio's exact stem and suffix, never overwriting a file.
        // A companion collision means this suffix is not clean for the whole set: roll back only
        // the links this attempt created and try the next one so audio and sidecars stay together.
        // A non-collision error on a companion is best-effort — the audio still lands without it.
        let mut created = vec![destination.clone()];
        let mut attempt_warnings: Vec<String> = Vec::new();
        let mut collision = false;
        for (ext, source) in &companions {
            let companion = if suffix == 0 {
                parent.join(format!("{stem}.{ext}"))
            } else {
                parent.join(format!("{stem} ({suffix}).{ext}"))
            };
            match std::fs::hard_link(source, &companion) {
                Ok(()) => created.push(companion),
                Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => {
                    collision = true;
                    break;
                }
                // A non-collision failure (the filesystem rejected the link) never fails the audio,
                // but the user must see the companion was skipped rather than lose it silently.
                Err(error) => attempt_warnings
                    .push(format!("Couldn't save the {} file: {error}", companion_label(ext))),
            }
        }
        if collision {
            for link in &created {
                let _ = std::fs::remove_file(link);
            }
            continue;
        }
        warnings.append(&mut attempt_warnings);
        return Ok(destination.to_string_lossy().into_owned());
    }
    Err("Too many files share this track's filename. Choose another download folder.".into())
}

/// Validate the enrichment's companion files: each must be a regular, non-empty file that lives
/// directly in this job's staging directory and carries a supported sidecar extension. Duplicates
/// of an extension and anything failing a check are dropped, so publication never links a symlink,
/// a directory, an empty file, or a path outside staging.
fn collect_sidecars(staging: &Path, sidecars: &[PathBuf]) -> Vec<(String, PathBuf)> {
    let mut out: Vec<(String, PathBuf)> = Vec::new();
    for path in sidecars {
        if path.parent() != Some(staging) {
            continue;
        }
        let ext = match path.extension().and_then(|value| value.to_str()) {
            Some(value) => value.to_ascii_lowercase(),
            None => continue,
        };
        if !matches!(ext.as_str(), "jpg" | "png" | "lrc" | "txt") {
            continue;
        }
        if out.iter().any(|(seen, _)| seen == &ext) {
            continue;
        }
        match std::fs::symlink_metadata(path) {
            Ok(meta) if meta.is_file() && meta.len() > 0 => out.push((ext, path.clone())),
            _ => continue,
        }
    }
    out
}

/// A friendly noun for a companion extension, used in the non-fatal warning shown in history.
fn companion_label(ext: &str) -> &'static str {
    match ext {
        "jpg" | "png" => "cover art",
        "lrc" => "synced lyrics",
        "txt" => "lyrics",
        _ => "companion",
    }
}

fn find_output_file(directory: &Path) -> Result<PathBuf, String> {
    let mut output = None;
    for entry in std::fs::read_dir(directory).map_err(|error| error.to_string())? {
        let entry = entry.map_err(|error| error.to_string())?;
        let path = entry.path();
        let metadata = std::fs::symlink_metadata(&path).map_err(|error| error.to_string())?;
        let extension = path.extension().and_then(|value| value.to_str()).unwrap_or_default();
        if metadata.is_file()
            && metadata.len() > 0
            && path.file_stem().and_then(|value| value.to_str()) == Some("audio")
            && matches!(
                extension,
                "mp3" | "m4a" | "opus" | "ogg" | "aac" | "flac" | "wav" | "webm" | "mp4"
            )
        {
            if output.is_some() {
                return Err("The downloader produced ambiguous audio output.".into());
            }
            output = Some(path);
        }
    }
    output.ok_or_else(|| "The download produced no finished audio file.".into())
}

fn file_present(job: &DownloadJob) -> bool {
    job.file_path
        .as_ref()
        .and_then(|path| std::fs::metadata(path).ok())
        .is_some_and(|metadata| metadata.is_file() && metadata.len() > 0)
}

fn open_path(path: &Path) -> Result<(), String> {
    let mut child = Command::new("xdg-open")
        .arg(path)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .map_err(|error| format!("Could not open the folder: {error}"))?;
    // A folder handler may remain open for hours. Reap asynchronously so an open file manager
    // cannot hold Tokio's blocking pool (and therefore daemon shutdown) alive.
    tokio::spawn(async move {
        let _ = child.wait().await;
    });
    Ok(())
}

fn sanitize_settings(mut settings: DownloadSettings) -> Result<DownloadSettings, String> {
    if !(1..=4).contains(&settings.workers) {
        return Err("Choose between 1 and 4 download workers.".into());
    }
    if settings.path.len() > 4096 || settings.path.contains('\0') {
        return Err("The download folder path is invalid.".into());
    }
    let path = if settings.path.trim().is_empty() {
        default_music_dir()
    } else {
        PathBuf::from(expand_tilde(settings.path.trim()))
    };
    if !path.is_absolute() || path.parent().is_none() {
        return Err("Choose an absolute download folder, not the filesystem root.".into());
    }
    std::fs::create_dir_all(&path)
        .map_err(|error| format!("Could not create the download folder: {error}"))?;
    let path = path.canonicalize().map_err(|error| error.to_string())?;
    if !path.is_dir() {
        return Err("The download destination is not a folder.".into());
    }
    settings.path = path.to_string_lossy().into_owned();
    Ok(settings)
}

#[derive(Serialize)]
struct PersistedRef<'a> {
    settings: &'a DownloadSettings,
    jobs: &'a [DownloadJob],
}

#[derive(Serialize, Deserialize)]
struct Persisted {
    settings: DownloadSettings,
    jobs: Vec<DownloadJob>,
}

fn load(directory: &Path) -> anyhow::Result<(DownloadSettings, Vec<DownloadJob>)> {
    let path = directory.join(STATE_FILE);
    // An interrupted atomic write is never authoritative.
    let _ = std::fs::remove_file(directory.join("downloads.json.tmp"));
    let file = match std::fs::File::open(&path) {
        Ok(file) => file,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            return Ok((DownloadSettings::defaults(), Vec::new()))
        }
        Err(error) => return Err(error.into()),
    };
    let mut bytes = Vec::new();
    file.take(8 * 1024 * 1024 + 1).read_to_end(&mut bytes)?;
    anyhow::ensure!(bytes.len() <= 8 * 1024 * 1024, "download history exceeds its safe size limit");
    let mut stored: Persisted = serde_json::from_slice(&bytes)?;
    stored.settings.workers = stored.settings.workers.clamp(1, 4);
    anyhow::ensure!(
        Path::new(&stored.settings.path).is_absolute(),
        "saved download folder is not absolute"
    );
    anyhow::ensure!(
        stored.jobs.len() <= MAX_ACTIVE_QUEUED + MAX_HISTORY,
        "too many saved download records"
    );
    for job in &mut stored.jobs {
        job.settings.workers = job.settings.workers.clamp(1, 4);
    }
    Ok((stored.settings, stored.jobs))
}

fn recover_interrupted(jobs: &mut [DownloadJob]) -> bool {
    let mut changed = false;
    for job in jobs.iter_mut().filter(|job| job.status.is_active()) {
        apply_result(job, Err(INTERRUPTED_MSG.into()));
        changed = true;
    }
    changed
}

fn trim_history(jobs: &mut Vec<DownloadJob>) {
    let mut excess =
        jobs.iter().filter(|job| !job.status.is_active()).count().saturating_sub(MAX_HISTORY);
    jobs.retain(|job| {
        if excess > 0 && !job.status.is_active() {
            excess -= 1;
            false
        } else {
            true
        }
    });
}

fn home_dir() -> PathBuf {
    std::env::var_os("HOME").map(PathBuf::from).unwrap_or_else(|| PathBuf::from("/"))
}

fn parse_progress(line: &str) -> Option<u32> {
    if !line.contains("[download]") {
        return None;
    }
    for token in line.split_whitespace() {
        if let Some(num) = token.strip_suffix('%') {
            if let Ok(pct) = num.parse::<f64>() {
                if pct.is_finite() {
                    return Some(pct.clamp(0.0, 100.0).round() as u32);
                }
            }
        }
    }
    None
}

/// Turn a bounded stderr tail into a one-line error, preferring yt-dlp's own `ERROR:` line.
fn error_message(tail: &str, code: Option<i32>) -> String {
    let mut last_error = None;
    let mut last_nonempty = None;
    for line in tail.lines() {
        let l = line.trim();
        if l.is_empty() {
            continue;
        }
        last_nonempty = Some(l);
        if l.contains("ERROR") {
            last_error = Some(l);
        }
    }
    let chosen = last_error.or(last_nonempty);
    match chosen {
        Some(l) => truncate_chars(l, MAX_ERROR_LEN),
        None => match code {
            Some(c) => format!("The download failed (exit code {c})."),
            None => "The download was terminated.".into(),
        },
    }
}

fn compute_final_path(
    settings: &DownloadSettings,
    title: &str,
    artists: &str,
    video_id: &str,
    ext: &str,
) -> PathBuf {
    let mut dir = PathBuf::from(&settings.path);
    if settings.organize_by_artist {
        dir = dir.join(sanitize_component(primary_artist(artists)));
    }
    let base = if settings.organize_by_artist || artists.trim().is_empty() {
        title.to_string()
    } else {
        format!("{} - {}", artists.trim(), title.trim())
    };
    let name = sanitize_component(&base);
    let id_tag = sanitize_component(video_id);
    dir.join(format!("{name} [{id_tag}].{ext}"))
}

/// The first credited artist, for the per-artist folder.
fn primary_artist(artists: &str) -> &str {
    let cut = artists
        .find(|c: char| matches!(c, ',' | '&' | '•' | ';'))
        .or_else(|| artists.find(" feat"))
        .or_else(|| artists.find(" ft."))
        .unwrap_or(artists.len());
    let first = artists[..cut].trim();
    if first.is_empty() {
        "Unknown Artist"
    } else {
        first
    }
}

/// Make one path component safe: drop control chars, replace path separators and other reserved
/// characters with `_`, strip leading/trailing dots and spaces (so no `.`/`..`/hidden component),
/// and bound the length. Falls back to `download` if nothing usable remains.
fn sanitize_component(input: &str) -> String {
    let mut out = String::with_capacity(input.len());
    let mut last_space = false;
    for ch in input.chars() {
        if ch.is_control() {
            continue;
        }
        let mapped = match ch {
            '/' | '\\' | ':' | '*' | '?' | '"' | '<' | '>' | '|' | '\0' => '_',
            c => c,
        };
        // Collapse runs of whitespace to single spaces.
        if mapped.is_whitespace() {
            if last_space {
                continue;
            }
            last_space = true;
            out.push(' ');
        } else {
            last_space = false;
            out.push(mapped);
        }
    }
    let trimmed = out.trim().trim_matches('.').trim();
    let bounded = truncate_chars(trimmed, MAX_COMPONENT_BYTES);
    let bounded = bounded.trim().trim_matches('.').trim();
    if bounded.is_empty() {
        "download".to_string()
    } else {
        bounded.to_string()
    }
}

fn expand_tilde(raw: &str) -> String {
    if raw == "~" {
        home_dir().to_string_lossy().into_owned()
    } else if let Some(rest) = raw.strip_prefix("~/") {
        home_dir().join(rest).to_string_lossy().into_owned()
    } else {
        raw.to_string()
    }
}

/// The default download folder: the XDG Music directory's `Ryotunes/`, falling back to
/// `$HOME/Music/Ryotunes`.
fn default_music_dir() -> PathBuf {
    music_root().join("Ryotunes")
}

/// The user's Music directory: `$XDG_MUSIC_DIR` if absolute, then the `user-dirs.dirs`
/// `XDG_MUSIC_DIR` entry, else `$HOME/Music`.
fn music_root() -> PathBuf {
    if let Some(dir) = std::env::var_os("XDG_MUSIC_DIR") {
        let p = PathBuf::from(dir);
        if p.is_absolute() {
            return p;
        }
    }
    if let Some(p) = xdg_user_music_dir() {
        return p;
    }
    home_dir().join("Music")
}

/// Parse `XDG_MUSIC_DIR="$HOME/Music"` out of `~/.config/user-dirs.dirs` (or `$XDG_CONFIG_HOME`).
fn xdg_user_music_dir() -> Option<PathBuf> {
    let config_home = std::env::var_os("XDG_CONFIG_HOME")
        .map(PathBuf::from)
        .filter(|p| p.is_absolute())
        .unwrap_or_else(|| home_dir().join(".config"));
    let text = std::fs::read_to_string(config_home.join("user-dirs.dirs")).ok()?;
    for line in text.lines() {
        let line = line.trim();
        if line.starts_with('#') {
            continue;
        }
        let Some(rest) = line.strip_prefix("XDG_MUSIC_DIR=") else { continue };
        let value = rest.trim().trim_matches('"');
        let expanded = if let Some(tail) = value.strip_prefix("$HOME/") {
            home_dir().join(tail)
        } else if value == "$HOME" {
            home_dir()
        } else {
            PathBuf::from(value)
        };
        if expanded.is_absolute() {
            return Some(expanded);
        }
    }
    None
}

fn truncate_chars(s: &str, max: usize) -> String {
    if s.len() <= max {
        return s.to_string();
    }
    let mut out = String::with_capacity(max);
    for ch in s.chars() {
        if out.len() + ch.len_utf8() > max {
            break;
        }
        out.push(ch);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Temp(PathBuf);
    impl Temp {
        fn new() -> Self {
            let path = std::env::temp_dir().join(format!(
                "ryotunes-download-test-{}-{}",
                std::process::id(),
                SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_nanos()
            ));
            std::fs::create_dir(&path).unwrap();
            Self(path)
        }
    }
    impl Drop for Temp {
        fn drop(&mut self) {
            let _ = std::fs::remove_dir_all(&self.0);
        }
    }
    struct Sink;
    impl EventSink for Sink {
        fn emit(&self, _: &'static str, _: Value) {}
    }

    fn manager(directory: &Path) -> Arc<Downloads> {
        let (quit, _) = tokio::sync::mpsc::unbounded_channel();
        Downloads::new(
            Arc::new(Sink),
            Arc::new(SoundCloud::new(directory.join("soundcloud"))),
            directory.join("state"),
            Lifecycle::new(quit),
            None,
        )
        .unwrap()
    }
    fn settings(directory: &Path, workers: u32) -> DownloadSettings {
        DownloadSettings {
            path: directory.to_string_lossy().into_owned(),
            workers,
            format: DownloadFormat::Original,
            organize_by_artist: false,
            embed_metadata: true,
        }
    }
    fn job(directory: &Path) -> DownloadJob {
        DownloadJob {
            id: "publication".into(),
            video_id: "jNQXAC9IVRw".into(),
            title: "../../a %(title)s".into(),
            artists: "../artist".into(),
            thumbnail: String::new(),
            status: DownloadStatus::Downloading,
            progress: 0,
            file_path: None,
            error: None,
            source: "YouTube".into(),
            created_at: 1,
            finished_at: None,
            settings: settings(directory, 1),
            warnings: Vec::new(),
        }
    }

    #[test]
    fn publication_never_replaces_existing_files_or_accepts_partials() {
        let directory = Temp::new();
        let staging = create_staging(directory.0.to_str().unwrap(), "publication").unwrap();
        std::fs::write(staging.join("audio.mp3.part"), b"partial").unwrap();
        assert!(publish(&staging, &job(&directory.0), &[], &mut Vec::new()).is_err());
        std::fs::write(staging.join("audio.mp3"), b"finished audio").unwrap();
        let j = job(&directory.0);
        let first = compute_final_path(&j.settings, &j.title, &j.artists, &j.video_id, "mp3");
        std::fs::write(&first, b"existing user music").unwrap();
        let second = publish(&staging, &job(&directory.0), &[], &mut Vec::new()).unwrap();
        assert_eq!(std::fs::read(&first).unwrap(), b"existing user music");
        assert_eq!(std::fs::read(&second).unwrap(), b"finished audio");
        assert_eq!(Path::new(&second).parent().unwrap(), directory.0);
    }

    #[test]
    fn symlinked_staging_and_audio_are_rejected() {
        use std::os::unix::fs::symlink;
        let directory = Temp::new();
        let elsewhere = Temp::new();
        symlink(&elsewhere.0, directory.0.join(STAGING_DIR)).unwrap();
        assert!(create_staging(directory.0.to_str().unwrap(), "blocked").is_err());
        std::fs::write(elsewhere.0.join("source"), b"not our output").unwrap();
        symlink(elsewhere.0.join("source"), elsewhere.0.join("audio.mp3")).unwrap();
        assert!(find_output_file(&elsewhere.0).is_err());
    }

    #[tokio::test]
    async fn cancellation_keeps_slots_reserved_and_retry_cannot_oversubscribe() {
        let directory = Temp::new();
        let manager = manager(&directory.0);
        manager.set_settings(settings(&directory.0.join("music"), 2)).unwrap();
        let first = manager
            .enqueue("jNQXAC9IVRw".into(), "one".into(), "artist".into(), "".into())
            .unwrap();
        manager.enqueue("dQw4w9WgXcQ".into(), "two".into(), "artist".into(), "".into()).unwrap();
        let third = manager
            .enqueue("aqz-KE-bpKQ".into(), "three".into(), "artist".into(), "".into())
            .unwrap();
        let duplicate = manager
            .enqueue("jNQXAC9IVRw".into(), "one".into(), "artist".into(), "".into())
            .unwrap();
        assert_eq!(duplicate.id, first.id);
        manager.cancel(&first.id).unwrap();
        let retried = manager.retry(&first.id).unwrap();
        assert_eq!(manager.snapshot()["activeCount"], 2);
        assert_eq!(manager.snapshot()["queuedCount"], 2);
        assert_eq!(retried.status, DownloadStatus::Queued);
        let mut next = settings(&directory.0.join("new-music"), 1);
        next.format = DownloadFormat::Mp3;
        manager.set_settings(next).unwrap();
        let frozen = manager.snapshot()["jobs"]
            .as_array()
            .unwrap()
            .iter()
            .find(|job| job["id"] == third.id)
            .unwrap()
            .clone();
        assert_eq!(frozen["settings"]["format"], "original");
        assert!(frozen["settings"]["path"].as_str().unwrap().ends_with("/music"));
        manager.shutdown().await;
        assert_eq!(manager.snapshot()["activeCount"], 0);
        assert_eq!(manager.snapshot()["queuedCount"], 0);
        assert!(manager
            .enqueue("jNQXAC9IVRw".into(), "one".into(), "a".into(), "".into())
            .is_err());
    }

    #[tokio::test]
    async fn failed_storage_does_not_accept_jobs_or_settings() {
        let directory = Temp::new();
        let manager = manager(&directory.0);
        let original = manager.settings().path;
        std::fs::create_dir(directory.0.join("state/downloads.json.tmp")).unwrap();
        assert!(manager
            .enqueue("jNQXAC9IVRw".into(), "one".into(), "artist".into(), "".into())
            .is_err());
        assert!(manager.snapshot()["jobs"].as_array().unwrap().is_empty());
        assert!(manager.set_settings(settings(&directory.0.join("music"), 2)).is_err());
        assert_eq!(manager.settings().path, original);
        std::fs::remove_dir(directory.0.join("state/downloads.json.tmp")).unwrap();
        manager.shutdown().await;
    }

    #[test]
    fn settings_reject_unsafe_limits_and_relative_destinations() {
        let directory = Temp::new();
        assert!(sanitize_settings(settings(&directory.0, 0)).is_err());
        assert!(sanitize_settings(settings(&directory.0, 5)).is_err());
        assert!(sanitize_settings(settings(Path::new("relative/music"), 1)).is_err());
        assert_eq!(sanitize_settings(settings(&directory.0, 4)).unwrap().workers, 4);
    }

    #[test]
    fn restart_recovers_pending_jobs_and_preserves_saved_files() {
        let directory = Temp::new();
        let mut queued = job(&directory.0);
        queued.status = DownloadStatus::Queued;
        let mut completed = job(&directory.0);
        completed.status = DownloadStatus::Completed;
        completed.file_path = Some(directory.0.join("kept.mp3").to_string_lossy().into_owned());
        let mut jobs = vec![queued, completed];
        assert!(recover_interrupted(&mut jobs));
        assert_eq!(jobs[0].status, DownloadStatus::Failed);
        assert!(jobs[0].error.is_some());
        assert_eq!(jobs[1].status, DownloadStatus::Completed);
        assert!(jobs[1].file_path.is_some());
        assert!(!recover_interrupted(&mut jobs));
    }

    #[test]
    fn companions_publish_under_the_audio_stem_without_overwriting() {
        let directory = Temp::new();
        let staging = create_staging(directory.0.to_str().unwrap(), "companions").unwrap();
        std::fs::write(staging.join("audio.mp3"), b"finished audio").unwrap();
        std::fs::write(staging.join("audio.jpg"), b"cover art").unwrap();
        std::fs::write(staging.join("audio.lrc"), b"[00:00.00] la").unwrap();
        // Empty and out-of-staging entries are dropped, never published.
        std::fs::write(staging.join("audio.txt"), b"").unwrap();
        let outside = directory.0.join("stray.jpg");
        std::fs::write(&outside, b"not ours").unwrap();
        let sidecars = vec![
            staging.join("audio.jpg"),
            staging.join("audio.lrc"),
            staging.join("audio.txt"),
            outside.clone(),
        ];
        // Pre-place a cover at the base stem so a suffix-0 companion collision forces the whole set
        // (audio + sidecars) to the next suffix, and prove the existing cover is never overwritten.
        let j = job(&directory.0);
        let base_audio = compute_final_path(&j.settings, &j.title, &j.artists, &j.video_id, "mp3");
        let base_cover = base_audio.with_extension("jpg");
        std::fs::write(&base_cover, b"existing cover").unwrap();

        let mut warnings: Vec<String> = Vec::new();
        let published = publish(&staging, &job(&directory.0), &sidecars, &mut warnings).unwrap();
        let audio = PathBuf::from(&published);
        assert!(audio.file_name().unwrap().to_str().unwrap().contains("(1)"));
        assert_eq!(std::fs::read(&audio).unwrap(), b"finished audio");
        // Cover and lyrics share the audio's exact stem and suffix.
        assert_eq!(std::fs::read(audio.with_extension("jpg")).unwrap(), b"cover art");
        assert_eq!(std::fs::read(audio.with_extension("lrc")).unwrap(), b"[00:00.00] la");
        // The suffix-0 audio slot the collision rolled back is left empty, the pre-existing cover
        // is untouched, and the empty/out-of-staging companions were dropped.
        assert!(!base_audio.exists());
        assert_eq!(std::fs::read(&base_cover).unwrap(), b"existing cover");
        assert!(!audio.with_extension("txt").exists());
        // No filesystem error occurred, so the completed job carries no companion warnings.
        assert!(warnings.is_empty());
    }
}
