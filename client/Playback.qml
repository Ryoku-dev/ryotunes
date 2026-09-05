pragma Singleton
import QtQuick
import Quickshell
import "lib/playback.js" as PB

// The client's only playback state: a mirror of the daemon, never a source of truth. Every event
// from Daemon is applied by lib/playback.js applyEvent (ported from player.svelte.ts); every method
// is a daemon call. The opening snapshot from `subscribe` seeds the same properties. Optimistic UI
// is confined to `seekDrag` (the held seek thumb) and `volDrag` (the held volume slider).
Singleton {
    id: root

    // --- mirrored state ----------------------------------------------------------------------
    property var now: null
    // The playing cover's sampled accent (components/ArtAccent); transparent until sampled.
    property color artAccent: "transparent"
    property var queue: ({ items: [], currentIndex: 0 })
    property real position: 0
    property real duration: 0
    property bool paused: false
    property int volume: 100
    property bool stopAfterCurrent: false
    property string rating: "indifferent"
    property var pendingVideoId: null
    property var lastError: null
    property var lyrics: ({ synced: false, lines: [] })
    property var auth: ({ signedIn: false })
    property var settings: ({})
    property var lt: ({ role: "none" })
    // The Sound dialog's effects, mirrored from the daemon (the `audio-fx` event and the subscribe
    // snapshot). `bass` is dB, reverb/width are 0..1, speed is the tempo multiplier.
    property var audioFx: ({ speed: 1, semitones: 0, reverb: 0, bass: 0, width: 0 })
    // The active music provider ("youtube" | "spotify" | "soundcloud"), mirrored from the daemon
    // (the subscribe snapshot's `provider` and a `provider-changed` event). The title bar's switch
    // reads it; SoundCloud is a guest catalogue with no sign-in gate.
    property string provider: "youtube"
    // The Spotify account, mirrored from the daemon (the subscribe snapshot's `spotify` object and
    // the `spotify-auth` events). `signedIn` gates the Spotify catalogue; `premium` is null until the
    // profile is known. Never a source of truth — every field lands from the daemon.
    property var spotify: ({ signedIn: false, stored: false, name: null, premium: null })

    // --- SoundCloud waveform cache -----------------------------------------------------------
    // The playing track's 240 amplitude samples (get_waveform), for the Orange seek bar; null for a
    // non-SoundCloud track or before it resolves. Cached per video id so switching back to a track
    // (or reopening the stage) never refetches. Only SoundCloud ids ("sc:") carry a waveform.
    property var waveform: null
    property var waveformCache: ({})

    // --- optimistic drag state ---------------------------------------------------------------
    // NaN when the seek thumb is not held; a number pins the shown position and suppresses the
    // daemon's position echoes until release.
    property real seekDrag: NaN
    // The position the UI should render: the held thumb while dragging, else the live sample.
    readonly property real shownPosition: isNaN(root.seekDrag) ? root.position : root.seekDrag
    // True while the volume slider is held, so a volume echo cannot yank the thumb backwards.
    property bool volDrag: false

    // A message the daemon surfaced (error/notice/cover-error/lt-notice), for the toast layer.
    signal toast(string message, string kind)
    // A page asking the App to open the Now Playing overlay on a tab ("queue" | "lyrics").
    signal nowPlayingRequested(string tab)
    // A surface (player-bar tools, track menu) asking the App to open the Sound dialog.
    signal soundRequested()

    // --- events + opening snapshot -----------------------------------------------------------
    Connections {
        target: Daemon
        function onEvent(name, data) {
            var fx = PB.applyEvent(root, name, data);
            if (fx) {
                if (fx.toast) root.toast(fx.toast, fx.kind);
                // The spotify-auth url step asks the browser to open the OAuth page.
                if (fx.openUrl) Quickshell.execDetached(["xdg-open", fx.openUrl]);
            }
        }
        function onSnapshot(snap) { root.loadSnapshot(snap); }
    }

    // Seed state from the { playback, queue, settings, auth } reply the daemon sends on subscribe
    // (and re-sends after every reconnect), the socket equivalent of frontend_ready's resync.
    function loadSnapshot(snap) {
        if (!snap) return;
        if (snap.queue) root.queue = snap.queue;
        var pb = snap.playback;
        if (pb) {
            root.volume = pb.volume;         // before the now guard: the slider is stale either way
            if (!root.now) {                 // a real now-playing event may have beaten the snapshot
                root.now = pb.now;
                root.rating = (pb.now && pb.now.rating) ? pb.now.rating : "indifferent";
                root.paused = pb.paused;
                root.duration = pb.duration;
                root.position = PB.clampPosition(pb.duration, pb.position);
                root.stopAfterCurrent = pb.stopAfterCurrent || false;
            }
        }
        if (snap.settings) root.settings = snap.settings;
        if (snap.auth) root.auth = { signedIn: !!snap.auth.signedIn, name: snap.auth.name, avatar: snap.auth.avatar };
        if (snap.audioFx) root.audioFx = snap.audioFx;
        if (snap.provider) root.provider = snap.provider;
        if (snap.spotify)
            root.spotify = { signedIn: !!snap.spotify.signedIn, stored: !!snap.spotify.stored,
                name: snap.spotify.name, premium: snap.spotify.premium };
    }

    // --- SoundCloud waveform -----------------------------------------------------------------
    // On every track change resolve a SoundCloud track's waveform (get_waveform) into the per-id
    // cache and publish it as `waveform`; a non-SoundCloud track clears it. A cached id is served
    // straight away and never refetched. onNowChanged also covers the opening snapshot's `now`.
    onNowChanged: root.ensureWaveform(root.now ? root.now.videoId : null)
    function ensureWaveform(vid) {
        if (!vid || String(vid).indexOf("sc:") !== 0) {
            root.waveform = null;
            return;
        }
        if (root.waveformCache[vid]) {
            root.waveform = root.waveformCache[vid];
            return;
        }
        root.waveform = null;
        Daemon.call("get_waveform", { id: vid })
            .then((r) => {
                var s = (r && r.samples) ? r.samples : [];
                root.waveformCache[vid] = s;
                if (root.now && root.now.videoId === vid)
                    root.waveform = s;
            })
            .catch(() => {});
    }

    // --- methods (each a daemon call) --------------------------------------------------------
    function play(item) { return Daemon.call("play", { item: item }); }
    function playIndex(i) { return Daemon.call("play_index", { index: i }); }
    function togglePause() { return Daemon.call("toggle_pause"); }
    function next() { return Daemon.call("next_track"); }
    function prev() { return Daemon.call("prev_track"); }
    function seek(secs) { return Daemon.call("seek", { position: secs }); }
    function setVolume(v) { return Daemon.call("set_volume", { volume: v }); }
    // Send the Sound dialog's effects to the daemon; its audio-fx echo updates root.audioFx.
    function setAudioFx(fx) {
        return Daemon.call("set_audio_fx", { speed: fx.speed, semitones: fx.semitones, reverb: fx.reverb, bass: fx.bass, width: fx.width });
    }
    // Optimistic: the switch flips the local mirror at once and asks the daemon. The daemon method
    // lands later, so a failure is swallowed rather than surfaced.
    function setProvider(p) {
        root.provider = p;
        return Daemon.call("set_provider", { provider: p }).catch(() => {});
    }
    // Start the Spotify OAuth flow; the daemon spawns the task and drives the rest through
    // spotify-auth events (the url step opens the browser). Idempotent while a flow is running.
    function spotifySignIn() { return Daemon.call("spotify_sign_in"); }
    function spotifySignOut() { return Daemon.call("spotify_sign_out"); }
    function toggleShuffle() { return Daemon.call("toggle_shuffle"); }
    // off -> all -> one -> off, matching player.svelte.ts cycleRepeat.
    function cycleRepeat() {
        var r = (root.queue && root.queue.repeat) ? root.queue.repeat : "off";
        return Daemon.call("set_repeat", { mode: r === "off" ? "all" : r === "all" ? "one" : "off" });
    }
}
