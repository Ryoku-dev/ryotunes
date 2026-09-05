pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Quickshell.Io
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The library page, ported from ui/src/routes/library/+page.svelte. Account collections load in
// parallel on open (get_library / _albums / _artists) and fill the card tabs (Albums / Artists /
// Playlists); Songs, Local and Insights are the ported LibrarySongs / LocalMusic /
// ListeningInsights, loaded lazily the first time their tab is opened so an unopened tab costs
// nothing. A PageHero-less title row carries the count and the toolbar (create a playlist, import
// one from a file over the socket with a path); a segmented control switches the body.
Item {
    id: page

    property var playlists: []
    property var albums: []
    property var artists: []
    property bool loading: true
    property string errorMsg: ""
    property string tab: "songs"
    property var opened: []
    property bool creating: false
    property string newName: ""

    // The sidebar deep-links a tab (Router.push("library", { tab })). Honour it on every navigation,
    // not just the first mount, so clicking a Library sub-item while already on the page switches
    // tabs. A plain library nav (no tab) leaves the current tab alone. Tab clicks are local state
    // (no route push), so they never round-trip through here.
    readonly property var params: Router.current ? Router.current.params : ({})
    onParamsChanged: {
        var t = (page.params && page.params.tab) ? page.params.tab : "";
        if (t && t !== page.tab)
            page.selectTab(t);
    }

    readonly property var tabs: [
        { k: "songs", l: "Songs" },
        { k: "albums", l: "Albums" },
        { k: "artists", l: "Artists" },
        { k: "playlists", l: "Playlists" },
        { k: "local", l: "Local" },
        { k: "insights", l: "Insights" }
    ]
    readonly property bool gridTab: page.tab === "albums" || page.tab === "artists" || page.tab === "playlists"
    readonly property var gridModel: {
        if (page.tab === "playlists") return page.playlists;
        if (page.tab === "albums") return page.albums;
        if (page.tab === "artists") return page.artists;
        return [];
    }

    // The count meta beside the title reflects the active tab's collection size where the page owns
    // it (Songs / Local / Insights load their own rows, so they carry no page-level count).
    function tabMeta() {
        if (page.tab === "playlists") return page.playlists.length + (page.playlists.length === 1 ? " playlist" : " playlists");
        if (page.tab === "albums") return page.albums.length + (page.albums.length === 1 ? " album" : " albums");
        if (page.tab === "artists") return page.artists.length + (page.artists.length === 1 ? " artist" : " artists");
        return "";
    }

    Component.onCompleted: {
        var t = (Router.current && Router.current.params && Router.current.params.tab)
            ? Router.current.params.tab : "songs";
        page.selectTab(t);
        page.load();
    }

    function selectTab(k) {
        page.tab = k;
        if (page.opened.indexOf(k) < 0)
            page.opened = page.opened.concat([k]);
    }
    function isOpened(k) { return page.opened.indexOf(k) >= 0; }

    function load() {
        page.loading = true;
        page.errorMsg = "";
        Promise.all([
            Daemon.call("get_library").catch(() => []),
            Daemon.call("get_library_albums").catch(() => []),
            Daemon.call("get_library_artists").catch(() => [])
        ]).then((res) => {
            page.playlists = res[0] || [];
            page.albums = res[1] || [];
            page.artists = res[2] || [];
            page.loading = false;
        }).catch((e) => {
            page.errorMsg = (e && e.message) ? e.message : String(e);
            page.loading = false;
        });
    }

    function createPlaylist() {
        var title = page.newName.trim();
        if (!title || page.creating)
            return;
        page.creating = true;
        Daemon.call("create_playlist", { title: title })
            .then(() => { page.creating = false; page.newName = ""; newDialog.visible = false; page.load(); Playback.toast("Playlist created", "success"); })
            .catch((e) => { page.creating = false; Playback.toast((e && e.message) ? e.message : "Could not create", "error"); });
    }

    // import: zenity file picker -> import_playlist_file(path) -> create_playlist + add each
    Process {
        id: importPicker
        command: ["zenity", "--file-selection", "--title=Import a playlist file"]
        stdout: StdioCollector {
            id: importOut
            onStreamFinished: { var p = importOut.text.trim(); if (p) page.doImport(p); }
        }
    }
    function doImport(path) {
        Daemon.call("import_playlist_file", { path: path })
            .then((transfer) => {
                if (!transfer || !transfer.items || !transfer.items.length)
                    return;
                return Daemon.call("create_playlist", { title: transfer.title }).then((id) => {
                    var chain = Promise.resolve();
                    for (var i = 0; i < transfer.items.length; i++) {
                        (function (song) {
                            chain = chain.then(() => Daemon.call("add_to_playlist", { playlistId: id, videoId: song.video_id }).catch(() => {}));
                        })(transfer.items[i]);
                    }
                    return chain.then(() => { page.load(); Playback.toast("Imported " + transfer.title, "success"); });
                });
            })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not import", "error"));
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: Style.sp(4)

        // title row: display title + active-tab count, then the collection toolbar
        RowLayout {
            Layout.fillWidth: true
            spacing: Style.sp(4)
            ColumnLayout {
                spacing: Style.sp(1)
                Text {
                    text: "Library"
                    color: Tokens.ink
                    font.family: Style.fontDisplay
                    font.pixelSize: Style.fs.title
                }
                Text {
                    visible: text !== ""
                    text: page.tabMeta()
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                }
            }
            Item { Layout.fillWidth: true }
            Pill { label: "Import"; icon: "add"; onClicked: importPicker.running = true }
            Pill { label: "New playlist"; icon: "playlist"; primary: true; onClicked: { page.newName = ""; newDialog.visible = true; } }
        }

        // segmented control
        Flickable {
            Layout.fillWidth: true
            implicitHeight: tabRow.implicitHeight
            contentWidth: tabRow.implicitWidth
            contentHeight: tabRow.implicitHeight
            flickableDirection: Flickable.HorizontalFlick
            clip: true
            Row {
                id: tabRow
                spacing: Style.sp(2)
                Repeater {
                    model: page.tabs
                    delegate: Chip {
                        required property var modelData
                        text: modelData.l
                        active: page.tab === modelData.k
                        onClicked: page.selectTab(modelData.k)
                    }
                }
            }
        }

        Hairline { Layout.fillWidth: true }

        // body: card grids for the account collections, lazy loaders for the ported sub-surfaces
        Item {
            Layout.fillWidth: true
            Layout.fillHeight: true

            CardGrid {
                anchors.fill: parent
                visible: page.gridTab
                loading: page.loading
                model: page.gridModel
                pad: 0
                emptyText: (Playback.auth && Playback.auth.signedIn) ? "Nothing saved yet." : "Sign in to see your library."
            }

            Loader {
                anchors.fill: parent
                active: page.isOpened("songs")
                visible: page.tab === "songs"
                sourceComponent: LibrarySongs {}
            }
            Loader {
                anchors.fill: parent
                active: page.isOpened("local")
                visible: page.tab === "local"
                sourceComponent: LocalMusic {}
            }
            Loader {
                anchors.fill: parent
                active: page.isOpened("insights")
                visible: page.tab === "insights"
                sourceComponent: ListeningInsights {}
            }
        }
    }

    // new-playlist dialog
    Item {
        id: newDialog
        anchors.fill: parent
        visible: false
        z: 210
        MouseArea { anchors.fill: parent; onClicked: newDialog.visible = false }
        Rectangle { anchors.fill: parent; color: "#000000"; opacity: 0.5 }
        Rectangle {
            anchors.centerIn: parent
            width: Style.sp(90)
            implicitHeight: ndCol.implicitHeight + Style.sp(8)
            height: implicitHeight
            radius: Style.radiusCard
            color: Tokens.paperLift
            border.width: 1
            border.color: Tokens.lineStrong
            MouseArea { anchors.fill: parent }
            ColumnLayout {
                id: ndCol
                anchors.fill: parent
                anchors.margins: Style.sp(4)
                spacing: Style.sp(3)
                Text { text: "New playlist"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.lg; font.weight: Font.DemiBold }
                Rectangle {
                    Layout.fillWidth: true
                    implicitHeight: Style.sp(10)
                    radius: Style.radius
                    color: Tokens.paper
                    border.width: 1
                    border.color: ndField.activeFocus ? Tokens.lineStrong : Tokens.line
                    TextInput {
                        id: ndField
                        anchors.fill: parent
                        anchors.leftMargin: Style.sp(2)
                        anchors.rightMargin: Style.sp(2)
                        verticalAlignment: TextInput.AlignVCenter
                        clip: true
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        text: page.newName
                        onTextChanged: page.newName = text
                        onAccepted: page.createPlaylist()
                        Component.onCompleted: if (newDialog.visible) ndField.forceActiveFocus()
                        Text {
                            anchors.verticalCenter: parent.verticalCenter
                            visible: ndField.text.length === 0
                            text: "Playlist name"
                            color: Tokens.inkFaint
                            font: ndField.font
                        }
                    }
                }
                RowLayout {
                    Layout.alignment: Qt.AlignRight
                    spacing: Style.sp(2)
                    Pill { label: "Cancel"; onClicked: newDialog.visible = false }
                    Pill { label: "Create"; primary: true; enabled: !page.creating && page.newName.trim().length > 0; onClicked: page.createPlaylist() }
                }
            }
        }
        onVisibleChanged: if (visible) ndField.forceActiveFocus()
    }
}
