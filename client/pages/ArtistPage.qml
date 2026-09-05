pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Quickshell
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The artist page, ported from ui/src/routes/artist/[id]/+page.svelte. get_artist(id) once; the page
// is one TrackList whose header is a PageHero (round artist art, name, counts, Play/Shuffle/Radio/
// Subscribe) followed by the "Popular" heading, whose rows are the five top songs (Show all opens the
// full top-songs playlist), and whose footer is Releases — a CardGrid with a Singles/Albums/EPs chip
// filter — and the remaining carousels. Shuffle prefers the full top-songs playlist (topSongsId) so it
// covers more than the visible rows. Data flows are unchanged.
Item {
    id: page

    readonly property var params: Router.current ? Router.current.params : ({})
    readonly property string artistId: page.params && page.params.id ? page.params.id : ""

    property var artist: null
    property bool loading: true
    property string errorMsg: ""
    property bool expanded: false
    property bool subscribed: false
    property bool subBusy: false
    property int releaseTab: 0

    readonly property bool signedIn: !!(Playback.auth && Playback.auth.signedIn)
    readonly property var sections: {
        if (!page.artist || !page.artist.sections)
            return [];
        return page.artist.sections.filter((s) => !/music\s*videos?|video\s+for\s+you/i.test(s.title));
    }
    readonly property var releaseSections: page.sections.filter((s) => /album|single|\bep/i.test(s.title))
    readonly property var otherSections: page.sections.filter((s) => !/album|single|\bep/i.test(s.title))
    readonly property var topSongs: (page.artist && page.artist.topSongs) ? page.artist.topSongs : []
    readonly property var popular: page.topSongs.slice(0, 5)

    onParamsChanged: page.load()
    Component.onCompleted: page.load()

    function asItem() {
        return {
            kind: "artist",
            id: page.artistId,
            title: page.artist ? page.artist.name : "Artist",
            subtitle: page.artist ? page.artist.subscribers : "",
            thumbnail: page.artist ? page.artist.thumbnail : ""
        };
    }

    function metaLine() {
        var a = page.artist;
        if (!a)
            return "";
        var parts = [];
        if (a.subscribers) parts.push(a.subscribers);
        if (a.monthlyListeners) parts.push(a.monthlyListeners);
        return parts.join("  \u00b7  ");
    }

    function load() {
        if (!page.artistId)
            return;
        page.loading = true;
        page.errorMsg = "";
        page.expanded = false;
        page.releaseTab = 0;
        var reqId = page.artistId;
        Daemon.call("get_artist", { id: page.artistId })
            .then((a) => {
                if (page.artistId !== reqId)
                    return;
                page.artist = a;
                page.subscribed = !!a.subscribed;
                page.loading = false;
                Qt.callLater(page.scrollTop);
            })
            .catch((e) => {
                if (page.artistId !== reqId)
                    return;
                page.errorMsg = (e && e.message) ? e.message : String(e);
                page.loading = false;
            });
    }

    function scrollTop() {
        if (body.visible && body.view)
            body.view.positionViewAtBeginning();
    }

    function playTop(start) {
        if (!page.artist || !page.topSongs.length)
            return;
        Daemon.call("play_playlist", {
            items: page.artist.topSongs,
            start: start,
            sourceName: page.artist.name
        }).catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }
    function shuffle() {
        if (!page.artist)
            return;
        var pid = page.artist.topSongsId;
        if (pid) {
            Daemon.call("get_playlist", { id: pid })
                .then((pl) => {
                    if (pl.items && pl.items.length)
                        return Daemon.call("play_playlist", {
                            items: pl.items, start: null, sourceId: pid,
                            sourceName: page.artist.name, shuffle: true, continuation: pl.continuation
                        });
                    return page.shuffleVisible();
                })
                .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
            return;
        }
        page.shuffleVisible();
    }
    function shuffleVisible() {
        if (!page.artist || !page.topSongs.length)
            return Promise.resolve();
        return Daemon.call("play_playlist", {
            items: page.artist.topSongs, start: null, sourceName: page.artist.name, shuffle: true
        });
    }
    function radio() {
        Playback.toast("Starting radio…", "info");
        Daemon.call("start_radio", { kind: "artist", id: page.artistId, name: page.artist ? page.artist.name : null })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not start radio", "error"));
    }
    function toggleSub() {
        if (!page.artist || page.subBusy)
            return;
        if (!page.signedIn) {
            Playback.toast("Sign in to subscribe", "info");
            return;
        }
        var next = !page.subscribed;
        page.subBusy = true;
        page.subscribed = next;
        Daemon.call("subscribe", { channelId: page.artist.channelId, subscribed: next })
            .then(() => { page.subBusy = false; Playback.toast(next ? ("Subscribed to " + (page.artist.name || "")) : "Unsubscribed", "success"); })
            .catch((e) => {
                page.subscribed = !next;
                page.subBusy = false;
                Playback.toast((e && e.message) ? e.message : "Could not subscribe", "error");
            });
    }
    function share() {
        var url = "https://music.youtube.com/channel/" + encodeURIComponent(page.artistId);
        Quickshell.execDetached(["sh", "-c", "printf %s \"$1\" | wl-copy", "sh", url]);
        Playback.toast("Link copied", "success");
    }
    function showMore(section) {
        Router.push("list", { id: section.moreBrowseId, title: section.title, params: section.moreParams });
    }
    function seeAllTop() {
        if (page.artist && page.artist.topSongsId)
            Router.push("playlist", { id: page.artist.topSongsId, title: "Top songs" });
    }
    function displaySectionTitle(title) {
        var name = page.artist && page.artist.name ? page.artist.name.trim() : "";
        if (name && title.trim().toLowerCase() === name.toLowerCase())
            return "More like " + name;
        return title;
    }

    Text {
        anchors.centerIn: parent
        visible: page.loading || page.errorMsg !== ""
        text: page.loading ? "Loading artist…" : page.errorMsg
        color: Tokens.inkMuted
        font.family: Style.fontUi
        font.pixelSize: Style.fs.md
    }

    TrackList {
        id: body
        anchors.fill: parent
        visible: !page.loading && page.errorMsg === "" && page.artist !== null
        items: page.popular
        showHeader: true
        showAlbum: true
        showPlays: true
        source: page.artist ? page.artist.name : ""
        onActivated: (i) => page.playTop(i)
        header: artistHeader
        footer: artistFooter
    }

    Component {
        id: artistHeader
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
                    round: true
                    eyebrow: "Artist"
                    title: (page.artist && page.artist.name) ? page.artist.name : "Artist"
                    meta: page.metaLine()
                    art: (page.artist && page.artist.thumbnail) ? page.artist.thumbnail : ""
                    placeholderIcon: "user"
                    primaryLabel: "Play"
                    likeable: false
                    showMore: true
                    onPrimary: page.playTop(null)
                    onMore: {
                        var p = hero.mapToItem(page, Style.sp(44), hero.height - Style.sp(8));
                        artistMenu.openAt(p.x, p.y);
                    }
                }

                // controls: Shuffle, Radio, Subscribe
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(3)
                    Btn {
                        text: "Shuffle"
                        icon: "shuffle"
                        enabled: page.topSongs.length > 0
                        onClicked: page.shuffle()
                    }
                    Btn {
                        text: "Radio"
                        icon: "radio"
                        onClicked: page.radio()
                    }
                    Btn {
                        text: !page.signedIn ? "Save to library" : (page.subscribed ? "Subscribed" : "Subscribe")
                        icon: page.subscribed ? "check-circle" : "add"
                        enabled: !page.subBusy
                        onClicked: page.toggleSub()
                    }
                    Item { Layout.fillWidth: true }
                }

                // description (collapsible)
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: !!(page.artist && page.artist.description)
                    spacing: Style.sp(0.5)
                    Text {
                        Layout.fillWidth: true
                        text: (page.artist && page.artist.description) ? page.artist.description : ""
                        color: Tokens.inkDim
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        wrapMode: Text.WordWrap
                        maximumLineCount: page.expanded ? 999 : 2
                        elide: Text.ElideRight
                    }
                    Text {
                        text: page.expanded ? "LESS" : "MORE"
                        color: descHover.hovered ? Tokens.ink : Tokens.inkMuted
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        font.letterSpacing: Style.trackMicro
                        HoverHandler { id: descHover }
                        MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: page.expanded = !page.expanded }
                    }
                }

                // "Popular" heading over the rows
                SectionHeading {
                    Layout.fillWidth: true
                    Layout.topMargin: Style.sp(2)
                    visible: page.popular.length > 0
                    title: "Popular"
                    more: !!(page.artist && page.artist.topSongsId)
                    onMoreClicked: page.seeAllTop()
                }
            }
        }
    }

    Component {
        id: artistFooter
        Item {
            width: body.view.width
            implicitHeight: footerCol.implicitHeight + Style.sp(24)
            ColumnLayout {
                id: footerCol
                width: parent.width
                y: Style.sp(6)
                spacing: Style.sp(9)

                // Releases: chip filter + card grid
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: page.releaseSections.length > 0
                    spacing: Style.sp(4)
                    SectionHeading { Layout.fillWidth: true; title: "Releases" }
                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(2)
                        Repeater {
                            model: page.releaseSections
                            delegate: Chip {
                                required property var modelData
                                required property int index
                                text: modelData.title
                                active: page.releaseTab === index
                                onClicked: page.releaseTab = index
                            }
                        }
                        Item { Layout.fillWidth: true }
                    }
                    Grid {
                        id: relGrid
                        Layout.fillWidth: true
                        readonly property int cols: Math.max(2, Math.floor(width / (Style.cardW + Style.sp(4))))
                        readonly property real cw: relGrid.cols > 0 ? (width - (relGrid.cols - 1) * Style.sp(4)) / relGrid.cols : Style.cardW
                        readonly property var relItems: (page.releaseSections[page.releaseTab] && page.releaseSections[page.releaseTab].items)
                            ? page.releaseSections[page.releaseTab].items : []
                        columns: relGrid.cols
                        columnSpacing: Style.sp(4)
                        rowSpacing: Style.sp(6)
                        Repeater {
                            model: relGrid.relItems
                            delegate: MediaCard {
                                required property var modelData
                                item: modelData
                                cardWidth: relGrid.cw
                            }
                        }
                    }
                }

                // remaining carousels (related artists, featured on, …)
                Repeater {
                    model: page.otherSections
                    delegate: Shelf {
                        required property var modelData
                        Layout.fillWidth: true
                        section: {
                            return { title: page.displaySectionTitle(modelData.title), items: modelData.items,
                                moreBrowseId: modelData.moreBrowseId, moreParams: modelData.moreParams };
                        }
                    }
                }
            }
        }
    }

    Menu {
        id: artistMenu
        customItems: [
            { icon: "dashboard", label: "Add to shortcuts", danger: false, act: () => page.addArtistShortcut() },
            { icon: "link", label: "Share", danger: false, act: () => page.share() }
        ]
    }

    // Personal-store action (Task 4b): pin this artist to the Home shortcuts grid. Wired by PersonalStore.
    function addArtistShortcut() {
        Playback.toast(Personal.addPick(page.asItem()) ? "Added to shortcuts" : "Already in shortcuts", "success");
    }
}
