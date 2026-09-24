.pragma library

// Collection grouping for the downloads queue/history. The daemon stamps every batch-added job
// with its `collection` label and `collectionKind`; the page collapses jobs that share a label
// into one card so a 40-track album reads as one line, while single tracks stay flat rows.
// Pure over plain objects (the daemon's job JSON) so it runs under qmltestrunner.
//
// Result shape: a list of `{ job }` (flat row) or `{ group }` entries, where a group is
// `{ label, kind, jobs }`. A group is placed where its first member sorted, preserving the
// caller's queue/history order.

function groupJobs(list) {
    var out = [];
    var byLabel = {};
    for (var i = 0; i < list.length; i++) {
        var j = list[i];
        var label = (j && j.collection) ? String(j.collection) : "";
        if (label === "") {
            out.push({ job: j });
            continue;
        }
        var g = byLabel[label];
        if (!g) {
            g = { label: label, kind: j.collectionKind || "collection", jobs: [] };
            byLabel[label] = g;
            out.push({ group: g });
        }
        g.jobs.push(j);
    }
    return out;
}

// Aggregate progress for one collection card. Completed tracks count as full; a downloading
// one contributes its own percent; failed/cancelled count as finished attempts (they stop the
// bar from ever reaching 100 while nothing is moving).
function stats(jobs) {
    var s = { done: 0, waiting: 0, moving: 0, failed: 0, cancelled: 0, percent: 0 };
    if (!jobs || jobs.length === 0)
        return s;
    var sum = 0;
    for (var i = 0; i < jobs.length; i++) {
        var j = jobs[i];
        if (j.status === "completed") {
            s.done++;
            sum += 100;
        } else if (j.status === "downloading") {
            s.moving++;
            sum += Math.max(0, Math.min(100, j.progress || 0));
        } else if (j.status === "queued") {
            s.waiting++;
        } else if (j.status === "failed") {
            s.failed++;
            sum += 100;
        } else if (j.status === "cancelled") {
            s.cancelled++;
            sum += 100;
        }
    }
    s.percent = Math.round(sum / jobs.length);
    return s;
}
