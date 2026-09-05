pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"
import "../lib/search.js" as Search
import "../lib/browse.js" as Browse

// The Ctrl+K command palette, ported from CommandPalette.svelte. A modal typeahead over the same
// search_all → previewMix rows the field shows; Up/Down move, Enter takes the row (or, at the tail,
// "All results" routes to the full search page), Escape closes. Opened by the App's global shortcut.
// The modal card sits over a scrim; the page behind is frozen and blurred by App while any modal is
// open (spec 9's snapshot blur is a `layer.live:false` pass on the content, owned by App).
Item {
    id: root

    anchors.fill: parent
    visible: root.open
    z: 400

    property bool open: false
    property string query: ""
    property var items: []
    property bool loading: false
    property int active: 0

    onOpenChanged: {
        if (root.open) {
            root.query = "";
            root.items = [];
            root.active = 0;
            input.forceActiveFocus();
        }
    }

    // "All results" sits at the tail when the query is long enough.
    readonly property bool hasAll: root.query.trim().length >= 2
    readonly property int rowCount: root.items.length + (root.hasAll ? 1 : 0)

    function openItem(item) {
        if (item.kind === "song")
            Playback.play(Browse.asSong(item));
        else
            Router.push(item.kind, { id: item.id, title: item.title });
    }
    function choose(i) {
        if (root.hasAll && i === root.items.length) {
            Router.push("search", { q: root.query.trim() });
            root.open = false;
            return;
        }
        var item = root.items[i];
        if (!item)
            return;
        root.openItem(item);
        root.open = false;
    }
    function runQuery() {
        var q = root.query.trim().replace(/\s+/g, " ");
        if (q.length < 2) {
            root.items = [];
            root.loading = false;
            return;
        }
        root.loading = true;
        var reqFor = q;
        Daemon.call("search_all", { query: q })
            .then((res) => {
                if (root.query.trim().replace(/\s+/g, " ") !== reqFor)
                    return;
                root.items = Search.previewMix(res, 12);
                root.active = 0;
                root.loading = false;
            })
            .catch(() => root.loading = false);
    }

    Timer { id: debounce; interval: 150; onTriggered: root.runQuery() }
    onQueryChanged: {
        if (root.query.trim().length < 2) { debounce.stop(); root.items = []; return; }
        root.loading = true;
        debounce.restart();
    }

    // Scrim over the (App-blurred) page; a click outside the card dismisses.
    Rectangle { anchors.fill: parent; color: "#000000"; opacity: 0.45 }
    MouseArea { anchors.fill: parent; onClicked: root.open = false }

    // The modal card.
    Rectangle {
        anchors.top: parent.top
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.topMargin: Style.sp(20)
        width: Style.sp(160)
        implicitHeight: paletteCol.implicitHeight
        height: implicitHeight
        radius: Style.radiusCard
        color: Tokens.paperLift
        border.width: 1
        border.color: Tokens.lineStrong
        MouseArea { anchors.fill: parent }

        ColumnLayout {
            id: paletteCol
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            spacing: 0

            // field
            RowLayout {
                Layout.fillWidth: true
                Layout.margins: Style.sp(3)
                spacing: Style.sp(2)
                Icon { name: "search"; size: Style.fs.lg; color: Tokens.inkMuted }
                TextInput {
                    id: input
                    Layout.fillWidth: true
                    verticalAlignment: TextInput.AlignVCenter
                    clip: true
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.lg
                    text: root.query
                    onTextChanged: root.query = text
                    onAccepted: root.choose(root.active)
                    Keys.onPressed: (e) => {
                        if (e.key === Qt.Key_Down) { if (root.rowCount) root.active = (root.active + 1) % root.rowCount; e.accepted = true; }
                        else if (e.key === Qt.Key_Up) { if (root.rowCount) root.active = (root.active - 1 + root.rowCount) % root.rowCount; e.accepted = true; }
                        else if (e.key === Qt.Key_Escape) { root.open = false; e.accepted = true; }
                    }
                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        visible: input.text.length === 0
                        text: "Search everything…"
                        color: Tokens.inkFaint
                        font: input.font
                    }
                }
            }

            Hairline { Layout.fillWidth: true }

            Text {
                Layout.fillWidth: true
                Layout.margins: Style.sp(3)
                visible: root.query.trim().length < 2
                text: "Type to search."
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
            }
            Text {
                Layout.fillWidth: true
                Layout.margins: Style.sp(3)
                visible: root.query.trim().length >= 2 && !root.loading && root.items.length === 0
                text: "Nothing quick for that."
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
            }

            ListView {
                id: palList
                Layout.fillWidth: true
                Layout.preferredHeight: Math.min(contentHeight, Style.sp(120))
                visible: root.items.length > 0
                clip: true
                reuseItems: true
                boundsBehavior: Flickable.StopAtBounds
                model: root.items
                delegate: Rectangle {
                    id: pRow
                    required property var modelData
                    required property int index
                    width: palList.width
                    implicitHeight: Style.sp(11)
                    color: (root.active === pRow.index) ? Tokens.tint10 : pHover.hovered ? Tokens.tint5 : "transparent"
                    RowLayout {
                        anchors.fill: parent
                        anchors.leftMargin: Style.sp(3)
                        anchors.rightMargin: Style.sp(3)
                        spacing: Style.sp(2)
                        Artwork {
                            url: pRow.modelData.thumbnail ? pRow.modelData.thumbnail : ""
                            px: Style.sp(10)
                            round: pRow.modelData.kind === "artist"
                            placeholderIcon: pRow.modelData.kind === "artist" ? "user" : pRow.modelData.kind === "album" ? "cd" : pRow.modelData.kind === "playlist" ? "playlist" : "music"
                        }
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 0
                            Text { Layout.fillWidth: true; text: pRow.modelData.title; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium; elide: Text.ElideRight }
                            Text { Layout.fillWidth: true; text: pRow.modelData.subtitle ? pRow.modelData.subtitle : pRow.modelData.kind; color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; elide: Text.ElideRight }
                        }
                        Text { text: pRow.modelData.kind.toUpperCase(); color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
                    }
                    HoverHandler { id: pHover }
                    MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; hoverEnabled: true; onEntered: root.active = pRow.index; onClicked: root.choose(pRow.index) }
                }
            }

            // "All results"
            Rectangle {
                Layout.fillWidth: true
                implicitHeight: Style.sp(11)
                visible: root.hasAll
                color: (root.active === root.items.length) ? Tokens.tint10 : allHover.hovered ? Tokens.tint5 : "transparent"
                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(3)
                    anchors.rightMargin: Style.sp(3)
                    spacing: Style.sp(2)
                    Icon { name: "search"; size: Style.fs.md; color: Tokens.inkMuted }
                    Text { Layout.fillWidth: true; text: "All results for \u201C" + root.query.trim() + "\u201D"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; elide: Text.ElideRight }
                    Icon { name: "arrow-right"; size: Style.fs.sm; color: Tokens.inkMuted }
                }
                HoverHandler { id: allHover }
                MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: root.choose(root.items.length) }
            }
        }
    }
}
