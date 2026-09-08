.pragma library

// A pasted YouTube / YouTube Music link turned into a playlist the shortcut picker can add (#63):
// some playlists are only reachable by URL — they don't show up in search and aren't in the
// library — so without this there's no way in. Pure and network-free: the picker resolves an id
// here, then fetches real metadata through `get_playlist` (never the pasted URL itself).
//
// The one target this resolves is a playlist. The picker's link field adds *playlist* shortcuts, so
// a watch/share link is read for its `list=` playlist rather than the single track it was shared
// from (the deliberate opposite of ShareDialog/ytlink, which open the shared track). Manual URL
// parsing on purpose: QML's JS engine has no `URL` constructor, and this must also run under
// qmltestrunner with no Quickshell.

// The exact hosts we open. A lookalike ("youtube.com.evil.test") is not on the list and is refused.
var HOSTS = {
    "youtube.com": true,
    "www.youtube.com": true,
    "m.youtube.com": true,
    "music.youtube.com": true,
    "youtu.be": true
};

// A YouTube playlist id: PL…, OLAK5uy_…, RDCLAK5uy_…, a bare VL browse id, etc.
var ID_RE = /^[A-Za-z0-9_-]+$/;
// A syntactically plausible hostname (letters/digits/dots/hyphens). Rejects "not a url at all".
var HOST_RE = /^[a-z0-9.-]+$/;

function fail(reason, message) {
    return { ok: false, reason: reason, message: message };
}

// Percent/`+` decode one query token, tolerating a malformed sequence rather than throwing.
function decodeToken(s) {
    try {
        return decodeURIComponent(s.replace(/\+/g, " "));
    } catch (e) {
        return s;
    }
}

// Parse a query string into { key: [values…] } with every value decoded. Repeated keys accumulate,
// so a link carrying two different `list=` can be spotted as ambiguous.
function parseQuery(q) {
    var out = Object.create(null);
    if (!q)
        return out;
    var parts = q.split("&");
    for (var i = 0; i < parts.length; i++) {
        if (!parts[i])
            continue;
        var eq = parts[i].indexOf("=");
        var k = eq < 0 ? parts[i] : parts[i].slice(0, eq);
        var v = eq < 0 ? "" : parts[i].slice(eq + 1);
        k = decodeToken(k);
        if (!Object.prototype.hasOwnProperty.call(out, k))
            out[k] = [];
        out[k].push(decodeToken(v));
    }
    return out;
}

// Resolve a pasted link to a playlist:
//   { ok: true, id, rawId }               — id is the `VL…` browse id `get_playlist` wants
//   { ok: false, reason, message }        — a clear, recoverable reason the field can show
// `reason: "empty"` (blank box) is neutral: the field shows nothing rather than an error.
function parsePlaylistLink(input) {
    var text = (input === undefined || input === null) ? "" : String(input).trim();
    if (!text)
        return fail("empty", "");
    if (text.length > 4096)
        return fail("invalid", "That link is too long.");

    // Scheme: require http/https. Assume https for a bare host paste ("music.youtube.com/…"), but
    // an explicit foreign scheme (ftp:, file:, javascript:, …) is refused outright.
    var schemeMatch = /^([a-zA-Z][a-zA-Z0-9+.-]*):\/\//.exec(text);
    var rest;
    if (schemeMatch) {
        var scheme = schemeMatch[1].toLowerCase();
        if (scheme !== "http" && scheme !== "https")
            return fail("scheme", "Only http and https links are supported.");
        rest = text.slice(schemeMatch[0].length);
    } else {
        rest = text;
    }

    // Split authority from the path/query (up to the first "/", "?" or "#").
    var cut = rest.search(/[\/?#]/);
    var authority = cut < 0 ? rest : rest.slice(0, cut);
    var tail = cut < 0 ? "" : rest.slice(cut);

    // Credentials are not part of a share link. No pasted authority is ever fetched.
    if (authority.indexOf("@") >= 0)
        return fail("invalid", "Paste a public share link without login details.");
    var host = authority.replace(/:\d+$/, "").toLowerCase();
    if (!host || !HOST_RE.test(host))
        return fail("invalid", "That does not look like a link.");
    if (!HOSTS.hasOwnProperty(host))
        return fail("host", "Only YouTube and YouTube Music links are supported.");

    // Fragment is irrelevant; separate path and query.
    var hash = tail.indexOf("#");
    if (hash >= 0)
        tail = tail.slice(0, hash);
    var path = tail;
    var query = "";
    var qi = tail.indexOf("?");
    if (qi >= 0) {
        path = tail.slice(0, qi);
        query = tail.slice(qi + 1);
    }
    var params = parseQuery(query);
    var segs = path.replace(/^\/+/, "").split("/").filter(function (s) { return s.length > 0; });

    // The playlist id. `list=` covers /playlist, a watch/share link, and youtu.be/<v>?list=…; a
    // YTM /browse/VL… page is the playlist's own browse id.
    var raw = null;
    var lists = params["list"] || [];
    if (lists.length > 0) {
        var seen = Object.create(null);
        for (var i = 0; i < lists.length; i++)
            seen[lists[i]] = true;
        if (Object.keys(seen).length > 1)
            return fail("ambiguous", "This link points at more than one playlist.");
        raw = lists[0];
    } else if (segs[0] === "browse" && segs[1]) {
        var bid = segs[1];
        if (/^VL/.test(bid))
            raw = bid;
        else if (bid.indexOf("MPRE") === 0)
            return fail("no-playlist", "That link is an album, not a playlist.");
        else if (bid.indexOf("UC") === 0)
            return fail("no-playlist", "That link is an artist, not a playlist.");
        else
            return fail("no-playlist", "That link is not a playlist.");
    } else {
        return fail("no-playlist", "That link has no playlist — paste a playlist or a \u201CShare playlist\u201D link.");
    }

    raw = String(raw).trim();
    // A playlist browse id carries a VL prefix the URL's `list=` does not; normalise so it is never
    // doubled and always canonical.
    var rawId = raw.replace(/^VL/, "");
    if (!rawId || rawId.length > 256 || !ID_RE.test(rawId))
        return fail("bad-id", "That does not look like a valid playlist id.");
    return { ok: true, id: "VL" + rawId, rawId: rawId };
}
