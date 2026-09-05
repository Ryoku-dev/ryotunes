pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The play queue, ported from ui/src/lib/components/QueueList.svelte and reshaped for the eye-candy
// pass. It shows the queue from the playing track forward: a highlighted NOW PLAYING card pinned at
// the top, then an UP NEXT list of 48 px rows (art, a fillWidth single-line title with the explicit
// badge after it, artist beneath, mono duration at the right, a drag grip revealed on hover), with
// "Stop after current" as a subtle outlined button at the foot. Every mutation is a daemon call: a
// grip drag commits move_in_queue (with the drop index corrected the same way the Svelte moveTarget
// does), the hover remove commits remove_from_queue, "Clear" commits clear_queued, and "Stop after
// current" commits set_stop_after_current. A search box filters the visible queue by title/artist/
// album without touching order or playback, keeping each row's real backend index so play/remove
// still hit the right item.
Item {
    id: root

    property string query: ""

    readonly property var q: Playback.queue
    readonly property int currentIndex: (root.q && root.q.currentIndex >= 0) ? root.q.currentIndex : 0
    readonly property var items: (root.q && root.q.items) ? root.q.items : []
    readonly property var nowItem: (root.items.length > root.currentIndex) ? root.items[root.currentIndex] : null

    readonly property bool searching: root.query.trim().length > 0

    // The row rhythm: 48 px art in a 56 px row, a 4 px gap between rows.
    readonly property int rowH: Style.sp(14)
    readonly property int rowStride: root.rowH + Style.sp(1)

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

    // --- drag-reorder state -----------------------------------------------------------------
    property int dragFrom: -1
    property int dragTo: -1
    property real ghostY: 0
    property string ghostTitle: ""
    function indexAt(y) {
        var yy = upList.contentY + y;
        return Math.max(0, Math.min(root.upcoming.length - 1, Math.floor(yy / root.rowStride)));
    }
    function beginDrag(from, y) {
        root.dragFrom = from;
        root.dragTo = from;
        root.ghostY = y;
        var it = root.upcoming[from];
        root.ghostTitle = it ? it.title : "";
    }
    function updateDrag(y) {
        root.ghostY = y;
        root.dragTo = root.indexAt(y);
        var edge = Style.sp(12);
        if (y < edge)
            upList.contentY = Math.max(0, upList.contentY - Style.sp(4));
        else if (y > upList.height - edge)
            upList.contentY = Math.min(Math.max(0, upList.contentHeight - upList.height), upList.contentY + Style.sp(4));
    }
    function endDrag() {
        if (root.dragFrom >= 0 && root.dragTo >= 0 && root.dragFrom !== root.dragTo)
            root.reorder(root.dragFrom, root.dragTo);
        root.dragFrom = -1;
        root.dragTo = -1;
    }

    // A tracked mono micro eyebrow (NOW PLAYING / UP NEXT), left-aligned to the row content.
    component Eyebrow: Text {
        Layout.fillWidth: true
        Layout.leftMargin: Style.sp(3)
        Layout.rightMargin: Style.sp(3)
        color: Tokens.inkMuted
        font.family: Style.fontMono
        font.pixelSize: Style.fs.micro
        font.letterSpacing: Style.trackMicro
    }

    // One queue row: 48 px art, a fillWidth single-line title with the explicit badge after it (the
    // badge is aligned, never stealing the title's width), the artist beneath, the mono duration at
    // the right (swapped for a remove control on hover), and a drag grip on hover for reorderable
    // rows. `now` draws the highlighted paper-lift card for the playing track.
    component QueueRow: Rectangle {
        id: qr
        property var song: null
        property bool now: false
        property bool reorderable: false
        property bool removable: false
        property int rowIndex: -1
        readonly property bool dragging: root.dragFrom === qr.rowIndex && qr.rowIndex >= 0
        signal play()
        signal remove()

        readonly property string dur: (song && song.duration && /^[\d:]+$/.test(song.duration)) ? song.duration : ""

        implicitHeight: root.rowH
        radius: Style.radius
        color: qr.now ? Tokens.paperLift : (rowHover.hovered ? Tokens.tint5 : "transparent")
        border.width: qr.now ? 1 : 0
        border.color: Tokens.lineSoft
        Behavior on color { ColorAnimation { duration: Style.motion.snap } }

        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: Style.sp(3)
            anchors.rightMargin: Style.sp(2)
            spacing: Style.sp(2)

            Artwork {
                Layout.preferredWidth: Style.sp(12)
                Layout.preferredHeight: Style.sp(12)
                url: (qr.song && qr.song.thumbnail) ? qr.song.thumbnail : ""
                px: Style.sp(12)
            }

            ColumnLayout {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                spacing: 0
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(1)
                    Text {
                        Layout.fillWidth: true
                        Layout.minimumWidth: 0
                        text: qr.song ? qr.song.title : ""
                        color: qr.now ? Style.accent : Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        font.weight: Font.Medium
                        elide: Text.ElideRight
                    }
                    Rectangle {
                        visible: !!(qr.song && qr.song.explicit)
                        Layout.alignment: Qt.AlignVCenter
                        Layout.preferredWidth: Style.sp(4)
                        Layout.preferredHeight: Style.sp(4)
                        radius: Style.sp(1)
                        color: "transparent"
                        border.width: 1
                        border.color: Tokens.inkFaint
                        Text { anchors.centerIn: parent; text: "E"; color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.xs; font.weight: Font.DemiBold }
                    }
                }
                Text {
                    Layout.fillWidth: true
                    Layout.minimumWidth: 0
                    text: (qr.song && qr.song.artists) ? qr.song.artists : ""
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    elide: Text.ElideRight
                }
            }

            // Duration, swapped for a remove control while hovering a removable row.
            Item {
                Layout.preferredWidth: Style.sp(9)
                Layout.fillHeight: true
                Text {
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    visible: qr.dur !== "" && !(qr.removable && rowHover.hovered)
                    text: qr.dur
                    color: Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.xs
                }
                IconButton {
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    visible: qr.removable && rowHover.hovered
                    icon: "close"
                    iconSize: Style.fs.sm
                    diameter: Style.sp(8)
                    tip: "Remove from queue"
                    onClicked: qr.remove()
                }
            }

            // Reorder grip: revealed on hover, drags the row to a new index (list coords).
            Item {
                id: gripArea
                visible: qr.reorderable
                Layout.preferredWidth: Style.sp(6)
                Layout.fillHeight: true
                opacity: (rowHover.hovered || qr.dragging) ? 1 : 0
                Behavior on opacity { NumberAnimation { duration: Style.motion.snap } }
                Column {
                    anchors.centerIn: parent
                    spacing: Style.sp(0.75)
                    Repeater {
                        model: 3
                        delegate: Rectangle {
                            width: Style.sp(4)
                            height: Math.max(1, Style.sp(0.5))
                            radius: height / 2
                            color: gripDrag.active ? Tokens.ink : Tokens.inkFaint
                        }
                    }
                }
                DragHandler {
                    id: gripDrag
                    target: null
                    onActiveChanged: {
                        if (active)
                            root.beginDrag(qr.rowIndex, gripArea.mapToItem(upList, 0, centroid.position.y).y);
                        else
                            root.endDrag();
                    }
                    onCentroidChanged: {
                        if (active)
                            root.updateDrag(gripArea.mapToItem(upList, 0, centroid.position.y).y);
                    }
                }
            }
        }

        HoverHandler { id: rowHover; cursorShape: Qt.PointingHandCursor }
        TapHandler { onTapped: qr.play() }
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 0

        // Search box --------------------------------------------------------------------
        Item {
            Layout.fillWidth: true
            Layout.leftMargin: Style.sp(3)
            Layout.rightMargin: Style.sp(3)
            Layout.topMargin: Style.sp(3)
            Layout.bottomMargin: Style.sp(1)
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
            Eyebrow {
                Layout.topMargin: Style.sp(1)
                Layout.bottomMargin: Style.sp(1)
                text: root.matches.length + (root.matches.length === 1 ? " MATCH" : " MATCHES")
            }
            ListView {
                id: searchList
                Layout.fillWidth: true
                Layout.fillHeight: true
                clip: true
                reuseItems: true
                cacheBuffer: Math.max(0, Math.round(height * 1.5))
                boundsBehavior: Flickable.StopAtBounds
                spacing: Style.sp(1)
                model: root.matchItems
                delegate: QueueRow {
                    id: sRow
                    required property var modelData
                    required property int index
                    width: searchList.width
                    song: sRow.modelData
                    removable: true
                    onPlay: root.playBackend(root.matches[sRow.index].i)
                    onRemove: root.removeBackend(root.matches[sRow.index].i)
                }
            }
            Text {
                Layout.fillWidth: true
                Layout.fillHeight: true
                Layout.leftMargin: Style.sp(3)
                Layout.rightMargin: Style.sp(3)
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
        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: !!root.nowItem && !root.searching
            spacing: 0

            Eyebrow {
                Layout.topMargin: Style.sp(1)
                Layout.bottomMargin: Style.sp(1)
                text: "NOW PLAYING"
            }
            QueueRow {
                Layout.fillWidth: true
                Layout.leftMargin: Style.sp(2)
                Layout.rightMargin: Style.sp(2)
                now: true
                song: root.nowItem
                onPlay: root.playBackend(root.currentIndex)
            }

            Eyebrow {
                Layout.topMargin: Style.sp(3)
                Layout.bottomMargin: Style.sp(1)
                text: "UP NEXT"
            }

            // The up-next list, with the reorder drop marker and the floating ghost row.
            Item {
                id: upWrap
                Layout.fillWidth: true
                Layout.fillHeight: true

                ListView {
                    id: upList
                    anchors.fill: parent
                    clip: true
                    reuseItems: true
                    cacheBuffer: Math.max(0, Math.round(height * 1.5))
                    boundsBehavior: Flickable.StopAtBounds
                    spacing: Style.sp(1)
                    model: root.upcoming
                    // A drag in progress must not also flick the list.
                    interactive: root.dragFrom < 0

                    delegate: Item {
                        id: wrap
                        required property var modelData
                        required property int index
                        width: upList.width
                        implicitHeight: root.rowH

                        Rectangle {
                            visible: root.dragFrom >= 0 && root.dragTo === wrap.index
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.top: parent.top
                            height: 2
                            color: Style.accent
                        }
                        QueueRow {
                            anchors.left: parent.left
                            anchors.right: parent.right
                            song: wrap.modelData
                            rowIndex: wrap.index
                            reorderable: true
                            removable: true
                            opacity: root.dragFrom === wrap.index ? 0.35 : 1
                            onPlay: root.playBackend(root.currentIndex + 1 + wrap.index)
                            onRemove: root.removeBackend(root.currentIndex + 1 + wrap.index)
                        }
                    }
                }

                Text {
                    anchors.centerIn: parent
                    visible: root.upcoming.length === 0
                    text: "Nothing up next."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                }

                // The floating row that follows the grip during a reorder.
                Rectangle {
                    visible: root.dragFrom >= 0
                    width: upWrap.width - Style.sp(6)
                    x: Style.sp(3)
                    y: Math.max(0, Math.min(upWrap.height - height, root.ghostY - height / 2))
                    implicitHeight: root.rowH
                    radius: Style.radius
                    color: Tokens.paperLift
                    border.width: 1
                    border.color: Tokens.lineStrong
                    opacity: 0.95
                    Text {
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        anchors.leftMargin: Style.sp(3)
                        anchors.rightMargin: Style.sp(3)
                        text: root.ghostTitle
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        font.weight: Font.Medium
                        elide: Text.ElideRight
                    }
                }
            }

            // Foot: Stop after current (subtle outlined), and Clear when there is a queued tail.
            ColumnLayout {
                Layout.fillWidth: true
                Layout.leftMargin: Style.sp(3)
                Layout.rightMargin: Style.sp(3)
                Layout.topMargin: Style.sp(2)
                Layout.bottomMargin: Style.sp(3)
                spacing: Style.sp(2)
                Hairline { Layout.fillWidth: true; soft: true }
                RowLayout {
                    Layout.fillWidth: true
                    Layout.topMargin: Style.sp(1)
                    spacing: Style.sp(2)
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
}
