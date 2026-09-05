pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import QtQuick.Effects
import Ryoku.Ui.Singletons
import "../"

// The "Add to playlist" picker, ported from AddToPlaylist.svelte. A reusable overlay any surface can
// raise with open(songs): it fills its parent, blurs the page behind it once on open (spec §9) under
// a 0.5 scrim, lists the library's editable playlists (On Repeat and Liked Music dropped — one takes
// local play counts, the other takes likes), and adds sequentially so a whole album is a handful of
// calls, never a parallel hammer. A track YouTube already holds is refused and simply not re-counted.
// `blurSource` is the sibling page content to snapshot behind the card; when unset (a Menu-embedded
// picker) it falls back to scrim only. Data flows are unchanged.
Item {
    id: root

    anchors.fill: parent
    visible: root.open
    z: 200

    property Item blurSource: null
    property bool open: false
    property var songs: []
    property var playlists: []
    property bool loading: false
    property string createName: ""
    property bool showCreate: false

    function openWith(list) {
        root.songs = list && list.length ? list : [];
        if (!root.songs.length)
            return;
        root.showCreate = false;
        root.createName = "";
        root.playlists = [];
        root.open = true;
        if (root.blurSource)
            snap.scheduleUpdate();
        root.load();
    }

    function close() {
        root.open = false;
        root.showCreate = false;
        root.createName = "";
    }

    function load() {
        root.loading = true;
        Daemon.call("get_library")
            .then((p) => {
                root.playlists = (p || []).filter((i) => i.kind === "playlist"
                    && i.id !== "RYOTUNES_ON_REPEAT" && i.id !== "RYOTUNES_RECENTLY_PLAYED"
                    && i.id !== "RYOTUNES_REDISCOVER" && i.id !== "VLLM");
                root.loading = false;
            })
            .catch((e) => {
                Playback.toast((e && e.message) ? e.message : "Could not load playlists", "error");
                root.loading = false;
            });
    }

    function pick(pl) {
        var list = root.songs.slice();
        root.close();
        if (!list.length)
            return;
        var isLocal = String(pl.id).indexOf("RYOTUNES_LOCAL_PLAYLIST:") === 0;
        var added = 0;
        var chain = Promise.resolve();
        for (var i = 0; i < list.length; i++) {
            (function (song) {
                chain = chain.then(() => (isLocal
                    ? Daemon.call("add_to_local_playlist", { playlistId: pl.id, item: song })
                    : Daemon.call("add_to_playlist", { playlistId: pl.id, videoId: song.video_id }))
                    .then((accepted) => { if (accepted) added++; }));
            })(list[i]);
        }
        chain.then(() => {
            var dupes = list.length - added;
            if (!added)
                Playback.toast(dupes > 1 ? ("All " + dupes + " are already in " + pl.title) : ("Already in " + pl.title), "info");
            else if (dupes)
                Playback.toast("Added " + added + " to " + pl.title + " (" + dupes + " already there)", "success");
            else
                Playback.toast(added > 1 ? ("Added " + added + " songs to " + pl.title) : ("Added to " + pl.title), "success");
        }).catch((e) => Playback.toast((e && e.message) ? e.message : "Could not add", "error"));
    }

    function createAndPick() {
        var title = root.createName.trim();
        if (!title)
            return;
        Daemon.call("create_playlist", { title: title })
            .then((id) => {
                root.showCreate = false;
                root.createName = "";
                root.pick({ id: id, title: title });
            })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not create", "error"));
    }

    // snapshot blur of the page behind, captured once on open (spec §9)
    ShaderEffectSource {
        id: snap
        anchors.fill: parent
        visible: false
        sourceItem: root.blurSource
        live: false
        hideSource: false
        recursive: false
    }
    MultiEffect {
        anchors.fill: parent
        visible: root.blurSource !== null && Style.blurEnabled
        source: snap
        blurEnabled: true
        blur: 1.0
        blurMax: 32
        autoPaddingEnabled: false
    }

    // dismiss layer + scrim
    MouseArea {
        anchors.fill: parent
        onClicked: root.close()
    }
    Rectangle {
        anchors.fill: parent
        color: "#000000"
        opacity: 0.5
    }

    Rectangle {
        anchors.centerIn: parent
        width: Style.sp(90)
        implicitHeight: sheet.implicitHeight + Style.sp(12)
        height: implicitHeight
        radius: Style.radiusCard
        color: Tokens.paper
        border.width: 1
        border.color: Tokens.line

        // swallow clicks so they don't reach the dismiss layer
        MouseArea { anchors.fill: parent }

        ColumnLayout {
            id: sheet
            anchors.fill: parent
            anchors.margins: Style.sp(6)
            spacing: Style.sp(3)

            RowLayout {
                Layout.fillWidth: true
                Text {
                    Layout.fillWidth: true
                    text: "Add to playlist"
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.lg
                    font.weight: Font.DemiBold
                }
                IconButton {
                    icon: "close"
                    iconSize: Style.fs.md
                    diameter: Style.sp(8)
                    onClicked: root.close()
                }
            }

            // create row / form
            Loader {
                Layout.fillWidth: true
                sourceComponent: root.showCreate ? createForm : createButton
            }

            // playlist list
            Text {
                visible: root.loading
                text: "Loading…"
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
            }
            ListView {
                Layout.fillWidth: true
                Layout.preferredHeight: Math.min(contentHeight, Style.sp(80))
                visible: !root.loading && root.playlists.length > 0
                clip: true
                reuseItems: true
                boundsBehavior: Flickable.StopAtBounds
                model: root.playlists
                delegate: Rectangle {
                    id: plRow
                    required property var modelData
                    width: ListView.view ? ListView.view.width : 0
                    implicitHeight: Style.sp(11)
                    radius: Style.radius
                    color: plHover.hovered ? Tokens.tint5 : "transparent"
                    RowLayout {
                        anchors.fill: parent
                        anchors.leftMargin: Style.sp(2)
                        anchors.rightMargin: Style.sp(2)
                        spacing: Style.sp(2)
                        Artwork {
                            url: plRow.modelData.thumbnail ? plRow.modelData.thumbnail : ""
                            px: Style.sp(9)
                            placeholderIcon: "playlist"
                        }
                        Text {
                            Layout.fillWidth: true
                            text: plRow.modelData.title
                            color: Tokens.ink
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.md
                            elide: Text.ElideRight
                        }
                    }
                    HoverHandler { id: plHover }
                    MouseArea {
                        anchors.fill: parent
                        cursorShape: Qt.PointingHandCursor
                        onClicked: root.pick(plRow.modelData)
                    }
                }
            }
            Text {
                visible: !root.loading && root.playlists.length === 0
                Layout.fillWidth: true
                text: "No playlists yet. Create one above and these songs will be added immediately."
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                wrapMode: Text.WordWrap
            }
        }
    }

    Component {
        id: createButton
        Rectangle {
            implicitHeight: Style.sp(10)
            radius: Style.radius
            color: cbHover.hovered ? Tokens.tint5 : "transparent"
            border.width: 1
            border.color: Tokens.line
            RowLayout {
                anchors.fill: parent
                anchors.leftMargin: Style.sp(2)
                spacing: Style.sp(2)
                Icon { name: "add"; size: Style.fs.md; color: Tokens.ink }
                Text {
                    Layout.fillWidth: true
                    text: "New playlist"
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.md
                }
            }
            HoverHandler { id: cbHover }
            MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: root.showCreate = true }
        }
    }

    Component {
        id: createForm
        RowLayout {
            spacing: Style.sp(2)
            Rectangle {
                Layout.fillWidth: true
                implicitHeight: Style.sp(10)
                radius: Style.radius
                color: Tokens.paperLift
                border.width: 1
                border.color: nameField.activeFocus ? Tokens.lineStrong : Tokens.line
                TextInput {
                    id: nameField
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(2)
                    verticalAlignment: TextInput.AlignVCenter
                    clip: true
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.md
                    text: root.createName
                    focus: true
                    Component.onCompleted: nameField.forceActiveFocus()
                    onTextChanged: root.createName = text
                    onAccepted: root.createAndPick()
                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        visible: nameField.text.length === 0
                        text: "Playlist name"
                        color: Tokens.inkFaint
                        font: nameField.font
                    }
                }
            }
            Rectangle {
                implicitWidth: Style.sp(14)
                implicitHeight: Style.sp(10)
                radius: Style.radius
                color: Tokens.bone
                Text {
                    anchors.centerIn: parent
                    text: "Create"
                    color: Tokens.inkOnBone
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    font.weight: Font.Medium
                }
                MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: root.createAndPick() }
            }
        }
    }
}
