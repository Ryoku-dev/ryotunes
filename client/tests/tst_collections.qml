import QtQuick
import QtTest
import "../lib/collections.js" as Collections

// groupJobs collapses batch-added download jobs into one card per collection while leaving
// single tracks as flat rows; stats aggregates a card's progress. Both are pure over the
// daemon's job JSON, so this runs under qmltestrunner without Quickshell.
TestCase {
    name: "Collections"

    function job(id, status, collection, kind, progress) {
        var j = { videoId: id, status: status };
        if (collection !== undefined)
            j.collection = collection;
        if (kind !== undefined)
            j.collectionKind = kind;
        if (progress !== undefined)
            j.progress = progress;
        return j;
    }

    // A mixed list: one album (3 jobs), one single, the same album continued after the single
    // (a later batch member must join the FIRST card, not open a second one).
    function test_groups_collapse_by_label_in_first_position() {
        var list = [
            job("a1", "downloading", "Discovery", "playlist", 40),
            job("s1", "queued"),
            job("a2", "queued", "Discovery", "playlist"),
            job("b1", "completed", "Singles Mix", "playlist"),
            job("a3", "completed", "Discovery", "playlist")
        ];
        var out = Collections.groupJobs(list);
        compare(out.length, 3, "2 cards + 1 flat row: the album collapses even across the single");
        verify(!!out[0].group);
        compare(out[0].group.label, "Discovery");
        compare(out[0].group.kind, "playlist");
        compare(out[0].group.jobs.length, 3);
        verify(!!out[1].job, "the single stays a flat row");
        compare(out[1].job.videoId, "s1");
        compare(out[2].group.label, "Singles Mix");
    }

    function test_empty_and_missing_labels_stay_flat() {
        var out = Collections.groupJobs([job("x", "completed"), job("y", "failed", "")]);
        compare(out.length, 2);
        verify(!!out[0].job && !!out[1].job);
        compare(Collections.groupJobs([]).length, 0);
    }

    function test_kind_defaults_to_collection() {
        var out = Collections.groupJobs([job("z", "queued", "Lo-fi Beats")]);
        compare(out[0].group.kind, "collection");
    }

    // Completed and terminal jobs count as full; the downloading one contributes its own
    // percent; waiting contributes nothing.
    function test_stats_weight_progress_across_statuses() {
        var s = Collections.stats([
            job("a", "completed"),
            job("b", "downloading", "X", "album", 50),
            job("c", "queued", "X", "album"),
            job("d", "failed", "X", "album")
        ]);
        compare(s.done, 1);
        compare(s.moving, 1);
        compare(s.waiting, 1);
        compare(s.failed, 1);
        // (100 + 50 + 0 + 100) / 4
        compare(s.percent, 63);
    }

    function test_stats_on_empty_is_quiet() {
        var s = Collections.stats([]);
        compare(s.percent, 0);
        compare(s.done, 0);
        compare(s.moving, 0);
    }
}
