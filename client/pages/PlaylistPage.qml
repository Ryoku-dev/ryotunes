pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Quickshell
import Ryoku.Ui.Singletons
import "../"
import "../components"
import "../lib/ids.js" as Ids

// The playlist page, ported from ui/src/routes/playlist/[id]/+page.svelte. get_playlist(id) once,
// then get_playlist_more on the scroll sentinel — a five-figure Liked Songs list stays a single
// reused TrackList so scrolling never mounts more than a couple of screenfuls. The visual system is
// spec §5: a PageHero (art, eyebrow, title, meta, Play/like/more), a slim controls row (Shuffle,
// sort, the 80 ms filter box), the optional description, then a TrackList with a table header. Smart
// and local playlists drop the controls YouTube can't do. All data flows are unchanged.
Item {
    id: page

    readonly property var params: Router.current ? Router.current.params : ({})
    readonly property string playlistId: page.params && page.params.id ? page.params.id : ""

    property var pl: null
    property bool loading: true
    property string errorMsg: ""
    property bool loadingMore: false
    property bool moreError: false
    property string query: ""
    property string applied: ""
    property bool expanded: false
    property string sortKey: "default"
    property bool busy: false
    property bool confirmingDelete: false
    property bool editing: false

    readonly property bool isLiked: page.playlistId === "VLLM"
    readonly property bool isSmart: Ids.isSmartPlaylistId(page.playlistId)
    readonly property bool isLocal: String(page.playlistId).indexOf("RYOTUNES_LOCAL_PLAYLIST:") === 0
    readonly property bool owned: !!(page.pl && page.pl.owned) && !page.isLiked
    readonly property bool hasSortMenu: !!(page.pl && page.pl.sortMenu)
    readonly property var sorts: [
        { key: "default", label: "Default" },
        { key: "newest", label: "Newest first" },
        { key: "oldest", label: "Oldest first" },
        { key: "title", label: "Title" },
        { key: "artist", label: "Artist" },
        { key: "album", label: "Album" }
    ]
    readonly property var shown: {
        var items = (page.pl && page.pl.items) ? page.pl.items : [];
        var q = page.applied.trim().toLowerCase();
        if (!q)
            return items;
        return items.filter((t) => (
            (t.title && t.title.toLowerCase().indexOf(q) >= 0)
            || (t.artists && t.artists.toLowerCase().indexOf(q) >= 0)
            || (t.album && t.album.toLowerCase().indexOf(q) >= 0)));
    }

    onParamsChanged: page.load()
    Component.onCompleted: page.load()

    // 80 ms filter debounce (clearing is instant).
    Timer { id: filterTimer; interval: 80; onTriggered: page.applied = page.query }
    onQueryChanged: {
        if (!page.query.trim()) { filterTimer.stop(); page.applied = ""; }
        else filterTimer.restart();
    }

    function fetchSort(k) { return k === "plays" ? "default" : k; }
    function sortLabel() {
        for (var i = 0; i < page.sorts.length; i++)
            if (page.sorts[i].key === page.sortKey)
                return page.sorts[i].label;
        return "Default";
    }

    // The Sonora meta line: prefer the daemon's formatted subtitle (owner / N tracks · duration);
    // only synthesise a track count when no subtitle is present, so we never double up the count.
    function metaLine() {
        if (!page.pl)
            return "";
        if (page.pl.subtitle)
            return page.pl.subtitle;
        var n = page.pl.items ? page.pl.items.length : 0;
        return n ? (n + (n === 1 ? " track" : " tracks") + (page.pl.continuation ? "+" : "")) : "";
    }
    function eyebrowText() {
        if (page.isSmart) return "Smart playlist";
        if (page.isLocal) return "Local playlist";
        if (page.isLiked) return "Your library";
        return "Playlist";
    }

    function load() {
        if (!page.playlistId)
            return;
        page.loading = true;
        page.errorMsg = "";
        page.query = "";
        page.applied = "";
        page.expanded = false;
        page.moreError = false;
        page.sortKey = "default";
        page.confirmingDelete = false;
        page.editing = false;
        var reqId = page.playlistId;
        Daemon.call("get_playlist", { id: page.playlistId })
            .then((p) => {
                if (page.playlistId !== reqId)
                    return;
                page.pl = p;
                if (p.sortMenu && p.sortMenu.selected)
                    page.sortKey = p.sortMenu.selected;
                page.loading = false;
                // A fresh navigation lands at the top: a ListView with a tall header can otherwise
                // settle with contentY > 0, clipping the hero. loadMore never calls this.
                Qt.callLater(page.scrollTop);
            })
            .catch((e) => {
                if (page.playlistId !== reqId)
                    return;
                page.errorMsg = (e && e.message) ? e.message : String(e);
                page.loading = false;
            });
    }

    function scrollTop() {
        if (body.visible && body.view)
            body.view.positionViewAtBeginning();
    }

    function loadMore() {
        if (!page.pl || !page.pl.continuation || page.loadingMore || page.moreError)
            return;
        page.loadingMore = true;
        var token = page.pl.continuation;
        Daemon.call("get_playlist_more", { token: token })
            .then((more) => {
                if (!page.pl || page.pl.continuation !== token)
                    return;
                page.pl = Object.assign({}, page.pl, {
                    items: page.pl.items.concat(more.items),
                    continuation: more.items.length ? more.continuation : undefined
                });
                page.loadingMore = false;
            })
            .catch(() => { page.moreError = true; page.loadingMore = false; });
    }
    function maybeLoadMore() {
        if (!page.pl || !page.pl.continuation || page.loadingMore || page.moreError)
            return;
        if (body.view.contentHeight <= 0)
            return;
        if (body.view.contentY + body.view.height > body.view.contentHeight - 600)
            page.loadMore();
    }

    function play(start) {
        if (!page.pl)
            return;
        var at = start === null ? null : page.pl.items.indexOf(page.shown[start]);
        Daemon.call("play_playlist", {
            items: page.pl.items,
            start: at === -1 ? null : at,
            sourceId: page.isSmart ? undefined : page.playlistId,
            sourceName: page.pl.title,
            continuation: page.pl.continuation
        }).catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }
    function shuffle() {
        if (!page.pl || !page.pl.items.length)
            return;
        Daemon.call("play_playlist", {
            items: page.pl.items, start: null,
            sourceId: page.isSmart ? undefined : page.playlistId,
            sourceName: page.pl.title, shuffle: true, continuation: page.pl.continuation
        }).catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }

    function chooseSort(k) {
        if (k === page.sortKey)
            return;
        page.sortKey = k;
        var reqId = page.playlistId;
        if (page.pl && page.pl.sortMenu && page.pl.sortMenu.editable)
            Daemon.call("set_playlist_sort", { playlistId: page.playlistId, sort: page.fetchSort(k) }).catch(() => {});
        Daemon.call("get_playlist", { id: page.playlistId, sort: page.fetchSort(k), desc: false })
            .then((p) => { if (page.playlistId === reqId) page.pl = p; })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not sort", "error"));
    }

    function removeAt(index) {
        var song = page.shown[index];
        if (!song || !song.set_video_id)
            return;
        var pid = page.playlistId;
        page.pl = Object.assign({}, page.pl, { items: page.pl.items.filter((t) => t !== song) });
        Daemon.call("remove_from_playlist", { playlistId: pid, videoId: song.video_id, setVideoId: song.set_video_id })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not remove", "error"));
    }

    function pickCover() {
        Daemon.call("set_playlist_cover", { playlistId: page.playlistId, pick: true })
            .then((res) => { if (res) page.load(); })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not set cover", "error"));
    }
    function doDelete() {
        Daemon.call("delete_playlist", { playlistId: page.playlistId })
            .then(() => { Playback.toast("Playlist deleted", "success"); Router.pop(); })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not delete", "error"));
    }
    function share() {
        var raw = String(page.playlistId).replace(/^VL/, "");
        var url = "https://music.youtube.com/playlist?list=" + encodeURIComponent(raw);
        Quickshell.execDetached(["sh", "-c", "printf %s \"$1\" | wl-copy", "sh", url]);
        Playback.toast("Link copied", "success");
    }
    function queuePlaylist(next) {
        if (!page.pl || !page.pl.items.length)
            return;
        Daemon.call(next ? "play_next" : "add_to_queue", {
            items: page.pl.items, from: page.pl.title, continuation: page.pl.continuation
        }).then(() => Playback.toast(next ? "Playing next" : "Added to queue", "success"))
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not queue", "error"));
    }
    function radio() {
        if (page.isSmart || page.isLocal)
            return;
        Playback.toast("Starting radio…", "info");
        Daemon.call("start_radio", { kind: "playlist", id: page.playlistId, name: page.pl ? page.pl.title : null })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not start radio", "error"));
    }

    function buildMenu() {
        var out = [];
        out.push({ icon: "arrow-up", label: "Play next", danger: false, act: () => page.queuePlaylist(true) });
        out.push({ icon: "queue", label: "Add to queue", danger: false, act: () => page.queuePlaylist(false) });
        if (!page.isSmart && !page.isLocal)
            out.push({ icon: "radio", label: "Start radio", danger: false, act: () => page.radio() });
        if (page.owned) {
            out.push({ icon: "edit", label: "Edit details", danger: false, act: () => page.editing = true });
            if (!page.isLocal)
                out.push({ icon: "music", label: "Change cover", danger: false, act: () => page.pickCover() });
            out.push({ icon: "close", label: "Delete playlist", danger: true, act: () => page.confirmingDelete = true });
        }
        if (!page.isSmart && !page.isLocal)
            out.push({ icon: "link", label: "Share", danger: false, act: () => page.share() });
        return out;
    }

    Text {
        anchors.centerIn: parent
        visible: page.loading || page.errorMsg !== ""
        text: page.loading ? "Loading playlist…" : page.errorMsg
        color: Tokens.inkMuted
        font.family: Style.fontUi
        font.pixelSize: Style.fs.md
    }

    TrackList {
        id: body
        anchors.fill: parent
        visible: !page.loading && page.errorMsg === "" && page.pl !== null
        items: page.shown
        showHeader: true
        showAlbum: true
        showPlays: true
        canAdd: !page.isLocal
        canRemove: page.owned
        removeLabel: "Remove from playlist"
        source: page.pl ? page.pl.title : ""
        onActivated: (i) => page.play(i)
        onRemoveAt: (i) => page.removeAt(i)
        header: plHeader
        footer: plFooter
        Component.onCompleted: body.view.contentYChanged.connect(page.maybeLoadMore)
    }

    Component {
        id: plHeader
        Item {
            width: body.view.width
            implicitHeight: headerCol.implicitHeight + Style.sp(3)

            ColumnLayout {
                id: headerCol
                width: parent.width
                spacing: Style.sp(4)

                PageHero {
                    id: hero
                    Layout.fillWidth: true
                    eyebrow: page.eyebrowText()
                    title: (page.pl && page.pl.title) ? page.pl.title : "Playlist"
                    meta: page.metaLine()
                    art: (page.pl && (page.pl.cover || page.pl.thumbnail)) ? (page.pl.cover || page.pl.thumbnail) : ""
                    placeholderIcon: page.isSmart ? "on-repeat" : "playlist"
                    primaryLabel: "Play"
                    likeable: false
                    showMore: true
                    onPrimary: page.play(null)
                    onMore: {
                        var p = hero.mapToItem(page, Style.sp(44), hero.height - Style.sp(8));
                        plMenu.openAt(p.x, p.y);
                    }
                }

                // controls: Shuffle, sort, filter
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(3)
                    Btn {
                        text: "Shuffle"
                        icon: "shuffle"
                        enabled: !!(page.pl && page.pl.items && page.pl.items.length)
                        onClicked: page.shuffle()
                    }
                    Btn {
                        id: sortBtn
                        visible: page.hasSortMenu
                        text: page.sortLabel()
                        icon: "sort"
                        onClicked: {
                            var p = sortBtn.mapToItem(page, 0, sortBtn.height);
                            sortMenu.openAt(p.x, p.y);
                        }
                    }
                    Item { Layout.fillWidth: true }
                    Rectangle {
                        Layout.preferredWidth: Style.sp(52)
                        Layout.alignment: Qt.AlignVCenter
                        implicitHeight: Style.ctlH
                        radius: Style.radius
                        color: Tokens.paperLift
                        border.width: 1
                        border.color: plFilter.activeFocus ? Tokens.lineStrong : Tokens.line
                        RowLayout {
                            anchors.fill: parent
                            anchors.leftMargin: Style.sp(2)
                            anchors.rightMargin: Style.sp(2)
                            spacing: Style.sp(2)
                            Icon { name: "search"; size: Style.fs.sm; color: Tokens.inkMuted }
                            TextInput {
                                id: plFilter
                                Layout.fillWidth: true
                                verticalAlignment: TextInput.AlignVCenter
                                clip: true
                                color: Tokens.ink
                                font.family: Style.fontUi
                                font.pixelSize: Style.fs.sm
                                text: page.query
                                onTextChanged: page.query = text
                                Text {
                                    anchors.verticalCenter: parent.verticalCenter
                                    visible: plFilter.text.length === 0
                                    text: "Filter"
                                    color: Tokens.inkFaint
                                    font: plFilter.font
                                }
                            }
                        }
                    }
                }

                // description (collapsible)
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: !!(page.pl && page.pl.description)
                    spacing: Style.sp(0.5)
                    Text {
                        Layout.fillWidth: true
                        text: (page.pl && page.pl.description) ? page.pl.description : ""
                        color: Tokens.inkDim
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        wrapMode: Text.WordWrap
                        maximumLineCount: page.expanded ? 999 : 2
                        elide: Text.ElideRight
                    }
                    Text {
                        visible: !!(page.pl && page.pl.description && page.pl.description.length > 120)
                        text: page.expanded ? "LESS" : "MORE"
                        color: descHover.hovered ? Tokens.ink : Tokens.inkMuted
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        font.letterSpacing: Style.trackMicro
                        HoverHandler { id: descHover }
                        MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: page.expanded = !page.expanded }
                    }
                }
            }
        }
    }

    Component {
        id: plFooter
        Item {
            width: body.view.width
            implicitHeight: Style.sp(24)
            ColumnLayout {
                anchors.centerIn: parent
                spacing: Style.sp(1)
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    visible: page.loadingMore
                    text: "Loading more…"
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                }
                Rectangle {
                    Layout.alignment: Qt.AlignHCenter
                    visible: page.moreError
                    implicitWidth: Style.sp(24); implicitHeight: Style.ctlH
                    radius: Style.radius
                    color: tryHover.hovered ? Tokens.tint10 : "transparent"
                    border.width: 1; border.color: Tokens.line
                    Text { anchors.centerIn: parent; text: "Try again"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.sm }
                    HoverHandler { id: tryHover }
                    MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: { page.moreError = false; page.loadMore(); } }
                }
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    visible: !page.loadingMore && !page.moreError && !!(page.pl && !page.pl.continuation) && !page.applied.trim() && !!(page.pl && page.pl.items.length)
                    text: (page.pl ? page.pl.items.length : 0) + " tracks"
                    color: Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
                }
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    visible: !!page.applied.trim() && page.shown.length === 0
                    text: "No tracks match \u201C" + page.applied.trim() + "\u201D."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                }
            }
        }
    }

    Menu {
        id: plMenu
        customItems: page.buildMenu()
    }
    Menu {
        id: sortMenu
        customItems: page.sorts.map((s) => ({ icon: page.sortKey === s.key ? "check-circle" : "sort",
            label: s.label, danger: false, act: () => page.chooseSort(s.key) }))
    }

    // --- edit details dialog ---------------------------------------------------------------
    Loader {
        anchors.fill: parent
        active: page.editing
        sourceComponent: editDialog
    }
    Component {
        id: editDialog
        EditPlaylist {
            blurSource: body
            playlistId: page.playlistId
            initialName: (page.pl && page.pl.title) ? page.pl.title : ""
            initialDescription: (page.pl && page.pl.description) ? page.pl.description : ""
            initialPublic: !!(page.pl && page.pl.privacy === "PUBLIC")
            onClosed: page.editing = false
            onSaved: { page.editing = false; page.load(); }
        }
    }

    // --- delete confirm --------------------------------------------------------------------
    Item {
        anchors.fill: parent
        visible: page.confirmingDelete
        z: 220
        MouseArea { anchors.fill: parent; onClicked: page.confirmingDelete = false }
        Rectangle { anchors.fill: parent; color: "#000000"; opacity: 0.5 }
        Rectangle {
            anchors.centerIn: parent
            width: Style.sp(90)
            implicitHeight: delCol.implicitHeight + Style.sp(12)
            height: implicitHeight
            radius: Style.radiusCard
            color: Tokens.paper
            border.width: 1
            border.color: Tokens.line
            MouseArea { anchors.fill: parent }
            ColumnLayout {
                id: delCol
                anchors.fill: parent
                anchors.margins: Style.sp(6)
                spacing: Style.sp(3)
                Text {
                    text: "Delete this playlist?"
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.lg
                    font.weight: Font.DemiBold
                }
                Text {
                    Layout.fillWidth: true
                    text: "This removes it from your account. It can't be undone."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    wrapMode: Text.WordWrap
                }
                RowLayout {
                    Layout.alignment: Qt.AlignRight
                    spacing: Style.sp(2)
                    Pill { label: "Cancel"; onClicked: page.confirmingDelete = false }
                    Pill { label: "Delete"; active: true; onClicked: { page.confirmingDelete = false; page.doDelete(); } }
                }
            }
        }
    }
}
