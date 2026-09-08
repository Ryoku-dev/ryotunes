.pragma library

// BrowseItem -> SongItem, ported from ui/src/lib/browse.ts asSong. A card or shelf row carries
// everything a queue entry needs; the daemon's `play`/`play_playlist` deserialize this snake_case
// shape straight into a SongItem. Kept pure so every view component maps items the same way.
function asSong(i) {
    var runId;
    if (i.artistRuns) {
        for (var k = 0; k < i.artistRuns.length; k++) {
            if (i.artistRuns[k].id) { runId = i.artistRuns[k].id; break; }
        }
    }
    return {
        video_id: i.id,
        title: i.title,
        artists: i.subtitle || "",
        artist_runs: i.artistRuns,
        artist_id: runId || i.artistId,
        duration: i.duration,
        play_count: i.playCount,
        thumbnail: i.thumbnail,
        explicit: i.explicit,
        is_upload: i.isUpload
    };
}

// The four-per-column song layout a "mostly songs" shelf uses: split an ordered song list into
// columns of `rows`. Mirrors Shelf.svelte's `columns` derivation.
function columnize(songs, rows) {
    var cols = [];
    for (var c = 0; c < Math.ceil(songs.length / rows); c++)
        cols.push(songs.slice(c * rows, c * rows + rows));
    return cols;
}

// The dominant kind of a shelf, and whether it clears the 75% "mostly one thing" bar that earns a
// per-kind form; below it the shelf is a mixed bag drawn as plain cards. Ported from Shelf.svelte.
function shelfMode(items) {
    if (!items || !items.length)
        return "card";
    var counts = {};
    var best = null;
    var bestN = 0;
    for (var i = 0; i < items.length; i++) {
        var k = items[i].kind;
        counts[k] = (counts[k] || 0) + 1;
        if (counts[k] > bestN) { bestN = counts[k]; best = k; }
    }
    return bestN / items.length >= 0.75 ? best : "card";
}
