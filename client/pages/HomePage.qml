pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"
import "../chrome"

// Home (spec section 5), ported from ui/src/routes/+page.svelte and reset onto the visual system.
// One vertical reused ListView of the feed's shelves; the header carries the greeting hero (with
// the listening deck at its right on a wide page), the mood-chip rail, the Pinned tile grid and the
// personal shelves — Listen again, Familiar artists and Forgotten favourites. The footer carries the
// loading skeletons, the empty / error states and the progressive get_home_more pagination
// (it fires when the tail comes within 400 px of the viewport bottom). The page fills the content
// rect the frame already pads (32 sides / 24 top / 32 bottom); it never insets itself.
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

    // Personal shelves, live off the shared store.
    readonly property var recents: Personal.recent(12)
    readonly property var forgottenList: page.forgottenSongs()

    // Familiar artists: the most-played artists (topArtistIds) resolved to round cards. Loaded once.
    property var famIds: Personal.topArtistIds(6)
    property var famCards: []
    property bool famLoaded: false
    onFamIdsChanged: page.loadFamiliar()

    Component.onCompleted: { page.load(""); page.loadFamiliar(); }

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
        // Our own Listen again (recents) sits above the feed; drop the feed's duplicate of it.
        var haveRecents = page.recents.length > 0;
        for (var i = 0; i < secs.length; i++) {
            if (page.isForgotten(secs[i])) {
                if (!fg)
                    fg = secs[i];
            } else if (haveRecents && /listen again/i.test(secs[i].title)) {
                // covered by the personal Listen again shelf
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

    function loadFamiliar() {
        if (page.famLoaded || page.famIds.length < 3)
            return;
        page.famLoaded = true;
        Promise.all(page.famIds.map((id) => Daemon.call("get_artist", { id: id }).catch(() => null)))
            .then((pages) => {
                page.famCards = pages.filter((p) => !!p).map((p) => ({
                    kind: "artist",
                    id: p.channelId,
                    title: p.name ? p.name : "Artist",
                    subtitle: p.monthlyListeners ? p.monthlyListeners : (p.subscribers ? p.subscribers : ""),
                    thumbnail: p.thumbnail
                }));
            });
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
                list.stickTop = true;
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
        cacheBuffer: Math.max(0, Math.round(height * 1.5))
        boundsBehavior: Flickable.StopAtBounds
        model: page.blocks
        spacing: Style.sp(9)

        // The header (hero + chips + pinned + personal shelves) is taller than the viewport and
        // grows as the recents / familiar / feed shelves resolve. While the user has not scrolled,
        // keep it pinned to the very top so a late-arriving shelf never nudges the greeting off the
        // edge; the first drag or wheel releases the pin. A chip switch re-arms it (load()).
        property bool stickTop: true
        onMovementStarted: list.stickTop = false
        Binding { target: list; property: "contentY"; value: list.originY; when: list.stickTop }
        onContentYChanged: page.maybeLoadMore()
        onContentHeightChanged: page.maybeLoadMore()

        delegate: Item {
            required property var modelData
            width: list.width
            implicitHeight: shelf.implicitHeight
            Shelf {
                id: shelf
                width: parent.width
                section: parent.modelData
                mark: Style.decorRich ? "章" : ""
            }
        }

        header: Item {
            width: list.width
            implicitHeight: headerCol.implicitHeight + Style.sp(9)

            ColumnLayout {
                id: headerCol
                width: parent.width
                spacing: Style.sp(6)

                // hero: greeting, search and key hints on the left; the listening deck on the right
                // when the page is wide enough (>= 1240 px, i.e. the panel closed or a wider window).
                GridLayout {
                    id: hero
                    Layout.fillWidth: true
                    readonly property bool wide: width >= Style.sp(310)
                    columns: wide ? 2 : 1
                    columnSpacing: Style.sp(10)
                    rowSpacing: Style.sp(5)

                    ColumnLayout {
                        Layout.fillWidth: true
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
                                font.pixelSize: Style.fs.micro
                                font.letterSpacing: Style.trackMicro
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
                            font.pixelSize: Style.fs.sm
                            wrapMode: Text.WordWrap
                        }
                        SearchSuggest {
                            id: heroSearch
                            Layout.preferredWidth: Style.sp(80)
                            Layout.maximumWidth: Style.sp(80)
                            Layout.topMargin: Style.sp(2)
                            placeholder: "Search tracks, albums, artists…"
                            onSubmitted: if (value.trim() !== "") Router.push("search", { q: value.trim() })
                            onPicked: if (value.trim() !== "") Router.push("search", { q: value.trim() })
                            z: 40
                        }
                        RowLayout {
                            Layout.topMargin: Style.sp(1)
                            spacing: Style.sp(2)
                            Text { text: "CTRL K"; color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
                            Text { text: "command search"; color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.xs }
                            Rectangle { Layout.preferredWidth: Style.sp(4); Layout.preferredHeight: 1; Layout.alignment: Qt.AlignVCenter; color: Tokens.lineSoft }
                            Text { text: "SPACE"; color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
                            Text { text: "play / pause"; color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.xs }
                        }
                    }

                    MusicDeck {
                        visible: hero.wide
                        Layout.preferredWidth: Style.sp(140)
                        Layout.preferredHeight: Style.sp(40)
                        Layout.alignment: Qt.AlignVCenter | Qt.AlignRight
                        onOpenNowPlaying: (tab) => Playback.nowPlayingRequested(tab)
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

                // pinned (unfiltered only)
                Shortcuts {
                    Layout.fillWidth: true
                    visible: page.selected === ""
                    picks: Personal.picks
                    onRemoved: (id) => Personal.removePick(id)
                }

                // listen again (recents, unfiltered only)
                Shelf {
                    Layout.fillWidth: true
                    visible: page.selected === "" && page.recents.length > 0
                    title: "Listen again"
                    mark: Style.decorRich ? "再" : ""
                    items: page.recents
                }

                // familiar artists (round, unfiltered only)
                Shelf {
                    Layout.fillWidth: true
                    visible: page.selected === "" && page.famCards.length >= 3
                    title: "Familiar artists"
                    mark: Style.decorRich ? "馴" : ""
                    items: page.famCards
                }

                // forgotten favourites (from the feed)
                Shelf {
                    Layout.fillWidth: true
                    visible: !!page.forgotten && page.forgottenList.length > 0
                    title: page.forgotten ? page.forgotten.title : "Forgotten favourites"
                    mark: Style.decorRich ? "忘" : ""
                    items: page.forgottenList
                }
            }
        }

        footer: Item {
            width: list.width
            implicitHeight: footerCol.implicitHeight + Style.sp(20)

            ColumnLayout {
                id: footerCol
                width: parent.width
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
                            spacing: Style.sp(4)
                            Repeater {
                                model: 6
                                delegate: Skeleton {
                                    required property int index
                                    Layout.preferredWidth: Style.cardW
                                    Layout.preferredHeight: Style.cardW
                                    corner: Style.radiusCard
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
