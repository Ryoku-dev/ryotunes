pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"

// The library Songs tab, ported from LibrarySongs.svelte: the account's saved songs, which browse
// like a playlist (FEmusic_liked_videos). One get_playlist + get_playlist_more on the scroll
// sentinel; a reused TrackList keeps the five-figure list cheap. The filter narrows the rows loaded
// so far, and Play All / Shuffle All seed the queue with the whole list (continuation and all).
Item {
    id: root

    readonly property string songsId: "FEmusic_liked_videos"
    readonly property string sourceName: "Your songs"

    property var pl: null
    property bool loading: true
    property string errorMsg: ""
    property bool loadingMore: false
    property bool moreError: false
    property bool started: false
    property string query: ""

    readonly property var shown: {
        var items = (root.pl && root.pl.items) ? root.pl.items : [];
        var q = root.query.trim().toLowerCase();
        if (!q)
            return items;
        return items.filter((t) => (
            (t.title && t.title.toLowerCase().indexOf(q) >= 0)
            || (t.artists && t.artists.toLowerCase().indexOf(q) >= 0)
            || (t.album && t.album.toLowerCase().indexOf(q) >= 0)));
    }

    function ensureLoaded() {
        if (root.started)
            return;
        root.started = true;
        root.load();
    }
    function load() {
        root.loading = true;
        root.errorMsg = "";
        Daemon.call("get_playlist", { id: root.songsId })
            .then((p) => { root.pl = p; root.loading = false; Qt.callLater(root.scrollTop); })
            .catch((e) => { root.errorMsg = (e && e.message) ? e.message : String(e); root.loading = false; });
    }

    function scrollTop() {
        if (body.visible && body.view)
            body.view.positionViewAtBeginning();
    }
    function loadMore() {
        if (!root.pl || !root.pl.continuation || root.loadingMore || root.moreError)
            return;
        root.loadingMore = true;
        var token = root.pl.continuation;
        Daemon.call("get_playlist_more", { token: token })
            .then((more) => {
                if (!root.pl || root.pl.continuation !== token)
                    return;
                root.pl = Object.assign({}, root.pl, {
                    items: root.pl.items.concat(more.items),
                    continuation: more.items.length ? more.continuation : undefined
                });
                root.loadingMore = false;
            })
            .catch(() => { root.moreError = true; root.loadingMore = false; });
    }
    function maybeLoadMore() {
        if (!root.pl || !root.pl.continuation || root.loadingMore || root.moreError)
            return;
        if (body.view.contentHeight <= 0)
            return;
        if (body.view.contentY + body.view.height > body.view.contentHeight - 600)
            root.loadMore();
    }
    function play(start) {
        if (!root.pl)
            return;
        var at = start === null ? null : root.pl.items.indexOf(root.shown[start]);
        Daemon.call("play_playlist", {
            items: root.pl.items, start: at === -1 ? null : at,
            sourceName: root.sourceName, continuation: root.pl.continuation
        }).catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }
    function shuffle() {
        if (!root.pl || !root.pl.items.length)
            return;
        Daemon.call("play_playlist", {
            items: root.pl.items, start: null, sourceName: root.sourceName,
            shuffle: true, continuation: root.pl.continuation
        }).catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }

    Component.onCompleted: root.ensureLoaded()

    Text {
        anchors.centerIn: parent
        visible: root.loading || root.errorMsg !== ""
        text: root.loading ? "Loading your songs…" : root.errorMsg
        color: Tokens.inkMuted
        font.family: Style.fontUi
        font.pixelSize: Style.fs.md
    }

    TrackList {
        id: body
        anchors.fill: parent
        visible: !root.loading && root.errorMsg === ""
        items: root.shown
        showHeader: true
        showAlbum: true
        showPlays: true
        canAdd: true
        source: root.sourceName
        onActivated: (i) => root.play(i)
        header: songsHeader
        footer: songsFooter
        Component.onCompleted: body.view.contentYChanged.connect(root.maybeLoadMore)
    }

    Component {
        id: songsHeader
        Item {
            width: body.view.width
            implicitHeight: hc.implicitHeight + Style.sp(6)
            ColumnLayout {
                id: hc
                width: parent.width
                y: Style.sp(2)
                spacing: Style.sp(3)
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(3)
                    Pill {
                        label: "Play all"; icon: "play"; primary: true
                        enabled: !!(root.pl && root.pl.items.length)
                        onClicked: root.play(null)
                    }
                    Pill {
                        label: "Shuffle"; icon: "shuffle"
                        enabled: !!(root.pl && root.pl.items.length)
                        onClicked: root.shuffle()
                    }
                    Item { Layout.fillWidth: true }
                    Rectangle {
                        Layout.preferredWidth: Style.sp(52)
                        implicitHeight: Style.sp(10)
                        radius: Style.radius
                        color: Tokens.paperLift
                        border.width: 1
                        border.color: songFilter.activeFocus ? Tokens.lineStrong : Tokens.line
                        RowLayout {
                            anchors.fill: parent
                            anchors.leftMargin: Style.sp(2)
                            anchors.rightMargin: Style.sp(2)
                            spacing: Style.sp(2)
                            Icon { name: "search"; size: Style.fs.sm; color: Tokens.inkMuted }
                            TextInput {
                                id: songFilter
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
                                    visible: songFilter.text.length === 0
                                    text: "Filter songs"
                                    color: Tokens.inkFaint
                                    font: songFilter.font
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    Component {
        id: songsFooter
        Item {
            width: body.view.width
            implicitHeight: Style.sp(20)
            ColumnLayout {
                anchors.centerIn: parent
                spacing: Style.sp(1)
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    visible: root.loadingMore
                    text: "Loading more…"
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                }
                Rectangle {
                    Layout.alignment: Qt.AlignHCenter
                    visible: root.moreError
                    implicitWidth: Style.sp(24); implicitHeight: Style.sp(9)
                    radius: Style.radius
                    color: tryHover.hovered ? Tokens.tint10 : "transparent"
                    border.width: 1; border.color: Tokens.line
                    Text { anchors.centerIn: parent; text: "Try again"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.sm }
                    HoverHandler { id: tryHover }
                    MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: { root.moreError = false; root.loadMore(); } }
                }
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    visible: !root.loading && !root.shown.length
                    text: root.query.trim() ? "No songs match." : "No songs in your library yet."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                }
            }
        }
    }
}
