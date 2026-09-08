.pragma library

// The personal store's pure reducers, a port of ui/src/lib/personal.ts. Same shapes, same rules,
// no storage or network: Personal.qml holds the live blob and persists it through the daemon, this
// module only transforms it. Kept pure so tst_personal.qml exercises it exactly as the Svelte
// build's personal.check.ts does. The daemon serialises this blob under `personal_json`.

var MAX_PICKS = 18;
var MAX_PINS = 3;
var MAX_RECENT = 100;
var MAX_ARTISTS = 100;
var ON_REPEAT_ID = "RYOTUNES_ON_REPEAT";

function normalizeLegacyId(id) {
    return id.slice(-10) === "_ON_REPEAT" ? ON_REPEAT_ID : id;
}

function empty() {
    return {
        picks: [],
        saved: [],
        pins: [],
        recent: {},
        artists: {},
        dismissedSeeds: [],
        home: { order: [], hidden: [], seen: [] }
    };
}

// Tolerant parse of a persisted blob — a corrupt or older shape degrades to empty, never throws.
function hydrate(raw) {
    var base = empty();
    if (!raw || typeof raw !== "object")
        return base;
    var o = raw;
    if (Array.isArray(o.picks)) {
        // `manual: false` marks a tile from the old auto-seeding build; those are dropped rather
        // than inherited forever. Today's seedPick writes no `manual` flag.
        base.picks = o.picks
            .filter(function (p) { return p && typeof p.id === "string" && p.manual !== false; })
            .map(function (p) {
                var t = {};
                for (var k in p) t[k] = p[k];
                t.id = normalizeLegacyId(p.id);
                return t;
            });
    }
    if (Array.isArray(o.saved)) {
        base.saved = o.saved
            .filter(function (s) { return s && typeof s.id === "string" && typeof s.kind === "string"; })
            .map(function (s) {
                var t = {};
                for (var k in s) t[k] = s[k];
                t.id = normalizeLegacyId(s.id);
                return t;
            });
    }
    if (Array.isArray(o.pins)) {
        base.pins = o.pins
            .filter(function (p) { return typeof p === "string"; })
            .map(normalizeLegacyId)
            .slice(0, MAX_PINS);
    }
    if (o.recent && typeof o.recent === "object")
        base.recent = o.recent;
    if (o.artists && typeof o.artists === "object")
        base.artists = o.artists;
    if (Array.isArray(o.dismissedSeeds)) {
        base.dismissedSeeds = o.dismissedSeeds
            .filter(function (id) { return typeof id === "string"; })
            .map(normalizeLegacyId);
    }
    if (o.home && typeof o.home === "object") {
        var keys = function (v) {
            return Array.isArray(v) ? v.filter(function (k) { return typeof k === "string"; }) : [];
        };
        base.home = { order: keys(o.home.order), hidden: keys(o.home.hidden), seen: keys(o.home.seen) };
        // '@familiar' shipped after some users saved an arrangement; slot it where the code puts it,
        // once, so an unranked key doesn't sink to the bottom of the feed.
        if (base.home.order.length && base.home.order.indexOf("@familiar") < 0) {
            var at = base.home.order.indexOf("@recent");
            base.home.order.splice(at < 0 ? base.home.order.length : at + 1, 0, "@familiar");
        }
    }
    return base;
}

// --- Shortcuts grid -----------------------------------------------------------------------------

function withStamp(item, now) {
    var t = {};
    for (var k in item) t[k] = item[k];
    t.lastUsedAt = now;
    return t;
}

// Over capacity: drop the tile gone longest without a play or open.
function evictStalest(p) {
    while (p.picks.length > MAX_PICKS) {
        var stalest = p.picks[0];
        for (var i = 1; i < p.picks.length; i++)
            if (p.picks[i].lastUsedAt < stalest.lastUsedAt)
                stalest = p.picks[i];
        p.picks = p.picks.filter(function (x) { return x !== stalest; });
    }
}

// Append. Returns false when it was already on the grid (its recency is refreshed instead).
function addPick(p, item, now) {
    for (var i = 0; i < p.picks.length; i++) {
        if (p.picks[i].id === item.id) {
            p.picks[i].lastUsedAt = now;
            return false;
        }
    }
    p.picks.push(withStamp(item, now));
    evictStalest(p);
    return true;
}

// A tile the app suggests rather than one the user added. Refused if it is already on the grid, if
// the user has ever removed it, or if the grid is full (a suggestion never evicts a hand-picked
// one). Returns whether the grid changed.
function seedPick(p, item, now) {
    if (p.picks.length >= MAX_PICKS)
        return false;
    if (p.dismissedSeeds.indexOf(item.id) >= 0)
        return false;
    for (var i = 0; i < p.picks.length; i++)
        if (p.picks[i].id === item.id)
            return false;
    p.picks.push(withStamp(item, now));
    return true;
}

function removePick(p, id) {
    p.picks = p.picks.filter(function (x) { return x.id !== id; });
    // Every removal is remembered so seedPick won't suggest it again; addPick ignores the list, so a
    // hand-added tile is unaffected.
    if (p.dismissedSeeds.indexOf(id) < 0)
        p.dismissedSeeds.push(id);
}

// Mark a tile as used (played or clicked). Returns whether anything changed — most calls come from
// cards that aren't on the grid, so the caller can skip a pointless write.
function touchPick(p, id, now) {
    for (var i = 0; i < p.picks.length; i++) {
        if (p.picks[i].id === id) {
            p.picks[i].lastUsedAt = now;
            return true;
        }
    }
    return false;
}

// --- Sidebar pins -------------------------------------------------------------------------------

function togglePin(p, id) {
    if (p.pins.indexOf(id) >= 0) {
        p.pins = p.pins.filter(function (x) { return x !== id; });
        return "unpinned";
    }
    if (p.pins.length >= MAX_PINS)
        return "full";
    p.pins.push(id);
    return "pinned";
}

// --- Home arrangement ---------------------------------------------------------------------------

// Home's sections in the order the user put them, hidden ones included. A section that has no rank
// (one that arrived after the last save) sorts to the end, keeping the feed's own order among peers.
// Sorting only, never filtering — hiddenSections is the other half.
function arrangeSections(sections, p) {
    var ranks = {};
    for (var i = 0; i < p.home.order.length; i++)
        ranks[p.home.order[i]] = i;
    var MAXR = 9007199254740991; // Number.MAX_SAFE_INTEGER; a finite sentinel, not Infinity
    var rank = function (key) { return (key in ranks) ? ranks[key] : MAXR; };
    // Decorate-sort-undecorate for a stable sort: equal ranks keep arrival order.
    return sections
        .map(function (s, i) { return { s: s, i: i }; })
        .sort(function (a, b) { return (rank(a.s.key) - rank(b.s.key)) || (a.i - b.i); })
        .map(function (e) { return e.s; });
}

function hiddenSections(p) {
    return p.home.hidden.slice();
}

// --- Recency + artist counts --------------------------------------------------------------------

// Record that a song, playlist, album or artist was played (songs from a genuine now-playing,
// the rest from a play-from-source). The entry is a BrowseItem clone stamped with `at`.
function noteRecent(p, item, now) {
    var entry = {};
    for (var k in item) entry[k] = item[k];
    entry.at = now;
    p.recent[item.id] = entry;
    var ids = Object.keys(p.recent);
    if (ids.length > MAX_RECENT) {
        var recent = p.recent;
        ids.sort(function (a, b) { return recent[b].at - recent[a].at; });
        for (var i = MAX_RECENT; i < ids.length; i++)
            delete p.recent[ids[i]];
    }
}

// The most recently played songs and played-from playlists/albums/artists, newest first.
function recentItems(p, n) {
    var recent = p.recent;
    var vals = Object.keys(recent).map(function (id) { return recent[id]; });
    vals.sort(function (a, b) { return b.at - a.at; });
    return vals.slice(0, n === undefined ? 12 : n);
}

// --- account sign-out: forget only what that session can reopen ---------------------------------
// Splitting the purge by provider is deliberate: signing out of YouTube must not drop a Spotify
// recent (or the reverse), and neither touches songs — a public video/track still plays signed out.

// Recents only a signed-in YouTube session can reopen: Liked Music (LM / VLLM) and the account's
// own library playlists (their subtitle reads "Your …"). Public playlists (a shared link, a chart)
// and everything from other providers stay. Returns how many entries were dropped.
function forgetYouTubeRecents(p) {
    var n = 0;
    for (var id in p.recent) {
        // A Spotify recent is never a YouTube one, even if its subtitle happens to read "Your …".
        if (id.indexOf("spotify:") === 0)
            continue;
        var r = p.recent[id];
        if (id === "LM" || id === "VLLM"
            || (r && r.kind === "playlist" && String(r.subtitle || "").indexOf("Your") === 0)) {
            delete p.recent[id];
            n++;
        }
    }
    return n;
}

// Recents only a signed-in Spotify session can reopen: every provider-qualified spotify: id.
// Returns how many entries were dropped.
function forgetSpotifyRecents(p) {
    var n = 0;
    for (var id in p.recent) {
        if (id.indexOf("spotify:") === 0) {
            delete p.recent[id];
            n++;
        }
    }
    return n;
}

// The lead artist out of a joined credit string — a usable search seed, unlike the whole list.
function firstArtist(artists) {
    return artists.split(/[,&•]|\sfeat\.?\s|\sft\.?\s/i)[0].trim();
}

function noteArtist(p, key, name, now) {
    var cur = p.artists[key];
    if (cur) {
        cur.count++;
        if (name) cur.name = name;
        if (now !== undefined) cur.lastPlayedAt = now;
    } else {
        p.artists[key] = (now !== undefined)
            ? { name: name, count: 1, lastPlayedAt: now }
            : { name: name, count: 1 };
    }
    var keys = Object.keys(p.artists);
    if (keys.length > MAX_ARTISTS) {
        var artists = p.artists;
        keys.sort(function (a, b) { return artists[b].count - artists[a].count; });
        for (var i = MAX_ARTISTS; i < keys.length; i++)
            delete p.artists[keys[i]];
    }
}

// Top artists by play count that carry a real channel id. Names-only keys (a song whose artist was
// not linked) are dropped: without an id there is no artist page to look up or open.
function topArtistIds(p, n) {
    var artists = p.artists;
    return Object.keys(artists)
        .filter(function (id) { return id.slice(0, 2) === "UC"; })
        .sort(function (a, b) { return artists[b].count - artists[a].count; })
        .slice(0, n === undefined ? 7 : n);
}

// The user's most-played artist names — the seed for the community shelf.
function topArtists(p, n) {
    var artists = p.artists;
    return Object.keys(artists)
        .map(function (id) { return artists[id]; })
        .filter(function (a) { return a.name; })
        .sort(function (a, b) { return b.count - a.count; })
        .slice(0, n === undefined ? 3 : n)
        .map(function (a) { return a.name; });
}

// Structural equality, independent of object key order. The daemon round-trips a saved blob
// through serde before echoing it back on `personal-changed`, and that can reorder keys, so a byte
// comparison of the echo against the blob we sent would spuriously differ. Personal blobs hold only
// JSON scalars, arrays and plain objects — no functions, dates or cycles — so a plain recursion is
// enough. Used to recognise (and drop) our own save echo.
function deepEqual(a, b) {
    if (a === b) return true;
    if (a === null || b === null || typeof a !== "object" || typeof b !== "object") return false;
    var aArr = Array.isArray(a), bArr = Array.isArray(b);
    if (aArr !== bArr) return false;
    if (aArr) {
        if (a.length !== b.length) return false;
        for (var i = 0; i < a.length; i++)
            if (!deepEqual(a[i], b[i])) return false;
        return true;
    }
    var ak = Object.keys(a), bk = Object.keys(b);
    if (ak.length !== bk.length) return false;
    for (var j = 0; j < ak.length; j++) {
        var k = ak[j];
        if (!b.hasOwnProperty(k) || !deepEqual(a[k], b[k])) return false;
    }
    return true;
}
