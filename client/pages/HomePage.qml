pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"
import "../chrome"
import "../lib/browse.js" as Browse
import "../lib/ids.js" as Ids

// Home, ported from ui/src/routes/+page.svelte. One vertical reused ListView of shelves; the header
// carries the greeting, the mood-chip rail, the Shortcuts pinboard and (when the feed supplies it)
// the Forgotten favourites block; the footer carries the loading skeletons, empty/error states and
// the progressive get_home_more pagination. get_home_more fires when the tail comes within 400 px of
// the viewport bottom, exactly like the Svelte sentinel. Every list is a ListView with reuseItems
// and a bounded cache, so scrolling a long feed never mounts more than a screenful or two.
Item {
    id: page

    property var home: null
    property var chips: []
    property var forgotten: null
    property string selected: ""
    property bool loading: true
    property string errorMsg: ""
    property bool loadingMore: false
    property bool moreError: false
    property var blocks: []

    readonly property int pad: Style.sp(8)

    Component.onCompleted: page.load("")

    function greeting() {
        var h = new Date().getHours();
        return h < 5 ? "Still up" : h < 12 ? "Good morning" : h < 17 ? "Good afternoon" : h < 22 ? "Good evening" : "Good night";
    }

    function isForgotten(s) {
        if (!/forgotten/i.test(s.title))
            return false;
        for (var i = 0; i < s.items.length; i++)
            if (s.items[i].kind === "song")
                return true;
        return false;
    }

    function rebuild() {
        var arr = [];
        var fg = null;
        var secs = (page.home && page.home.sections) ? page.home.sections : [];
        for (var i = 0; i < secs.length; i++) {
            if (page.isForgotten(secs[i])) {
                if (!fg)
                    fg = secs[i];
            } else {
                arr.push(secs[i]);
            }
        }
        page.forgotten = fg;
        page.blocks = arr;
    }

    function forgottenSongs() {
        if (!page.forgotten)
            return [];
        return page.forgotten.items.filter((i) => i.kind === "song").slice(0, 15);
    }

    function playForgotten(start) {
        var songs = page.forgottenSongs();
        Daemon.call("play_playlist", {
            items: songs.map(Browse.asSong),
            start: start,
            sourceName: page.forgotten ? page.forgotten.title : null
        }).catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }

    // Open a "Jump back in" recent: a song plays, a collection routes to its page. Mirrors
    // MediaCard.open / browse.ts openItem.
    function openRecent(it) {
        if (!it)
            return;
        if (it.kind === "song")
            Playback.play(Browse.asSong(it));
        else
            Router.push(it.kind, { id: it.id, title: it.title });
        Personal.touchPick(it.id);
    }

    // Play a recent without leaving Home: an album/playlist is fetched then played, its source id
    // set so autoplay continues with that context's radio (never for a smart playlist).
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

    // Play a familiar artist's top songs (its top-songs shelf becomes the queue), recording the
    // artist as a recent. Mirrors ArtistIndex.svelte playArtist / player.svelte.ts playFrom.
    function playArtist(a) {
        if (!a || !a.topSongs || !a.topSongs.length)
            return;
        Personal.noteRecent({ id: a.channelId, kind: "artist", title: a.name, subtitle: a.subscribers, thumbnail: a.thumbnail });
        Daemon.call("play_playlist", { items: a.topSongs, sourceName: a.name })
            .catch(() => Playback.toast("Could not play", "error"));
    }

    function load(params) {
        page.selected = params;
        page.loading = true;
        page.errorMsg = "";
        page.moreError = false;
        Daemon.call("get_home", { params: params ? params : null })
            .then((h) => {
                if (page.selected !== params)
                    return;
                page.home = h;
                if (h.chips && h.chips.length)
                    page.chips = h.chips.filter((c) => c.title !== "Podcasts");
                page.rebuild();
                page.loading = false;
                list.positionViewAtBeginning();
            })
            .catch((e) => {
                if (page.selected !== params)
                    return;
                page.errorMsg = (e && e.message) ? e.message : String(e);
                page.loading = false;
            });
    }

    function loadMore() {
        if (!page.home || !page.home.continuation || page.loadingMore || page.moreError)
            return;
        page.loadingMore = true;
        var token = page.home.continuation;
        var params = page.selected;
        Daemon.call("get_home_more", { token: token })
            .then((more) => {
                if (page.selected !== params || !page.home || page.home.continuation !== token)
                    return;
                page.home = {
                    chips: page.home.chips,
                    sections: page.home.sections.concat(more.sections),
                    continuation: more.sections.length ? more.continuation : undefined
                };
                page.rebuild();
                page.loadingMore = false;
            })
            .catch(() => {
                page.moreError = true;
                page.loadingMore = false;
                Playback.toast("Could not load more", "error");
            });
    }

    function maybeLoadMore() {
        if (!page.home || !page.home.continuation || page.loadingMore || page.moreError)
            return;
        if (list.contentHeight <= 0)
            return;
        if (list.contentY + list.height > list.contentHeight - 400)
            page.loadMore();
    }

    ListView {
        id: list
        anchors.fill: parent
        clip: true
        reuseItems: true
        cacheBuffer: Math.round(height * 1.5)
        boundsBehavior: Flickable.StopAtBounds
        model: page.blocks
        spacing: Style.sp(9)
        onContentYChanged: page.maybeLoadMore()
        onContentHeightChanged: page.maybeLoadMore()

        delegate: Item {
            required property var modelData
            width: list.width
            implicitHeight: shelf.implicitHeight
            Shelf {
                id: shelf
                x: page.pad
                width: parent.width - page.pad * 2
                section: parent.modelData
            }
        }

        header: Item {
            width: list.width
            implicitHeight: headerCol.implicitHeight + Style.sp(9)

            ColumnLayout {
                id: headerCol
                x: page.pad
                width: parent.width - page.pad * 2
                y: Style.sp(6)
                spacing: Style.sp(5)

                // hero: the copy, search and key hints on the left, the listening deck on the right
                // (HomeHero.svelte's two-column head). Narrow windows stack the deck under the copy.
                GridLayout {
                    id: hero
                    Layout.fillWidth: true
                    readonly property bool wide: width >= Style.sp(200)
                    columns: wide ? 2 : 1
                    columnSpacing: Style.sp(12)
                    rowSpacing: Style.sp(5)

                    ColumnLayout {
                        Layout.preferredWidth: hero.wide ? Style.sp(90) : hero.width
                        Layout.maximumWidth: hero.wide ? Style.sp(90) : hero.width
                        Layout.alignment: Qt.AlignVCenter
                        spacing: Style.sp(2)
                        RowLayout {
                            spacing: Style.sp(2)
                            Rectangle { Layout.preferredWidth: Style.sp(4); Layout.preferredHeight: 1; Layout.alignment: Qt.AlignVCenter; color: Tokens.ink }
                            Text { text: "聴"; color: Tokens.ink; font.family: Tokens.jp; font.pixelSize: Style.fs.sm }
                            Rectangle { Layout.preferredWidth: Style.sp(13); Layout.preferredHeight: 1; Layout.alignment: Qt.AlignVCenter; color: Tokens.lineSoft }
                            Text {
                                text: "RYOKU // MUSIC"
                                color: Tokens.inkFaint
                                font.family: Style.fontMono
                                font.pixelSize: Style.fs.xs
                                font.letterSpacing: 1
                            }
                        }
                        Text {
                            Layout.fillWidth: true
                            text: page.greeting() + ((Playback.auth && Playback.auth.signedIn && Playback.auth.name) ? (", " + Playback.auth.name) : "")
                            color: Tokens.ink
                            font.family: Tokens.display
                            font.pixelSize: Style.fs.hero
                            elide: Text.ElideRight
                        }
                        Text {
                            Layout.fillWidth: true
                            text: "Pick up where you left off, or find the next thing worth hearing."
                            color: Tokens.inkMuted
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.md
                            wrapMode: Text.WordWrap
                        }
                        SearchSuggest {
                            id: heroSearch
                            Layout.fillWidth: true
                            Layout.topMargin: Style.sp(2)
                            placeholder: "Search tracks, albums, artists…"
                            onSubmitted: if (value.trim() !== "") Router.push("search", { q: value.trim() })
                            onPicked: if (value.trim() !== "") Router.push("search", { q: value.trim() })
                            z: 40
                        }
                        RowLayout {
                            spacing: Style.sp(2)
                            Text { text: "CTRL K"; color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 0.7 }
                            Text { text: "command search"; color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.xs }
                            Rectangle { Layout.preferredWidth: Style.sp(4); Layout.preferredHeight: 1; color: Tokens.lineSoft }
                            Text { text: "SPACE"; color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 0.7 }
                            Text { text: "play / pause"; color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.xs }
                        }
                    }

                    MusicDeck {
                        Layout.fillWidth: true
                        Layout.alignment: Qt.AlignVCenter
                        onOpenNowPlaying: tab => Playback.nowPlayingRequested(tab)
                    }
                }

                // chip rail
                Flickable {
                    Layout.fillWidth: true
                    implicitHeight: chipRow.implicitHeight
                    contentWidth: chipRow.implicitWidth
                    contentHeight: chipRow.implicitHeight
                    flickableDirection: Flickable.HorizontalFlick
                    boundsBehavior: Flickable.StopAtBounds
                    clip: true
                    visible: page.chips.length > 0
                    Row {
                        id: chipRow
                        spacing: Style.sp(2)
                        Chip {
                            text: "All"
                            active: page.selected === ""
                            onClicked: page.load("")
                        }
                        Repeater {
                            model: page.chips
                            delegate: Chip {
                                required property var modelData
                                text: modelData.title
                                active: page.selected === modelData.params
                                onClicked: page.load(page.selected === modelData.params ? "" : modelData.params)
                            }
                        }
                    }
                }

                // shortcuts (unfiltered only), fed from the shared personal store
                Shortcuts {
                    Layout.fillWidth: true
                    visible: page.selected === ""
                    picks: Personal.picks
                    onRemoved: id => Personal.removePick(id)
                }

                // jump back in (recents, unfiltered only): bare rows, against the surfaced Shortcuts
                // tiles above — the things you chose are elevated, the ones the app noticed are not.
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: page.selected === "" && Personal.recent().length > 0
                    spacing: Style.sp(3)
                    SectionHeading {
                        Layout.fillWidth: true
                        title: "Jump back in"
                        icon: "jump-back"
                    }
                    GridLayout {
                        Layout.fillWidth: true
                        columns: 3
                        columnSpacing: Style.sp(6)
                        rowSpacing: Style.sp(1)
                        Repeater {
                            model: Personal.recent()
                            delegate: Item {
                                id: recRow
                                required property var modelData
                                readonly property bool round: recRow.modelData && recRow.modelData.kind === "artist"
                                Layout.fillWidth: true
                                Layout.preferredWidth: 1
                                implicitHeight: recLayout.implicitHeight + Style.sp(3)

                                Rectangle {
                                    anchors.fill: parent
                                    radius: Style.radius
                                    color: recHover.hovered ? Tokens.tint5 : "transparent"
                                }
                                RowLayout {
                                    id: recLayout
                                    anchors.left: parent.left
                                    anchors.right: parent.right
                                    anchors.verticalCenter: parent.verticalCenter
                                    anchors.leftMargin: Style.sp(1.5)
                                    anchors.rightMargin: Style.sp(1.5)
                                    spacing: Style.sp(2)
                                    Artwork {
                                        url: recRow.modelData && recRow.modelData.thumbnail ? recRow.modelData.thumbnail : ""
                                        px: Style.sp(10)
                                        round: recRow.round
                                        placeholderIcon: recRow.round ? "user"
                                            : (recRow.modelData && Ids.isOnRepeatId(recRow.modelData.id)) ? "on-repeat" : "music"
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 0
                                        Text {
                                            Layout.fillWidth: true
                                            text: recRow.modelData ? recRow.modelData.title : ""
                                            color: Tokens.ink
                                            font.family: Style.fontUi
                                            font.pixelSize: Style.fs.sm
                                            font.weight: Font.Medium
                                            elide: Text.ElideRight
                                        }
                                        Text {
                                            Layout.fillWidth: true
                                            text: (recRow.modelData && recRow.modelData.subtitle) ? recRow.modelData.subtitle
                                                : (recRow.modelData ? recRow.modelData.kind : "")
                                            color: Tokens.inkMuted
                                            font.family: Style.fontUi
                                            font.pixelSize: Style.fs.xs
                                            elide: Text.ElideRight
                                            textFormat: Text.PlainText
                                        }
                                    }
                                    IconButton {
                                        visible: !recRow.round && recHover.hovered
                                        icon: "play"
                                        iconSize: Style.fs.sm
                                        diameter: Style.sp(7)
                                        onClicked: page.playRecent(recRow.modelData)
                                    }
                                }
                                HoverHandler { id: recHover }
                                MouseArea {
                                    anchors.fill: parent
                                    acceptedButtons: Qt.LeftButton
                                    cursorShape: Qt.PointingHandCursor
                                    onClicked: page.openRecent(recRow.modelData)
                                }
                            }
                        }
                    }
                }

                // forgotten favourites
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: !!page.forgotten
                    spacing: Style.sp(3)
                    SectionHeading {
                        Layout.fillWidth: true
                        title: page.forgotten ? page.forgotten.title : ""
                        icon: "clock"
                    }
                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(6)
                        Repeater {
                            model: 3
                            delegate: ColumnLayout {
                                id: fgCol
                                required property int index
                                Layout.fillWidth: true
                                Layout.alignment: Qt.AlignTop
                                spacing: Style.sp(0.5)
                                Repeater {
                                    model: {
                                        var songs = page.forgottenSongs();
                                        var per = Math.ceil(songs.length / 3);
                                        return songs.slice(fgCol.index * per, fgCol.index * per + per);
                                    }
                                    delegate: Item {
                                        id: fgRow
                                        required property var modelData
                                        required property int index
                                        readonly property int per: Math.ceil(page.forgottenSongs().length / 3)
                                        readonly property int globalIndex: fgCol.index * per + index
                                        Layout.fillWidth: true
                                        implicitHeight: fgTrack.implicitHeight
                                        TrackRow {
                                            id: fgTrack
                                            anchors.left: parent.left
                                            anchors.right: parent.right
                                            song: fgRow.modelData
                                            compact: true
                                            active: !!(Playback.now && fgRow.modelData
                                                && Playback.now.videoId === fgRow.modelData.video_id)
                                            onPlay: page.playForgotten(fgRow.globalIndex)
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                // familiar artists (unfiltered only): the artist index, keyed off the shared play
                // counts (topArtistIds). A list of the most-played artists on the left, an inspector
                // for the selected one on the right. Ported from ArtistIndex.svelte.
                Item {
                    id: familiar
                    Layout.fillWidth: true
                    visible: page.selected === "" && familiar.artists.length >= 3
                    implicitHeight: famCol.implicitHeight

                    property var ids: Personal.topArtistIds(6)
                    property var artists: []
                    property string activeId: ""
                    property bool loaded: false
                    readonly property var active: {
                        for (var i = 0; i < familiar.artists.length; i++)
                            if (familiar.artists[i].channelId === familiar.activeId)
                                return familiar.artists[i];
                        return familiar.artists.length ? familiar.artists[0] : null;
                    }

                    onIdsChanged: familiar.load()
                    Component.onCompleted: familiar.load()
                    function load() {
                        if (familiar.loaded || familiar.ids.length < 3)
                            return;
                        familiar.loaded = true;
                        Promise.all(familiar.ids.map((id) => Daemon.call("get_artist", { id: id }).catch(() => null)))
                            .then((pages) => {
                                familiar.artists = pages.filter((p) => !!p);
                                familiar.activeId = familiar.artists.length ? familiar.artists[0].channelId : "";
                            });
                    }

                    ColumnLayout {
                        id: famCol
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.top: parent.top
                        spacing: Style.sp(3)

                        SectionHeading {
                            Layout.fillWidth: true
                            title: "Familiar artists"
                            icon: "artists"
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            Layout.alignment: Qt.AlignTop
                            spacing: Style.sp(6)

                            // the index
                            ColumnLayout {
                                Layout.preferredWidth: Style.sp(78)
                                Layout.alignment: Qt.AlignTop
                                spacing: Style.sp(0.5)
                                Repeater {
                                    model: familiar.artists.slice(0, 6)
                                    delegate: Rectangle {
                                        id: artRow
                                        required property var modelData
                                        required property int index
                                        readonly property bool sel: familiar.activeId === artRow.modelData.channelId
                                        Layout.fillWidth: true
                                        implicitHeight: artLayout.implicitHeight + Style.sp(2)
                                        radius: Style.radius
                                        color: (artRow.sel || artHover.hovered) ? Tokens.tint5 : "transparent"
                                        RowLayout {
                                            id: artLayout
                                            anchors.left: parent.left
                                            anchors.right: parent.right
                                            anchors.verticalCenter: parent.verticalCenter
                                            anchors.leftMargin: Style.sp(1.5)
                                            anchors.rightMargin: Style.sp(1.5)
                                            spacing: Style.sp(2)
                                            Text {
                                                text: (artRow.index + 1 < 10 ? "0" : "") + (artRow.index + 1)
                                                color: Tokens.inkFaint
                                                font.family: Style.fontMono
                                                font.pixelSize: Style.fs.xs
                                            }
                                            Artwork {
                                                url: artRow.modelData.thumbnail ? artRow.modelData.thumbnail : ""
                                                px: Style.sp(9)
                                                round: true
                                                placeholderIcon: "user"
                                            }
                                            ColumnLayout {
                                                Layout.fillWidth: true
                                                spacing: 0
                                                Text {
                                                    Layout.fillWidth: true
                                                    text: artRow.modelData.name ? artRow.modelData.name : "Artist"
                                                    color: Tokens.ink
                                                    font.family: Style.fontUi
                                                    font.pixelSize: Style.fs.sm
                                                    font.weight: Font.Medium
                                                    elide: Text.ElideRight
                                                }
                                                Text {
                                                    Layout.fillWidth: true
                                                    text: artRow.modelData.monthlyListeners ? artRow.modelData.monthlyListeners
                                                        : (artRow.modelData.subscribers ? artRow.modelData.subscribers : "Artist")
                                                    color: Tokens.inkMuted
                                                    font.family: Style.fontUi
                                                    font.pixelSize: Style.fs.xs
                                                    elide: Text.ElideRight
                                                }
                                            }
                                            Text {
                                                text: artRow.sel ? "//" : "聴"
                                                color: artRow.sel ? Tokens.ink : Tokens.inkFaint
                                                font.family: artRow.sel ? Style.fontMono : Tokens.jp
                                                font.pixelSize: Style.fs.xs
                                            }
                                        }
                                        HoverHandler { id: artHover }
                                        MouseArea {
                                            anchors.fill: parent
                                            cursorShape: Qt.PointingHandCursor
                                            onClicked: familiar.activeId = artRow.modelData.channelId
                                            onDoubleClicked: Router.push("artist", { id: artRow.modelData.channelId, title: artRow.modelData.name })
                                        }
                                    }
                                }
                            }

                            // the inspector
                            ColumnLayout {
                                Layout.fillWidth: true
                                Layout.alignment: Qt.AlignTop
                                spacing: Style.sp(3)
                                visible: !!familiar.active

                                RowLayout {
                                    Layout.fillWidth: true
                                    spacing: Style.sp(3)
                                    Artwork {
                                        url: familiar.active && familiar.active.thumbnail ? familiar.active.thumbnail : ""
                                        px: Style.sp(22)
                                        round: true
                                        placeholderIcon: "user"
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Layout.alignment: Qt.AlignVCenter
                                        spacing: Style.sp(1)
                                        Text {
                                            Layout.fillWidth: true
                                            text: "SELECTED ARTIST · " + (familiar.active
                                                ? (familiar.active.monthlyListeners ? familiar.active.monthlyListeners
                                                    : (familiar.active.subscribers ? familiar.active.subscribers : "LIBRARY SIGNAL"))
                                                : "")
                                            color: Tokens.inkFaint
                                            font.family: Style.fontMono
                                            font.pixelSize: Style.fs.xs
                                            font.letterSpacing: 1
                                            elide: Text.ElideRight
                                        }
                                        Text {
                                            Layout.fillWidth: true
                                            text: (familiar.active && familiar.active.name) ? familiar.active.name : "Artist"
                                            color: Tokens.ink
                                            font.family: Tokens.display
                                            font.pixelSize: Style.fs.xl
                                            elide: Text.ElideRight
                                        }
                                        RowLayout {
                                            spacing: Style.sp(2)
                                            Rectangle {
                                                implicitWidth: playRow.implicitWidth + Style.sp(5)
                                                implicitHeight: Style.sp(9)
                                                radius: Style.radius
                                                color: Tokens.ink
                                                RowLayout {
                                                    id: playRow
                                                    anchors.centerIn: parent
                                                    spacing: Style.sp(1.5)
                                                    Icon { name: "play"; size: Style.fs.sm; color: Tokens.paper }
                                                    Text { text: "Play"; color: Tokens.paper; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; font.weight: Font.Medium }
                                                }
                                                MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: page.playArtist(familiar.active) }
                                            }
                                            Rectangle {
                                                implicitWidth: openRow.implicitWidth + Style.sp(5)
                                                implicitHeight: Style.sp(9)
                                                radius: Style.radius
                                                color: "transparent"
                                                border.width: 1
                                                border.color: Tokens.line
                                                RowLayout {
                                                    id: openRow
                                                    anchors.centerIn: parent
                                                    spacing: Style.sp(1.5)
                                                    Text { text: "Open artist"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; font.weight: Font.Medium }
                                                    Icon { name: "arrow-right"; size: Style.fs.sm; color: Tokens.ink }
                                                }
                                                MouseArea {
                                                    anchors.fill: parent
                                                    cursorShape: Qt.PointingHandCursor
                                                    onClicked: if (familiar.active) Router.push("artist", { id: familiar.active.channelId, title: familiar.active.name })
                                                }
                                            }
                                        }
                                    }
                                }

                                // top tracks preview
                                ColumnLayout {
                                    Layout.fillWidth: true
                                    spacing: Style.sp(0.5)
                                    visible: !!(familiar.active && familiar.active.topSongs && familiar.active.topSongs.length)
                                    Repeater {
                                        model: (familiar.active && familiar.active.topSongs) ? familiar.active.topSongs.slice(0, 4) : []
                                        delegate: Item {
                                            id: topRow
                                            required property var modelData
                                            required property int index
                                            Layout.fillWidth: true
                                            implicitHeight: topLayout.implicitHeight + Style.sp(2)
                                            Rectangle {
                                                anchors.fill: parent
                                                radius: Style.radius
                                                color: topHover.hovered ? Tokens.tint5 : "transparent"
                                            }
                                            RowLayout {
                                                id: topLayout
                                                anchors.left: parent.left
                                                anchors.right: parent.right
                                                anchors.verticalCenter: parent.verticalCenter
                                                anchors.leftMargin: Style.sp(1.5)
                                                anchors.rightMargin: Style.sp(1.5)
                                                spacing: Style.sp(2)
                                                Text {
                                                    text: (topRow.index + 1 < 10 ? "0" : "") + (topRow.index + 1)
                                                    color: Tokens.inkFaint
                                                    font.family: Style.fontMono
                                                    font.pixelSize: Style.fs.xs
                                                }
                                                Text {
                                                    Layout.fillWidth: true
                                                    text: topRow.modelData.title ? topRow.modelData.title : ""
                                                    color: Tokens.ink
                                                    font.family: Style.fontUi
                                                    font.pixelSize: Style.fs.sm
                                                    elide: Text.ElideRight
                                                }
                                                Icon { visible: topHover.hovered; name: "play"; size: Style.fs.xs; color: Tokens.inkMuted }
                                            }
                                            HoverHandler { id: topHover }
                                            MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: Playback.play(topRow.modelData) }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                Hairline { Layout.fillWidth: true }
            }
        }

        footer: Item {
            width: list.width
            implicitHeight: footerCol.implicitHeight + Style.sp(20)

            ColumnLayout {
                id: footerCol
                x: page.pad
                width: parent.width - page.pad * 2
                y: Style.sp(4)
                spacing: Style.sp(9)

                // loading skeletons
                Repeater {
                    model: page.loading ? 3 : 0
                    delegate: ColumnLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(3)
                        Skeleton { Layout.preferredWidth: Style.sp(40); Layout.preferredHeight: Style.sp(4) }
                        RowLayout {
                            Layout.fillWidth: true
                            spacing: Style.sp(3)
                            Repeater {
                                model: 6
                                delegate: Skeleton {
                                    required property int index
                                    Layout.preferredWidth: Style.sp(40)
                                    Layout.preferredHeight: Style.sp(50)
                                }
                            }
                        }
                    }
                }

                // error
                ColumnLayout {
                    Layout.alignment: Qt.AlignHCenter
                    visible: !page.loading && page.errorMsg !== ""
                    spacing: Style.sp(2)
                    Text {
                        Layout.alignment: Qt.AlignHCenter
                        text: page.errorMsg
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                    }
                    Chip {
                        Layout.alignment: Qt.AlignHCenter
                        text: "Try again"
                        onClicked: page.load(page.selected)
                    }
                }

                // empty
                ColumnLayout {
                    Layout.alignment: Qt.AlignHCenter
                    Layout.topMargin: Style.sp(16)
                    visible: !page.loading && page.errorMsg === "" && page.blocks.length === 0 && !page.forgotten
                    spacing: Style.sp(3)
                    Icon { Layout.alignment: Qt.AlignHCenter; name: "music"; size: Style.fs.hero; color: Tokens.inkFaint }
                    Text {
                        Layout.alignment: Qt.AlignHCenter
                        Layout.maximumWidth: Style.sp(90)
                        horizontalAlignment: Text.AlignHCenter
                        wrapMode: Text.WordWrap
                        text: (Playback.auth && Playback.auth.signedIn)
                            ? "Your home feed came back empty this time."
                            : "Sign in and home fills up with mixes and playlists built from what you listen to."
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                    }
                    Chip {
                        Layout.alignment: Qt.AlignHCenter
                        text: (Playback.auth && Playback.auth.signedIn) ? "Try again" : "Sign in with Google"
                        active: !(Playback.auth && Playback.auth.signedIn)
                        onClicked: (Playback.auth && Playback.auth.signedIn)
                            ? page.load(page.selected)
                            : Daemon.call("sign_in").catch(() => {})
                    }
                }

                // load-more affordance
                Chip {
                    Layout.alignment: Qt.AlignHCenter
                    visible: !page.loading && !!(page.home && page.home.continuation) && page.moreError
                    text: page.loadingMore ? "Loading…" : "Try again"
                    onClicked: { page.moreError = false; page.loadMore(); }
                }
            }
        }
    }
}
