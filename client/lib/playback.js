.pragma library

// The daemon-event -> state reducer, ported from ui/src/lib/player.svelte.ts (initApp, lines
// ~916-1009) and setPlaybackPosition. It lives in a pure module so the QtTest case can exercise it
// without loading Quickshell (Playback imports Daemon, which imports Quickshell). Playback.qml holds
// the reactive state and calls applyEvent(root, name, data) from its Daemon.event handler; the same
// function mutates a plain object in the test. Side effects the daemon surfaces as toasts/errors are
// returned as a descriptor (or null) for the caller to route, since a JS module cannot emit signals.

// "2:53" -> 173. Undefined for a missing or malformed duration string.
function durationToSeconds(d) {
    if (!d) return undefined;
    var parts = String(d).split(":").map(Number);
    if (!parts.length || parts.some(function (n) { return isNaN(n); })) return undefined;
    return parts.reduce(function (a, b) { return a * 60 + b; }, 0);
}

// mpv's authoritative transport sample, clamped to [0, duration].
function clampPosition(duration, position) {
    var max = duration > 0 ? duration : Infinity;
    return Math.max(0, Math.min(max, isFinite(position) ? position : 0));
}

// True while the seek thumb is held: a real number pins the shown position, NaN means released.
function dragging(v) {
    return typeof v === "number" && !isNaN(v);
}

// Apply one daemon event to `s`. Returns a { toast, kind } descriptor for the events that surface a
// message, else null. Event payload shapes are the daemon's, verified against ryotunesd: position
// and duration wrap their value in an object; playback-state and volume are bare scalars.
function applyEvent(s, name, data) {
    switch (name) {
    case "now-playing": {
        var n = data;
        var trackChanged = !s.now || s.now.videoId !== n.videoId;
        s.now = n;
        if (trackChanged) {
            var d = durationToSeconds(n.duration);
            s.duration = (d === undefined) ? 0 : d;
            s.position = clampPosition(s.duration, 0);
        }
        s.pendingVideoId = null;
        s.lastError = null;
        // Initial row snapshot; a backend rating refresh may correct it.
        s.rating = n.rating || "indifferent";
        return null;
    }
    case "rating":
        if (s.now && s.now.videoId === data.videoId) s.rating = data.rating;
        return null;
    case "queue-changed":
        s.queue = data;
        return null;
    case "queue-index": {
        // The items did not change, so keep the array already held and patch the rest. Splice the
        // playing row back in: start_current backfills its duration/artists after the stream
        // resolves, and that repair rides on this event rather than a whole new queue.
        var items = (s.queue && s.queue.items) ? s.queue.items : [];
        if (data.current && items[data.currentIndex] !== undefined) items[data.currentIndex] = data.current;
        s.queue = {
            items: items,
            currentIndex: data.currentIndex,
            playedFrom: data.playedFrom,
            shuffle: data.shuffle,
            repeat: data.repeat,
            sourceName: data.sourceName
        };
        return null;
    }
    case "position":
        // Not while our own seek drag is in flight: the pointer already moved past this sample.
        if (!dragging(s.seekDrag)) s.position = clampPosition(s.duration, data.position);
        return null;
    case "duration":
        s.duration = data.duration;
        return null;
    case "playback-state":
        s.paused = (data === "paused");
        return null;
    case "stop-after-current":
        s.stopAfterCurrent = data;
        return null;
    case "volume":
        // Not while the volume slider drags: the echo is a value the pointer moved past already.
        if (!s.volDrag) s.volume = data;
        return null;
    case "audio-fx":
        // The daemon's authoritative effects, echoed after every apply and on subscribe.
        s.audioFx = data;
        return null;
    case "provider-changed":
        // The active provider, echoed when it switches; shape is { provider }.
        s.provider = data.provider;
        return null;
    case "spotify-auth": {
        // The Spotify OAuth flow's progress, surfaced by the daemon; no provider-changed rides with
        // it. The url step is opened in the browser by the caller (a JS library cannot reach
        // Quickshell), so it returns an openUrl the Playback singleton hands to execDetached.
        var st = (data && data.state) ? data.state : "";
        if (st === "url") {
            s.spotify = Object.assign({}, s.spotify, { flow: "browser", error: "" });
            return { toast: "Opening Spotify sign-in in your browser", kind: "info", openUrl: data.url };
        }
        if (st === "signed_in") {
            s.spotify = { signedIn: true, stored: true, name: (data && data.name) ? data.name : null,
                premium: (s.spotify && s.spotify.premium !== undefined) ? s.spotify.premium : null,
                flow: "", error: "" };
            return { toast: "Signed in to Spotify" + ((data && data.name) ? (" as " + data.name) : ""), kind: "success" };
        }
        if (st === "signed_out") {
            s.spotify = { signedIn: false, stored: false, name: null, premium: null };
            return null;
        }
        if (st === "error") {
            var msg = (data && data.message) ? data.message : "Spotify sign-in failed";
            // Keep the reason on the gate: a toast alone reads as "nothing happened".
            s.spotify = Object.assign({}, s.spotify, { flow: "", error: msg });
            return { toast: msg, kind: "error" };
        }
        return null;
    }
    case "playback-error":
        s.lastError = (data && data.message !== undefined) ? data.message : String(data);
        s.pendingVideoId = null;
        return { toast: s.lastError, kind: "error" };
    case "playback-notice": // auto-skipped an unplayable track
        return { toast: (data && data.message !== undefined) ? data.message : String(data), kind: "info" };
    case "cover-error": // playlist artwork YouTube would not take
        return { toast: (data && data.message !== undefined) ? data.message : String(data), kind: "error" };
    case "auth-changed":
        s.auth = { signedIn: !!data.signedIn, name: data.name, avatar: data.avatar };
        return null;
    case "lt-state":
        s.lt = data;
        return null;
    case "lt-notice":
        return { toast: String(data), kind: "info" };
    default:
        return null;
    }
}
