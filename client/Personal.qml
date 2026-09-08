pragma Singleton
import QtQuick
import Quickshell
import "lib/personal.js" as P
import "lib/ids.js" as Ids

// The shared personal store, mirrored from the daemon (AppState::{personal,set_personal}). It moved
// off the Svelte app's browser localStorage so a native client can read it too. `get_personal` seeds
// the mirror on every (re)subscribe and the `personal-changed` event keeps it live, so a second
// client follows along. Mutations are held until hydration, then persisted immediately
// through serialized writes. All shape logic stays in lib/personal.js.
Singleton {
    id: root

    // The whole blob, always a full personal object, plus the slices Home reads.
    property var blob: P.empty()
    readonly property var picks: (root.blob && root.blob.picks) ? root.blob.picks : []
    readonly property var pins: (root.blob && root.blob.pins) ? root.blob.pins : []
    readonly property var homeArrange: (root.blob && root.blob.home) ? root.blob.home
        : ({ order: [], hidden: [], seen: [] })

    // Recording state kept out of the persisted blob. `lastNowId` is the last track we recorded a
    // play for, so a repeated now-playing for the same track (a metadata refresh) or a reconnect —
    // which replays state through the snapshot, never a now-playing event — is not a fresh play.
    // `wasSignedIn`/`wasSpotifyIn` remember each session's state so only a real sign-out transition
    // sweeps its recents. `pendingSaves` holds the blobs of set_personal writes still awaiting their
    // personal-changed echo, so the daemon's echo of our own save can be dropped instead of applied.
    property string lastNowId: ""
    property bool wasSignedIn: false
    property bool wasSpotifyIn: false
    property var pendingSaves: []
    property bool hydrated: false
    property bool dirty: false
    property bool saving: false
    property int revision: 0
    property int reloadId: 0
    property var deferredMutations: []

    // The "Jump back in" rail: recents newest-first, minus anything already a shortcut (otherwise
    // the two lists converge on the same handful of items in two shapes), capped at nine — three
    // full columns once the window is filtered.
    function recent(n) {
        var pinned = ({});
        for (var i = 0; i < root.picks.length; i++)
            pinned[root.picks[i].id] = true;
        var out = P.recentItems(root.blob, 100).filter(function (r) { return !pinned[r.id]; });
        return out.slice(0, n === undefined ? 9 : n);
    }
    function topArtistIds(n) { return P.topArtistIds(root.blob, n); }
    function firstArtist(s) { return P.firstArtist(s); }
    function isPinned(id) { return root.pins.indexOf(id) >= 0; }
    // Order feed sections by the user's saved Home arrangement (identity while it is empty).
    function arrange(sections) { return P.arrangeSections(sections, root.blob); }

    // --- live sync from the daemon -----------------------------------------------------------
    Connections {
        target: Daemon
        function onEvent(name, data) {
            if (name === "personal-changed") {
                // The daemon echoes every set_personal back to us. Applying our own echo is a no-op
                // at best and, if a mutation landed in the few ms since the save, reverts it — so
                // drop the echo. A genuine change from another client never matches and still applies.
                if (root.consumeSelfEcho(data))
                    return;
                root.apply(data);
            } else if (name === "now-playing") {
                root.onNowPlaying(data);
            }
        }
        // Fired on the subscribe reply (first connect and every reconnect) — reload the store then.
        function onSnapshot(snap) { root.reload(); }
    }
    // Seed the sign-out watchers from the current sessions (a singleton created after the auth
    // snapshot would otherwise miss the transition), then mirror the store if already connected.
    Component.onCompleted: {
        root.wasSignedIn = !!(Playback.auth && Playback.auth.signedIn);
        root.wasSpotifyIn = !!(Playback.spotify && Playback.spotify.signedIn);
        if (Daemon.connected) root.reload();
    }

    function reload() {
        var request = ++root.reloadId;
        var revision = root.revision;
        Daemon.call("get_personal")
            .then(function (r) {
                if (request !== root.reloadId)
                    return;
                if (!root.hydrated || root.revision === revision)
                    root.apply(r ? r.personal : null);
                root.save();
            })
            .catch(function () {});
    }

    // Replace the mirror from a daemon blob (a get_personal result or a personal-changed payload).
    // No account sweep here: purging must ride the real sign-out transition, not an initial
    // hydration. The auth snapshot can still hold its default signed-out value when the first blob
    // lands, and sweeping then would wrongly erase recents belonging to a session that is signed in.
    function apply(b) {
        if (root.hydrated && (root.dirty || root.saving))
            return;
        root.blob = P.hydrate(b);
        root.revision++;
        root.hydrated = true;
        var deferred = root.deferredMutations;
        root.deferredMutations = [];
        for (var i = 0; i < deferred.length; i++)
            root.mutate(deferred[i]);
    }

    // A track started playing: record it as a recent song, refresh any matching shortcut's recency
    // and count its artist, as player.svelte.ts does on `now-playing`. Guarded on a genuine track
    // change so a repeated event for the same track, or a reconnect (which replays through the
    // snapshot, never this event), is not counted as a new play. Radio carries no navigable song,
    // shortcut or artist page.
    function onNowPlaying(n) {
        if (!n || !n.videoId || Ids.isRadioId(n.videoId)) {
            root.lastNowId = "";
            return;
        }
        if (n.videoId === root.lastNowId)
            return;
        root.lastNowId = n.videoId;
        root.mutate(function (b) {
            var now = Date.now();
            var song = { kind: "song", id: n.videoId, title: n.title || "",
                subtitle: n.artists || "", thumbnail: n.thumbnail || "" };
            if (n.artistId) song.artistId = n.artistId;
            P.noteRecent(b, song, now);
            P.touchPick(b, n.videoId, now);
            if (n.artists)
                P.noteArtist(b, n.artistId ? n.artistId : n.artists, P.firstArtist(n.artists), now);
            return true;
        });
    }

    // --- reducers: mutate a clone, publish, persist ------------------------------------------
    // A JSON clone so the reassignment is a new object (QML change detection) and the pure reducer
    // never aliases the live blob's nested recent/artists maps.
    function mutate(fn) {
        if (!root.hydrated) {
            root.deferredMutations = root.deferredMutations.concat([fn]);
            return true;
        }
        var b = JSON.parse(JSON.stringify(root.blob));
        if (fn(b) === false)
            return false;
        root.blob = b;
        root.revision++;
        root.dirty = true;
        root.save();
        return true;
    }

    // Add to Shortcuts (evicting the tile gone longest unplayed when the grid is full). Returns
    // false when it was already there. This is the call the pages' pin controls make.
    function addPick(item) { return root.mutate(function (b) { return P.addPick(b, item, Date.now()); }); }
    function removePick(id) { return root.mutate(function (b) { P.removePick(b, id); return true; }); }
    function touchPick(id) { return root.mutate(function (b) { return P.touchPick(b, id, Date.now()); }); }
    function seedPick(item) { return root.mutate(function (b) { return P.seedPick(b, item, Date.now()); }); }
    function noteRecent(item) { return root.mutate(function (b) { P.noteRecent(b, item, Date.now()); return true; }); }
    // Signing out of an account drops only the recents that belong to *that* session (Home's "Jump
    // back in" must not keep offering a library the daemon can no longer fetch), and only on a real
    // signed-in -> signed-out transition — never on the initial snapshot, whose signed-out default
    // would otherwise erase a signed-in session's history. The two providers are independent.
    Connections {
        target: Playback
        function onAuthChanged(): void {
            var nowIn = !!(Playback.auth && Playback.auth.signedIn);
            if (root.wasSignedIn && !nowIn)
                root.mutate(function (b) { return P.forgetYouTubeRecents(b) > 0; });
            root.wasSignedIn = nowIn;
        }
        function onSpotifyChanged(): void {
            var nowIn = !!(Playback.spotify && Playback.spotify.signedIn);
            if (root.wasSpotifyIn && !nowIn)
                root.mutate(function (b) { return P.forgetSpotifyRecents(b) > 0; });
            root.wasSpotifyIn = nowIn;
        }
    }

    // pin / unpin, returning "pinned" | "unpinned" | "full" like personal.ts togglePin.
    function togglePin(id) {
        var result = "";
        root.mutate(function (b) {
            result = P.togglePin(b, id);
            return result !== "full";   // a refused pin changed nothing
        });
        return result;
    }
    function pin(id) { return root.isPinned(id) ? "pinned" : root.togglePin(id); }
    function unpin(id) { if (root.isPinned(id)) root.togglePin(id); }

    // Recognise (and drop) the personal-changed echo of a set_personal we sent. The daemon echoes
    // each save back in the order it received them, so the oldest pending blob is the one to match;
    // deepEqual ignores serde's key reordering. Anything else is a real change from another client.
    function consumeSelfEcho(blob) {
        for (var i = 0; i < root.pendingSaves.length; i++) {
            if (P.deepEqual(root.pendingSaves[i], blob)) {
                var pending = root.pendingSaves.slice();
                pending.splice(i, 1);
                root.pendingSaves = pending;
                return true;
            }
        }
        return false;
    }
    function dropPendingSave(snap) {
        var i = root.pendingSaves.indexOf(snap);
        if (i >= 0) {
            var arr = root.pendingSaves.slice();
            arr.splice(i, 1);
            root.pendingSaves = arr;
        }
    }

    // No close-time debounce gap and no overlapping full-blob writes. A new mutation
    // during an in-flight save is sent as soon as that save acknowledges.
    function save() {
        if (!root.hydrated || !root.dirty || root.saving || !Daemon.connected)
            return;
        var snap = root.blob;
        root.pendingSaves = root.pendingSaves.concat([snap]);
        root.saving = true;
        root.dirty = false;
        Daemon.call("set_personal", { personal: snap })
            .then(function () {
                root.saving = false;
                root.save();
            })
            .catch(function () {
                root.dropPendingSave(snap);
                root.saving = false;
                root.dirty = true;
                Playback.toast("Could not save your listening history and shortcuts", "error");
            });
    }
}
