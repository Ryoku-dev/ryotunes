.pragma library

// Pure helpers shared by Style.qml and its QtTest case. They live here rather than as methods on
// the Style singleton because a QtTest process cannot load Quickshell (Style imports Tokens, which
// imports Quickshell), so the test imports this module directly. Style re-exposes them unchanged.

// Rewrite a Google image URL to about the pixel size a slot actually renders, so the client does
// not decode a 544 px image for a 40 px row. Line-for-line port of ui/src/lib/thumb.ts with
// Tauri's convertFileSrc replaced by a "file://" prefix (Quickshell Image opens file URLs directly).
// Quantize requested thumbnail sizes to standard buckets so Google FIFE CDN hits standard
// cached sizes and does not fail with HTTP 500 on dynamic non-standard dimensions.
var BUCKETS = [64, 96, 120, 128, 160, 226, 256, 300, 320, 384, 512, 544, 640, 800, 1024, 1200];
function bucket(px) {
    var n = Number(px);
    if (!isFinite(n) || n <= 64) return 64;
    for (var i = 0; i < BUCKETS.length; i++) {
        if (BUCKETS[i] >= n) return BUCKETS[i];
    }
    return 1200;
}

function thumb(url, px) {
    if (!url) return undefined;
    // Local library artwork is a path on this machine, not a URL. Kept as the real path everywhere
    // it is stored (queue, MPRIS); only rewritten to a file URL at the point of display.
    if (url.charAt(0) === "/" || /^[A-Za-z]:[\\/]/.test(url)) return "file://" + url;
    var b = bucket(px);
    if (/=w\d+-h\d+/.test(url)) return url.replace(/=w\d+-h\d+/, "=w" + b + "-h" + b);
    if (/=s\d+/.test(url)) return url.replace(/=s\d+/, "=s" + b);
    return url;
}

// Seconds -> "m:ss" (minutes not zero-padded, seconds always two digits). Floors sub-second input,
// clamps NaN/negative to zero, so a live position sample renders without flicker.
function fmtTime(secs) {
    if (!(secs > 0) || !isFinite(secs)) secs = 0;
    var s = Math.floor(secs);
    var m = Math.floor(s / 60);
    var r = s % 60;
    return m + ":" + (r < 10 ? "0" + r : r);
}

// A play / like / follower count -> a compact label ("27.3K", "1.2M"). A string passes through (the
// daemon may already have formatted it); undefined/null/empty and non-finite values yield "".
function fmtCount(v) {
    if (v === undefined || v === null || v === "") return "";
    if (typeof v === "string") return v;
    var n = Number(v);
    if (!isFinite(n)) return "";
    if (n >= 1000000) return (n / 1000000).toFixed(1).replace(/\.0$/, "") + "M";
    if (n >= 1000) return (n / 1000).toFixed(1).replace(/\.0$/, "") + "K";
    return String(Math.round(n));
}
