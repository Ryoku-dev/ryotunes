pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Quickshell.Io
import Ryoku.Ui.Singletons
import "../"

// The library Local tab, ported from LocalMusic.svelte: files on disk, scanned from the watched
// folders. get_local_library on open (cheap rescan); add/remove a folder over the socket with an
// explicit path (the daemon dropped the Tauri dialog), so the picker here is zenity — the same
// choice Task 7's settings pickers make. Sub-tabs Albums / Artists / Songs, filtered in memory.
Item {
    id: root

    property var lib: ({ folders: [], albums: [], artists: [], songs: [] })
    property bool loading: true
    property string errorMsg: ""
    property bool started: false
    property string view: "songs"
    property string query: ""

    readonly property var songs: {
        var s = (root.lib && root.lib.songs) ? root.lib.songs : [];
        var q = root.query.trim().toLowerCase();
        if (!q)
            return s;
        return s.filter((t) => (
            (t.title && t.title.toLowerCase().indexOf(q) >= 0)
            || (t.artists && t.artists.toLowerCase().indexOf(q) >= 0)
            || (t.album && t.album.toLowerCase().indexOf(q) >= 0)));
    }
    readonly property var cards: {
        var arr = root.view === "albums" ? (root.lib.albums || []) : (root.lib.artists || []);
        var q = root.query.trim().toLowerCase();
        if (!q)
            return arr;
        return arr.filter((c) => (c.title && c.title.toLowerCase().indexOf(q) >= 0)
            || (c.subtitle && c.subtitle.toLowerCase().indexOf(q) >= 0));
    }

    function ensureLoaded() {
        if (root.started)
            return;
        root.started = true;
        root.scan();
    }
    function scan() {
        root.loading = true;
        root.errorMsg = "";
        Daemon.call("get_local_library")
            .then((l) => { root.lib = l || root.lib; root.loading = false; })
            .catch((e) => { root.errorMsg = (e && e.message) ? e.message : String(e); root.loading = false; });
    }
    function addFolder(path) {
        if (!path)
            return;
        Daemon.call("add_local_folder", { path: path })
            .then((l) => { if (l) root.lib = l; })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not add folder", "error"));
    }
    function removeFolder(path) {
        Daemon.call("remove_local_folder", { path: path })
            .then((l) => { root.lib = l || root.lib; })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not remove folder", "error"));
    }
    function playSongs(start) {
        if (!root.songs.length)
            return;
        var at = start === null ? null : (root.lib.songs || []).indexOf(root.songs[start]);
        Daemon.call("play_playlist", {
            items: root.lib.songs, start: at === -1 ? null : at, sourceName: "Local music"
        }).catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }

    Component.onCompleted: root.ensureLoaded()

    // zenity directory picker → add_local_folder
    Process {
        id: picker
        command: ["zenity", "--file-selection", "--directory", "--title=Choose a music folder"]
        stdout: StdioCollector {
            id: pickerOut
            onStreamFinished: { var p = pickerOut.text.trim(); if (p) root.addFolder(p); }
        }
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: Style.sp(4)

        // folders group
        SectionHeading { Layout.fillWidth: true; title: "Folders" }

        RowLayout {
            Layout.fillWidth: true
            spacing: Style.sp(2)
            Flickable {
                Layout.fillWidth: true
                implicitHeight: Style.sp(9)
                contentWidth: folderRow.implicitWidth
                contentHeight: folderRow.implicitHeight
                flickableDirection: Flickable.HorizontalFlick
                clip: true
                Row {
                    id: folderRow
                    spacing: Style.sp(2)
                    Repeater {
                        model: (root.lib && root.lib.folders) ? root.lib.folders : []
                        delegate: Rectangle {
                            id: folderChip
                            required property var modelData
                            implicitWidth: fLabel.implicitWidth + Style.sp(10)
                            implicitHeight: Style.sp(8)
                            radius: Style.radius
                            color: Tokens.tint5
                            border.width: 1
                            border.color: Tokens.line
                            RowLayout {
                                anchors.fill: parent
                                anchors.leftMargin: Style.sp(2)
                                anchors.rightMargin: Style.sp(1)
                                spacing: Style.sp(1)
                                Text {
                                    id: fLabel
                                    Layout.maximumWidth: Style.sp(60)
                                    text: folderChip.modelData
                                    color: Tokens.inkDim
                                    font.family: Style.fontMono
                                    font.pixelSize: Style.fs.xs
                                    elide: Text.ElideLeft
                                }
                                IconButton {
                                    icon: "close"
                                    iconSize: Style.fs.sm
                                    diameter: Style.sp(6)
                                    onClicked: root.removeFolder(folderChip.modelData)
                                }
                            }
                        }
                    }
                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        visible: !(root.lib && root.lib.folders && root.lib.folders.length)
                        text: "No folders yet. Add the one your music sits in."
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                    }
                }
            }
            Pill { label: "Add folder"; icon: "add"; onClicked: picker.running = true }
            Pill { label: "Rescan"; icon: "on-repeat"; onClicked: root.scan() }
        }

        // sub-tabs + filter
        RowLayout {
            Layout.fillWidth: true
            spacing: Style.sp(2)
            Repeater {
                model: [
                    { k: "albums", l: "Albums" },
                    { k: "artists", l: "Artists" },
                    { k: "songs", l: "Songs" }
                ]
                delegate: Chip {
                    required property var modelData
                    text: modelData.l
                    active: root.view === modelData.k
                    onClicked: root.view = modelData.k
                }
            }
            Item { Layout.fillWidth: true }
            Rectangle {
                Layout.preferredWidth: Style.sp(52)
                implicitHeight: Style.sp(9)
                radius: Style.radius
                color: Tokens.paperLift
                border.width: 1
                border.color: localFilter.activeFocus ? Tokens.lineStrong : Tokens.line
                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(2)
                    spacing: Style.sp(2)
                    Icon { name: "search"; size: Style.fs.sm; color: Tokens.inkMuted }
                    TextInput {
                        id: localFilter
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
                            visible: localFilter.text.length === 0
                            text: "Filter"
                            color: Tokens.inkFaint
                            font: localFilter.font
                        }
                    }
                }
            }
        }

        Hairline { Layout.fillWidth: true }

        // content
        Item {
            Layout.fillWidth: true
            Layout.fillHeight: true

            Text {
                anchors.centerIn: parent
                visible: root.loading || root.errorMsg !== ""
                text: root.loading ? "Scanning local music…" : root.errorMsg
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
            }

            // songs sub-tab
            TrackList {
                id: localBody
                anchors.fill: parent
                visible: !root.loading && root.view === "songs"
                items: root.songs
                showHeader: true
                showAlbum: true
                canAdd: false
                source: "Local music"
                onActivated: (i) => root.playSongs(i)
                header: localSongsHeader
            }

            // albums / artists sub-tab
            CardGrid {
                anchors.fill: parent
                visible: !root.loading && root.view !== "songs"
                pad: 0
                model: root.cards
                emptyText: "Nothing on this device matches."
            }
        }
    }

    Component {
        id: localSongsHeader
        Item {
            width: localBody.view.width
            implicitHeight: Style.sp(12)
            RowLayout {
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.top: parent.top
                anchors.topMargin: Style.sp(2)
                spacing: Style.sp(3)
                Pill {
                    label: "Play all"; icon: "play"; primary: true
                    enabled: root.songs.length > 0
                    onClicked: root.playSongs(null)
                }
                Pill {
                    label: "Shuffle"; icon: "shuffle"
                    enabled: root.songs.length > 0
                    onClicked: {
                        if (!root.songs.length) return;
                        Daemon.call("play_playlist", { items: root.lib.songs, start: null, sourceName: "Local music", shuffle: true })
                            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
                    }
                }
                Item { Layout.fillWidth: true }
            }
        }
    }
}
