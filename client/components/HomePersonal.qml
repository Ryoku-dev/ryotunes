pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../lib/browse.js" as Browse
import "../lib/ids.js" as Ids

// Home's personal blocks, the ones the original Ryotunes built its Home around: "Jump back in" as a
// two-column grid of what you opened last (art 48, title, meta, play on hover); "Familiar artists"
// as ONE bordered panel — the numbered ARTIST INDEX on the left, the selected artist's photo bleeding
// to the panel's edges in the centre, and the SELECTED ARTIST inspector (eyebrow, Fraunces name, TOP
// TRACKS) on the right; and "Listen again" as the feed's numbered two-column song list. Each section
// is headed by a glyph + `// Title`, 40 px below the one above it and 16 px above its own content.
ColumnLayout {
    id: root

    // Personal.recent(n) rows for Jump back in.
    property var recents: []
    // Artist pages (get_artist results) for the index, most played first.
    property var artists: []
    // The feed's "Listen again" section (cards), or null.
    property var listenAgain: null

    spacing: Style.sp(10)

    // ── jump back in ──────────────────────────────────────────────────────────────────────
    function openRecent(it) {
        if (!it)
            return;
        if (it.kind === "song")
            root.playRecent(it);
        else
            Router.push(it.kind, { id: it.id, title: it.title });
        Personal.touchPick(it.id);
    }
    function playRecent(it) {
        if (!it)
            return;
        if (it.kind === "song") {
            Playback.play(Browse.asSong(it))
                .catch(() => Playback.toast("Could not play this song", "error"));
            return;
        }
        if (it.kind !== "album" && it.kind !== "playlist")
            return;
        Daemon.call(it.kind === "album" ? "get_album" : "get_playlist", { id: it.id })
            .then((media) => Daemon.call("play_playlist", {
                items: media.items,
                sourceId: it.kind === "album" ? media.playlistId
                    : (Ids.isSmartPlaylistId(it.id) ? undefined : it.id),
                sourceName: it.title,
                continuation: media.continuation
            }).then(() => Personal.noteRecent(it)))
            .catch(() => Playback.toast("Could not play — try opening it", "error"));
    }

    function recentMeta(it) {
        if (!it)
            return "";
        var kind = it.kind === "song" ? "Song" : it.kind === "album" ? "Album"
            : it.kind === "playlist" ? "Playlist" : "Artist";
        var age = Math.max(0, Date.now() - Number(it.at || 0));
        var when = !it.at ? "" : age < 3600000 ? "Recently"
            : age < 86400000 ? "Today" : age < 172800000 ? "Yesterday"
            : Math.floor(age / 86400000) + " days ago";
        return [kind, it.subtitle || "", when].filter((part) => !!part).join(" · ");
    }

    ColumnLayout {
        Layout.fillWidth: true
        visible: root.recents.length > 0
        spacing: Style.sp(4)
        RowLayout {
            Layout.fillWidth: true
            spacing: Style.sp(2)
            Icon { Layout.alignment: Qt.AlignVCenter; name: "jump-back"; size: Style.fs.md; color: Tokens.inkMuted }
            SectionHeading { Layout.fillWidth: true; title: "Recently played"; mark: Style.decorRich ? "戻" : "" }
        }
        GridLayout {
            id: recGrid
            Layout.fillWidth: true
            columns: width >= Style.sp(150) ? 2 : 1
            columnSpacing: Style.sp(8)
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
                        anchors.leftMargin: Style.sp(2)
                        anchors.rightMargin: Style.sp(2)
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
                                text: root.recentMeta(recRow.modelData)
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
                            tip: "Play " + (recRow.modelData ? recRow.modelData.title : "")
                        }
                    }
                    HoverHandler { id: recHover }
                    TapHandler { onTapped: root.openRecent(recRow.modelData) }
                }
            }
        }
    }

    // ── familiar artists: the index, the bleeding photo and the inspector ──────────────────
    property string activeId: ""
    readonly property var active: {
        for (var i = 0; i < root.artists.length; i++)
            if (root.artists[i].channelId === root.activeId)
                return root.artists[i];
        return root.artists.length ? root.artists[0] : null;
    }
    readonly property var indexArtists: root.artists.slice(0, 5)
    function activeMeta(a) {
        if (!a)
            return "";
        return a.monthlyListeners ? a.monthlyListeners : (a.subscribers ? a.subscribers : "");
    }

    ColumnLayout {
        Layout.fillWidth: true
        visible: root.artists.length >= 3
        spacing: Style.sp(4)
        RowLayout {
            Layout.fillWidth: true
            spacing: Style.sp(2)
            Icon { Layout.alignment: Qt.AlignVCenter; name: "group"; size: Style.fs.md; color: Tokens.inkMuted }
            SectionHeading { Layout.fillWidth: true; title: "// Familiar artists"; mark: Style.decorRich ? "馴" : "" }
        }

        // the panel — one bordered card, three columns, the photo bleeding between them
        Rectangle {
            id: famPanel
            Layout.fillWidth: true
            implicitHeight: famRow.implicitHeight
            radius: Style.radiusCard
            color: Tokens.paper
            border.width: 1
            border.color: Tokens.line
            clip: true

            RowLayout {
                id: famRow
                anchors.fill: parent
                spacing: 0

                // the index
                ColumnLayout {
                    Layout.preferredWidth: Style.sp(106)
                    Layout.minimumWidth: Style.sp(60)
                    Layout.fillHeight: true
                    Layout.alignment: Qt.AlignTop
                    Layout.margins: Style.sp(3)
                    spacing: Style.sp(1)

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(2)
                        Text { text: "// ARTIST INDEX"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
                        Item { Layout.fillWidth: true }
                        Text { text: root.artists.length + " KNOWN"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
                    }
                    Rectangle { Layout.fillWidth: true; Layout.topMargin: Style.sp(0.5); Layout.preferredHeight: 1; color: Tokens.lineSoft }

                    Repeater {
                        model: root.indexArtists
                        delegate: Rectangle {
                            id: artRow
                            required property var modelData
                            required property int index
                            readonly property bool sel: root.active && root.active.channelId === artRow.modelData.channelId
                            Layout.fillWidth: true
                            Layout.topMargin: Style.sp(1)
                            implicitHeight: Style.sp(12)
                            radius: Style.radius
                            color: artRow.sel ? Tokens.paperLift : (artHover.hovered ? Tokens.tint5 : "transparent")
                            border.width: artRow.sel ? 1 : 0
                            border.color: Tokens.line
                            Behavior on color { ColorAnimation { duration: Style.motion.snap } }
                            RowLayout {
                                anchors.fill: parent
                                anchors.leftMargin: Style.sp(2)
                                anchors.rightMargin: Style.sp(2)
                                spacing: Style.sp(2.5)
                                Text {
                                    Layout.preferredWidth: Style.sp(5)
                                    text: (artRow.index + 1 < 10 ? "0" : "") + (artRow.index + 1)
                                    color: artRow.sel ? Tokens.inkMuted : Tokens.inkFaint
                                    font.family: Style.fontMono
                                    font.pixelSize: Style.fs.micro
                                    font.letterSpacing: Style.trackMicro
                                }
                                Artwork {
                                    Layout.alignment: Qt.AlignVCenter
                                    url: artRow.modelData.thumbnail ? artRow.modelData.thumbnail : ""
                                    px: Style.sp(9)
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
                    Item { Layout.fillHeight: true; Layout.preferredHeight: 1 }
                }

                Rectangle { Layout.preferredWidth: 1; Layout.fillHeight: true; color: Tokens.line }

                // the selected artist's photo, bleeding to the panel's top and bottom edges
                Item {
                    id: photo
                    Layout.fillHeight: true
                    Layout.preferredWidth: famPanel.height
                    Layout.minimumWidth: Style.sp(28)
                    Layout.maximumWidth: famPanel.height
                    visible: !!root.active
                    clip: true

                    Rectangle { anchors.fill: parent; color: Tokens.paperLift }
                    Icon {
                        anchors.centerIn: parent
                        visible: photoImg.status !== Image.Ready
                        name: "user"
                        size: Style.sp(12)
                        color: Tokens.inkFaint
                    }
                    Image {
                        id: photoImg
                        anchors.fill: parent
                        source: root.active && root.active.thumbnail ? Style.thumb(root.active.thumbnail, 480) : ""
                        sourceSize: Qt.size(480, 480)
                        fillMode: Image.PreserveAspectCrop
                        asynchronous: true
                        cache: true
                        opacity: status === Image.Ready ? 1 : 0
                        Behavior on opacity { NumberAnimation { duration: Style.motion.slow } }
                    }
                }

                Rectangle { Layout.preferredWidth: 1; Layout.fillHeight: true; color: Tokens.line }

                // the inspector
                ColumnLayout {
                    Layout.fillWidth: true
                    Layout.minimumWidth: Style.sp(52)
                    Layout.fillHeight: true
                    Layout.alignment: Qt.AlignTop
                    Layout.margins: Style.sp(4)
                    spacing: Style.sp(1)
                    visible: !!root.active

                    Text {
                        Layout.fillWidth: true
                        text: "SELECTED ARTIST" + (root.activeMeta(root.active) ? (" · " + root.activeMeta(root.active).toUpperCase()) : "")
                        color: Tokens.inkFaint
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        font.letterSpacing: Style.trackMicro
                        elide: Text.ElideRight
                    }
                    Text {
                        id: selName
                        Layout.fillWidth: true
                        Layout.topMargin: Style.sp(0.5)
                        text: (root.active && root.active.name) ? root.active.name : "Artist"
                        color: nameHover.hovered ? Tokens.inkDim : Tokens.ink
                        font.family: Style.fontDisplay
                        font.pixelSize: Style.fs.hero
                        elide: Text.ElideRight
                        HoverHandler { id: nameHover; cursorShape: Qt.PointingHandCursor }
                        TapHandler { onTapped: if (root.active) Router.push("artist", { id: root.active.channelId, title: root.active.name }) }
                    }

                    Item { Layout.fillHeight: true; Layout.preferredHeight: Style.sp(2) }

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(2)
                        visible: !!(root.active && root.active.topSongs && root.active.topSongs.length)
                        Text { text: "TOP TRACKS"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
                        Rectangle { Layout.fillWidth: true; Layout.alignment: Qt.AlignVCenter; Layout.preferredHeight: 1; color: Tokens.lineSoft }
                    }
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
                                IconButton {
                                    visible: topHover.hovered
                                    icon: "play"
                                    iconSize: Style.fs.sm
                                    diameter: Style.sp(7)
                                    onClicked: Playback.play(topRow.modelData)
                                }
                            }
                            HoverHandler { id: topHover }
                            TapHandler { onTapped: Playback.play(topRow.modelData) }
                        }
                    }
                    Item { Layout.fillHeight: true; Layout.preferredHeight: 1 }
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
        spacing: Style.sp(4)
        RowLayout {
            Layout.fillWidth: true
            spacing: Style.sp(2)
            Icon { Layout.alignment: Qt.AlignVCenter; name: "on-repeat"; size: Style.fs.md; color: Tokens.inkMuted }
            SectionHeading {
                Layout.fillWidth: true
                title: "// " + (root.listenAgain ? root.listenAgain.title : "Listen again")
                mark: Style.decorRich ? "再" : ""
                more: !!(root.listenAgain && root.listenAgain.moreBrowseId)
                onMoreClicked: if (root.listenAgain && root.listenAgain.moreBrowseId)
                    Router.push("list", { id: root.listenAgain.moreBrowseId, title: root.listenAgain.title })
            }
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
