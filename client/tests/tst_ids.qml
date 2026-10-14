import QtQuick
import QtTest
import "../lib/ids.js" as Ids

// The source tag on cards and rows: every catalogue id wears its catalogue's glyph, and Ryotunes'
// own collections (which share YouTube's unprefixed id space in providerOf) wear none, so a smart or
// local playlist never claims to come from YouTube Music.
TestCase {
    name: "Ids"

    function test_catalogue_ids_wear_their_glyph() {
        compare(Ids.sourceGlyph("dQw4w9WgXcQ"), "youtube-music");
        compare(Ids.sourceGlyph("MPREb_abc"), "youtube-music");
        compare(Ids.sourceGlyph("spotify:album:4uLU6hMCjMI75M1A2tKUQC"), "spotify");
        compare(Ids.sourceGlyph("spotify:playlist:liked"), "spotify");
        compare(Ids.sourceGlyph("sc:playlist:123"), "soundcloud");
        compare(Ids.sourceGlyph("sc:system:weekly"), "soundcloud");
        compare(Ids.sourceGlyph("tidal:track:12345"), "sound");
        compare(Ids.sourceGlyph("tidal:playlist:1f-2e-3d"), "sound");
    }

    // A tidal id must not fall through to YouTube the way an unprefixed id does: the fallthrough
    // is what makes a missed prefix a silent wrong badge rather than an error.
    function test_tidal_ids_are_their_own_provider() {
        compare(Ids.providerOf("tidal:track:1"), "tidal");
        compare(Ids.providerOf("tidal:album:1"), "tidal");
        compare(Ids.providerOf("tidalish"), "youtube");
    }

    function test_ryotunes_own_collections_wear_none() {
        compare(Ids.sourceGlyph("RYOTUNES_ON_REPEAT"), "");
        compare(Ids.sourceGlyph("RYOTUNES_REDISCOVER"), "");
        compare(Ids.sourceGlyph("RYOTUNES_LOCAL_PLAYLIST:42"), "");
        compare(Ids.sourceGlyph("RYOTUNES_RADIO:station"), "");
        compare(Ids.sourceGlyph("LOCALALBUM:x"), "");
        compare(Ids.sourceGlyph(""), "");
        compare(Ids.sourceGlyph(undefined), "");
    }
}
