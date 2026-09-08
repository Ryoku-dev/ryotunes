.pragma library

// Id prefix tests, ported from ui/src/lib/api.ts. A song/album/artist on disk carries a LOCAL*
// prefix and a live radio station carries the RYOTUNES_RADIO prefix; neither has a YouTube rating,
// so the like control hides for them exactly as the Svelte PlayerBar does. The three smart-playlist
// ids draw an icon cover instead of artwork. Kept in a pure module so both the singletons and the
// view components share one source of truth.
var LOCAL_SONG = "LOCAL:";
var LOCAL_ALBUM = "LOCALALBUM:";
var LOCAL_ARTIST = "LOCALARTIST:";
var RADIO = "RYOTUNES_RADIO:";
var ON_REPEAT = "RYOTUNES_ON_REPEAT";
var RECENTLY_PLAYED = "RYOTUNES_RECENTLY_PLAYED";
var REDISCOVER = "RYOTUNES_REDISCOVER";

function isLocalId(id) {
    return !!id && (id.indexOf(LOCAL_SONG) === 0 || id.indexOf(LOCAL_ALBUM) === 0 || id.indexOf(LOCAL_ARTIST) === 0);
}
function isRadioId(id) {
    return !!id && id.indexOf(RADIO) === 0;
}
function isOnRepeatId(id) {
    return id === ON_REPEAT;
}
function isSmartPlaylistId(id) {
    return id === ON_REPEAT || id === RECENTLY_PLAYED || id === REDISCOVER;
}

// The daemon's ids are namespaced across catalogues; keep Home history and
// recommendations on the selected provider without mistaking SoundCloud for YouTube.
function providerOf(id) {
    if (typeof id !== "string" || !id)
        return "";
    if (isLocalId(id))
        return "local";
    if (isRadioId(id))
        return "radio";
    if (id.indexOf("spotify:") === 0)
        return "spotify";
    if (id.indexOf("sc:") === 0)
        return "soundcloud";
    return "youtube";
}
