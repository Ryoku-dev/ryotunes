import QtQuick
import QtTest
import "../lib/playlist-links.js" as Links

// The pasted-link parser behind the shortcut picker's YouTube playlist field, run without
// Quickshell (it is a pure module, mirroring tst_personal). It must open exactly the playlist a
// supported YouTube/YTM link points at, canonicalise the id to the `VL…` browse id `get_playlist`
// wants, and refuse — clearly, by reason — a foreign host, a foreign scheme, or a link with a
// missing, malformed or ambiguous playlist id. Boundaries only; the picker owns the UI.
TestCase {
    name: "PlaylistLinks"

    function parse(s) { return Links.parsePlaylistLink(s); }

    // Every URL form we accept resolves to the same VL-prefixed browse id, from a /playlist link, a
    // track shared out of a playlist (watch / youtu.be with list=), a bare-host paste, and a YTM
    // /browse page. The VL prefix is added once and never doubled.
    function test_valid_forms() {
        var a = parse("https://music.youtube.com/playlist?list=PLabc123");
        verify(a.ok);
        compare(a.id, "VLPLabc123");
        compare(a.rawId, "PLabc123");

        // A list id already carrying VL is not doubled.
        compare(parse("https://music.youtube.com/playlist?list=VLPLabc123").id, "VLPLabc123");

        // A track shared out of a playlist: the picker adds the playlist, not the single track.
        compare(parse("https://music.youtube.com/watch?v=dQw4&list=PLabc").id, "VLPLabc");
        compare(parse("https://youtu.be/dQw4?list=PLabc&si=xyz").id, "VLPLabc");

        // An album's audio playlist opens as a playlist of the same tracks.
        compare(parse("https://www.youtube.com/playlist?list=OLAK5uy_abc").id, "VLOLAK5uy_abc");
        // An auto-generated mix carries its RD id through.
        compare(parse("https://music.youtube.com/playlist?list=RDCLAK5uy_x").id, "VLRDCLAK5uy_x");

        // Other supported hosts, a bare host paste, and surrounding whitespace.
        compare(parse("https://m.youtube.com/playlist?list=PLabc").id, "VLPLabc");
        compare(parse("music.youtube.com/playlist?list=PLabc").id, "VLPLabc");
        compare(parse("  https://youtu.be/dQw4?list=PLabc  ").id, "VLPLabc");

        // YTM's own /browse page for a playlist is already a VL browse id.
        var b = parse("https://music.youtube.com/browse/VLPLabc");
        verify(b.ok);
        compare(b.id, "VLPLabc");
        compare(b.rawId, "PLabc");
    }

    // A host that is not YouTube, and a lookalike host, are refused as "host" — never fetched.
    function test_wrong_host() {
        var a = parse("https://example.com/playlist?list=PLabc");
        verify(!a.ok);
        compare(a.reason, "host");

        var b = parse("https://youtube.com.evil.test/playlist?list=PLabc");
        verify(!b.ok);
        compare(b.reason, "host");
    }

    // A foreign scheme is refused as "scheme"; only http/https (or a bare host) are opened.
    function test_wrong_scheme() {
        var a = parse("ftp://music.youtube.com/playlist?list=PLabc");
        verify(!a.ok);
        compare(a.reason, "scheme");

        // file: and other schemes are refused too.
        verify(!parse("file:///etc/passwd").ok);
        // Plain text that is not a link at all is neither a host nor a scheme.
        compare(parse("not a url at all").reason, "invalid");
    }

    // A supported link with no playlist to open — the bare site, a single track, an empty /playlist,
    // or a /browse page that is an album/artist — reports the missing playlist rather than guessing.
    function test_missing_playlist() {
        compare(parse("https://music.youtube.com/").reason, "no-playlist");
        compare(parse("https://music.youtube.com/watch?v=dQw4").reason, "no-playlist");
        compare(parse("https://music.youtube.com/playlist").reason, "no-playlist");
        compare(parse("https://music.youtube.com/browse/FEmusic_home").reason, "no-playlist");
        compare(parse("https://music.youtube.com/browse/MPREb_album").reason, "no-playlist");
        compare(parse("https://music.youtube.com/browse/UCartist").reason, "no-playlist");
    }

    // A list= that is present but malformed — empty, VL with nothing after it, or non-id characters
    // — is a "bad-id", not a fetchable playlist.
    function test_bad_id() {
        compare(parse("https://music.youtube.com/playlist?list=").reason, "bad-id");
        compare(parse("https://music.youtube.com/playlist?list=VL").reason, "bad-id");
        // A decoded space is not a valid id character.
        compare(parse("https://music.youtube.com/playlist?list=PL%20abc").reason, "bad-id");
    }

    // Two different list= values are ambiguous; the same value twice is not.
    function test_ambiguous_id() {
        var a = parse("https://music.youtube.com/playlist?list=PLaaa&list=PLbbb");
        verify(!a.ok);
        compare(a.reason, "ambiguous");

        compare(parse("https://music.youtube.com/playlist?list=PLaaa&list=PLaaa").id, "VLPLaaa");
    }

    // A blank box is a neutral "empty" state, not an error the field should shout about.
    function test_empty_is_neutral() {
        compare(parse("").reason, "empty");
        compare(parse("   ").reason, "empty");
        compare(parse(null).reason, "empty");
        compare(parse(undefined).reason, "empty");
    }
}
