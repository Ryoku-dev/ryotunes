pragma Singleton
import QtQuick
import Quickshell
import "lib/ids.js" as Ids

// YouTube's library browse is eventually consistent: for a few seconds after a create or a delete,
// get_library still answers with the old list, so a playlist appeared or vanished only after a
// restart. This keeps the pending edits and lays them over every get_library answer (the sidebar
// and the Library page), drops each one once YouTube's list reflects it, and asks the lists to
// reload a few times meanwhile. ui/src/lib/player.svelte.ts does the same for a create.
Singleton {
    id: root

    property var created: []      // [{ kind, id, title }], newest first, browse ids
    property var deleted: ({})    // id -> true
    readonly property bool pending: root.created.length > 0 || Object.keys(root.deleted).length > 0

    // The lists reload on this until YouTube has caught up (or the tries run out).
    signal refreshRequested()

    function noteCreated(id, title) {
        var bid = Ids.playlistBrowseId(id);
        if (!bid)
            return;
        root.created = [{ kind: "playlist", id: bid, title: title || "New playlist" }]
            .concat(root.created.filter((c) => c.id !== bid));
        if (root.deleted[bid]) {
            var d = Object.assign({}, root.deleted);
            delete d[bid];
            root.deleted = d;
        }
        root.arm();
    }

    function noteDeleted(id) {
        if (!id)
            return;
        var d = Object.assign({}, root.deleted);
        d[id] = true;
        root.deleted = d;
        root.created = root.created.filter((c) => c.id !== id);
        root.arm();
    }

    // A get_library answer with the pending edits applied. Pure, so it can sit in a binding.
    function overlay(items) {
        var list = items || [];
        var seen = ({});
        for (var i = 0; i < list.length; i++)
            if (list[i] && list[i].id)
                seen[list[i].id] = true;
        var out = root.created.filter((c) => !seen[c.id]);
        for (var j = 0; j < list.length; j++)
            if (!(list[j] && root.deleted[list[j].id]))
                out.push(list[j]);
        return out;
    }

    // Forget the edits a fresh get_library answer already shows.
    function reconcile(items) {
        if (!root.pending)
            return;
        var list = items || [];
        var seen = ({});
        for (var i = 0; i < list.length; i++)
            if (list[i] && list[i].id)
                seen[list[i].id] = true;
        var c = root.created.filter((x) => !seen[x.id]);
        if (c.length !== root.created.length)
            root.created = c;
        var d = ({}), changed = false;
        for (var k in root.deleted) {
            if (seen[k]) d[k] = true;
            else changed = true;
        }
        if (changed)
            root.deleted = d;
    }

    property int tries: 0
    function arm() {
        root.tries = 0;
        settle.restart();
    }
    Timer {
        id: settle
        interval: 3000
        repeat: true
        onTriggered: {
            root.tries++;
            // Out of tries the edits stay laid over (a deleted playlist must not come back); only
            // the reloads stop.
            if (!root.pending || root.tries > 5)
                settle.stop();
            root.refreshRequested();
        }
    }
}
