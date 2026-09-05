pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../lib/browse.js" as Browse
import "../lib/ids.js" as Ids

// Home's personal blocks, the ones the original Ryotunes built its Home around and the shelf
// rewrite flattened into card rails: "Jump back in" as compact rows of what you opened last
// (art 48, title, kind, play on hover, three columns); "Familiar artists" as the artist index
// (numbered rows of your most-played artists) beside an inspector for the selected one (big
// avatar, Play / Open artist, four top tracks); "Listen again" as the feed's own section, a
// numbered two-column song list. All three live above the feed's shelves.
ColumnLayout {
    id: root

    // Personal.recent(n) rows for Jump back in.
    property var recents: []
    // Artist pages (get_artist results) for the index, most played first.
    property var artists: []
    // The feed's "Listen again" section (cards), or null.
    property var listenAgain: null

    spacing: Style.sp(8)

    // ── jump back in ──────────────────────────────────────────────────────────────────────
    function openRecent(it) {
        if (!it)
            return;
        if (it.kind === "song")
            Playback.play(Browse.asSong(it));
        else
            Router.push(it.kind, { id: it.id, title: it.title });
        Personal.touchPick(it.id);
    }
    function playRecent(it) {
        if (!it)
            return;
        Personal.noteRecent(it);
        if (it.kind === "album") {
            Daemon.call("get_album", { id: it.id })
                .then((a) => Daemon.call("play_playlist", { items: a.items, sourceId: a.playlistId, sourceName: it.title }))
                .catch(() => Playback.toast("Could not play — try opening it", "error"));
        } else {
            Daemon.call("get_playlist", { id: it.id })
                .then((p) => Daemon.call("play_playlist", {
                    items: p.items,
                    sourceId: Ids.isSmartPlaylistId(it.id) ? undefined : it.id,
                    sourceName: it.title,
                    continuation: p.continuation
                }))
                .catch(() => Playback.toast("Could not play — try opening it", "error"));
        }
    }

    ColumnLayout {
        Layout.fillWidth: true
        visible: root.recents.length > 0
        spacing: Style.sp(3)
        SectionHeading { Layout.fillWidth: true; title: "Jump back in"; mark: Style.decorRich ? "戻" : "" }
        GridLayout {
            id: recGrid
            Layout.fillWidth: true
            columns: width >= Style.sp(200) ? 3 : 2
            columnSpacing: Style.sp(6)
            rowSpacing: Style.sp(1)
            Repeater {
                model: root.recents
                delegate: Item {
                    id: recRow
                    required property var modelData
                    readonly property bool round: recRow.modelData && recRow.modelData.kind === "artist"
                    Layout.fillWidth: true
                    Layout.preferredWidth: 1
                    implicitHeight: Style.sp(14)

                    Rectangle {
                        anchors.fill: parent
                        radius: Style.radius
                        color: recHover.hovered ? Tokens.tint5 : "transparent"
                        Behavior on color { ColorAnimation { duration: Style.motion.snap } }
                    }
                    RowLayout {
                        anchors.fill: parent
                        anchors.leftMargin: Style.sp(1.5)
                        anchors.rightMargin: Style.sp(1.5)
                        spacing: Style.sp(3)
                        Artwork {
                            Layout.alignment: Qt.AlignVCenter
                            url: recRow.modelData && recRow.modelData.thumbnail ? recRow.modelData.thumbnail : ""
                            px: Style.sp(12)
                            round: recRow.round
                            placeholderIcon: recRow.round ? "user"
                                : (recRow.modelData && Ids.isOnRepeatId(recRow.modelData.id)) ? "on-repeat" : "music"
                        }
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: Style.sp(0.5)
                            Text {
                                Layout.fillWidth: true
                                text: recRow.modelData ? recRow.modelData.title : ""
                                color: Tokens.ink
                                font.family: Style.fontUi
                                font.pixelSize: Style.fs.md
                                font.weight: Font.Medium
                                elide: Text.ElideRight
                            }
                            Text {
                                Layout.fillWidth: true
                                text: (recRow.modelData && recRow.modelData.subtitle) ? recRow.modelData.subtitle
                                    : (recRow.modelData ? recRow.modelData.kind : "")
                                color: Tokens.inkMuted
                                font.family: Style.fontUi
                                font.pixelSize: Style.fs.sm
                                elide: Text.ElideRight
                                textFormat: Text.PlainText
                            }
                        }
                        IconButton {
                            visible: !recRow.round && recHover.hovered
                            icon: "play"
                            iconSize: Style.fs.sm
                            diameter: Style.sp(8)
                            onClicked: root.playRecent(recRow.modelData)
                        }
                    }
                    HoverHandler { id: recHover }
                    TapHandler { onTapped: root.openRecent(recRow.modelData) }
                }
            }
        }
    }

    // ── familiar artists: the index and the inspector ─────────────────────────────────────
    property string activeId: ""
    readonly property var active: {
        for (var i = 0; i < root.artists.length; i++)
            if (root.artists[i].channelId === root.activeId)
                return root.artists[i];
        return root.artists.length ? root.artists[0] : null;
    }
    function playArtist(a) {
        if (!a || !a.topSongs || !a.topSongs.length)
            return;
        Personal.noteRecent({ id: a.channelId, kind: "artist", title: a.name, subtitle: a.subscribers, thumbnail: a.thumbnail });
        Daemon.call("play_playlist", { items: a.topSongs, sourceName: a.name })
            .catch(() => Playback.toast("Could not play", "error"));
    }

    ColumnLayout {
        Layout.fillWidth: true
        visible: root.artists.length >= 3
        spacing: Style.sp(3)
        SectionHeading { Layout.fillWidth: true; title: "Familiar artists"; mark: Style.decorRich ? "馴" : "" }

        RowLayout {
            Layout.fillWidth: true
            Layout.alignment: Qt.AlignTop
            spacing: Style.sp(8)

            // the index
            ColumnLayout {
                Layout.preferredWidth: Style.sp(100)
                Layout.maximumWidth: Style.sp(120)
                Layout.fillWidth: true
                Layout.alignment: Qt.AlignTop
                spacing: Style.sp(0.5)
                Repeater {
                    model: root.artists.slice(0, 6)
                    delegate: Rectangle {
                        id: artRow
                        required property var modelData
                        required property int index
                        readonly property bool sel: root.active && root.active.channelId === artRow.modelData.channelId
                        Layout.fillWidth: true
                        implicitHeight: Style.sp(13)
                        radius: Style.radius
                        color: artRow.sel ? Tokens.tint10 : (artHover.hovered ? Tokens.tint5 : "transparent")
                        Behavior on color { ColorAnimation { duration: Style.motion.snap } }
                        RowLayout {
                            anchors.fill: parent
                            anchors.leftMargin: Style.sp(2)
                            anchors.rightMargin: Style.sp(2)
                            spacing: Style.sp(3)
                            Text {
                                Layout.preferredWidth: Style.sp(5)
                                text: (artRow.index + 1 < 10 ? "0" : "") + (artRow.index + 1)
                                color: Tokens.inkFaint
                                font.family: Style.fontMono
                                font.pixelSize: Style.fs.micro
                                font.letterSpacing: Style.trackMicro
                            }
                            Artwork {
                                Layout.alignment: Qt.AlignVCenter
                                url: artRow.modelData.thumbnail ? artRow.modelData.thumbnail : ""
                                px: Style.sp(10)
                                round: true
                                placeholderIcon: "user"
                            }
                            ColumnLayout {
                                Layout.fillWidth: true
                                spacing: Style.sp(0.5)
                                Text {
                                    Layout.fillWidth: true
                                    text: artRow.modelData.name ? artRow.modelData.name : "Artist"
                                    color: Tokens.ink
                                    font.family: Style.fontUi
                                    font.pixelSize: Style.fs.md
                                    font.weight: Font.Medium
                                    elide: Text.ElideRight
                                }
                                Text {
                                    Layout.fillWidth: true
                                    text: artRow.modelData.monthlyListeners ? artRow.modelData.monthlyListeners
                                        : (artRow.modelData.subscribers ? artRow.modelData.subscribers : "Artist")
                                    color: Tokens.inkMuted
                                    font.family: Style.fontUi
                                    font.pixelSize: Style.fs.sm
                                    elide: Text.ElideRight
                                }
                            }
                            Text {
                                text: artRow.sel ? "//" : (Style.decorRich ? "聴" : "")
                                color: artRow.sel ? Tokens.ink : Tokens.inkFaint
                                font.family: artRow.sel ? Style.fontMono : Tokens.jp
                                font.pixelSize: Style.fs.sm
                            }
                        }
                        HoverHandler { id: artHover }
                        TapHandler {
                            onTapped: root.activeId = artRow.modelData.channelId
                            onDoubleTapped: Router.push("artist", { id: artRow.modelData.channelId, title: artRow.modelData.name })
                        }
                    }
                }
            }

            // the inspector
            ColumnLayout {
                Layout.fillWidth: true
                Layout.alignment: Qt.AlignTop
                spacing: Style.sp(4)
                visible: !!root.active

                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(5)
                    Artwork {
                        Layout.alignment: Qt.AlignVCenter
                        url: root.active && root.active.thumbnail ? root.active.thumbnail : ""
                        px: Style.sp(30)
                        round: true
                        placeholderIcon: "user"
                    }
                    ColumnLayout {
                        Layout.fillWidth: true
                        Layout.alignment: Qt.AlignVCenter
                        spacing: Style.sp(1.5)
                        Text {
                            Layout.fillWidth: true
                            text: "SELECTED ARTIST · " + (root.active
                                ? (root.active.monthlyListeners ? root.active.monthlyListeners
                                    : (root.active.subscribers ? root.active.subscribers : "LIBRARY SIGNAL"))
                                : "").toUpperCase()
                            color: Tokens.inkFaint
                            font.family: Style.fontMono
                            font.pixelSize: Style.fs.micro
                            font.letterSpacing: Style.trackMicro
                            elide: Text.ElideRight
                        }
                        Text {
                            Layout.fillWidth: true
                            text: (root.active && root.active.name) ? root.active.name : "Artist"
                            color: Tokens.ink
                            font.family: Style.fontDisplay
                            font.pixelSize: Style.fs.xl
                            elide: Text.ElideRight
                        }
                        RowLayout {
                            Layout.topMargin: Style.sp(1)
                            spacing: Style.sp(2)
                            Btn { primary: true; icon: "play"; text: "Play"; onClicked: root.playArtist(root.active) }
                            Btn {
                                text: "Open artist"
                                icon: "chevron-right"
                                onClicked: if (root.active) Router.push("artist", { id: root.active.channelId, title: root.active.name })
                            }
                        }
                    }
                }

                // top tracks preview
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 0
                    visible: !!(root.active && root.active.topSongs && root.active.topSongs.length)
                    Repeater {
                        model: (root.active && root.active.topSongs) ? root.active.topSongs.slice(0, 4) : []
                        delegate: Item {
                            id: topRow
                            required property var modelData
                            required property int index
                            Layout.fillWidth: true
                            implicitHeight: Style.sp(9)
                            Rectangle {
                                anchors.fill: parent
                                radius: Style.radius
                                color: topHover.hovered ? Tokens.tint5 : "transparent"
                            }
                            RowLayout {
                                anchors.fill: parent
                                anchors.leftMargin: Style.sp(2)
                                anchors.rightMargin: Style.sp(2)
                                spacing: Style.sp(3)
                                Text {
                                    Layout.preferredWidth: Style.sp(5)
                                    text: (topRow.index + 1 < 10 ? "0" : "") + (topRow.index + 1)
                                    color: Tokens.inkFaint
                                    font.family: Style.fontMono
                                    font.pixelSize: Style.fs.micro
                                    font.letterSpacing: Style.trackMicro
                                }
                                Text {
                                    Layout.fillWidth: true
                                    text: topRow.modelData.title ? topRow.modelData.title : ""
                                    color: Tokens.ink
                                    font.family: Style.fontUi
                                    font.pixelSize: Style.fs.md
                                    elide: Text.ElideRight
                                }
                                Text {
                                    text: topRow.modelData.duration ? topRow.modelData.duration : ""
                                    color: Tokens.inkFaint
                                    font.family: Style.fontMono
                                    font.pixelSize: Style.fs.xs
                                }
                                Icon { visible: topHover.hovered; name: "play"; size: Style.fs.sm; color: Tokens.inkMuted }
                            }
                            HoverHandler { id: topHover }
                            TapHandler { onTapped: Playback.play(topRow.modelData) }
                        }
                    }
                }
            }
        }
    }

    // ── listen again: the feed's own section as a numbered two-column list ───────────────
    readonly property var listenSongs: (root.listenAgain && root.listenAgain.items)
        ? root.listenAgain.items.filter((i) => i.kind === "song").slice(0, 10) : []
    function playListen(start) {
        Daemon.call("play_playlist", {
            items: root.listenSongs.map(Browse.asSong),
            start: start,
            sourceName: root.listenAgain ? root.listenAgain.title : null
        }).catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }

    ColumnLayout {
        Layout.fillWidth: true
        visible: root.listenSongs.length > 0
        spacing: Style.sp(3)
        SectionHeading {
            Layout.fillWidth: true
            title: root.listenAgain ? root.listenAgain.title : "Listen again"
            mark: Style.decorRich ? "再" : ""
            more: !!(root.listenAgain && root.listenAgain.moreBrowseId)
            onMoreClicked: if (root.listenAgain && root.listenAgain.moreBrowseId)
                Router.push("list", { id: root.listenAgain.moreBrowseId, title: root.listenAgain.title })
        }
        GridLayout {
            Layout.fillWidth: true
            columns: 2
            columnSpacing: Style.sp(8)
            rowSpacing: 0
            flow: GridLayout.TopToBottom
            rows: Math.ceil(root.listenSongs.length / 2)
            Repeater {
                model: root.listenSongs
                delegate: Item {
                    id: laRow
                    required property var modelData
                    required property int index
                    readonly property var song: Browse.asSong(laRow.modelData)
                    Layout.fillWidth: true
                    Layout.preferredWidth: 1
                    implicitHeight: laTrack.implicitHeight
                    TrackRow {
                        id: laTrack
                        anchors.left: parent.left
                        anchors.right: parent.right
                        index: laRow.index
                        song: laRow.song
                        active: !!(Playback.now && laRow.song && Playback.now.videoId === laRow.song.video_id)
                        onPlay: root.playListen(laRow.index)
                    }
                }
            }
        }
    }
}
