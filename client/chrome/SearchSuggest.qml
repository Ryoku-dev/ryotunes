pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"
import "../lib/search.js" as Search
import "../lib/browse.js" as Browse

// The search field with a live typeahead, ported from SearchSuggest.svelte. The input feeds a
// 150 ms-debounced search_all whose result is previewMixed into a small ranked set; Up/Down move the
// selection, Enter takes the highlighted row (or submits the query), Escape closes. Picking a song
// plays it, anything else routes to its page — the same openItem every browse surface uses.
Item {
    id: root

    property string value: ""
    property string placeholder: "Search songs, albums, artists, playlists…"
    property var items: []
    property bool loading: false
    property int active: -1
    property bool panelOpen: false
    property string loadedFor: ""

    signal picked()
    signal submitted()

    implicitHeight: field.height

    function openItem(item) {
        if (item.kind === "song")
            Playback.play(Browse.asSong(item));
        else
            Router.push(item.kind, { id: item.id, title: item.title });
    }
    function choose(item) {
        root.panelOpen = false;
        root.openItem(item);
        root.picked();
    }
    function forceFocus() { input.forceActiveFocus(); }
    function runQuery() {
        var q = root.value.trim().replace(/\s+/g, " ");
        if (q.length < 2) {
            root.items = [];
            root.panelOpen = false;
            return;
        }
        root.loading = true;
        root.panelOpen = true;
        var reqFor = q;
        Daemon.call("search_all", { query: q })
            .then((res) => {
                if (root.value.trim().replace(/\s+/g, " ") !== reqFor)
                    return;
                root.items = Search.previewMix(res, 12);
                root.active = root.items.length ? 0 : -1;
                root.loadedFor = reqFor;
                root.loading = false;
            })
            .catch(() => { root.loading = false; });
    }

    Timer { id: debounce; interval: 150; onTriggered: root.runQuery() }
    onValueChanged: {
        // A programmatic value (a ?q= arrival that binds `value`) must not pop the dropdown; only a
        // user typing into the focused field does.
        if (!input.activeFocus) { debounce.stop(); return; }
        if (root.value.trim().length < 2) { debounce.stop(); root.items = []; root.panelOpen = false; return; }
        root.loading = true;
        root.panelOpen = true;
        debounce.restart();
    }

    // the field
    Rectangle {
        id: field
        width: parent.width
        implicitHeight: Style.sp(11)
        radius: Style.radius
        color: Tokens.paperLift
        border.width: 1
        border.color: input.activeFocus ? Tokens.lineStrong : Tokens.line
        Behavior on border.color { ColorAnimation { duration: Style.motion.snap } }
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: Style.sp(3)
            anchors.rightMargin: Style.sp(2)
            spacing: Style.sp(2)
            Icon { name: "search"; size: Style.fs.md; color: Tokens.inkMuted }
            TextInput {
                id: input
                Layout.fillWidth: true
                verticalAlignment: TextInput.AlignVCenter
                clip: true
                color: Tokens.ink
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
                text: root.value
                onTextChanged: root.value = text
                onAccepted: {
                    if (root.panelOpen && root.active >= 0 && root.active < root.items.length) {
                        root.choose(root.items[root.active]);
                    } else {
                        root.panelOpen = false;
                        root.submitted();
                    }
                }
                Keys.onPressed: (e) => {
                    if (e.key === Qt.Key_Down) { if (root.items.length) root.active = (root.active + 1) % root.items.length; e.accepted = true; }
                    else if (e.key === Qt.Key_Up) { if (root.items.length) root.active = (root.active - 1 + root.items.length) % root.items.length; e.accepted = true; }
                    else if (e.key === Qt.Key_Escape) { root.panelOpen = false; e.accepted = true; }
                }
                Text {
                    anchors.verticalCenter: parent.verticalCenter
                    visible: input.text.length === 0
                    text: root.placeholder
                    color: Tokens.inkFaint
                    font: input.font
                }
            }
            IconButton {
                visible: root.value.length > 0
                icon: "close"
                iconSize: Style.fs.sm
                diameter: Style.sp(8)
                onClicked: { root.value = ""; root.items = []; root.panelOpen = false; }
            }
        }
    }

    // typeahead panel: paper over the backdrop, a hairline frame, a tracked micro section label and
    // ranked rows (top hit slightly taller, a per-kind tracked micro tag, hairlines between rows).
    Rectangle {
        id: panel
        visible: root.panelOpen && (root.items.length > 0 || root.loading)
        anchors.top: field.bottom
        anchors.topMargin: Style.sp(1)
        anchors.left: field.left
        width: field.width
        implicitHeight: Math.min(sugg.contentHeight + Style.sp(2), Style.sp(110))
        height: implicitHeight
        radius: Style.radius
        color: Tokens.paperLift
        border.width: 1
        border.color: Tokens.lineStrong
        z: 60

        ListView {
            id: sugg
            anchors.fill: parent
            anchors.margins: Style.sp(1)
            clip: true
            reuseItems: true
            boundsBehavior: Flickable.StopAtBounds
            model: root.items

            header: Item {
                width: sugg.width
                implicitHeight: Style.sp(8)
                Text {
                    anchors.left: parent.left
                    anchors.bottom: parent.bottom
                    anchors.leftMargin: Style.sp(2)
                    anchors.bottomMargin: Style.sp(1.5)
                    text: "SUGGESTIONS"
                    color: Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
                }
            }

            delegate: Rectangle {
                id: sRow
                required property var modelData
                required property int index
                width: sugg.width
                implicitHeight: sRow.index === 0 ? Style.sp(14) : Style.sp(12)
                color: (root.active === sRow.index) ? Tokens.tint10 : sHover.hovered ? Tokens.tint5 : "transparent"

                // hairline between rows
                Rectangle {
                    visible: sRow.index < root.items.length - 1
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(2)
                    height: 1
                    color: Tokens.lineSoft
                }

                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(2)
                    spacing: Style.sp(2)
                    Artwork {
                        url: sRow.modelData.thumbnail ? sRow.modelData.thumbnail : ""
                        px: sRow.index === 0 ? Style.sp(11) : Style.sp(9)
                        round: sRow.modelData.kind === "artist"
                        placeholderIcon: sRow.modelData.kind === "artist" ? "user" : sRow.modelData.kind === "album" ? "cd" : sRow.modelData.kind === "playlist" ? "playlist" : "music"
                    }
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: 0
                        Text {
                            Layout.fillWidth: true
                            text: sRow.modelData.title
                            color: Tokens.ink
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.md
                            font.weight: sRow.index === 0 ? Font.DemiBold : Font.Medium
                            elide: Text.ElideRight
                        }
                        Text {
                            Layout.fillWidth: true
                            text: sRow.modelData.subtitle ? sRow.modelData.subtitle : sRow.modelData.kind
                            color: Tokens.inkMuted
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.sm
                            elide: Text.ElideRight
                        }
                    }
                    Text {
                        text: sRow.modelData.kind.toUpperCase()
                        color: Tokens.inkFaint
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        font.letterSpacing: Style.trackMicro
                    }
                }
                HoverHandler { id: sHover }
                MouseArea {
                    anchors.fill: parent
                    cursorShape: Qt.PointingHandCursor
                    onEntered: root.active = sRow.index
                    hoverEnabled: true
                    onClicked: root.choose(sRow.modelData)
                }
            }
        }
    }
}
