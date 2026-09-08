.pragma library
.import "ids.js" as Ids

// A small, explainable, on-device discovery ranker for Home. It re-ranks the items the active
// provider already put in the feed (get_home sections) using the local listening signals the
// personal store records — nothing leaves the machine, nothing is trained, nothing is invented.
//
// It is a lightweight hybrid, written from scratch: no model, no service, no third-party code. The
// shape borrows only ideas that are public:
//   - a linear blend of a few normalised signals (the "hybrid" of retrieval systems such as
//     implicit — benfred/implicit, MIT — and LightFM — lyst/lightfm, Apache-2.0 — without adopting
//     any of their code, weights, or matrix factorisation: a single user's play history does not
//     justify training an ALS/WARP model, and pretending it did would be dishonest);
//   - Maximal Marginal Relevance for the diversity pass, the standard
//     mmr = lambda * relevance - (1 - lambda) * maxSimilarityToAlreadyChosen
//     (see elastic.co/search-labs/blog/maximum-marginal-relevance-diversify-results). The formula
//     is math, not licensed code; the implementation below is original.
//
// Signals, all clamped to [0, 1] before weighting:
//   provider rank  — the feed's own ordering, trusted as a prior (the provider ranked it for a
//                    reason); earlier => higher. This carries a cold start honestly: with no local
//                    history the shelf is simply the provider's discovery order, diversified.
//   artist affinity— how often the user plays this item's artist (personal.artists counts).
//   artist recency — how lately they have been on that artist (decayed, artists[].lastPlayedAt).
//   pick affinity  — shares an artist with one of the user's shortcut tiles.
//   replay fatigue — a penalty when this exact id was played recently (decayed): a discovery shelf
//                    should not hand back the track you just finished.
//   metadata       — a small penalty for artwork-less rows so thin/placeholder items do not lead.
//
// Provider boundaries are kept: personal history is scoped to the id family of the active provider,
// so a Spotify/SoundCloud feed does not inherit YouTube Music affinities (it stays an honest cold
// start), and feed rows that positively belong to a *different* provider are dropped outright.
//
// build(sections, personalBlob, provider, nowMs) -> { items, explanation }
//   items       : up to 12 original BrowseItem-compatible rows (shallow clones + an optional
//                 `reason` string), deduped by id, never a mutation of the inputs.
//   explanation : a short, honest sentence about how this set was chosen (cold vs. personalised),
//                 always present — even when items is empty.

var OUT_MAX = 12;            // the shelf holds twelve cards
var POOL_MAX = 300;          // bound the candidate pool: the feed can be long, ranking is not free

var DAY_MS = 86400000;
var RECENCY_HALF_LIFE_MS = 14 * DAY_MS;  // an artist you were on two weeks ago is half as "current"
var FATIGUE_HALF_LIFE_MS = 3 * DAY_MS;   // a track replayed three days ago fatigues half as much

var RANK_DECAY = 0.06;   // provider-rank prior falloff: rank 0 -> 1.0, ~10 -> 0.63, ~25 -> 0.4
var W_RANK = 0.45;       // weight on the provider's own ordering (the base everyone starts from)
var W_ARTIST = 0.40;     // weight on play-count affinity
var W_RECENCY = 0.20;    // weight on how lately that artist was played
var W_PICK = 0.15;       // weight on sharing an artist with a shortcut
var W_FATIGUE = 0.70;    // strength of the "just played this" penalty
var W_META = 0.30;       // strength of the artwork-less penalty

var MMR_LAMBDA = 0.72;   // relevance vs. diversity: high enough that a genuinely better item wins
var SIM_ARTIST = 1.0;    // two rows by the same artist are maximally redundant
var SIM_KIND = 0.28;     // two rows of the same kind are mildly redundant

function providerOf(id) {
    return Ids.providerOf(id);
}

function isForeign(id, provider) {
    return providerOf(id) !== provider;
}

// The lead artist of a joined credit string — same rule as personal.js firstArtist, kept local so
// this module stays a standalone pure library. Tolerant of non-strings.
function firstArtist(artists) {
    if (typeof artists !== "string" || !artists)
        return "";
    return artists.split(/[,&\u2022]|\sfeat\.?\s|\sft\.?\s/i)[0].trim();
}

// 2^(-elapsed / halfLife). Future/negative elapsed (clock skew) reads as "just now" -> 1.
function decay(elapsedMs, halfLifeMs) {
    if (!(elapsedMs > 0))
        return 1;
    return Math.pow(2, -elapsedMs / halfLifeMs);
}

// The store keys an artist by its channel id when it has one, otherwise by the whole credit string
// (personal.js noteArtist). Match that exactly so affinity lookups actually hit.
function affinityKeyOf(item) {
    if (item.artistId && typeof item.artistId === "string")
        return item.artistId;
    if (Array.isArray(item.artistRuns)) {
        for (var i = 0; i < item.artistRuns.length; i++)
            if (item.artistRuns[i] && item.artistRuns[i].id)
                return item.artistRuns[i].id;
    }
    if (item.kind === "artist")
        return item.id;
    return firstArtist(item.subtitle);
}

// A looser key for the diversity pass: the lead artist, case-folded, so two songs sharing a lead
// (even across different collab credits) read as the same artist. Empty when there is no artist.
function simArtistOf(item) {
    return String(affinityKeyOf(item) || "").toLowerCase();
}

// A row worth ranking: a real id and a title. Rows missing either are the "fake"/placeholder rows a
// discovery shelf must not promote — dropped, not penalised.
function usable(item) {
    return !!item
        && typeof item.id === "string" && item.id.length > 0
        && typeof item.title === "string" && item.title.trim().length > 0
        && ["song", "album", "playlist", "artist"].indexOf(item.kind) >= 0;
}

// Fold the personal blob into the fast lookups scoring needs, scoped to the active provider so one
// catalogue's history never leaks into another's feed. Returns null-ish maps that are always safe
// to read.
function personalIndex(blob, provider, nowMs) {
    var idx = {
        recentAt: {},      // id -> last-played epoch ms (for replay fatigue)
        artists: {},       // artist key -> { count, lastPlayedAt }
        artistNames: Object.create(null),
        maxCount: 0,       // for normalising affinity
        pickIds: {},       // shortcut ids -> true (skip: already surfaced)
        pickArtists: {},   // shortcut artist keys -> true (affinity)
        personalized: false
    };
    if (!blob || typeof blob !== "object")
        return idx;

    var recent = (blob.recent && typeof blob.recent === "object") ? blob.recent : {};
    for (var id in recent) {
        if (!recent.hasOwnProperty(id))
            continue;
        if (providerOf(id) !== provider)
            continue;                          // cross-provider: not this catalogue's history
        var r = recent[id];
        var at = r && typeof r.at === "number" ? r.at : 0;
        idx.recentAt[id] = at;
        idx.personalized = true;
    }

    var artists = (blob.artists && typeof blob.artists === "object") ? blob.artists : {};
    for (var key in artists) {
        if (!artists.hasOwnProperty(key))
            continue;
        if (providerOf(key) !== provider)
            continue;
        var a = artists[key];
        if (!a || typeof a !== "object")
            continue;
        var count = typeof a.count === "number" && a.count > 0 ? a.count : 0;
        if (count <= 0)
            continue;
        idx.artists[key] = {
            count: count,
            lastPlayedAt: typeof a.lastPlayedAt === "number" ? a.lastPlayedAt : 0,
            name: typeof a.name === "string" ? a.name : ""
        };
        var nameKey = firstArtist(idx.artists[key].name).toLowerCase();
        if (nameKey && (!idx.artistNames[nameKey] || idx.artistNames[nameKey].count < count))
            idx.artistNames[nameKey] = idx.artists[key];
        if (count > idx.maxCount)
            idx.maxCount = count;
        idx.personalized = true;
    }

    var picks = Array.isArray(blob.picks) ? blob.picks : [];
    for (var i = 0; i < picks.length; i++) {
        var p = picks[i];
        if (!p || typeof p.id !== "string")
            continue;
        idx.pickIds[p.id] = true;
        if (providerOf(p.id) !== provider)
            continue;
        var pk = affinityKeyOf(p);
        if (pk)
            idx.pickArtists[pk] = true;
        idx.personalized = true;
    }
    return idx;
}

// Build the deduped, provider-scoped, bounded candidate pool out of the feed's sections, preserving
// the provider's ordering as the rank. Never mutates a section or an item.
function poolFrom(sections, provider, idx) {
    var pool = [];
    var seen = {};
    var list = Array.isArray(sections) ? sections : [];
    for (var s = 0; s < list.length && pool.length < POOL_MAX; s++) {
        var sec = list[s];
        var items = (sec && Array.isArray(sec.items)) ? sec.items : [];
        for (var i = 0; i < items.length && pool.length < POOL_MAX; i++) {
            var it = items[i];
            if (!usable(it))
                continue;
            if (seen[it.id])
                continue;                       // full id dedup — first (best-ranked) wins
            seen[it.id] = true;
            if (isForeign(it.id, provider))
                continue;                       // cross-provider exclusion
            if (idx.pickIds[it.id])
                continue;                       // already a shortcut tile — redundant to re-offer
            pool.push({ item: it, rank: pool.length });
        }
    }
    return pool;
}

// Score one candidate: a relevance number plus the per-signal parts (kept for the reason string).
function score(entry, idx, nowMs) {
    var item = entry.item;
    var prior = 1 / (1 + RANK_DECAY * entry.rank);

    var akey = affinityKeyOf(item);
    var art = idx.artists[akey] || idx.artistNames[firstArtist(item.subtitle).toLowerCase()];
    var affinity = (art && idx.maxCount > 0) ? (art.count / idx.maxCount) : 0;
    var recency = (art && art.lastPlayedAt > 0) ? decay(nowMs - art.lastPlayedAt, RECENCY_HALF_LIFE_MS) : 0;
    var pick = (akey && idx.pickArtists[akey]) ? 1 : 0;

    var playedAt = idx.recentAt.hasOwnProperty(item.id) ? idx.recentAt[item.id] : null;
    var fatigue = (playedAt !== null) ? decay(nowMs - playedAt, FATIGUE_HALF_LIFE_MS) : 0;

    var thin = (typeof item.thumbnail === "string" && item.thumbnail.length > 0) ? 0 : 1;

    var relevance = W_RANK * prior
        + W_ARTIST * affinity
        + W_RECENCY * recency
        + W_PICK * pick
        - W_FATIGUE * fatigue
        - W_META * thin;

    entry.rel = relevance;
    entry.parts = { affinity: W_ARTIST * affinity, recency: W_RECENCY * recency,
                    pick: W_PICK * pick, fatigue: fatigue };
    entry.sim = { artist: art && art.name ? firstArtist(art.name).toLowerCase() : simArtistOf(item),
                  kind: typeof item.kind === "string" ? item.kind : "" };
    entry.artistName = (art && art.name) ? art.name : firstArtist(item.subtitle);
    return entry;
}

// Redundancy of a candidate against an already-chosen one: same artist dominates, same kind is a
// gentle nudge. Max over the chosen set is what MMR subtracts.
function similarity(a, b) {
    if (a.sim.artist && b.sim.artist && a.sim.artist === b.sim.artist)
        return SIM_ARTIST;
    if (a.sim.kind && b.sim.kind && a.sim.kind === b.sim.kind)
        return SIM_KIND;
    return 0;
}

// Order-stable comparison for candidates: relevance desc, then the provider's rank, then id. Pure
// so ties never depend on array order or floating jitter — the shelf is the same every build.
function beforeByRelevance(x, y) {
    if (y.rel !== x.rel)
        return y.rel - x.rel;
    if (x.rank !== y.rank)
        return x.rank - y.rank;
    return x.item.id < y.item.id ? -1 : (x.item.id > y.item.id ? 1 : 0);
}

// The MMR diversity pass. Greedily pick the candidate maximising
//   lambda * relevance - (1 - lambda) * maxSimilarity(candidate, chosen)
// Ties resolve by the stable relevance order above, so the result is deterministic. A much stronger
// candidate still wins over a diverse-but-weak one (relevance survives); a near-tie by the same
// artist yields to a fresh one (diversity survives).
function rerank(scored, limit) {
    var chosen = [];
    for (var i = 0; i < scored.length; i++) {
        scored[i].maxSimilarity = 0;
        scored[i].selected = false;
    }
    while (chosen.length < limit && chosen.length < scored.length) {
        var previous = chosen.length ? chosen[chosen.length - 1] : null;
        var best = null;
        var bestScore = 0;
        for (var j = 0; j < scored.length; j++) {
            var candidate = scored[j];
            if (candidate.selected)
                continue;
            // Update against only the newly selected item: previous maxima are still
            // valid. This is O(pool × output), not a repeated scan of the chosen set.
            if (previous)
                candidate.maxSimilarity = Math.max(candidate.maxSimilarity, similarity(candidate, previous));
            var mmr = MMR_LAMBDA * candidate.rel - (1 - MMR_LAMBDA) * candidate.maxSimilarity;
            if (best === null || mmr > bestScore
                || (mmr === bestScore && beforeByRelevance(candidate, best) < 0)) {
                best = candidate;
                bestScore = mmr;
            }
        }
        if (!best)
            break;
        best.selected = true;
        chosen.push(best);
    }
    return chosen;
}

// A short, honest reason for one row, from whichever positive personal signal contributed most.
// Falls back to the provider prior (a genuine cold-start row) rather than claiming a learned pick.
function reasonFor(entry, provider) {
    var p = entry.parts;
    var name = entry.artistName;
    if (name && p.affinity >= p.recency && p.affinity >= p.pick && p.affinity > 0)
        return "More from " + name + ", one of your regulars";
    if (name && p.recency >= p.pick && p.recency > 0)
        return "You've had " + name + " on lately";
    if (name && p.pick > 0)
        return "Close to your shortcut " + name;
    return "From " + providerLabel(provider) + "'s recommendations";
}

function providerLabel(provider) {
    if (provider === "youtube")
        return "YouTube Music";
    if (provider === "spotify")
        return "Spotify";
    if (provider === "soundcloud")
        return "SoundCloud";
    return "your feed";
}

// A shallow clone with the reason attached — the output row is BrowseItem-compatible and never the
// caller's own object, so scoring can annotate without touching the feed.
function present(entry, provider) {
    var out = {};
    var item = entry.item;
    for (var k in item)
        if (item.hasOwnProperty(k))
            out[k] = item[k];
    out.reason = reasonFor(entry, provider);
    return out;
}

// The whole feed-resolved explanation: personalised when local history drove the order, an honest
// cold-start line otherwise, and a useful line even when there is nothing to show.
function explain(personalized, provider, count) {
    if (count === 0) {
        return "Nothing new to surface here yet — this fills in as your "
            + providerLabel(provider) + " feed grows.";
    }
    if (personalized) {
        return "Based on your recent listening and shortcuts, with room for different artists. Ranked on your device.";
    }
    return "A varied selection from " + providerLabel(provider) + ". Your listening will help shape future picks.";
}

// Entry point. Pure: reads sections/personalBlob, returns a new { items, explanation }.
function build(sections, personalBlob, provider, nowMs) {
    var prov = typeof provider === "string" && provider ? provider : "youtube";
    var now = typeof nowMs === "number" && isFinite(nowMs) ? nowMs : Date.now();

    var idx = personalIndex(personalBlob, prov, now);
    var pool = poolFrom(sections, prov, idx);

    if (!pool.length)
        return { items: [], explanation: explain(idx.personalized, prov, 0) };

    for (var i = 0; i < pool.length; i++)
        score(pool[i], idx, now);

    var chosen = rerank(pool, OUT_MAX);
    var items = chosen.map(function (e) { return present(e, prov); });
    return { items: items, explanation: explain(idx.personalized, prov, items.length) };
}
