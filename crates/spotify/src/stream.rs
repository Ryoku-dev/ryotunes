//! Spotify audio, routed into Ryotunes' single libmpv engine.
//!
//! Ryotunes plays everything through one libmpv instance (tempo/pitch/fx live on its filter chain),
//! so Spotify audio must land in mpv rather than cpal/rodio. This replaces Sonora's
//! `playback.rs`/`sink.rs`/`audio.rs` (a cpal+rodio sink) with librespot-playback's built-in `pipe`
//! backend: one librespot [`Player`] per [`crate::Client`] decodes and writes **raw S16LE / 44100 Hz
//! / stereo** into a per-session FIFO. The daemon points mpv at that FIFO:
//!
//! ```text
//! loadfile <fifo> replace \
//!   demuxer=rawaudio,demuxer-rawaudio-format=s16le,\
//!   demuxer-rawaudio-rate=44100,demuxer-rawaudio-channels=2
//! ```
//!
//! Note (S16 endianness): librespot emits native-endian S16, which is little-endian on Ryotunes'
//! Linux x86-64 target — matching `s16le`. See `README.md` for the pause/EOF caveat.

use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context as _, Result};
use librespot_core::{Session, SpotifyUri};
use librespot_playback::audio_backend;
use librespot_playback::config::{AudioFormat, Bitrate, PlayerConfig};
use librespot_playback::mixer::NoOpVolume;
use librespot_playback::player::{Player, PlayerEvent, PlayerEventChannel};

const TRACK_PREFIX: &str = "spotify:track:";
const BACKEND: &str = "pipe";
const POSITION_INTERVAL: Duration = Duration::from_millis(500);

/// A playback event, mapped from librespot's [`PlayerEvent`] onto the small set the daemon reacts to.
/// Positions are measured from the start of the track.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum StreamEvent {
    Playing(Duration),
    Paused(Duration),
    Position(Duration),
    EndOfTrack,
    Unavailable,
}

/// The librespot player and its FIFO. One per [`crate::Client`]: the sink is built once (librespot's
/// builder is `FnOnce`), so the FIFO path is fixed for the client's lifetime and every track streams
/// through it.
pub(crate) struct Engine {
    player: Arc<Player>,
    fifo: Arc<Fifo>,
}

impl Engine {
    pub(crate) fn new(session: Session) -> Result<Self> {
        let fifo = Arc::new(Fifo::create()?);

        let config = PlayerConfig {
            bitrate: Bitrate::Bitrate320,
            gapless: false,
            position_update_interval: Some(POSITION_INTERVAL),
            ..Default::default()
        };

        let builder = audio_backend::find(Some(BACKEND.to_owned()))
            .context("librespot-playback has no `pipe` backend")?;
        let device = fifo.path().to_string_lossy().into_owned();
        let player = Player::new(config, session, Box::new(NoOpVolume), move || {
            builder(Some(device), AudioFormat::S16)
        });

        Ok(Self { player, fifo })
    }

    /// Load `track_id` (a bare Spotify track id or a `spotify:track:` URI) and start playing. The
    /// returned handle drives this one track; each call subscribes its own event stream.
    pub(crate) fn stream(&self, track_id: &str) -> Result<StreamHandle> {
        let uri = track_uri(track_id)?;
        let events = self.player.get_player_event_channel();
        self.player.load(uri, true, 0);
        Ok(StreamHandle { player: self.player.clone(), fifo: self.fifo.clone(), events })
    }
}

/// A handle to one streaming track. Holds a clone of the client's player and FIFO, plus its own
/// event stream. Dropping it does not tear the player down; the FIFO is removed when the last handle
/// (and the owning client) is dropped.
pub struct StreamHandle {
    player: Arc<Player>,
    fifo: Arc<Fifo>,
    events: PlayerEventChannel,
}

impl StreamHandle {
    /// The FIFO mpv should `loadfile` with the rawaudio demuxer options (see the module docs).
    pub fn fifo_path(&self) -> &Path {
        self.fifo.path()
    }

    /// A cheap, cloneable transport handle (seek/play + FIFO path). The daemon keeps this to
    /// drive scrubbing while a separate task owns the [`StreamHandle`] to poll
    /// [`StreamHandle::next_event`] — the two halves can't share `&mut self`.
    pub fn controls(&self) -> StreamControls {
        StreamControls { player: self.player.clone(), fifo: self.fifo.clone() }
    }

    /// Seek within the current track.
    pub fn seek(&self, position: Duration) {
        self.player.seek(position.as_millis() as u32);
    }

    /// Resume librespot decoding. See the README pause caveat: librespot closing the pipe on pause
    /// looks like EOF to mpv, so the daemon's normal pause should pause mpv (FIFO backpressure stalls
    /// librespot) rather than call [`StreamHandle::pause`].
    pub fn play(&self) {
        self.player.play();
    }

    /// Pause librespot decoding. Closes the pipe's write end; see [`StreamHandle::play`].
    pub fn pause(&self) {
        self.player.pause();
    }

    /// The next playback event, or `None` once the player has shut down. librespot events that do not
    /// map onto [`StreamEvent`] (e.g. `Loading`) are skipped.
    pub async fn next_event(&mut self) -> Option<StreamEvent> {
        loop {
            let event = self.events.recv().await?;
            if let Some(event) = translate(event) {
                return Some(event);
            }
        }
    }
}

/// The transport half of a live stream, split off from [`StreamHandle`] so the daemon can seek
/// and reload from one place while the handle itself is owned by the task polling its events.
/// Cheap to clone: two `Arc`s. Dropping it does not tear the player down (see [`StreamHandle`]).
#[derive(Clone)]
pub struct StreamControls {
    player: Arc<Player>,
    fifo: Arc<Fifo>,
}

impl StreamControls {
    /// The FIFO mpv should `loadfile` with the rawaudio demuxer options (see the module docs).
    pub fn fifo_path(&self) -> &Path {
        self.fifo.path()
    }

    /// Seek within the current track. The daemon reloads mpv on the FIFO afterwards so it reads
    /// the post-seek PCM (mpv cannot seek a pipe itself).
    pub fn seek(&self, position: Duration) {
        self.player.seek(position.as_millis() as u32);
    }

    /// Resume librespot decoding. Never used for an ordinary pause (that pauses mpv and lets FIFO
    /// backpressure stall librespot); only after a seek reload.
    pub fn play(&self) {
        self.player.play();
    }
}

fn track_uri(track_id: &str) -> Result<SpotifyUri> {
    let uri = if track_id.starts_with(TRACK_PREFIX) {
        track_id.to_owned()
    } else {
        format!("{TRACK_PREFIX}{track_id}")
    };
    SpotifyUri::from_uri(&uri).with_context(|| format!("{track_id} is not a Spotify track id"))
}

fn translate(event: PlayerEvent) -> Option<StreamEvent> {
    let millis = |position_ms: u32| Duration::from_millis(position_ms as u64);
    match event {
        PlayerEvent::Playing { position_ms, .. } => Some(StreamEvent::Playing(millis(position_ms))),
        PlayerEvent::Paused { position_ms, .. } => Some(StreamEvent::Paused(millis(position_ms))),
        PlayerEvent::PositionChanged { position_ms, .. }
        | PlayerEvent::PositionCorrection { position_ms, .. } => {
            Some(StreamEvent::Position(millis(position_ms)))
        }
        PlayerEvent::Stopped { .. } | PlayerEvent::EndOfTrack { .. } => {
            Some(StreamEvent::EndOfTrack)
        }
        PlayerEvent::Unavailable { .. } => Some(StreamEvent::Unavailable),
        _ => None,
    }
}

/// A named pipe under `$XDG_RUNTIME_DIR/ryotunes/`, removed on drop.
struct Fifo {
    path: PathBuf,
}

impl Fifo {
    fn create() -> Result<Self> {
        let dir = runtime_dir().join("ryotunes");
        std::fs::create_dir_all(&dir)
            .with_context(|| format!("cannot create the FIFO directory {}", dir.display()))?;
        let path = dir.join(format!("spotify-{}.pcm", std::process::id()));
        // A stale FIFO from a crashed run would be reused with the wrong permissions/owner; start
        // fresh.
        let _ = std::fs::remove_file(&path);
        mkfifo(&path)?;
        Ok(Self { path })
    }

    fn path(&self) -> &Path {
        &self.path
    }
}

impl Drop for Fifo {
    fn drop(&mut self) {
        let _ = std::fs::remove_file(&self.path);
    }
}

fn runtime_dir() -> PathBuf {
    std::env::var_os("XDG_RUNTIME_DIR").map(PathBuf::from).unwrap_or_else(std::env::temp_dir)
}

#[cfg(unix)]
fn mkfifo(path: &Path) -> Result<()> {
    use std::ffi::CString;
    use std::os::unix::ffi::OsStrExt as _;

    let c_path = CString::new(path.as_os_str().as_bytes())
        .with_context(|| format!("the FIFO path {} is not a valid C string", path.display()))?;
    // SAFETY: `c_path` is a valid NUL-terminated string that outlives the call.
    let rc = unsafe { libc::mkfifo(c_path.as_ptr(), 0o600) };
    if rc != 0 {
        return Err(std::io::Error::last_os_error())
            .with_context(|| format!("mkfifo failed for {}", path.display()));
    }
    Ok(())
}

#[cfg(not(unix))]
fn mkfifo(_path: &Path) -> Result<()> {
    anyhow::bail!("Spotify streaming needs a Unix FIFO; this platform is unsupported")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_bare_id_and_full_uri() {
        let bare = track_uri("4uLU6hMCjMI75M1A2tKUQC").unwrap();
        let full = track_uri("spotify:track:4uLU6hMCjMI75M1A2tKUQC").unwrap();
        assert_eq!(bare.to_uri().unwrap(), "spotify:track:4uLU6hMCjMI75M1A2tKUQC");
        assert_eq!(bare.to_uri().unwrap(), full.to_uri().unwrap());
    }

    #[test]
    fn maps_librespot_events() {
        assert_eq!(
            translate(PlayerEvent::Playing {
                play_request_id: 0,
                track_id: SpotifyUri::from_uri("spotify:track:4uLU6hMCjMI75M1A2tKUQC").unwrap(),
                position_ms: 1500,
            }),
            Some(StreamEvent::Playing(Duration::from_millis(1500)))
        );
        assert_eq!(
            translate(PlayerEvent::EndOfTrack {
                play_request_id: 0,
                track_id: SpotifyUri::from_uri("spotify:track:4uLU6hMCjMI75M1A2tKUQC").unwrap(),
            }),
            Some(StreamEvent::EndOfTrack)
        );
    }
}
