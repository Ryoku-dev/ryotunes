import QtQuick
import QtTest
import "../lib/recommendations.js" as R

// The Home discovery ranker, exercised as the pure function it is — no Quickshell, no daemon, no
// network — the way tst_personal.qml runs the personal reducers. Each case pins a rule that is easy
// to get wrong: full id dedup, the provider boundary, decayed replay fatigue, the MMR diversity
// pass (both directions of the relevance/diversity trade), an honest zero-history cold start, and
// that build never mutates the feed or the personal blob it reads.
TestCase {
    name: "Recommendations"

    // A fixed clock so decay maths is exact and the suite is deterministic.
    readonly property double now: 1000 * 86400000   // day 1000, in ms
    readonly property double day: 86400000

    function song(id, artist) {
        return { id: id, kind: "song", title: id, subtitle: artist, artistId: artist, thumbnail: "t" };
    }
    function sec(title, items) { return { title: title, items: items }; }
    function ids(list) { return list.map(function (x) { return x.id; }).join(","); }

    // Full id dedup across sections: an id seen again — even in a later shelf, even with different
    // metadata — is dropped, and the first (best-ranked) occurrence is the one kept.
    function test_dedup_across_sections() {
        var secs = [
            sec("A", [song("x", "UCx"), song("y", "UCy")]),
            sec("B", [song("x", "UCx"), song("z", "UCz")])
        ];
        var out = R.build(secs, {}, "youtube", now).items;
        compare(out.filter(function (i) { return i.id === "x"; }).length, 1); // x appears once
        compare(out.length, 3);                                               // x, y, z
    }

    // Provider boundary: a YouTube feed drops rows that positively belong to another catalogue
    // (Spotify/SoundCloud uris and on-disk LOCAL ids); the provider's own rows stay.
    function test_cross_provider_exclusion() {
        var secs = [sec("A", [
            song("vid1", "UCa"),
            song("spotify:track:foo", "spotify:artist:z"),
            song("sc:track:123", "sc:artist:45"),
            song("LOCAL:1", "UCb"),
            song("vid2", "UCc")
        ])];
        var out = R.build(secs, {}, "youtube", now).items;
        compare(ids(out), "vid1,vid2");
    }

    // Replay fatigue: a track played very recently is pushed below an otherwise weaker (later-ranked)
    // row, but the penalty decays — the same play long ago no longer demotes it.
    function test_replay_fatigue_decays() {
        var secs = [sec("A", [song("A", "UCa"), song("B", "UCb")])];

        var justPlayed = { recent: { "A": { at: now - day } } };            // A played ~1 day ago
        var fresh = R.build(secs, justPlayed, "youtube", now).items;
        verify(ids(fresh).indexOf("B") < ids(fresh).indexOf("A"));          // B overtakes fatigued A

        var longAgo = { recent: { "A": { at: now - 40 * day } } };          // fatigue decayed away
        var stale = R.build(secs, longAgo, "youtube", now).items;
        compare(ids(stale), "A,B");                                         // rank order restored
    }

    // MMR diversity, both directions. When a different-artist row is close in relevance it is lifted
    // above the second same-artist row; when it is genuinely far weaker, the strong same-artist row
    // still wins. And the whole pass is deterministic — same inputs, same order, every call.
    function test_diversity_rerank_and_determinism() {
        // Close call: A,B share an artist, C is a fresh artist one rank back -> C beats B.
        var close = [sec("S", [song("A", "UCx"), song("B", "UCx"), song("C", "UCy")])];
        var lifted = R.build(close, {}, "youtube", now).items;
        compare(lifted[0].id, "A");                                    // top relevance leads
        verify(ids(lifted).indexOf("C") < ids(lifted).indexOf("B"));   // diversity lifts C

        // Genuinely weak alternative: the fresh-artist D sits far down the feed; the strong
        // same-artist B still outranks it — relevance is allowed to win.
        var items = [song("A", "UCx"), song("B", "UCx")];
        for (var k = 0; k < 40; k++)
            items.push(song("pad" + k, "UCx"));
        items.push(song("D", "UCy"));
        var out = R.build([sec("S", items)], {}, "youtube", now).items;
        verify(ids(out).indexOf("B") < ids(out).indexOf("D"));

        // Determinism: identical inputs yield an identical order.
        var a = R.build(close, {}, "youtube", now).items;
        var b = R.build(close, {}, "youtube", now).items;
        compare(ids(a), ids(b));
    }

    // Without history, distinct artists retain provider order and the feed stays untouched.
    function test_zero_history_cold_start_and_purity() {
        var items = [song("a", "UCa"), song("b", "UCb"), song("c", "UCc")];
        var secs = [sec("S", items)];
        var frozen = JSON.stringify(secs);

        var res = R.build(secs, {}, "youtube", now);
        compare(ids(res.items), "a,b,c");                      // provider order preserved
        compare(JSON.stringify(secs), frozen);                // feed untouched
    }

    // Empty and placeholder rows must not become playable recommendations.
    function test_empty_and_unplayable_feed() {
        var empty = R.build([], {}, "youtube", now);
        compare(empty.items.length, 0);

        // Rows missing an id or a title are the placeholder rows a discovery shelf must not promote.
        var junk = R.build([sec("S", [{ id: "", title: "" }, { kind: "song" }, null])], {}, "youtube", now);
        compare(junk.items.length, 0);
    }

    // The shelf is bounded to twelve however long the feed is.
    function test_caps_at_twelve() {
        var items = [];
        for (var k = 0; k < 20; k++)
            items.push(song("s" + k, "UC" + k));
        compare(R.build([sec("S", items)], {}, "youtube", now).items.length, 12);
    }

    // Listening history can change ranking only within its own provider.
    function test_personalization_is_provider_scoped() {
        var yt = [sec("S", [song("a", "UCa"), song("b", "UCb")])];
        var blob = { artists: { "UCb": { name: "B", count: 5, lastPlayedAt: now - 2 * day } }, recent: {} };
        var frozen = JSON.stringify(blob);
        var hot = R.build(yt, blob, "youtube", now);
        compare(ids(hot.items), "b,a");

        var sp = [sec("S", [song("spotify:track:1", "spotify:artist:1"),
                            song("spotify:track:2", "spotify:artist:2")])];
        var cold = R.build(sp, blob, "spotify", now);
        compare(ids(cold.items), "spotify:track:1,spotify:track:2");
        compare(JSON.stringify(blob), frozen);
    }
}
