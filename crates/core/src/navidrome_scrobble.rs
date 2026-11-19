//! Reporting a Navidrome listen to the server, the way [`crate::lastfm`] reports one to Last.fm.
//!
//! Two sends per play: "now playing" when the track starts (`submission=false`, which never touches
//! the play count) and one submission once the track passes the scrobble threshold, after which the
//! latch stays shut — a listen counted twice would double the server's play counts.
//!
//! The state is bare here: this module decides *when* and never talks to the network, so the timing
//! rules are testable without a server, a queue or a Tokio runtime. Best-effort throughout: a failed
//! send is a `debug!` line in the caller, never a user-facing error and never a blocked playback.
//!
//! The threshold is [`crate::lastfm::crosses_threshold`] on purpose: one rule means the two
//! scrobblers can never disagree about what counts as a play.

/// The track in flight on a Navidrome server. Everything is reset by [`Scrobbler::start`].
#[derive(Default)]
pub struct Scrobbler {
    /// The bare server id (no `nd:` prefix) while the running track is one of the server's.
    id: Option<String>,
    /// Epoch secs when it started — the submission's `time`, in milliseconds.
    started_at: i64,
    submitted: bool,
}

impl Scrobbler {
    /// A track change. Returns the bare song id when the new track is the server's — the caller
    /// sends it as "now playing" — and `None` for every other track, which also clears the latch so
    /// a resumed server track starts a fresh listen.
    pub fn start(&mut self, video_id: &str, now: i64) -> Option<String> {
        let id = crate::spotify::nd_track_id(video_id).map(str::to_owned);
        self.started_at = now;
        self.submitted = false;
        self.id = id.clone();
        id
    }

    /// A position tick. Returns the listen to submit — bare id + start time in Unix millis — the
    /// first time the threshold is crossed, and nothing at any other tick.
    ///
    /// The latch closes even when the caller's send fails, matching Last.fm: the tick repeats about
    /// once a second, and retrying would submit the same listen many times.
    pub fn tick(&mut self, pos: f64, duration: f64) -> Option<(String, i64)> {
        let id = self.id.as_ref()?;
        if self.submitted || !crate::lastfm::crosses_threshold(pos, duration) {
            return None;
        }
        self.submitted = true;
        Some((id.clone(), self.started_at * 1000))
    }
}

#[cfg(test)]
mod tests {
    use super::Scrobbler;

    const START: i64 = 1_700_000_000;

    #[test]
    fn one_play_is_submitted_exactly_once() {
        let mut s = Scrobbler::default();
        assert_eq!(s.start("nd:track:8f3a", START).as_deref(), Some("8f3a"));
        // Half of 200s is 100s.
        assert_eq!(s.tick(99.0, 200.0), None);
        assert_eq!(s.tick(100.0, 200.0), Some(("8f3a".to_owned(), START * 1000)));
        // Every later tick stays shut, however far the position runs.
        assert_eq!(s.tick(101.0, 200.0), None);
        assert_eq!(s.tick(200.0, 200.0), None);
    }

    #[test]
    fn a_non_navidrome_track_never_scrobbles() {
        let mut s = Scrobbler::default();
        assert_eq!(s.start("dQw4w9WgXcQ", START), None);
        assert_eq!(s.start("sc:track:123", START), None);
        assert_eq!(s.start("nd:album:9", START), None);
        assert_eq!(s.tick(500.0, 600.0), None);
    }

    #[test]
    fn a_new_track_rearms_the_latch() {
        let mut s = Scrobbler::default();
        s.start("nd:track:one", START);
        assert!(s.tick(100.0, 200.0).is_some());
        assert_eq!(s.tick(100.0, 200.0), None, "the same play is not resent");
        s.start("nd:track:two", START + 200);
        assert_eq!(s.tick(100.0, 200.0), Some(("two".to_owned(), (START + 200) * 1000)));
    }

    #[test]
    fn leaving_the_server_drops_its_pending_listen() {
        let mut s = Scrobbler::default();
        s.start("nd:track:one", START);
        s.start("dQw4w9WgXcQ", START);
        assert_eq!(s.tick(500.0, 600.0), None);
    }

    #[test]
    fn short_tracks_and_unknown_durations_follow_lastfm() {
        let mut s = Scrobbler::default();
        s.start("nd:track:short", START);
        assert_eq!(s.tick(19.0, 20.0), None, "a track under 30s never scrobbles");

        // mpv has not reported a duration yet: only the 4-minute rule applies.
        s.start("nd:track:long", START);
        assert_eq!(s.tick(239.0, 0.0), None);
        assert!(s.tick(240.0, 0.0).is_some());
    }
}
