pragma Singleton
import QtQuick
import Quickshell
import "lib/ids.js" as Ids

// The client's mirror of ryotunesd's download queue and history, plus the one place a surface asks
// to save a track. Like Playback it holds no truth: the daemon owns every job, this singleton just
// reflects it. State is seeded by the `subscribe` snapshot's `downloads` block and refreshed by the
// `downloads-changed` event, so the queue page and every download button update without polling; a
// reconnect re-runs subscribe, which re-delivers the snapshot, so the mirror resynchronises on its
// own. Actions are daemon calls that surface success/failure through Playback's toast layer.
Singleton {
    id: root

    // The daemon's { jobs, activeCount, queuedCount } object. Replaced whole on each update so the
    // derived aliases and any binding on `jobs` re-run.
    property var snapshot: ({ jobs: [], activeCount: 0, queuedCount: 0 })
    readonly property var jobs: (root.snapshot && root.snapshot.jobs) ? root.snapshot.jobs : []
    readonly property int activeCount: root.snapshot ? (root.snapshot.activeCount || 0) : 0
    readonly property int queuedCount: root.snapshot ? (root.snapshot.queuedCount || 0) : 0
    property bool loaded: false
    property bool loading: false
    property string error: ""
    property bool addingAlbum: false

    // The daemon's persisted download preferences, fetched lazily on demand; null until loaded. The
    // settings page owns editing them (it calls the daemon directly); this is an optional read for a
    // surface that wants to show the destination folder.
    property var settings: null

    // --- events + opening snapshot -----------------------------------------------------------
    Connections {
        target: Daemon
        function onEvent(name, data) {
            if (name === "downloads-changed")
                root.apply(data);
        }
        function onSnapshot(snap) {
            if (snap && snap.downloads)
                root.apply(snap.downloads);
            else
                root.refresh();
        }
    }
    // A late-instantiated singleton (first referenced after the opening snapshot already fired) still
    // pulls the current state; on a live connection this is a no-op-cheap single call.
    Component.onCompleted: if (Daemon.connected) root.refresh();

    function apply(d) {
        if (!d)
            return;
        root.snapshot = {
            jobs: d.jobs || [],
            activeCount: d.activeCount || 0,
            queuedCount: d.queuedCount || 0
        };
        root.loaded = true;
        root.error = "";
    }

    // Pull the queue/history from the daemon. Called on demand and whenever a reconnect snapshot
    // arrives without a downloads block. Returns the Promise so a page can show a first-load state.
    function refresh() {
        root.loading = true;
        root.error = "";
        return Daemon.call("get_downloads")
            .then((d) => { root.apply(d); root.loading = false; return d; })
            .catch((e) => {
                root.loading = false;
                root.error = (e && e.message) ? e.message : "Could not load downloads";
            });
    }

    function loadSettings() {
        return Daemon.call("get_download_settings")
            .then((s) => { root.settings = s; return s; })
            .catch(() => {});
    }

    // --- track classification ----------------------------------------------------------------
    // The download source of a Playback.now-shaped track, told from its id exactly the way the
    // daemon does: a strict 11-char YouTube id, a numeric SoundCloud track (sc:track:<num>) or a
    // base62 Spotify track (spotify:track:<id>). Local files and live radio need no download; any
    // other shape (a Spotify/SoundCloud album or artist id that somehow reached now-playing) is
    // "unknown" and not downloadable.
    function kindOf(track) {
        if (!track || !track.videoId)
            return "none";
        var id = String(track.videoId);
        if (Ids.isLocalId(id))
            return "local";
        if (Ids.isRadioId(id))
            return "radio";
        if (/^spotify:track:[0-9A-Za-z]+$/.test(id))
            return "spotify";
        if (/^sc:track:[0-9]+$/.test(id))
            return "soundcloud";
        if (/^[0-9A-Za-z_-]{11}$/.test(id))
            return "youtube";
        return "unknown";
    }
    function canDownload(track) {
        var k = root.kindOf(track);
        return k === "youtube" || k === "soundcloud" || k === "spotify";
    }
    // A human hint for the download button's tooltip / accessible description: why a track cannot be
    // saved, and the one caveat that matters when it can — a Spotify track saves a matched YouTube
    // audio track, not the Spotify stream.
    function reason(track) {
        switch (root.kindOf(track)) {
        case "none": return "Nothing is playing.";
        case "local": return "This track is already a local file.";
        case "radio": return "Live radio can\u2019t be downloaded.";
        case "spotify": return "Saves a matching YouTube audio track.";
        case "soundcloud": return "Save this SoundCloud track.";
        case "youtube": return "Save this track.";
        default: return "This track can\u2019t be downloaded.";
        }
    }

    // The job for a given video id (the newest matching record), or null. Reads `jobs`, so a binding
    // that calls this re-evaluates when the queue changes.
    function jobFor(videoId) {
        if (!videoId)
            return null;
        var js = root.jobs;
        for (var i = 0; i < js.length; i++)
            if (js[i] && js[i].videoId === videoId)
                return js[i];
        return null;
    }

    // --- actions (each a daemon call) --------------------------------------------------------
    function enqueueRequest(track) {
        return Daemon.call("enqueue_download", {
            videoId: track.videoId,
            title: track.title || "",
            artists: track.artists || "",
            thumbnail: track.thumbnail || ""
        });
    }

    // Gather the complete album before admission, then use the same bounded queue as single
    // downloads. This singleton owns the operation so navigating away cannot interrupt it.
    function enqueueAlbum(album) {
        if (!album || root.addingAlbum)
            return;
        root.addingAlbum = true;
        var tokens = Object.create(null);
        var seen = Object.create(null);
        var tracks = (album.items || []).slice();
        var accepted = 0;
        var skipped = 0;
        Playback.toast("Preparing album downloads…", "info");

        function collect(token) {
            if (!token)
                return Promise.resolve();
            if (tokens[token])
                return Promise.reject(new Error("Album pagination repeated; no downloads were added."));
            tokens[token] = true;
            return Daemon.call("get_playlist_more", { token: token }).then((more) => {
                tracks = tracks.concat(more.items || []);
                return collect(more.continuation);
            });
        }
        function admit(index) {
            while (index < tracks.length) {
                var song = tracks[index++];
                var track = {
                    videoId: song.video_id,
                    title: song.title,
                    artists: song.artists || album.artist,
                    thumbnail: song.thumbnail || album.thumbnail
                };
                if (seen[track.videoId])
                    continue;
                seen[track.videoId] = true;
                if (!root.canDownload(track)) {
                    skipped++;
                    continue;
                }
                return root.enqueueRequest(track).then(() => {
                    accepted++;
                    return admit(index);
                });
            }
            return Promise.resolve();
        }
        return collect(album.continuation).then(() => admit(0)).then(() => {
            root.addingAlbum = false;
            var message = accepted + (accepted === 1 ? " track in Downloads" : " tracks in Downloads");
            if (skipped)
                message += " · " + skipped + " unavailable";
            Playback.toast(message, accepted ? "success" : "info");
        }).catch((e) => {
            root.addingAlbum = false;
            Playback.toast(accepted + " tracks in Downloads · "
                + ((e && e.message) ? e.message : "Could not download album"), "error");
        });
    }

    // Save the given track (a Playback.now-shaped object). Immediate: a toast confirms or surfaces
    // the daemon's error, and the queue updates through the downloads-changed event. A track that
    // cannot be downloaded is reported rather than sent.
    function enqueue(track) {
        if (!track || !track.videoId)
            return;
        if (!root.canDownload(track)) {
            Playback.toast(root.reason(track), "info");
            return;
        }
        root.enqueueRequest(track)
        .then((job) => {
            var done = job && job.status === "completed";
            Playback.toast(done ? "Already downloaded" : "Added to downloads", "success");
        })
        .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not start download", "error"));
    }
    function cancel(id) {
        return Daemon.call("cancel_download", { id: id })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not cancel", "error"));
    }
    function retry(id) {
        return Daemon.call("retry_download", { id: id })
            .then((job) => job)
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not retry", "error"));
    }
    // Reveal a completed download's file in its folder.
    function open(id) {
        return Daemon.call("open_download", { id: id })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not open file", "error"));
    }
    // Open the configured download folder.
    function openFolder() {
        return Daemon.call("open_download_folder")
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not open folder", "error"));
    }
    // Remove terminal records (completed/failed/cancelled) from history. Never touches saved files.
    function clearHistory() {
        return Daemon.call("clear_download_history")
            .then(() => Playback.toast("Download history cleared", "success"))
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not clear history", "error"));
    }
}
