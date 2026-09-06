pragma Singleton
import QtQuick
import Quickshell
import "lib/personal.js" as P
import "lib/ids.js" as Ids

// The shared personal store, mirrored from the daemon (AppState::{personal,set_personal}). It moved
// off the Svelte app's browser localStorage so a native client can read it too. `get_personal` seeds
// the mirror on every (re)subscribe and the `personal-changed` event keeps it live, so a second
// client — the mini window, another shell — follows along. Reducers mutate a clone, publish it
// optimistically for an instant UI, and persist through `set_personal` (debounced, because now-playing
// records recency on every track). All the shape logic is the pure port in lib/personal.js.
Singleton {
    id: root

    // The whole blob, always a full personal object, plus the slices Home reads.
    property var blob: P.empty()
    readonly property var picks: (root.blob && root.blob.picks) ? root.blob.picks : []
    readonly property var pins: (root.blob && root.blob.pins) ? root.blob.pins : []
    readonly property var homeArrange: (root.blob && root.blob.home) ? root.blob.home
        : ({ order: [], hidden: [], seen: [] })

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
            if (name === "personal-changed")
                root.apply(data);
            else if (name === "now-playing")
                root.onNowPlaying(data);
        }
        // Fired on the subscribe reply (first connect and every reconnect) — reload the store then.
        function onSnapshot(snap) { root.reload(); }
    }
    Component.onCompleted: if (Daemon.connected) root.reload();

    function reload() {
        Daemon.call("get_personal")
            .then(function (r) { root.apply(r ? r.personal : null); })
            .catch(function () {});
    }

    // Replace the mirror from a daemon blob (a get_personal result or a personal-changed payload).
    function apply(b) { root.blob = P.hydrate(b); }

    // A track started: refresh its shortcut's recency and count its artist, exactly as
    // player.svelte.ts does on `now-playing`. Radio stations carry no shortcut or artist page.
    function onNowPlaying(n) {
        if (!n || !n.videoId || Ids.isRadioId(n.videoId))
            return;
        root.mutate(function (b) {
            var touched = P.touchPick(b, n.videoId, Date.now());
            var noted = false;
            if (n.artists) {
                P.noteArtist(b, n.artistId ? n.artistId : n.artists, P.firstArtist(n.artists));
                noted = true;
            }
            return touched || noted;
        });
    }

    // --- reducers: mutate a clone, publish, persist ------------------------------------------
    // A JSON clone so the reassignment is a new object (QML change detection) and the pure reducer
    // never aliases the live blob's nested recent/artists maps.
    function mutate(fn) {
        var b = JSON.parse(JSON.stringify(root.blob));
        if (fn(b) === false)
            return false;
        root.blob = b;
        saveTimer.restart();
        return true;
    }

    // Add to Shortcuts (evicting the tile gone longest unplayed when the grid is full). Returns
    // false when it was already there. This is the call the pages' pin controls make.
    function addPick(item) { return root.mutate(function (b) { return P.addPick(b, item, Date.now()); }); }
    function removePick(id) { return root.mutate(function (b) { P.removePick(b, id); return true; }); }
    function touchPick(id) { return root.mutate(function (b) { return P.touchPick(b, id, Date.now()); }); }
    function seedPick(item) { return root.mutate(function (b) { return P.seedPick(b, item, Date.now()); }); }
    function noteRecent(item) { return root.mutate(function (b) { P.noteRecent(b, item, Date.now()); return true; }); }
    // Signing out of the account drops the recents that belong to it: Liked Music (LM), the
    // library playlists and anything else that only resolves with that session. Home's "Jump
    // back in" must not keep offering a library the daemon can no longer fetch.
    function forgetAccountRecents() {
        return root.mutate(function (b) {
            var n = 0;
            for (var id in b.recent) {
                var r = b.recent[id];
                if (id === "LM" || id.indexOf("spotify:") === 0 || (r && r.kind === "playlist")) { delete b.recent[id]; n++; }
            }
            return n > 0;
        });
    }
    Connections {
        target: Playback
        function onAuthChanged(): void { if (!(Playback.auth && Playback.auth.signedIn)) root.forgetAccountRecents(); }
        function onSpotifyChanged(): void { if (!(Playback.spotify && Playback.spotify.signedIn)) root.forgetAccountRecents(); }
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

    // Persist the current blob, coalescing a burst (now-playing recency, a drag) into one write.
    Timer {
        id: saveTimer
        interval: 300
        onTriggered: Daemon.call("set_personal", { personal: root.blob }).catch(function () {})
    }
}
