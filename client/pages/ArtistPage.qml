pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import QtQuick.Effects
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

    // SoundCloud user page (the Orange look): get_artist for an sc:user id returns kind:"soundcloud"
    // with the user's own tracks, albums, playlists and likes rather than the YouTube sections.
    readonly property bool isSC: !!(page.artist && page.artist.kind === "soundcloud")
    readonly property var scTracks: (page.artist && page.artist.tracks) || []
    readonly property var scAlbums: (page.artist && page.artist.albums) || []
    readonly property var scPlaylists: (page.artist && page.artist.playlists) || []
    readonly property var scLikes: (page.artist && page.artist.likes) || []

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

    function scMetaLine() {
        var a = page.artist;
        if (!a)
            return "";
        var parts = [];
        if (a.city) parts.push(String(a.city));
        var f = Style.fmtCount(a.followers);
        if (f) parts.push(f + " followers");
        var t = Style.fmtCount(a.trackCount);
        if (t) parts.push(t + " tracks");
        return parts.join("  \u00b7  ");
    }
    function playSC(start) {
        if (!page.scTracks.length)
            return;
        Daemon.call("play_playlist", { items: page.scTracks, start: start, sourceName: page.artist ? page.artist.name : "" })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }
    function shuffleSC() {
        if (!page.scTracks.length)
            return;
        Daemon.call("play_playlist", { items: page.scTracks, start: null, shuffle: true, sourceName: page.artist ? page.artist.name : "" })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
    }
    function playLikes(start) {
        if (!page.scLikes.length)
            return;
        Daemon.call("play_playlist", { items: page.scLikes, start: start, sourceName: (page.artist ? page.artist.name : "") + " \u00b7 Likes" })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not play", "error"));
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
        visible: !page.loading && page.errorMsg === "" && page.artist !== null && !page.isSC
        items: page.popular
        showHeader: true
        showAlbum: true
        showPlays: true
        source: page.artist ? page.artist.name : ""
        onActivated: (i) => page.playTop(i)
        header: artistHeader
        footer: artistFooter
    }

    // ── SoundCloud user (the Orange look) ───────────────────────────────────────────────────
    // A blurred banner with the round avatar overlapping its foot, the name + verified tick and a
    // city · followers · tracks line, then two columns: the user's tracks on the left, and the
    // albums / playlists / likes rail (300 px) on the right.
    component ScMiniCard: Item {
        id: mc
        property var card: null
        property bool playable: false      // a liked track plays; an album/playlist routes
        signal activated()
        Layout.fillWidth: true
        implicitHeight: Style.sp(14) + Style.sp(2)

        Rectangle {
            anchors.fill: parent
            radius: Style.radius
            color: mcHover.hovered ? Tokens.tint5 : "transparent"
            Behavior on color { ColorAnimation { duration: Style.motion.snap } }
        }
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: Style.sp(1)
            anchors.rightMargin: Style.sp(1)
            spacing: Style.sp(2)
            Artwork {
                Layout.alignment: Qt.AlignVCenter
                url: (mc.card && mc.card.thumbnail) ? mc.card.thumbnail : ""
                px: Style.sp(14)
                placeholderIcon: mc.playable ? "music" : "music"
            }
            ColumnLayout {
                Layout.fillWidth: true
                spacing: 1
                Text {
                    Layout.fillWidth: true
                    text: (mc.card && mc.card.title) ? mc.card.title : ""
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    font.weight: Font.Medium
                    elide: Text.ElideRight
                }
                Text {
                    Layout.fillWidth: true
                    visible: text !== ""
                    text: mc.card ? (mc.card.subtitle || mc.card.artists || "") : ""
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.xs
                    elide: Text.ElideRight
                }
            }
        }
        HoverHandler { id: mcHover }
        MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: mc.activated() }
    }

    Flickable {
        id: scView
        anchors.fill: parent
        visible: !page.loading && page.errorMsg === "" && page.isSC
        clip: true
        contentWidth: width
        contentHeight: scCol.implicitHeight
        boundsBehavior: Flickable.StopAtBounds

        ColumnLayout {
            id: scCol
            width: scView.width
            spacing: Style.sp(5)

            // banner + avatar + identity
            Item {
                id: scHead
                Layout.fillWidth: true
                readonly property int bannerH: Style.sp(50)     // 200
                readonly property int avatarPx: Style.sp(30)    // 120
                readonly property int pad: Style.pagePad
                implicitHeight: Math.max(banner.height + scHead.avatarPx / 2,
                    banner.height + Style.sp(2) + identity.implicitHeight) + Style.sp(3)

                Item {
                    id: banner
                    width: parent.width
                    height: scHead.bannerH
                    clip: true
                    Rectangle { anchors.fill: parent; color: Tokens.paperLift }
                    Image {
                        id: bannerImg
                        anchors.fill: parent
                        source: (page.artist && page.artist.banner) ? Style.thumb(page.artist.banner, 1200) : ""
                        fillMode: Image.PreserveAspectCrop
                        asynchronous: true
                        cache: true
                        visible: false
                    }
                    MultiEffect {
                        anchors.fill: parent
                        source: bannerImg
                        visible: Style.blurEnabled && bannerImg.status === Image.Ready
                        blurEnabled: true
                        blur: 1.0
                        blurMax: 48
                        saturation: -0.2
                        opacity: 0.55
                    }
                    // fade the banner to paper so the avatar and name sit on the page
                    Rectangle {
                        anchors.fill: parent
                        gradient: Gradient {
                            GradientStop { position: 0.0; color: Qt.rgba(Tokens.paper.r, Tokens.paper.g, Tokens.paper.b, 0.35) }
                            GradientStop { position: 0.6; color: Qt.rgba(Tokens.paper.r, Tokens.paper.g, Tokens.paper.b, 0.55) }
                            GradientStop { position: 1.0; color: Tokens.paper }
                        }
                    }
                    Hairline { anchors.bottom: parent.bottom; width: parent.width; height: 1 }
                }

                // round avatar overlapping the banner's bottom edge
                Item {
                    x: scHead.pad
                    y: banner.height - scHead.avatarPx / 2
                    width: scHead.avatarPx
                    height: scHead.avatarPx
                    Artwork {
                        anchors.fill: parent
                        url: (page.artist && page.artist.thumbnail) ? page.artist.thumbnail : ""
                        px: scHead.avatarPx
                        round: true
                        placeholderIcon: "user"
                    }
                    Rectangle {
                        anchors.fill: parent
                        radius: width / 2
                        color: "transparent"
                        border.width: 2
                        border.color: Tokens.paper
                    }
                }

                ColumnLayout {
                    id: identity
                    x: scHead.pad + scHead.avatarPx + Style.sp(4)
                    y: banner.height + Style.sp(2)
                    width: Math.max(Style.sp(40), scHead.width - x - scHead.pad)
                    spacing: Style.sp(1)

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(1.5)
                        Text {
                            Layout.maximumWidth: identity.width - Style.sp(6)
                            text: (page.artist && page.artist.name) ? page.artist.name : "SoundCloud"
                            color: Tokens.ink
                            font.family: Style.fontDisplay
                            font.pixelSize: Style.fs.title
                            elide: Text.ElideRight
                        }
                        Icon {
                            visible: !!(page.artist && page.artist.verified)
                            Layout.alignment: Qt.AlignVCenter
                            name: "check-circle"
                            size: Style.fs.md
                            color: Style.providerColors.soundcloud
                        }
                        Item { Layout.fillWidth: true }
                    }
                    Text {
                        Layout.fillWidth: true
                        visible: text !== ""
                        text: page.scMetaLine()
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        elide: Text.ElideRight
                    }
                    ColumnLayout {
                        Layout.fillWidth: true
                        Layout.topMargin: Style.sp(1)
                        visible: !!(page.artist && page.artist.description)
                        spacing: Style.sp(0.5)
                        Text {
                            Layout.fillWidth: true
                            text: (page.artist && page.artist.description) ? page.artist.description : ""
                            color: Tokens.inkDim
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.sm
                            wrapMode: Text.WordWrap
                            maximumLineCount: page.expanded ? 999 : 3
                            elide: Text.ElideRight
                        }
                        Text {
                            text: page.expanded ? "LESS" : "MORE"
                            color: scDescHover.hovered ? Tokens.ink : Tokens.inkMuted
                            font.family: Style.fontMono
                            font.pixelSize: Style.fs.micro
                            font.letterSpacing: Style.trackMicro
                            HoverHandler { id: scDescHover }
                            MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: page.expanded = !page.expanded }
                        }
                    }
                }
            }

            // play / shuffle
            RowLayout {
                Layout.fillWidth: true
                Layout.leftMargin: Style.pagePad
                Layout.rightMargin: Style.pagePad
                spacing: Style.sp(3)
                Btn { text: "Play"; icon: "play"; primary: true; enabled: page.scTracks.length > 0; onClicked: page.playSC(0) }
                Btn { text: "Shuffle"; icon: "shuffle"; enabled: page.scTracks.length > 0; onClicked: page.shuffleSC() }
                Item { Layout.fillWidth: true }
            }

            // two columns: tracks (left) + albums/playlists/likes rail (right)
            RowLayout {
                Layout.fillWidth: true
                Layout.leftMargin: Style.pagePad
                Layout.rightMargin: Style.pagePad
                Layout.bottomMargin: Style.sp(20)
                Layout.alignment: Qt.AlignTop
                spacing: Style.sp(6)

                ColumnLayout {
                    Layout.fillWidth: true
                    Layout.alignment: Qt.AlignTop
                    spacing: 0
                    SectionHeading { Layout.fillWidth: true; Layout.bottomMargin: Style.sp(1); title: "Tracks" }
                    Repeater {
                        model: page.scTracks
                        delegate: Item {
                            id: trackWrap
                            required property var modelData
                            required property int index
                            Layout.fillWidth: true
                            implicitHeight: Style.rowH
                            TrackRow {
                                anchors.fill: parent
                                song: trackWrap.modelData
                                index: trackWrap.index
                                showPlays: true
                                menu: false
                                active: !!(Playback.now && trackWrap.modelData && Playback.now.videoId === trackWrap.modelData.video_id)
                                onPlay: page.playSC(trackWrap.index)
                            }
                        }
                    }
                    Text {
                        Layout.fillWidth: true
                        Layout.topMargin: Style.sp(2)
                        visible: page.scTracks.length === 0
                        text: "No public tracks."
                        color: Tokens.inkFaint
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                    }
                }

                ColumnLayout {
                    Layout.preferredWidth: Style.sp(75)     // 300
                    Layout.maximumWidth: Style.sp(75)
                    Layout.alignment: Qt.AlignTop
                    spacing: Style.sp(4)

                    ColumnLayout {
                        Layout.fillWidth: true
                        visible: page.scAlbums.length > 0
                        spacing: Style.sp(0.5)
                        SectionHeading { Layout.fillWidth: true; Layout.bottomMargin: Style.sp(1); title: "Albums from this user" }
                        Repeater {
                            model: page.scAlbums
                            delegate: ScMiniCard {
                                required property var modelData
                                card: modelData
                                onActivated: Router.push(modelData.kind || "album", { id: modelData.id, title: modelData.title })
                            }
                        }
                    }
                    ColumnLayout {
                        Layout.fillWidth: true
                        visible: page.scPlaylists.length > 0
                        spacing: Style.sp(0.5)
                        SectionHeading { Layout.fillWidth: true; Layout.bottomMargin: Style.sp(1); title: "Playlists" }
                        Repeater {
                            model: page.scPlaylists
                            delegate: ScMiniCard {
                                required property var modelData
                                card: modelData
                                onActivated: Router.push(modelData.kind || "playlist", { id: modelData.id, title: modelData.title })
                            }
                        }
                    }
                    ColumnLayout {
                        Layout.fillWidth: true
                        visible: page.scLikes.length > 0
                        spacing: Style.sp(0.5)
                        SectionHeading { Layout.fillWidth: true; Layout.bottomMargin: Style.sp(1); title: "Likes" }
                        Repeater {
                            model: page.scLikes
                            delegate: ScMiniCard {
                                required property var modelData
                                required property int index
                                card: modelData
                                playable: true
                                onActivated: page.playLikes(index)
                            }
                        }
                    }
                }
            }
        }
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
