import QtQuick
import QtTest
import "../lib/personal.js" as P

// The personal store's pure reducers, run without Quickshell (Personal.qml imports Daemon). These
// are ports of ui/src/lib/personal.check.ts: the rules that are easy to get wrong — recency that
// refreshes but never reorders, a removal that permanently blocks the suggestion, the Home
// arrangement's stable unranked sort and one-time '@familiar' slotting, and hydrate degrading junk
// to empty instead of throwing.
TestCase {
    name: "Personal"

    function item(id) { return { kind: "playlist", id: id, title: id }; }
    function song(id, artistId) {
        var s = { kind: "song", id: id, title: id.toUpperCase(), subtitle: "Artist " + id, thumbnail: id + ".jpg" };
        if (artistId) s.artistId = artistId;
        return s;
    }
    function ids(list) { return list.map(function (x) { return x.id; }).join(","); }
    function keys(list) { return list.map(function (x) { return x.key; }).join(","); }
    function secs() { return Array.prototype.slice.call(arguments).map(function (k) { return { key: k }; }); }

    // addPick appends and refreshes recency; touchPick refreshes without reordering.
    function test_addpick_and_touchpick() {
        var p = P.empty();
        P.addPick(p, item("a"), 100);
        P.addPick(p, item("b"), 200);
        compare(P.addPick(p, item("a"), 300), false);   // a repeat add reports "already there"
        compare(p.picks.length, 2);                      // and does not duplicate
        compare(p.picks.filter(function (x) { return x.id === "a"; })[0].lastUsedAt, 300); // but refreshes
        P.addPick(p, item("c"), 250);
        compare(ids(p.picks), "a,b,c");                  // display order is add order
        compare(P.touchPick(p, "b", 99999), true);
        compare(ids(p.picks), "a,b,c");                  // playing a tile does not move it
        compare(P.touchPick(p, "nope", 1), false);       // a tile not on the grid reports no change
    }

    // A seeded suggestion is refusable, and a removal permanently blocks re-suggestion while still
    // letting the user add it back by hand.
    function test_seed_and_remove_are_permanent() {
        var p = P.empty();
        verify(P.seedPick(p, item("onrepeat")));
        verify(!P.seedPick(p, item("onrepeat")));        // already on the grid
        P.removePick(p, "onrepeat");
        compare(p.picks.length, 0);
        verify(!P.seedPick(p, item("onrepeat")));        // a removed suggestion never comes back
        verify(!P.seedPick(p, item("onrepeat"), 9999));  // not on a later visit either
        compare(P.addPick(p, item("onrepeat"), 1), true); // the user can still add it manually
        compare(ids(p.picks), "onrepeat");
        P.removePick(p, "onrepeat");
        verify(!P.seedPick(p, item("onrepeat")));        // removing again re-arms the dismissal
    }

    // recentItems: newest played-from first, capped by n, empty when nothing played.
    function test_recent_items() {
        var p = P.empty();
        compare(P.recentItems(p).length, 0);
        P.noteRecent(p, item("a"), 100);
        P.noteRecent(p, item("b"), 300);
        P.noteRecent(p, item("c"), 200);
        compare(ids(P.recentItems(p)), "b,c,a");
        compare(ids(P.recentItems(p, 2)), "b,c");
    }

    // noteArtist accumulates play counts and updates the name; topArtistIds keeps only channel-keyed
    // (UC…) entries, ordered by plays.
    function test_note_artist_and_top_ids() {
        var p = P.empty();
        P.noteArtist(p, "UCb", "B");
        P.noteArtist(p, "UCb", "B");
        P.noteArtist(p, "UCa", "A");
        P.noteArtist(p, "Some Band", "Some Band"); // name-keyed: no channel to open
        compare(p.artists["UCb"].count, 2);
        compare(p.artists["UCa"].count, 1);
        compare(P.topArtistIds(p).join(","), "UCb,UCa"); // plays desc, name-keyed dropped
        compare(P.topArtistIds(p, 1).join(","), "UCb");
        compare(P.topArtistIds(P.empty()).length, 0);
        // A later note updates the display name even when it started blank.
        P.noteArtist(p, "UCc", "");
        P.noteArtist(p, "UCc", "C");
        compare(p.artists["UCc"].name, "C");
        compare(p.artists["UCc"].count, 2);
    }

    // firstArtist takes the lead credit out of a joined artist string.
    function test_first_artist() {
        compare(P.firstArtist("Daft Punk"), "Daft Punk");
        compare(P.firstArtist("Daft Punk, Pharrell Williams"), "Daft Punk");
        compare(P.firstArtist("The Weeknd & Ariana Grande"), "The Weeknd");
        compare(P.firstArtist("Drake feat. Rihanna"), "Drake");
    }

    // arrangeSections: saved order wins, hidden ones still return, unranked hold their feed order
    // (they must not compare as NaN), an empty feed is fine.
    function test_arrange_sections() {
        var p = P.empty();
        compare(keys(P.arrangeSections(secs("a", "b", "c"), p)), "a,b,c");
        p.home = { order: ["c", "a"], hidden: ["a"], seen: [] };
        compare(keys(P.arrangeSections(secs("a", "b", "c"), p)), "c,a,b");
        compare(keys(P.arrangeSections(secs("z", "y", "c"), p)), "c,z,y");
        compare(keys(P.arrangeSections([], p)), "");
    }

    // hydrate degrades junk to empty rather than throwing, caps pins, drops malformed tiles and
    // auto-seeded ones from the old build, and filters junk dismissals.
    function test_hydrate_tolerates_junk() {
        compare(P.hydrate(null).picks.length, 0);
        compare(P.hydrate("nonsense").pins.length, 0);
        compare(P.hydrate({ pins: ["a", "b", "c", "d", "e"] }).pins.length, 3);
        compare(P.hydrate({ picks: [{ id: "a" }, {}] }).picks.length, 1);
        var migrated = P.hydrate({ picks: [{ id: "kept", manual: true }, { id: "seeded", manual: false }, { id: "new" }] });
        compare(migrated.picks.map(function (x) { return x.id; }).join(","), "kept,new");
        compare(P.hydrate({ dismissedSeeds: ["a", 7] }).dismissedSeeds.join(","), "a");
    }

    // The Home arrangement survives a round trip, slots '@familiar' into an order saved before it
    // existed exactly once, and degrades a corrupt one instead of throwing.
    function test_hydrate_home_arrangement() {
        var p = P.empty();
        p.home = { order: ["@recent", "Listen again"], hidden: ["@forgotten"], seen: ["Listen again"] };
        var back = P.hydrate(JSON.parse(JSON.stringify(p)));
        compare(back.home.order.join(","), "@recent,@familiar,Listen again");
        compare(back.home.hidden.join(","), "@forgotten");
        compare(back.home.seen.join(","), "Listen again");
        compare(P.hydrate({ home: { order: ["Listen again"], hidden: [] } }).home.order.join(","), "Listen again,@familiar");
        // Slotting happens once, not on every load.
        compare(P.hydrate(JSON.parse(JSON.stringify(back))).home.order.join(","), back.home.order.join(","));
        compare(P.hydrate({}).home.order.length, 0);
        compare(P.hydrate({ home: { order: [1, "a"], hidden: "nope" } }).home.order.join(","), "a,@familiar");
    }

    // A played song is stored as a full navigable BrowseItem, keyed by video id, and shares the
    // recent window with played-from playlists/albums without either shape clobbering the other.
    function test_song_recents_shape_and_mix() {
        var p = P.empty();
        P.noteRecent(p, song("v1", "UCx"), 100);
        P.noteRecent(p, item("PL1"), 200);      // a played-from playlist
        P.noteRecent(p, song("v2"), 300);
        compare(ids(P.recentItems(p)), "v2,PL1,v1"); // newest first, songs and playlists together
        var v1 = p.recent["v1"];
        compare(v1.kind, "song");
        compare(v1.title, "V1");
        compare(v1.subtitle, "Artist v1");
        compare(v1.thumbnail, "v1.jpg");
        compare(v1.artistId, "UCx");
        verify(p.recent["v2"].artistId === undefined); // an unlinked-artist song omits it
        compare(v1.at, 100);
    }

    // Replaying an id refreshes its recency in place (one entry, newest timestamp), never a
    // duplicate row.
    function test_song_recent_replay_refreshes() {
        var p = P.empty();
        P.noteRecent(p, song("v1"), 100);
        P.noteRecent(p, song("v2"), 200);
        P.noteRecent(p, song("v1"), 300);
        compare(Object.keys(p.recent).length, 2);
        compare(p.recent["v1"].at, 300);
        compare(ids(P.recentItems(p)), "v1,v2");
    }

    // Bounded recents: at capacity the OLDEST entries are evicted, never the newest — losing
    // recent listening to a flood of stale rows would be the regression.
    function test_recent_evicts_oldest() {
        var p = P.empty();
        for (var i = 0; i < 105; i++)
            P.noteRecent(p, song("v" + i), i + 1);   // strictly increasing timestamps
        compare(Object.keys(p.recent).length, 100);
        verify(p.recent["v104"] !== undefined);       // newest kept
        verify(p.recent["v5"] !== undefined);         // the five oldest gone, nothing newer
        verify(p.recent["v4"] === undefined);
    }

    // noteArtist stamps lastPlayedAt when a time is given (and refreshes it on later plays) while
    // the count still climbs; called without one it stays backward-compatible and adds no field.
    function test_note_artist_last_played() {
        var p = P.empty();
        P.noteArtist(p, "UCa", "A", 100);
        compare(p.artists["UCa"].lastPlayedAt, 100);
        P.noteArtist(p, "UCa", "A", 500);
        compare(p.artists["UCa"].count, 2);
        compare(p.artists["UCa"].lastPlayedAt, 500);
        P.noteArtist(p, "UCb", "B");                   // legacy call: no timestamp, no field
        verify(!("lastPlayedAt" in p.artists["UCb"]));
    }

    // Signing out of YouTube forgets only what that session can reopen — Liked Music and the
    // account's own "Your …" library playlists — and leaves public playlists, songs and Spotify
    // recents. The cross-provider boundary: a YouTube sign-out must not drop a Spotify recent.
    function test_forget_youtube_recents() {
        var p = P.empty();
        P.noteRecent(p, { kind: "playlist", id: "VLLM", title: "Liked Music" }, 1);
        P.noteRecent(p, { kind: "playlist", id: "LM", title: "LM" }, 2);
        P.noteRecent(p, { kind: "playlist", id: "PLmine", title: "Mine", subtitle: "Your playlist \u00b7 12 songs" }, 3);
        P.noteRecent(p, { kind: "playlist", id: "PLpublic", title: "Chart", subtitle: "YouTube Music \u00b7 50 songs" }, 4);
        P.noteRecent(p, { kind: "playlist", id: "spotify:playlist:abc", title: "Mix", subtitle: "Your Mix" }, 5);
        P.noteRecent(p, song("v1"), 6);
        compare(P.forgetYouTubeRecents(p), 3);
        verify(!p.recent["VLLM"] && !p.recent["LM"] && !p.recent["PLmine"]);
        verify(!!p.recent["PLpublic"] && !!p.recent["v1"]);
        verify(!!p.recent["spotify:playlist:abc"]);   // a Spotify item survives a YouTube sign-out
        compare(P.forgetYouTubeRecents(P.empty()), 0);
    }

    // Signing out of Spotify forgets only provider-qualified spotify: recents and leaves everything
    // else, including a YouTube "Your …" playlist and songs — the mirror image of the boundary above.
    function test_forget_spotify_recents() {
        var p = P.empty();
        P.noteRecent(p, { kind: "playlist", id: "spotify:playlist:abc", title: "Mix", subtitle: "Your Mix" }, 1);
        P.noteRecent(p, { kind: "album", id: "spotify:album:xyz", title: "Album" }, 2);
        P.noteRecent(p, { kind: "playlist", id: "VLLM", title: "Liked Music" }, 3);
        P.noteRecent(p, song("v1"), 4);
        compare(P.forgetSpotifyRecents(p), 2);
        verify(!p.recent["spotify:playlist:abc"] && !p.recent["spotify:album:xyz"]);
        verify(!!p.recent["VLLM"] && !!p.recent["v1"]);
    }

    // deepEqual recognises a save's echo despite serde reordering keys at any depth (so the client
    // drops it), and reports a genuine change as different (so another client's edit still applies)
    // — what keeps a stale full-blob echo from reverting a mutation made while the save was in flight.
    function test_deep_equal_echo_detection() {
        var a = { picks: [], recent: { v1: { kind: "song", id: "v1", at: 100 } }, artists: { UCx: { name: "X", count: 1 } } };
        var b = { artists: { UCx: { count: 1, name: "X" } }, recent: { v1: { at: 100, id: "v1", kind: "song" } }, picks: [] };
        verify(P.deepEqual(a, b));                     // key order never matters, at any depth
        var c = JSON.parse(JSON.stringify(a));
        c.recent["v2"] = { kind: "song", id: "v2", at: 200 };
        verify(!P.deepEqual(a, c));                    // a newer mutation makes the echo stale
        verify(P.deepEqual([1, "a", true, null], [1, "a", true, null]));
        verify(!P.deepEqual([1, 2], [1, 2, 3]));
        verify(!P.deepEqual({ a: 1 }, { a: 1, b: 2 }));
        verify(!P.deepEqual({ a: 1 }, null));
    }
}
