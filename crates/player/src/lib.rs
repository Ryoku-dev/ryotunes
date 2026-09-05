//! libmpv wrapper. YouTube-agnostic: takes a fully-resolved URL + headers, never
//! a videoId. Gapless via mpv's internal playlist (1-track lookahead fed by the orchestrator).

use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use libmpv2::events::{Event, EventContext, PropertyData};
use libmpv2::{Format, Mpv};
use tokio::sync::mpsc::{unbounded_channel, UnboundedReceiver};

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("mpv: {0}")]
    Mpv(#[from] libmpv2::Error),
    /// mpv refused a chain carrying the pitch filter, which means this libmpv was built without
    /// librubberband. Its own answer is `Raw(-9)`, so it needs saying in words: this one reaches
    /// the user as a toast.
    #[error("Pitch shifting isn't available in this build")]
    NoPitchFilter,
}

/// Events pumped from mpv's event thread.
#[derive(Debug, Clone)]
pub enum PlayerEvent {
    Position(f64),
    Duration(f64),
    /// Playback started or stopped, emitted only on a real change.
    ///
    /// Derived from mpv's `pause` **and** `idle-active`, because `pause` alone is a trap: it starts
    /// out `false` and a `loadfile` doesn't touch it, so starting a track sets `false` → `false`
    /// and fires **no** property event at all. `idle-active` is the one that actually flips when a
    /// file starts (and when the playlist runs dry). Anything reading playback state off `pause`
    /// alone never hears that a track began, and only recovers on a manual pause/unpause.
    Playing(bool),
    /// One track finished normally (EOF) — orchestrator advances the queue.
    TrackEnded,
    /// One track died (end-file with error, e.g. its URL 403'd). mpv may have auto-advanced
    /// into the next playlist entry or gone idle — the orchestrator asks [`Player::is_idle`].
    TrackFailed(String),
    Error(String),
}

/// mpv end-file reasons (from `mpv_end_file_reason`).
const EOF: i32 = 0;

/// User-facing message for a failed track — raw mpv codes ("Raw(-13)") mean nothing to users.
fn friendly_error(e: &libmpv2::Error) -> String {
    use libmpv2::mpv_error;
    match e {
        libmpv2::Error::Loadfile { error } => friendly_error(error),
        libmpv2::Error::Raw(code) => match *code {
            mpv_error::LoadingFailed => {
                "Couldn't load this track — YouTube rejected the stream link".to_owned()
            }
            mpv_error::NothingToPlay => "This stream contains no playable audio".to_owned(),
            mpv_error::UnknownFormat => "Unrecognized audio format".to_owned(),
            mpv_error::AoInitFailed => "Couldn't start audio output".to_owned(),
            other => format!("Playback failed (mpv error {other})"),
        },
        other => format!("Playback failed ({other})"),
    }
}

/// The user-tunable audio effects the Sound dialog drives (spec section 7). Applied as one mpv
/// `af` chain alongside the loudness gain; `speed` is mpv's `speed` property, not part of the
/// chain. Serialized `{speed, semitones, reverb, bass, width}` for the socket — `bass_db` is
/// `bass` on the wire.
#[derive(Debug, Clone, Copy, PartialEq, serde::Serialize, serde::Deserialize)]
pub struct AudioFx {
    /// Tempo multiplier, 0.25–2.0 (mpv `speed`: time-stretch, pitch preserved).
    pub speed: f64,
    /// Pitch shift in semitones, −12..=12, via the rubberband filter.
    pub semitones: i32,
    /// Reverb depth 0..1: scales the echo decays; 0 omits the filter.
    pub reverb: f64,
    /// Bass shelf gain, −6..+12 dB; 0 omits the filter.
    #[serde(rename = "bass")]
    pub bass_db: f64,
    /// Stereo width 0..1; 0 omits the filter.
    pub width: f64,
}

impl Default for AudioFx {
    /// Everything off: speed 1×, no pitch shift, no effects — the same filterless path playback
    /// took before the Sound dialog existed.
    fn default() -> Self {
        AudioFx { speed: 1.0, semitones: 0, reverb: 0.0, bass_db: 0.0, width: 0.0 }
    }
}

/// The player. Wraps `Arc<Mpv>` (Send+Sync); the event loop runs on a dedicated OS thread and
/// pumps [`PlayerEvent`]s into a channel taken once via [`Player::take_events`].
pub struct Player {
    mpv: Arc<Mpv>,
    events: Option<UnboundedReceiver<PlayerEvent>>,
    /// Event-driven mirror of mpv `idle-active`. Lifecycle code must not synchronously query mpv
    /// from the async event pump: that can race/stall during pause and gapless transitions.
    idle_active: Arc<AtomicBool>,
    /// `(loudness gain dB, audio-fx)`. mpv's `af` is one global chain, so everything that writes
    /// to it — the loudness gain and every effect in [`AudioFx`] — has to be re-applied together:
    /// a bare `set_property("af", ...)` from any one of them would drop the others' filters.
    af: parking_lot::Mutex<(Option<f64>, AudioFx)>,
}

impl Player {
    /// Create a player with a disk audio cache under `cache_dir`.
    pub fn new(cache_dir: &str) -> Result<Self, Error> {
        // libmpv requires LC_NUMERIC=="C" to parse internal option values; Tauri/GTK's init
        // resets the process locale from the system locale first, which makes mpv_create()
        // return null (Note: locale reset only, revisit if other LC_* categories start
        // tripping mpv too).
        unsafe {
            libc::setlocale(libc::LC_NUMERIC, c"C".as_ptr());
        }

        // Create mpv first, then set properties; this build rejects some options during the
        // pre-init phase returns PROPERTY_NOT_FOUND on this mpv build).
        let mpv = Mpv::new()?;
        mpv.set_property("vid", "no")?; // audio only
        mpv.set_property("gapless-audio", "yes")?;
        mpv.set_property("cache", "yes")?;
        mpv.set_property("cache-on-disk", "yes")?;
        mpv.set_property("demuxer-cache-dir", cache_dir)?;
        let mpv = Arc::new(mpv);

        let (tx, rx) = unbounded_channel();
        // mpv starts idle. `load()` flips this eagerly after a successful loadfile command and the
        // observed `idle-active` property remains authoritative thereafter.
        let idle_active = Arc::new(AtomicBool::new(true));
        let ev = EventContext::new(mpv.ctx);
        ev.disable_deprecated_events().ok();
        ev.observe_property("time-pos", Format::Double, 0)?;
        ev.observe_property("duration", Format::Double, 1)?;
        ev.observe_property("pause", Format::Flag, 2)?;
        ev.observe_property("idle-active", Format::Flag, 3)?;

        let event_idle_active = idle_active.clone();
        std::thread::Builder::new()
            .name("mpv-events".into())
            .spawn(move || event_loop(ev, tx, event_idle_active))
            .expect("spawn mpv event thread");

        Ok(Player { mpv, events: Some(rx), idle_active, af: parking_lot::Mutex::new((None, AudioFx::default())) })
    }

    /// Take the event receiver (once).
    pub fn take_events(&mut self) -> Option<UnboundedReceiver<PlayerEvent>> {
        self.events.take()
    }

    /// Load and play a fresh URL, replacing the playlist. `title` names the stream for mpv
    /// (and so for the PipeWire node); without it mpv uses the URL, which for a signed
    /// googlevideo link puts the whole token into the audio graph.
    pub fn load(
        &self,
        url: &str,
        headers: &HashMap<String, String>,
        gain_db: Option<f64>,
        title: &str,
    ) -> Result<(), Error> {
        self.apply_headers(headers)?;
        self.set_gain(gain_db)?;
        self.mpv.command("loadfile", &[&quoted(url), "replace", "-1", &title_option(title)])?;
        // Close→pause can happen before mpv's property notification reaches the event thread.
        // Mark the session loaded immediately so background lifecycle can never mistake that tiny
        // transition window for an empty player. The observed property corrects this on failure/EOF.
        self.idle_active.store(false, Ordering::Release);
        Ok(())
    }

    /// Append the next track for a gapless transition (the one-track lookahead).
    ///
    /// Note: mpv's `http-header-fields`/`user-agent` are global properties, so appended tracks
    /// inherit the currently-set headers. Ordinary direct-URL streams do not require per-track
    /// cookies; upload-specific headers are handled before a track is loaded.
    pub fn enqueue(&self, url: &str, title: &str) -> Result<(), Error> {
        self.mpv.command("loadfile", &[&quoted(url), "append", "-1", &title_option(title)])?;
        Ok(())
    }

    /// Clear the mpv playlist (e.g. when the user jumps to a new track).
    pub fn clear_playlist(&self) -> Result<(), Error> {
        self.mpv.command("playlist-clear", &[])?;
        Ok(())
    }

    /// True when mpv has nothing loaded (playlist exhausted or the last load failed). The
    /// orchestrator uses this after a track ends/fails to tell "gaplessly advanced into the
    /// lookahead" apart from "stalled — load the next track explicitly".
    pub fn is_idle(&self) -> bool {
        self.mpv.get_property::<bool>("idle-active").unwrap_or(true)
    }

    /// The title mpv reports for the current file (the per-file `force-media-title` once set).
    pub fn media_title(&self) -> Option<String> {
        self.mpv.get_property::<String>("media-title").ok()
    }

    /// Cheap event-driven loaded-media state for background lifecycle decisions. Unlike
    /// [`Self::is_idle`], this never takes mpv's core lock and therefore cannot turn a transient
    /// property-query failure during Pause into a false "nothing loaded" result.
    pub fn has_loaded_media(&self) -> bool {
        !self.idle_active.load(Ordering::Acquire)
    }

    pub fn play(&self) -> Result<(), Error> {
        self.mpv.set_property("pause", false)?;
        Ok(())
    }

    pub fn pause(&self) -> Result<(), Error> {
        self.mpv.set_property("pause", true)?;
        Ok(())
    }

    /// Stop the current mpv session without destroying the player. Used only for an explicit
    /// application quit: unlike Pause, Stop means there is no resumable background session.
    pub fn stop(&self) -> Result<(), Error> {
        self.mpv.command("stop", &[])?;
        // The observed idle-active event remains authoritative, but flip eagerly so shutdown and
        // lifecycle code cannot race the event thread during an explicit quit.
        self.idle_active.store(true, Ordering::Release);
        Ok(())
    }

    pub fn toggle(&self) -> Result<(), Error> {
        self.mpv.command("cycle", &["pause"])?;
        Ok(())
    }

    /// Loop the current file seamlessly (repeat-one). mpv restarts the file at EOF *without*
    /// emitting end-file, so the queue logic upstream never advances while this is on — by design.
    pub fn set_loop_file(&self, on: bool) -> Result<(), Error> {
        self.mpv.set_property("loop-file", if on { "inf" } else { "no" })?;
        Ok(())
    }

    /// Read the current mpv position for an explicit state snapshot. This is intentionally not
    /// used by the event pump; it is a low-frequency recovery path for newly opened or stale UIs.
    pub fn position(&self) -> Option<f64> {
        self.mpv.get_property::<f64>("time-pos").ok().filter(|p| p.is_finite())
    }

    /// Absolute seek in seconds.
    pub fn seek(&self, position_secs: f64) -> Result<(), Error> {
        self.mpv.command("seek", &[&position_secs.to_string(), "absolute"])?;
        Ok(())
    }

    /// Set output volume (0–100). The slider percent is perceptual, not mpv's raw scale:
    /// mpv cubes its `volume` property (gain = (v/100)³), which makes a 10-step drag near
    /// the bottom jump ~18 dB while the same drag near the top moves ~3 dB. Map the percent
    /// onto a 60 dB loudness range instead (see [`perceptual_to_mpv`]), so steps stay roughly
    /// the same size and the bottom of the slider is actually quiet rather than just near-floor.
    pub fn set_volume(&self, volume: i64) -> Result<(), Error> {
        self.mpv.set_property("volume", perceptual_to_mpv(volume))?;
        Ok(())
    }

    fn apply_headers(&self, headers: &HashMap<String, String>) -> Result<(), Error> {
        // User-Agent has its own mpv property; everything else joins http-header-fields.
        if let Some(ua) = headers.get("User-Agent").or_else(|| headers.get("user-agent")) {
            self.mpv.set_property("user-agent", ua.as_str())?;
        }
        let fields: String = headers
            .iter()
            .filter(|(k, _)| !k.eq_ignore_ascii_case("user-agent"))
            .map(|(k, v)| format!("{k}: {v}"))
            .collect::<Vec<_>>()
            .join(",");
        self.mpv.set_property("http-header-fields", fields.as_str())?;
        Ok(())
    }

    /// Apply a per-track loudness gain (dB) as an mpv `volume` audio filter. Kept
    /// YouTube-agnostic: the caller computes the gain from `loudnessDb` (see `state::loudness_gain`);
    /// this just applies whatever dB it's handed.
    ///
    /// `af` is a **global** mpv property, not a per-playlist-entry one, so a gaplessly-advanced
    /// track keeps whatever the last [`Self::load`] set. The orchestrator has to call this itself
    /// on a gapless advance or every track after the first plays at the first track's gain.
    // Note: set on advance, so the head of a gapless track carries the old gain for the event
    // round-trip (a few ms) and the filter chain reinits mid-stream. If that ever clicks audibly,
    // keep one labelled filter (`af=@gain:lavfi=[volume=0dB]`) and retune it with `af-command`.
    pub fn set_gain(&self, gain_db: Option<f64>) -> Result<(), Error> {
        self.af.lock().0 = gain_db;
        self.apply_af()
    }

    /// Tempo, 0.25–2.0. Pitch is unaffected: `audio-pitch-correction` (mpv's default) time-stretches
    /// rather than resamples, so this is Metrolist's `PlaybackParameters.speed` exactly.
    pub fn set_speed(&self, speed: f64) -> Result<(), Error> {
        self.mpv.set_property("speed", speed.clamp(0.25, 2.0))?;
        Ok(())
    }

    /// Apply the full [`AudioFx`] set. Speed is mpv's own `speed` property; the pitch shift and
    /// the reverb/bass/width filters share the one `af` chain, applied together with the loudness
    /// gain. Pitch is the only part that can fail — a libmpv built without librubberband rejects
    /// the whole chain — so a rejection rolls the fx back to what was playing and re-applies it,
    /// exactly as the old pitch setter did, and surfaces [`Error::NoPitchFilter`] to the user.
    // Note: native `rubberband` only. If a Windows/macOS build ever turns up without it, wire the
    // `lavfi=[rubberband=pitch=...]` fallback here.
    pub fn set_fx(&self, fx: &AudioFx) -> Result<(), Error> {
        self.set_speed(fx.speed)?;
        let previous = std::mem::replace(&mut self.af.lock().1, *fx);
        if let Err(e) = self.apply_af() {
            // No librubberband: mpv rejected the *whole* chain, loudness gain included, so put the
            // old fx back rather than leave every later set_gain failing. (mpv never applied the
            // bad chain, so this restores what is already playing.)
            self.af.lock().1 = previous;
            let _ = self.apply_af();
            return Err(if fx.semitones == 0 { e } else { Error::NoPitchFilter });
        }
        Ok(())
    }

    fn apply_af(&self) -> Result<(), Error> {
        let (gain_db, fx) = *self.af.lock();
        self.mpv.set_property("af", af_chain(gain_db, &fx).as_str())?;
        Ok(())
    }
}

/// The whole `af` chain: loudness gain, then the [`AudioFx`] filters in order — pitch
/// (rubberband), reverb (`aecho`), bass shelf (`bass`), stereo width (`stereowiden`). Empty when
/// nothing is in play, so the default path stays exactly the filterless one it was before any of
/// this existed. Each effect is omitted at its neutral value, so a lone gain or a lone reverb is
/// just that one filter.
fn af_chain(gain_db: Option<f64>, fx: &AudioFx) -> String {
    let mut chain = Vec::new();
    if let Some(g) = gain_db {
        chain.push(format!("lavfi=[volume={g}dB]"));
    }
    if fx.semitones != 0 {
        // Semitones → frequency multiplier (equal temperament).
        chain.push(format!("{}=pitch-scale={}", pitch_filter(), 2f64.powf(fx.semitones as f64 / 12.0)));
    }
    if fx.reverb > 0.0 {
        // Three taps at 40/80/120 ms; their decays fall 0.4/0.3/0.2 at full depth, scaled down
        // toward silence as the depth drops.
        chain.push(format!(
            "lavfi=[aecho=0.8:0.9:40|80|120:{}|{}|{}]",
            0.4 * fx.reverb,
            0.3 * fx.reverb,
            0.2 * fx.reverb
        ));
    }
    if fx.bass_db != 0.0 {
        chain.push(format!("lavfi=[bass=g={}:f=110:w=0.6]", fx.bass_db));
    }
    if fx.width > 0.0 {
        // Widen the stereo image; drymix falls from 1 toward 0.4 as width rises, mixing in more
        // of the widened signal.
        chain.push(format!(
            "lavfi=[stereowiden=delay=20:feedback=0.3:crossfeed=0.3:drymix={}]",
            1.0 - 0.6 * fx.width
        ));
    }
    chain.join(",")
}

/// Test seam. Set it to reproduce a libmpv built without librubberband: mpv then rejects the whole
/// `af` chain, loudness gain included, which is the failure [`Player::set_fx`] rolls back from.
/// A machine that has the filter can't reach that path any other way. Not compiled into the app.
#[cfg(test)]
static NO_RUBBERBAND: std::sync::atomic::AtomicBool = std::sync::atomic::AtomicBool::new(false);

fn pitch_filter() -> &'static str {
    #[cfg(test)]
    if NO_RUBBERBAND.load(std::sync::atomic::Ordering::Relaxed) {
        return "rubberband_this_build_does_not_have";
    }
    "rubberband"
}

fn event_loop(
    mut ev: EventContext,
    tx: tokio::sync::mpsc::UnboundedSender<PlayerEvent>,
    idle_active: Arc<AtomicBool>,
) {
    // Playback state is derived from two properties, never polled: mpv answers `mpv_get_property`
    // synchronously on its core lock, so asking it from the app's async event pump can stall that
    // pump exactly when mpv is busiest (a gapless transition opening the next stream) — and a
    // stalled pump stops draining mpv's events, so track-end is never handled and playback wedges.
    // These arrive as events; nothing has to ask.
    //
    // mpv reports the initial value of an observed property immediately, so both are seeded here
    // before anything is loaded: `pause: false`, `idle-active: true` ⇒ not playing.
    let mut paused = false;
    let mut idle = true;
    let mut playing = false;
    loop {
        match ev.wait_event(1.0) {
            Some(Ok(event)) => {
                let out = match event {
                    Event::PropertyChange {
                        name: "time-pos",
                        change: PropertyData::Double(p),
                        ..
                    } => Some(PlayerEvent::Position(p)),
                    Event::PropertyChange {
                        name: "duration",
                        change: PropertyData::Double(d),
                        ..
                    } => Some(PlayerEvent::Duration(d)),
                    Event::PropertyChange {
                        name: "pause", change: PropertyData::Flag(p), ..
                    } => {
                        paused = p;
                        None
                    }
                    Event::PropertyChange {
                        name: "idle-active",
                        change: PropertyData::Flag(i),
                        ..
                    } => {
                        idle = i;
                        idle_active.store(i, Ordering::Release);
                        None
                    }
                    Event::EndFile(reason) => match reason as i32 {
                        EOF => Some(PlayerEvent::TrackEnded),
                        // STOP/QUIT/REDIRECT are deliberate (loadfile replace, shutdown) — ignore.
                        // ERROR never reaches this arm: libmpv2 surfaces end-file-with-error as
                        // Err from wait_event (see below).
                        _ => None,
                    },
                    _ => None,
                };
                if let Some(e) = out {
                    // Receiver dropped ⇒ player gone ⇒ stop the thread.
                    if tx.send(e).is_err() {
                        break;
                    }
                }
                // A gapless advance never touches either property, so no spurious stop/start is
                // emitted between tracks.
                let now = !paused && !idle;
                if now != playing {
                    playing = now;
                    if tx.send(PlayerEvent::Playing(now)).is_err() {
                        break;
                    }
                }
            }
            Some(Err(e)) => {
                // libmpv2 routes MPV_EVENT_END_FILE with an error (dead URL, 403, bad format)
                // through here instead of Event::EndFile — in our usage (no async get/set/command
                // replies) an Err from wait_event *is* a failed track.
                if tx.send(PlayerEvent::TrackFailed(friendly_error(&e))).is_err() {
                    break;
                }
            }
            None => {}
        }
    }
}

/// Quote a filename/URL for mpv's command parser.
///
/// libmpv2's `command` builds one space-joined string and hands it to `mpv_command_string`, which
/// splits it back apart on whitespace. So `loadfile /music/My music/a, b.mp3 replace` reaches mpv
/// as six arguments and fails with INVALID_PARAMETER (-4) — which is every local file whose path
/// has a space in it. Inside double quotes mpv only treats `\` specially, so escaping those two
/// characters is the whole job (verified against libmpv: quotes, commas, `$` and backslashes all
/// round-trip byte for byte through `playlist/0/filename`).
fn quoted(arg: &str) -> String {
    format!("\"{}\"", arg.replace('\\', "\\\\").replace('"', "\\\""))
}

/// Per-file `force-media-title` for `loadfile`'s options argument. Two parsers see it: the
/// command-string parser (libmpv2 joins the args into one line) needs the token quoted, and the
/// comma-separated option list inside needs the value length-prefixed (`%len%...`), which takes
/// any bytes verbatim. A per-file option also leaves the gapless lookahead's title untouched.
fn title_option(title: &str) -> String {
    quoted(&format!("force-media-title=%{}%{}", title.len(), title))
}

/// Slider percent → mpv `volume` value, over a 60 dB range. mpv applies gain = (v/100)³,
/// i.e. 60·log10(v/100) dB, so v = 100·10^(−(1−s/100)^1.5) yields −60·(1−s/100)^1.5 dB:
/// 50% is −21 dB, 25% is −39 dB, 1% is −59 dB. 0 stays a hard mute.
///
/// The 1.5 exponent buys the low end its range without moving anyone's saved setting much
/// (the old linear-in-dB curve put 50% at −20 dB, this one at −21). Steps are 0.9 dB at the
/// quiet end and 0.3 dB near the top, which is the right way round: fine control is wanted
/// where a dB is loud, and the bottom of the slider needs to reach somewhere quiet.
fn perceptual_to_mpv(percent: i64) -> f64 {
    if percent <= 0 {
        return 0.0;
    }
    100.0 * 10f64.powf(-(1.0 - percent.min(100) as f64 / 100.0).powf(1.5))
}

#[cfg(test)]
mod tests {
    use super::{af_chain, perceptual_to_mpv, quoted, AudioFx};

    #[test]
    fn chain_builds_each_effect() {
        // All off: the filterless path is preserved exactly.
        assert_eq!(af_chain(None, &AudioFx::default()), "");
        // Gain alone.
        assert_eq!(af_chain(Some(-3.5), &AudioFx::default()), "lavfi=[volume=-3.5dB]");
        // Pitch alone: one semitone up is the twelfth root of two, an octave up is ×2.
        assert_eq!(af_chain(None, &fx(0.0, 0.0, 0.0, 12)), "rubberband=pitch-scale=2");
        assert!(af_chain(None, &fx(0.0, 0.0, 0.0, 1)).ends_with("1.0594630943592953"));
        // Reverb alone: decays 0.4/0.3/0.2 scaled by the depth.
        assert_eq!(af_chain(None, &fx(1.0, 0.0, 0.0, 0)), "lavfi=[aecho=0.8:0.9:40|80|120:0.4|0.3|0.2]");
        assert_eq!(af_chain(None, &fx(0.5, 0.0, 0.0, 0)), "lavfi=[aecho=0.8:0.9:40|80|120:0.2|0.15|0.1]");
        // Bass alone.
        assert_eq!(af_chain(None, &fx(0.0, 8.0, 0.0, 0)), "lavfi=[bass=g=8:f=110:w=0.6]");
        // Width alone: drymix = 1 − 0.6·width.
        assert_eq!(
            af_chain(None, &fx(0.0, 0.0, 0.5, 0)),
            "lavfi=[stereowiden=delay=20:feedback=0.3:crossfeed=0.3:drymix=0.7]"
        );
        // Everything at once, in chain order: gain, pitch, reverb, bass, width.
        assert_eq!(
            af_chain(Some(-6.0), &fx(1.0, -6.0, 0.5, -12)),
            "lavfi=[volume=-6dB],rubberband=pitch-scale=0.5,\
             lavfi=[aecho=0.8:0.9:40|80|120:0.4|0.3|0.2],\
             lavfi=[bass=g=-6:f=110:w=0.6],\
             lavfi=[stereowiden=delay=20:feedback=0.3:crossfeed=0.3:drymix=0.7]"
        );
    }

    /// A convenience constructor: every field but the four under test at its neutral value.
    fn fx(reverb: f64, bass_db: f64, width: f64, semitones: i32) -> AudioFx {
        AudioFx { speed: 1.0, semitones, reverb, bass_db, width }
    }

    /// Everything above is string-building; this drives a real libmpv and reads `af` back out of
    /// it, because the questions that matter ("is the gain still in the chain", "what does mpv keep
    /// when it rejects a chain") are answered by mpv, not by us. Nothing is played, so no audio
    /// device is opened. One test rather than four: `NO_RUBBERBAND` is process-global and cargo
    /// runs tests in parallel.
    #[test]
    fn mpv_keeps_the_gain_through_pitch_changes_and_failures() {
        use super::{AudioFx, Error, Player, NO_RUBBERBAND};
        use std::sync::atomic::Ordering;

        let dir = std::env::temp_dir().join("ryotunes-af-test");
        std::fs::create_dir_all(&dir).unwrap();
        let p = Player::new(dir.to_str().unwrap()).expect("libmpv");
        let af = || p.mpv.get_property::<String>("af").unwrap();
        let pitch = |semitones| AudioFx { semitones, ..AudioFx::default() };

        // 1. Loudness normalization, then a pitch round trip. The gain has to survive both steps.
        p.set_gain(Some(-7.7)).unwrap();
        assert!(af().contains("volume=-7.7dB"), "gain missing: {}", af());
        p.set_fx(&pitch(2)).unwrap();
        assert!(af().contains("volume=-7.7dB"), "pitch dropped the gain: {}", af());
        assert!(af().contains("rubberband"), "pitch missing: {}", af());
        p.set_fx(&pitch(0)).unwrap();
        assert!(af().contains("volume=-7.7dB"), "reset dropped the gain: {}", af());
        assert!(!af().contains("rubberband"), "pitch 0 left a filter behind: {}", af());

        // 2. Gapless advance: the orchestrator retunes the gain for the next track. A pitch the
        // user set must not fall out of the chain when it does.
        p.set_fx(&pitch(-5)).unwrap();
        p.set_gain(Some(-2.5)).unwrap();
        assert!(af().contains("volume=-2.5dB"), "retune missed: {}", af());
        assert!(af().contains("rubberband"), "retune dropped the pitch: {}", af());
        p.set_fx(&pitch(0)).unwrap();

        // 3. A libmpv without librubberband. mpv rejects the chain wholesale, so this is also the
        // case where loudness normalization could silently disappear.
        let before = af();
        NO_RUBBERBAND.store(true, Ordering::Relaxed);
        let err = p.set_fx(&pitch(3)).unwrap_err();
        NO_RUBBERBAND.store(false, Ordering::Relaxed);
        // The user is told, in words. mpv's own answer is `Raw(-9)`, which says nothing.
        assert!(matches!(err, Error::NoPitchFilter), "rejection must surface: {err}");
        assert_eq!(err.to_string(), "Pitch shifting isn't available in this build");
        // mpv never applied the bad chain, and the rollback re-applied the good one either way.
        assert_eq!(af(), before, "a rejected pitch changed the live chain");
        assert!(af().contains("volume=-2.5dB"), "normalization lost: {}", af());
        // And the rolled-back state is clean: the next per-track retune is gain-only.
        p.set_gain(Some(-4.0)).unwrap();
        let after = af(); // mpv hands the chain back in its own escaped form, hence `contains`
        assert!(after.contains("volume=-4dB"), "retune after a rejection failed: {after}");
        assert!(!after.contains("rubberband"), "stored pitch survived the rollback: {after}");

        // 4. The reverb/bass/width filters reach the live chain and mpv accepts their syntax
        // (set_property would error otherwise — this is the only proof aecho/bass/stereowiden
        // parse on this build).
        p.set_gain(None).unwrap();
        p.set_fx(&AudioFx { speed: 1.0, semitones: 0, reverb: 1.0, bass_db: 8.0, width: 0.8 }).unwrap();
        let live = af();
        assert!(live.contains("aecho"), "reverb missing from live chain: {live}");
        assert!(live.contains("bass"), "bass missing from live chain: {live}");
        assert!(live.contains("stereowiden"), "width missing from live chain: {live}");
    }

    #[test]
    fn paths_survive_mpvs_command_parser() {
        // The bug this exists for: a space used to end the argument.
        assert_eq!(quoted("/music/My music/a, b.mp3"), "\"/music/My music/a, b.mp3\"");
        // Only backslash and double quote mean anything inside the quotes.
        assert_eq!(quoted(r#"/m/say "hi".mp3"#), r#""/m/say \"hi\".mp3""#);
        assert_eq!(quoted(r"C:\Music\x.mp3"), r#""C:\\Music\\x.mp3""#);
        // A stream URL is unchanged apart from the wrapper.
        assert_eq!(quoted("https://x/y?a=1&b=2"), "\"https://x/y?a=1&b=2\"");
    }

    #[test]
    fn volume_curve() {
        let db = |s| 60.0 * (perceptual_to_mpv(s) / 100.0).log10();
        assert_eq!(perceptual_to_mpv(0), 0.0); // hard mute, not just very quiet
        assert_eq!(perceptual_to_mpv(100), 100.0);
        assert!((db(50) + 21.21).abs() < 0.01);
        // The point of the curve: 1% has somewhere to go. The old 40 dB range bottomed out
        // here, which left anyone listening quietly pinned to the floor.
        assert!((db(1) + 59.10).abs() < 0.01);
        // Monotonic, and finer steps at the loud end than the quiet one.
        assert!((1..=100).all(|s| perceptual_to_mpv(s) > perceptual_to_mpv(s - 1)));
        assert!(db(100) - db(99) < db(2) - db(1));
    }
}
