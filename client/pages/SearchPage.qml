pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"
import "../chrome"
import "../lib/browse.js" as Browse
import "../lib/ids.js" as Ids

// The search page, ported from ui/src/routes/search/+page.svelte. The field runs one mixed page
// (search_all) plus one songs page (search_page) in parallel, then lays the result out as the same
// categorised sections the Svelte page has — Top (4), Songs (6), Albums (5), Artists (3), Playlists
// (5) — each with a "Show more" that expands that category in place (search_cards / search_page_more,
// the folded-in search-more route). The SearchSuggest typeahead handles quick keyboard navigation.
Item {
    id: page

    property string query: ""
    property var res: null
    property var songs: []
    property string searched: ""
    property bool searching: false
    property string errorMsg: ""
    property var history: []
    property string latest: ""

    // expanded category ("" = the sections view)
    property string expandedCat: ""
    property var expandedItems: []
    property string expandedCont: ""
    property bool expandedLoading: false

    // Spotify is selected but not signed in: show the sign-in empty state instead of searching the
    // daemon (search is gated on a signed-in Spotify account).
    readonly property bool spotifyGate: Playback.provider === "spotify" && !(Playback.spotify && Playback.spotify.signedIn)

    // A ?q= arrival (the Ctrl+K palette's "All results", or a card's search intent) runs the search.
    readonly property var params: Router.current ? Router.current.params : ({})
    onParamsChanged: page.applyParams()
    Component.onCompleted: page.applyParams()
    function applyParams() {
        if (page.params && page.params.q && page.params.q !== page.searched) {
            page.query = page.params.q;
            page.runSearch();
        }
    }

    // A provider switch swaps the catalogue: re-run the current query against the new provider (or
    // fall to the sign-in card).
    Connections {
        target: Playback
        function onProviderChanged(): void {
            if (page.spotifyGate || page.searched === "")
                return;
            page.res = null;
            page.songs = [];
            page.expandedCat = "";
            page.query = page.searched;
            page.runSearch();
        }
    }

    readonly property var songRows: {
        var s = page.songs.length ? page.songs : ((page.res && page.res.songs) ? page.res.songs.map(Browse.asSong) : []);
        return s.filter((song) => !song.is_video);
    }
    readonly property var sections: {
        if (!page.res)
            return [];
        var out = [
            { key: "top", label: "Top results", items: page.res.top, max: 4, more: false, list: false },
            { key: "songs", label: "Songs", items: page.res.songs, max: 6, more: true, list: true },
            { key: "albums", label: "Albums", items: page.res.albums, max: 5, more: true, list: false },
            { key: "artists", label: "Artists", items: page.res.artists, max: 3, more: true, list: false },
            { key: "playlists", label: "Playlists", items: page.res.playlists, max: 5, more: true, list: false }
        ];
        return out.filter((s) => s.list ? page.songRows.length : (s.items && s.items.length));
    }

    // Derived views onto the result shape for the rebuilt layout: the single hero Top result, the
    // capped song rows the TrackList shows, and the album/artist/playlist card shelves.
    readonly property var topItem: (page.res && page.res.top && page.res.top.length) ? page.res.top[0] : null
    readonly property int songsMax: 6
    readonly property var songsShown: page.songRows.slice(0, page.songsMax)
    readonly property var cardSections: page.sections.filter((s) => !s.list && s.key !== "top")

    function rememberQuery(q) {
        if (!q)
            return;
        var next = [q];
        for (var i = 0; i < page.history.length; i++)
            if (page.history[i].toLowerCase() !== q.toLowerCase())
                next.push(page.history[i]);
        page.history = next.slice(0, 6);
    }

    function runSearch() {
        var q = page.query.trim().replace(/\s+/g, " ");
        // Gated: no daemon call, the sign-in card carries the page.
        if (!q || page.spotifyGate)
            return;
        page.latest = q;
        page.searched = q;
        page.expandedCat = "";
        page.rememberQuery(q);
        if (!page.res || page.searched !== q)
            page.searching = true;
        page.errorMsg = "";
        Promise.all([
            Daemon.call("search_all", { query: q }),
            Daemon.call("search_page", { query: q }).catch(() => ({ items: [] }))
        ]).then((r) => {
            if (page.latest !== q)
                return;
            page.res = r[0];
            page.songs = r[1].items || [];
            page.searched = q;
            page.searching = false;
            // Land the results list at the top: a ListView with a tall header (the Top-result card)
            // can otherwise settle with contentY > 0 and clip the card.
            Qt.callLater(page.scrollTop);
        }).catch((e) => {
            if (page.latest !== q)
                return;
            page.errorMsg = (e && e.message) ? e.message : String(e);
            page.searching = false;
        });
    }

    function scrollTop() {
        if (body.visible && body.view)
            body.view.positionViewAtBeginning();
    }

    function playSong(song) {
        Playback.play(song).catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }

    // The hero Top result: a click opens it (a song plays, a collection routes), the Play affordance
    // plays it — mirroring MediaCard so an album/playlist top hit still starts straight from the card.
    function openTop(item) {
        if (!item)
            return;
        if (item.kind === "song")
            Playback.play(Browse.asSong(item));
        else
            Router.push(item.kind, { id: item.id, title: item.title });
    }
    function playTop(item) {
        if (!item)
            return;
        if (item.kind === "song") {
            page.playSong(Browse.asSong(item));
        } else if (item.kind === "album") {
            Daemon.call("get_album", { id: item.id })
                .then((a) => Daemon.call("play_playlist", { items: a.items, sourceId: a.playlistId, sourceName: item.title }))
                .catch(() => Playback.toast("Could not play — try opening it", "error"));
        } else if (item.kind === "playlist") {
            Daemon.call("get_playlist", { id: item.id })
                .then((p) => Daemon.call("play_playlist", {
                    items: p.items,
                    sourceId: Ids.isSmartPlaylistId(item.id) ? undefined : item.id,
                    sourceName: item.title,
                    continuation: p.continuation
                }))
                .catch(() => Playback.toast("Could not play — try opening it", "error"));
        } else {
            Router.push(item.kind, { id: item.id, title: item.title });
        }
    }

    function showMore(sec) {
        page.expandedCat = sec.key;
        page.expandedItems = [];
        page.expandedCont = "";
        page.expandedLoading = true;
        if (sec.key === "songs") {
            Daemon.call("search_page", { query: page.searched })
                .then((r) => { page.expandedItems = (r.items || []).filter((s) => !s.is_video); page.expandedCont = r.continuation || ""; page.expandedLoading = false; })
                .catch(() => page.expandedLoading = false);
        } else {
            Daemon.call("search_cards", { query: page.searched, category: sec.key })
                .then((items) => { page.expandedItems = items || []; page.expandedLoading = false; })
                .catch(() => page.expandedLoading = false);
        }
    }
    function loadMoreExpanded() {
        if (page.expandedCat !== "songs" || !page.expandedCont || page.expandedLoading)
            return;
        page.expandedLoading = true;
        var token = page.expandedCont;
        Daemon.call("search_page_more", { token: token })
            .then((r) => {
                page.expandedItems = page.expandedItems.concat((r.items || []).filter((s) => !s.is_video));
                page.expandedCont = r.continuation || "";
                page.expandedLoading = false;
            })
            .catch(() => page.expandedLoading = false);
    }

    // --- sections view -------------------------------------------------------------------------
    // The field sits fixed above the results (its typeahead dropdown then overlays them), and the
    // resolved sections scroll inside one TrackList: the Top card and Songs heading ride in the
    // header, the song rows are the list body, and the card shelves ride in the footer.
    ColumnLayout {
        id: root
        anchors.fill: parent
        spacing: Style.sp(4)
        visible: page.expandedCat === "" && !page.spotifyGate

        ColumnLayout {
            Layout.fillWidth: true
            spacing: Style.sp(1)
            Text {
                text: "// MUSIC / DISCOVERY"
                color: Tokens.inkFaint
                font.family: Style.fontMono
                font.pixelSize: Style.fs.micro
                font.letterSpacing: Style.trackMicro
            }
            Text {
                text: "Search"
                color: Tokens.ink
                font.family: Style.fontDisplay
                font.pixelSize: Style.fs.title
            }
        }

        SearchSuggest {
            id: suggest
            Layout.fillWidth: true
            Layout.maximumWidth: Style.sp(160)
            value: page.query
            onValueChanged: page.query = value
            onSubmitted: page.runSearch()
            onPicked: page.query = value
            z: 40
        }

        Item {
            Layout.fillWidth: true
            Layout.fillHeight: true

            TrackList {
                id: body
                anchors.fill: parent
                visible: !page.searching && !!page.res && page.sections.length > 0
                items: page.songsShown
                showHeader: true
                showAlbum: true
                showPlays: true
                canAdd: true
                onActivated: (i) => { if (page.songsShown[i]) page.playSong(page.songsShown[i]); }
                header: searchHeader
                footer: searchFooter
            }

            Text {
                anchors.top: parent.top
                anchors.left: parent.left
                anchors.topMargin: Style.sp(2)
                visible: page.searching
                text: "Resolving songs, artists, albums and playlists…"
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
            }

            Text {
                anchors.top: parent.top
                anchors.left: parent.left
                anchors.topMargin: Style.sp(2)
                visible: page.errorMsg !== ""
                text: page.errorMsg
                color: Style.alert
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
            }

            Text {
                anchors.top: parent.top
                anchors.left: parent.left
                anchors.topMargin: Style.sp(2)
                visible: !page.searching && !!page.res && page.sections.length === 0
                text: "No results for \u201C" + page.searched + "\u201D."
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
            }

            // idle: recent searches
            ColumnLayout {
                anchors.top: parent.top
                anchors.left: parent.left
                anchors.right: parent.right
                anchors.topMargin: Style.sp(2)
                visible: !page.searching && !page.res && page.errorMsg === ""
                spacing: Style.sp(2)
                Text {
                    text: "RECENT SEARCHES"
                    color: Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
                }
                Repeater {
                    model: page.history
                    delegate: Rectangle {
                        id: histRow
                        required property var modelData
                        required property int index
                        Layout.fillWidth: true
                        implicitHeight: Style.sp(9)
                        radius: Style.radius
                        color: hHover.hovered ? Tokens.tint5 : "transparent"
                        RowLayout {
                            anchors.fill: parent
                            anchors.leftMargin: Style.sp(2)
                            anchors.rightMargin: Style.sp(2)
                            spacing: Style.sp(3)
                            Text { text: String(histRow.index + 1).padStart(2, "0"); color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.sm }
                            Text { Layout.fillWidth: true; text: histRow.modelData; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; elide: Text.ElideRight }
                        }
                        HoverHandler { id: hHover }
                        MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: { page.query = histRow.modelData; page.runSearch(); } }
                    }
                }
                Text {
                    visible: page.history.length === 0
                    text: "Search songs, artists, albums and playlists. Press Ctrl K from anywhere."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                }
            }
        }
    }

    // header: the Top result card + the Songs heading, riding above the song rows.
    Component {
        id: searchHeader
        Item {
            width: body.view.width
            implicitHeight: headerCol.implicitHeight + Style.sp(6)

            ColumnLayout {
                id: headerCol
                width: parent.width
                spacing: Style.sp(6)

                // Top result — a wide card
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: page.topItem !== null
                    spacing: Style.sp(3)

                    SectionHeading {
                        Layout.fillWidth: true
                        title: "Top result"
                    }

                    Rectangle {
                        id: topCard
                        Layout.fillWidth: true
                        Layout.maximumWidth: Style.sp(140)
                        implicitHeight: topRow.implicitHeight + Style.sp(6)
                        radius: Style.radiusCard
                        color: Tokens.paperLift
                        border.width: 1
                        border.color: topHover.hovered ? Tokens.lineStrong : Tokens.line
                        Behavior on border.color { ColorAnimation { duration: Style.motion.snap } }

                        HoverHandler { id: topHover }

                        RowLayout {
                            id: topRow
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.verticalCenter: parent.verticalCenter
                            anchors.leftMargin: Style.sp(3)
                            anchors.rightMargin: Style.sp(3)
                            spacing: Style.sp(3)

                            Artwork {
                                Layout.preferredWidth: Style.sp(24)
                                Layout.preferredHeight: Style.sp(24)
                                url: (page.topItem && page.topItem.thumbnail) ? page.topItem.thumbnail : ""
                                px: Style.sp(24)
                                round: !!(page.topItem && page.topItem.kind === "artist")
                                placeholderIcon: (page.topItem && page.topItem.kind === "artist") ? "user"
                                    : (page.topItem && page.topItem.kind === "album") ? "cd"
                                    : (page.topItem && page.topItem.kind === "playlist") ? "playlist" : "music"
                            }

                            // identity block — clicking it opens the item
                            MouseArea {
                                Layout.fillWidth: true
                                Layout.fillHeight: true
                                cursorShape: Qt.PointingHandCursor
                                onClicked: page.openTop(page.topItem)
                                ColumnLayout {
                                    anchors.left: parent.left
                                    anchors.right: parent.right
                                    anchors.verticalCenter: parent.verticalCenter
                                    spacing: Style.sp(1)
                                    Text {
                                        Layout.fillWidth: true
                                        text: page.topItem ? page.topItem.kind.toUpperCase() : ""
                                        color: Tokens.inkFaint
                                        font.family: Style.fontMono
                                        font.pixelSize: Style.fs.micro
                                        font.letterSpacing: Style.trackMicro
                                    }
                                    Text {
                                        Layout.fillWidth: true
                                        text: page.topItem ? page.topItem.title : ""
                                        color: Tokens.ink
                                        font.family: Style.fontUi
                                        font.pixelSize: Style.fs.md
                                        font.weight: Font.DemiBold
                                        elide: Text.ElideRight
                                    }
                                    Text {
                                        Layout.fillWidth: true
                                        visible: !!(page.topItem && page.topItem.subtitle)
                                        text: (page.topItem && page.topItem.subtitle) ? page.topItem.subtitle : ""
                                        color: Tokens.inkMuted
                                        font.family: Style.fontUi
                                        font.pixelSize: Style.fs.sm
                                        elide: Text.ElideRight
                                    }
                                }
                            }

                            Btn {
                                Layout.alignment: Qt.AlignVCenter
                                visible: !!(page.topItem && page.topItem.kind !== "artist")
                                text: "Play"
                                icon: "play"
                                primary: true
                                onClicked: page.playTop(page.topItem)
                            }
                        }
                    }
                }

                // Songs heading — the list rows below carry the songs
                SectionHeading {
                    Layout.fillWidth: true
                    visible: page.songRows.length > 0
                    title: "Songs"
                    more: true
                    onMoreClicked: page.showMore({ key: "songs" })
                }
            }
        }
    }

    // footer: album / artist / playlist card shelves, each with an in-place "Show more".
    Component {
        id: searchFooter
        Item {
            width: body.view.width
            implicitHeight: footerCol.implicitHeight + Style.sp(20)

            ColumnLayout {
                id: footerCol
                width: parent.width
                spacing: Style.sp(6)

                Repeater {
                    model: page.cardSections
                    delegate: ColumnLayout {
                        id: secCol
                        required property var modelData
                        Layout.fillWidth: true
                        Layout.topMargin: Style.sp(3)
                        spacing: Style.sp(3)

                        SectionHeading {
                            Layout.fillWidth: true
                            title: secCol.modelData.label
                            more: secCol.modelData.more
                            onMoreClicked: page.showMore(secCol.modelData)
                        }

                        ListView {
                            Layout.fillWidth: true
                            Layout.preferredHeight: Style.sp(66)
                            orientation: ListView.Horizontal
                            flickableDirection: Flickable.HorizontalFlick
                            boundsBehavior: Flickable.StopAtBounds
                            snapMode: ListView.SnapOneItem
                            reuseItems: true
                            clip: true
                            cacheBuffer: Math.round(width)
                            spacing: Style.sp(3)
                            model: secCol.modelData.items.slice(0, secCol.modelData.max)
                            delegate: MediaCard {
                                required property var modelData
                                item: modelData
                                cardWidth: Style.sp(40)
                            }
                        }
                    }
                }
            }
        }
    }

    // --- expanded category view ----------------------------------------------------------------
    Item {
        anchors.fill: parent
        visible: page.expandedCat !== "" && !page.spotifyGate

        RowLayout {
            id: backBar
            anchors.top: parent.top
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.topMargin: Style.sp(2)
            spacing: Style.sp(2)
            IconButton {
                icon: "arrow-left"
                iconSize: Style.fs.lg
                diameter: Style.sp(9)
                onClicked: page.expandedCat = ""
            }
            Text {
                Layout.fillWidth: true
                text: page.expandedCat.toUpperCase() + " · \u201C" + page.searched + "\u201D"
                color: Tokens.ink
                font.family: Style.fontUi
                font.pixelSize: Style.fs.lg
                font.weight: Font.DemiBold
                elide: Text.ElideRight
            }
        }

        // songs expanded
        TrackList {
            anchors.top: backBar.bottom
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            anchors.topMargin: Style.sp(3)
            visible: page.expandedCat === "songs"
            items: page.expandedItems
            showHeader: true
            showAlbum: true
            showPlays: true
            canAdd: true
            onActivated: (i) => { if (page.expandedItems[i]) page.playSong(page.expandedItems[i]); }
            Component.onCompleted: view.contentYChanged.connect(page.loadMoreExpanded)
        }

        // cards expanded
        CardGrid {
            anchors.top: backBar.bottom
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            anchors.topMargin: Style.sp(3)
            visible: page.expandedCat !== "" && page.expandedCat !== "songs"
            pad: 0
            loading: page.expandedLoading && page.expandedItems.length === 0
            model: page.expandedItems
            emptyText: "No results."
        }
    }

    // The Spotify sign-in empty state: shown when Spotify is selected but not signed in, in place of
    // the search surface. The button starts the OAuth flow; nothing here touches the daemon.
    SpotifyGate {
        anchors.centerIn: parent
        visible: page.spotifyGate
    }
}
