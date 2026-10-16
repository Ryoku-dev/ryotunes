pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Window
import QtQuick.Layouts
import Quickshell
import Ryoku.Ui.Singletons
import "../"
import "../lib/ids.js" as Ids

// The track ⋯ options menu, ported from TrackMenu.svelte. One shared instance per list surface,
// opened at the ⋯ trigger or a right-click on the row (openAt in this item's own space, so the
// menu is never clipped by the list's own clip rect). The queue actions are universal; go-to
// artist/album, like, radio, share and the YouTube-playlist writes hide for local files and radio
// stations, which carry no YouTube identity. "Add to playlist" is self-served by the embedded
// picker; "Remove" is context-specific, so it calls back through removeRequested().
Item {
    id: root

    // The menu lives on the window, not inside the list that opened it: its dismiss layer has to
    // cover everything a click can land on, and a list that clips its children would otherwise cut
    // the panel off. Callers map the press into this item's space with mapFromItem(null, ...),
    // which stays correct because the window's content item is the scene.
    parent: root.Window.contentItem ? root.Window.contentItem : undefined
    anchors.fill: parent
    z: 150

    property var song: null
    // A caller-supplied item list ([{icon,label,act,danger}]) for page-level menus (album/artist
    // headers). When set it replaces the song-derived track items.
    property var customItems: null
    // "Add to playlist" shows when true (a page that can't add — a local list — passes false).
    property bool canAdd: true
    // "Remove from …" shows when true; the host owns what removal means for its list.
    property bool canRemove: false
    property string removeLabel: "Remove from playlist"
    // The "from" heading a queued block wears in the queue panel.
    property string source: ""

    signal removeRequested()

    readonly property bool open: panel.visible
    readonly property bool noYouTubeTrack: !!root.song
        && (Ids.isLocalId(root.song.video_id) || Ids.isRadioId(root.song.video_id))
    // The track this menu is about is the one playing. "Play next" and "Add to queue" are a no-op
    // on it, so they are left out rather than offered and ignored.
    readonly property bool isNowPlaying: !!root.song && !!Playback.now
        && Playback.now.videoId === root.song.video_id
    // A host can force the queue actions back on (the same song sitting twice in a queue).
    property bool canQueue: !root.isNowPlaying

    readonly property string rated: {
        if (root.song && Playback.now && Playback.now.videoId === root.song.video_id)
            return Playback.rating;
        return (root.song && root.song.rating) ? root.song.rating : "indifferent";
    }

    // Where the press landed, in this item's space. The placement is a binding on the panel rather
    // than a calculation here, because the panel's height follows the items the song produces and
    // is not known yet at the moment it is asked for: reading it here placed a menu opened near the
    // bottom of the window half off-screen.
    property real askedX: 0
    property real askedY: 0

    function openAt(sx, sy) {
        if (!root.song && !root.customItems)
            return;
        root.askedX = sx;
        root.askedY = sy;
        panel.visible = true;
    }
    function close() { panel.visible = false; }

    function toast(msg, kind) { Playback.toast(msg, kind); }

    function enqueue(next) {
        Daemon.call(next ? "play_next" : "add_to_queue",
            { items: [root.song], from: root.source ? root.source : null })
            .then(() => root.toast(next ? "Playing next" : "Added to queue", "success"))
            .catch((e) => root.toast((e && e.message) ? e.message : "Could not queue", "error"));
    }
    function startRadio() {
        root.toast("Starting radio…", "info");
        Daemon.call("start_radio", { kind: "song", id: root.song.video_id, name: root.song.title })
            .catch((e) => root.toast((e && e.message) ? e.message : "Could not start radio", "error"));
    }
    function toggleLike() {
        var next = root.rated === "like" ? "indifferent" : "like";
        Daemon.call("rate", { videoId: root.song.video_id, rating: next })
            .catch((e) => root.toast((e && e.message) ? e.message : "Could not rate", "error"));
    }
    function share() {
        var id = root.song.video_id;
        var url = "https://music.youtube.com/watch?v=" + encodeURIComponent(id);
        Quickshell.execDetached(["sh", "-c", "printf %s \"$1\" | wl-copy", "sh", url]);
        root.toast("Link copied", "success");
    }

    // The item list for the current song, in TrackMenu.svelte order.
    function buildItems() {
        if (root.customItems)
            return root.customItems;
        var s = root.song;
        if (!s)
            return [];
        var out = [];
        if (root.canQueue) {
            out.push({ icon: "arrow-up", label: "Play next", danger: false, act: () => root.enqueue(true) });
            out.push({ icon: "queue", label: "Add to queue", danger: false, act: () => root.enqueue(false) });
        }
        if (!root.noYouTubeTrack)
            out.push({ icon: "radio", label: "Start radio", danger: false, act: () => root.startRadio() });
        if (!root.noYouTubeTrack)
            out.push({ icon: "heart", label: root.rated === "like" ? "Remove from Liked Songs" : "Save to Liked Songs",
                danger: false, act: () => root.toggleLike() });
        if (s.artist_id)
            out.push({ icon: "artists", label: "Go to artist", danger: false,
                act: () => Router.push("artist", { id: s.artist_id }) });
        if (s.album_id && !root.noYouTubeTrack)
            out.push({ icon: "cd", label: "Go to album", danger: false,
                act: () => Router.push("album", { id: s.album_id, title: s.album }) });
        if (root.canAdd && !root.noYouTubeTrack)
            out.push({ icon: "add", label: "Add to playlist", danger: false,
                act: () => picker.openWith([s]) });
        if (!root.noYouTubeTrack)
            out.push({ icon: "link", label: "Share", danger: false, act: () => root.share() });
        if (root.canRemove)
            out.push({ icon: "close", label: root.removeLabel, danger: true, act: () => root.removeRequested() });
        return out;
    }

    // dismiss layer
    MouseArea {
        anchors.fill: parent
        visible: root.open
        acceptedButtons: Qt.LeftButton | Qt.RightButton
        onClicked: root.close()

        // The menu sits on the window, so a list scrolling underneath would leave it hanging over
        // a row it no longer belongs to. The wheel closes it instead.
        WheelHandler {
            enabled: root.open
            // WheelHandler is the one handler Qt narrows to PointerDevice.Mouse, and the Wayland
            // plugin stamps wheel events with the seat's primary pointing device, which is reported
            // as a touchpad on machines that have no touchpad at all. Left on the default this
            // never fires, with no warning to say so.
            acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
            onWheel: root.close()
        }
    }

    Rectangle {
        id: panel
        visible: false
        width: Style.sp(54)
        implicitHeight: col.implicitHeight + Style.sp(2)
        height: implicitHeight
        x: Math.max(Style.sp(2), Math.min(root.askedX, root.width - panel.width - Style.sp(2)))
        y: (root.askedY + panel.height > root.height - Style.sp(2))
            ? Math.max(Style.sp(2), root.askedY - panel.height)
            : root.askedY
        radius: Style.radius
        color: Tokens.paperLift
        border.width: 1
        border.color: Tokens.lineStrong

        ColumnLayout {
            id: col
            anchors.fill: parent
            anchors.margins: Style.sp(1)
            spacing: 0

            Repeater {
                model: root.open ? root.buildItems() : []
                delegate: Rectangle {
                    id: item
                    required property var modelData
                    Layout.fillWidth: true
                    implicitHeight: Style.sp(8)
                    radius: Style.radius
                    color: itemHover.hovered ? (item.modelData.danger ? Tokens.tint10 : Tokens.tint5) : "transparent"
                    RowLayout {
                        anchors.fill: parent
                        anchors.leftMargin: Style.sp(2)
                        anchors.rightMargin: Style.sp(2)
                        spacing: Style.sp(2)
                        Icon {
                            name: item.modelData.icon
                            size: Style.fs.md
                            color: item.modelData.danger ? Style.alert : Tokens.inkDim
                        }
                        Text {
                            Layout.fillWidth: true
                            text: item.modelData.label
                            color: item.modelData.danger ? Style.alert : Tokens.ink
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.md
                            elide: Text.ElideRight
                        }
                    }
                    HoverHandler { id: itemHover }
                    MouseArea {
                        anchors.fill: parent
                        cursorShape: Qt.PointingHandCursor
                        onClicked: {
                            var act = item.modelData.act;
                            root.close();
                            act();
                        }
                    }
                }
            }
        }
    }

    AddToPlaylist { id: picker }
}
