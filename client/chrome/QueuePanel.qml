pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The play queue, ported from ui/src/lib/components/QueueList.svelte. It shows the queue from the
// playing track forward — the current row pinned at the top, then "Up next" — over a reused
// TrackList so a five-figure queue stays a couple of screenfuls of live delegates. Every mutation is
// a daemon call: a grip drag commits move_in_queue (with the drop index corrected the same way the
// Svelte moveTarget does), the row menu's remove commits remove_from_queue, "Clear queue" commits
// clear_queued, and "Stop after current" commits set_stop_after_current. A search box filters the
// visible queue by title/artist/album without touching order or playback, keeping each row's real
// backend index so play/remove still hit the right item.
Item {
    id: root

    property string query: ""

    readonly property var q: Playback.queue
    readonly property int currentIndex: (root.q && root.q.currentIndex >= 0) ? root.q.currentIndex : 0
    readonly property var items: (root.q && root.q.items) ? root.q.items : []
    readonly property var nowItem: (root.items.length > root.currentIndex) ? root.items[root.currentIndex] : null
    readonly property string sourceName: (root.q && root.q.sourceName) ? root.q.sourceName : "Queue"

    readonly property bool searching: root.query.trim().length > 0

    // Upcoming tracks (backend indices currentIndex+1 .. end), the draggable body of the panel.
    readonly property var upcoming: {
        var out = [];
        for (var i = root.currentIndex + 1; i < root.items.length; i++)
            out.push(root.items[i]);
        return out;
    }
    readonly property bool hasQueued: {
        for (var i = 0; i < root.upcoming.length; i++) {
            var t = root.upcoming[i];
            if (t.queued || t.queued_end)
                return true;
        }
        return false;
    }

    // Visual search over the playing track and everything after it, each row carrying its real
    // backend index so a hit still plays/removes the exact queue item.
    readonly property var matches: {
        var ql = root.query.trim().toLowerCase();
        var out = [];
        if (!ql)
            return out;
        for (var i = root.currentIndex; i < root.items.length; i++) {
            var t = root.items[i];
            if ((t.title && t.title.toLowerCase().indexOf(ql) >= 0)
                || (t.artists && t.artists.toLowerCase().indexOf(ql) >= 0)
                || (t.album && t.album.toLowerCase().indexOf(ql) >= 0))
                out.push({ item: t, i: i });
        }
        return out;
    }
    readonly property var matchItems: root.matches.map((m) => m.item)

    // --- daemon actions ---------------------------------------------------------------------
    function playBackend(i) {
        Daemon.call("play_index", { index: i })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }
    function removeBackend(i) {
        Daemon.call("remove_from_queue", { index: i })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not remove", "error"));
    }
    // A grip dropped in front of `toArr` lands one slot earlier once the dragged row is out of the
    // way when moving down — the moveTarget off-by-one, applied in backend space.
    function reorder(fromArr, toArr) {
        var offset = root.currentIndex + 1;
        var bf = offset + fromArr;
        var bDrop = offset + toArr;
        var bt = bDrop > bf ? bDrop - 1 : bDrop;
        if (bt === bf || bt < offset)
            return;
        Daemon.call("move_in_queue", { from: bf, to: bt })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not reorder", "error"));
    }
    function toggleStop() {
        Daemon.call("set_stop_after_current", { enabled: !Playback.stopAfterCurrent })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not update", "error"));
    }
    function clearQueue() {
        Daemon.call("clear_queued")
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not clear", "error"));
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 0

        // Search box --------------------------------------------------------------------
        Item {
            Layout.fillWidth: true
            Layout.margins: Style.sp(2)
            implicitHeight: Style.ctlH
            Rectangle {
                anchors.fill: parent
                radius: Style.radius
                color: Tokens.tint5
                border.width: 1
                border.color: queueFilter.activeFocus ? Tokens.line : Tokens.lineSoft
                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(2)
                    spacing: Style.sp(2)
                    Icon { name: "search"; size: Style.fs.sm; color: Tokens.inkMuted }
                    TextInput {
                        id: queueFilter
                        Layout.fillWidth: true
                        verticalAlignment: TextInput.AlignVCenter
                        clip: true
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        text: root.query
                        onTextChanged: root.query = text
                        Text {
                            anchors.verticalCenter: parent.verticalCenter
                            visible: queueFilter.text.length === 0
                            text: "Search queue"
                            color: Tokens.inkFaint
                            font: queueFilter.font
                        }
                    }
                }
            }
        }

        // Empty state -------------------------------------------------------------------
        Text {
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: !root.nowItem
            horizontalAlignment: Text.AlignHCenter
            verticalAlignment: Text.AlignVCenter
            text: "The queue is empty."
            color: Tokens.inkMuted
            font.family: Style.fontUi
            font.pixelSize: Style.fs.md
        }

        // Search results ----------------------------------------------------------------
        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: !!root.nowItem && root.searching
            spacing: 0
            Text {
                Layout.fillWidth: true
                Layout.leftMargin: Style.sp(3)
                Layout.rightMargin: Style.sp(3)
                Layout.bottomMargin: Style.sp(1)
                text: root.matches.length + (root.matches.length === 1 ? " match" : " matches")
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
            }
            TrackList {
                id: searchList
                Layout.fillWidth: true
                Layout.fillHeight: true
                items: root.matchItems
                hideThumb: false
                canAdd: true
                canRemove: true
                removeLabel: "Remove from queue"
                source: root.sourceName
                onActivated: (i) => root.playBackend(root.matches[i].i)
                onRemoveAt: (i) => root.removeBackend(root.matches[i].i)
            }
            Text {
                Layout.fillWidth: true
                Layout.fillHeight: true
                visible: root.matches.length === 0
                horizontalAlignment: Text.AlignHCenter
                verticalAlignment: Text.AlignVCenter
                text: "No queued songs match “" + root.query.trim() + "”."
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                wrapMode: Text.WordWrap
            }
        }

        // Now playing + Up next ---------------------------------------------------------
        TrackList {
            id: queueList
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: !!root.nowItem && !root.searching
            items: root.upcoming
            reorderable: true
            canAdd: true
            canRemove: true
            removeLabel: "Remove from queue"
            source: root.sourceName
            header: queueHeader
            footer: queueFooter
            onActivated: (i) => root.playBackend(root.currentIndex + 1 + i)
            onRemoveAt: (i) => root.removeBackend(root.currentIndex + 1 + i)
            onMoved: (from, to) => root.reorder(from, to)
        }
    }

    // The pinned now-playing row and the "Up next" heading, scrolling with the list.
    Component {
        id: queueHeader
        ColumnLayout {
            width: queueList.view.width
            spacing: 0

            Text {
                Layout.fillWidth: true
                Layout.leftMargin: Style.sp(3)
                Layout.topMargin: Style.sp(2)
                Layout.bottomMargin: Style.sp(1)
                text: "NOW PLAYING"
                color: Tokens.inkMuted
                font.family: Style.fontMono
                font.pixelSize: Style.fs.micro
                font.letterSpacing: Style.trackMicro
            }
            TrackRow {
                Layout.fillWidth: true
                song: root.nowItem
                index: root.currentIndex
                active: true
                menu: false
                onPlay: root.playBackend(root.currentIndex)
            }

            ColumnLayout {
                Layout.fillWidth: true
                Layout.leftMargin: Style.sp(3)
                Layout.rightMargin: Style.sp(2)
                Layout.topMargin: Style.sp(3)
                Layout.bottomMargin: Style.sp(1)
                spacing: Style.sp(1.5)
                Text {
                    Layout.fillWidth: true
                    text: "UP NEXT"
                    color: Tokens.inkMuted
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
                }
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(2)
                    visible: !!root.nowItem
                    Btn {
                        Layout.fillWidth: true
                        text: Playback.stopAfterCurrent ? "Stopping after this" : "Stop after current"
                        icon: Playback.stopAfterCurrent ? "check-circle" : ""
                        onClicked: root.toggleStop()
                    }
                    Btn {
                        visible: root.hasQueued
                        text: "Clear"
                        onClicked: root.clearQueue()
                    }
                }
            }
        }
    }

    Component {
        id: queueFooter
        Item {
            width: queueList.view.width
            implicitHeight: Style.sp(12)
            Text {
                anchors.centerIn: parent
                visible: root.upcoming.length === 0
                text: "Nothing up next."
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
            }
        }
    }
}
