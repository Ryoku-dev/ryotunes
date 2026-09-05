pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "chrome"
import "components"

// The app frame: the title register on top, the navigation rail beside the routed page, the
// transport pinned to the foot while something is loaded, and a foreground layer for the account
// menu, the toasts and the Ctrl+K palette. The page is chosen from Router.current.page and loaded
// by URL; Radio/Settings and any unknown route land on a placeholder until their task adds them.
Item {
    id: app
    anchors.fill: parent

    // Surface toggles the transport raises. Their panels/windows are later tasks; the state is real
    // now so those surfaces bind to it when they arrive, and the toggle buttons already reflect it.
    property bool queueOpen: false
    property bool lyricsOpen: false
    property bool nowPlayingOpen: false

    // The mini player window's visibility. shell.qml's "Ryotunes Mini" FloatingWindow binds its
    // visible to this; the title-bar and player-bar mini buttons toggle it, MiniPlayer clears it.
    property bool miniOpen: false

    // The persistent right panel (Queue | Lyrics). Open by default on a wide window.
    property bool panelOpen: width >= Style.sp(350)

    // The Now Playing overlay's coupling with the player bar. Exactly one of queue/lyrics is the
    // active tab while the overlay is open; opening a tab opens the overlay, and closing it clears
    // all three. NowPlaying reads these and asks for changes through its tabRequested/closeRequested.
    function npOpenTab(tab) {
        app.nowPlayingOpen = true;
        app.queueOpen = tab === "queue";
        app.lyricsOpen = tab === "lyrics";
    }
    function npClose() {
        app.nowPlayingOpen = false;
        app.queueOpen = false;
        app.lyricsOpen = false;
    }
    function npToggleTab(tab) {
        if (app.nowPlayingOpen && ((tab === "queue" && app.queueOpen) || (tab === "lyrics" && app.lyricsOpen)))
            app.npClose();
        else
            app.npOpenTab(tab);
    }
    function npToggle() {
        if (app.nowPlayingOpen)
            app.npClose();
        else
            app.npOpenTab(app.lyricsOpen ? "lyrics" : "queue");
    }

    // One accent sampler per window (Canvas paints only inside a rendering window).
    ArtAccent {}

    Connections {
        target: Playback
        function onNowPlayingRequested(tab: string): void { app.npOpenTab(tab); }
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 0

        TitleBar {
            Layout.fillWidth: true
            onAccountClicked: (gx, gy) => accountMenu.openAt(gx, gy)
            onListenTogetherClicked: listenTogether.open = !listenTogether.open
            onMiniClicked: app.miniOpen = !app.miniOpen
        }

        RowLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: 0

            Sidebar { Layout.fillHeight: true }

            Item {
                Layout.fillWidth: true
                Layout.fillHeight: true

                // The routed page is loaded by URL from its type name, so a new page lights up the
                // moment its file lands — no per-route wiring here. Radio/Settings and any unknown
                // route fall through to the placeholder until their task adds them.
                Loader {
                    id: pageLoader
                    anchors.fill: parent
                    readonly property string page: Router.current ? Router.current.page : "home"
                    source: {
                        var m = { home: "HomePage", search: "SearchPage", library: "LibraryPage",
                            playlist: "PlaylistPage", album: "AlbumPage", artist: "ArtistPage", list: "ListPage",
                            radio: "RadioPage", settings: "SettingsPage" };
                        return m[page] ? Qt.resolvedUrl("pages/" + m[page] + ".qml") : "";
                    }
                }
                Loader {
                    anchors.fill: parent
                    active: pageLoader.status !== Loader.Ready
                    sourceComponent: placeholder
                }

                // The Now Playing surface (artwork + Queue/Lyrics tabs) rises over the routed page,
                // right of the rail and above the transport, exactly as the Svelte overlay does.
                NowPlaying {
                    anchors.fill: parent
                    nowPlayingOpen: app.nowPlayingOpen
                    queueOpen: app.queueOpen
                    lyricsOpen: app.lyricsOpen
                    onTabRequested: (tab) => app.npOpenTab(tab)
                    onCloseRequested: app.npClose()
                }
            }
        }

        PlayerBar {
            Layout.fillWidth: true
            visible: !!Playback.now
            queueOpen: app.queueOpen
            lyricsOpen: app.lyricsOpen
            nowPlayingOpen: app.nowPlayingOpen
            onToggleQueue: app.npToggleTab("queue")
            onToggleLyrics: app.npToggleTab("lyrics")
            onToggleNowPlaying: app.npToggle()
            onMiniClicked: app.miniOpen = !app.miniOpen
        }
    }

    Component {
        id: placeholder
        Rectangle {
            color: Tokens.paper
            ColumnLayout {
                anchors.centerIn: parent
                spacing: Style.sp(2)
                Icon {
                    Layout.alignment: Qt.AlignHCenter
                    name: "music"
                    size: Style.fs.hero
                    color: Tokens.inkFaint
                }
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    text: (Router.current ? Router.current.page : "").toUpperCase()
                    color: Tokens.inkDim
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.sm
                    font.letterSpacing: 2
                }
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    text: "This surface arrives in a later build."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.md
                }
            }
        }
    }

    // ── foreground layer ────────────────────────────────────────────────────────────────
    Toast { }

    // The Ctrl+K command palette and its global shortcut. On top of everything, so it can't be
    // clipped and covers the whole frame while open.
    CommandPalette { id: palette }
    Shortcut {
        sequences: ["Ctrl+K"]
        context: Qt.WindowShortcut
        onActivated: palette.open = !palette.open
    }

    // Account menu (sign in / out via the daemon). A full-surface dismiss layer closes it.
    MouseArea {
        anchors.fill: parent
        visible: accountMenu.visible
        onClicked: accountMenu.visible = false
    }
    Rectangle {
        id: accountMenu
        visible: false
        width: Style.sp(56)
        implicitHeight: menuCol.implicitHeight + Style.sp(2)
        height: implicitHeight
        radius: Style.radius
        color: Tokens.paperLift
        border.width: 1
        border.color: Tokens.lineStrong

        function openAt(gx, gy) {
            x = Math.max(Style.sp(2), Math.min(gx - width, app.width - width - Style.sp(2)));
            y = gy + Style.sp(1);
            visible = true;
        }

        ColumnLayout {
            id: menuCol
            anchors.fill: parent
            anchors.margins: Style.sp(1)
            spacing: Style.sp(1)

            RowLayout {
                Layout.fillWidth: true
                Layout.margins: Style.sp(1)
                spacing: Style.sp(2)
                Artwork {
                    visible: !!(Playback.auth && Playback.auth.signedIn && Playback.auth.avatar)
                    url: (Playback.auth && Playback.auth.avatar) ? Playback.auth.avatar : ""
                    px: Style.sp(8)
                    round: true
                    placeholderIcon: "account"
                }
                Icon {
                    visible: !(Playback.auth && Playback.auth.signedIn && Playback.auth.avatar)
                    name: "account"
                    size: Style.fs.lg
                    color: Tokens.inkMuted
                }
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 0
                    Text {
                        text: (Playback.auth && Playback.auth.signedIn && Playback.auth.name)
                            ? Playback.auth.name : "Not signed in"
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        font.weight: Font.Medium
                        elide: Text.ElideRight
                        Layout.fillWidth: true
                    }
                    Text {
                        text: (Playback.auth && Playback.auth.signedIn) ? "YouTube Music" : "Sign in to sync your library"
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        elide: Text.ElideRight
                        Layout.fillWidth: true
                    }
                }
            }

            Hairline { Layout.fillWidth: true }

            // action row
            Rectangle {
                id: signAction
                Layout.fillWidth: true
                implicitHeight: Style.sp(9)
                radius: Style.radius
                color: actHover.hovered ? Tokens.tint5 : "transparent"
                readonly property bool signedIn: !!(Playback.auth && Playback.auth.signedIn)
                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(2)
                    spacing: Style.sp(2)
                    Icon {
                        name: signAction.signedIn ? "close" : "account"
                        size: Style.fs.md
                        color: signAction.signedIn ? Tokens.alert : Tokens.ink
                    }
                    Text {
                        Layout.fillWidth: true
                        text: signAction.signedIn ? "Sign out" : "Sign in with Google"
                        color: signAction.signedIn ? Tokens.alert : Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                    }
                }
                HoverHandler { id: actHover }
                MouseArea {
                    anchors.fill: parent
                    cursorShape: Qt.PointingHandCursor
                    onClicked: {
                        accountMenu.visible = false;
                        Daemon.call(signAction.signedIn ? "sign_out" : "sign_in").catch(() => {});
                    }
                }
            }
        }
    }

    // Listen Together session sheet: a foreground overlay toggled from the title bar. It anchors the
    // whole frame and is visible only while open.
    ListenTogether { id: listenTogether }
}
