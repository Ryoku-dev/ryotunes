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

    // --- events + opening snapshot -----------------------------------------------------------
    Connections {
        target: Daemon
        function onEvent(name, data) {
            var fx = PB.applyEvent(root, name, data);
            if (fx) root.toast(fx.toast, fx.kind);
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
    }

    // --- methods (each a daemon call) --------------------------------------------------------
    function play(item) { return Daemon.call("play", { item: item }); }
    function playIndex(i) { return Daemon.call("play_index", { index: i }); }
    function togglePause() { return Daemon.call("toggle_pause"); }
    function next() { return Daemon.call("next_track"); }
    function prev() { return Daemon.call("prev_track"); }
    function seek(secs) { return Daemon.call("seek", { position: secs }); }
    function setVolume(v) { return Daemon.call("set_volume", { volume: v }); }
    function toggleShuffle() { return Daemon.call("toggle_shuffle"); }
    // off -> all -> one -> off, matching player.svelte.ts cycleRepeat.
    function cycleRepeat() {
        var r = (root.queue && root.queue.repeat) ? root.queue.repeat : "off";
        return Daemon.call("set_repeat", { mode: r === "off" ? "all" : r === "all" ? "one" : "off" });
    }
}
