pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import QtQuick.Effects
import Ryoku.Ui.Singletons
import Ryoku.Ui as RU
import "chrome"
import "components"

// The app frame (spec section 3): the title register on top, the collapsible navigation rail and the
// routed page over the cover bloom, the persistent Queue/Lyrics panel on the right, and the transport
// pinned to the foot. Above the frame sits a foreground layer for the account menu, the toasts, the
// Ctrl+K palette, the Sound dialog and the Listen Together sheet. The page is chosen from
// Router.current.page and loaded by URL; any unknown route lands on a placeholder until its task adds
// it. The single Backdrop behind the content is the only saturated thing on screen; every chrome
// surface is Tokens paper (opaque or at alpha) so nothing else competes with the artwork.
Item {
    id: app
    anchors.fill: parent

    // The Now Playing stage's coupling with the transport. Exactly one of queue/lyrics is the active
    // tab while the stage is open; opening a tab opens it and closing clears all three. NowPlaying
    // reads these and asks for changes through tabRequested/closeRequested.
    property bool queueOpen: false
    property bool lyricsOpen: false
    property bool nowPlayingOpen: false

    // The persistent right panel (Queue | Lyrics) and its active tab. Open by default on a wide
    // window (>= 1400 px); hidden entirely below 1100 px, and yielded to the Now Playing stage.
    property bool panelOpen: app.width >= Style.sp(350)
    property string panelTab: "queue"
    readonly property bool panelShown: app.panelOpen && app.width >= Style.sp(275) && !app.nowPlayingOpen

    // The navigation rail's collapse state, toggled from the title bar.
    property bool sidebarOpen: true

    // The Sound dialog (tempo / pitch / reverb / bass / width). A modal Loader that only exists while
    // open; SoundWorker lands chrome/SoundDialog.qml and Playback.soundRequested.
    property bool soundOpen: false

    // The mini player window's visibility. shell.qml's mini PanelWindow binds its visible to this; the
    // title-bar and player-bar buttons toggle it, MiniPlayer clears it.
    property bool miniOpen: false

    // Any full-window modal (Sound dialog, Ctrl+K palette, Listen Together): the frame behind it is
    // snapshot-blurred while one of these is open (spec 9).
    readonly property bool modalOpen: app.soundOpen || palette.open || listenTogether.open

    // --- Now Playing stage helpers ----------------------------------------------------------
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

    // --- right panel helpers ----------------------------------------------------------------
    function panelOpenTab(tab) {
        app.panelTab = tab;
        app.panelOpen = true;
    }
    function panelToggleTab(tab) {
        if (app.panelShown && app.panelTab === tab)
            app.panelOpen = false;
        else
            app.panelOpenTab(tab);
    }

    // Rig hook: type into the current page's hero search (Home) without a keyboard.
    function devSuggest(q) {
        var pg = pageLoader.item;
        var field = (pg && "heroSearch" in pg) ? pg.heroSearch : null;
        if (field) { field.forceFocus(); field.value = q; }
    }

    // One accent sampler per window (Canvas paints only inside a rendering window).
    ArtAccent {}

    Connections {
        target: Playback
        function onNowPlayingRequested(tab: string): void { app.npOpenTab(tab); }
        function onSoundRequested(): void { app.soundOpen = true; }
    }

    // ── frame ────────────────────────────────────────────────────────────────────────────
    ColumnLayout {
        id: frame
        anchors.fill: parent
        spacing: 0

        // Spec-9 snapshot blur: a modal captures the whole frame behind it once (layer.live: false)
        // and blurs that frozen texture, then draws its scrim + card sharp on top. No live subtree is
        // ever blurred per frame; the overlays (Toast, SoundDialog, palette, menu) sit outside
        // this layer, so they stay crisp.
        layer.enabled: app.modalOpen
        layer.live: false
        layer.effect: MultiEffect {
            blurEnabled: true
            blur: 1.0
            blurMax: 32
        }

        TitleBar {
            Layout.fillWidth: true
            sidebarOpen: app.sidebarOpen
            panelOpen: app.panelShown
            onSidebarToggleClicked: app.sidebarOpen = !app.sidebarOpen
            onPanelToggleClicked: app.panelOpen = !app.panelOpen
            onSoundClicked: app.soundOpen = true
            onAccountClicked: (gx, gy) => accountMenu.openAt(gx, gy)
            onListenTogetherClicked: listenTogether.open = !listenTogether.open
            onMiniClicked: app.miniOpen = !app.miniOpen
        }

        RowLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: 0

            Sidebar {
                Layout.fillHeight: true
                open: app.sidebarOpen
                onCollapseRequested: app.sidebarOpen = false
            }

            // The content area: the single cover Backdrop, a paper glass over it so the bloom reads
            // as a subtle wash, and the routed page inset by the shared padding. The Now Playing stage
            // rises over all of it (right of the rail, above the transport) when open.
            Item {
                id: contentArea
                Layout.fillWidth: true
                Layout.fillHeight: true
                clip: true

                Backdrop {
                    anchors.fill: parent
                    strength: Style.paperDark ? 0.22 : 0.14
                    focusX: 0.82
                    focusY: 0.12
                }
                Rectangle {
                    anchors.fill: parent
                    color: Qt.rgba(Tokens.paper.r, Tokens.paper.g, Tokens.paper.b, 0.88)
                }
                // The provider's light over the glass: red for YouTube Music, green for Spotify.
                ProviderGlow { anchors.fill: parent; focusX: 0.82; focusY: 0.1 }

                Item {
                    id: pageClip
                    anchors.fill: parent
                    anchors.leftMargin: Style.pagePad
                    anchors.rightMargin: Style.pagePad
                    anchors.topMargin: Style.sp(6)
                    anchors.bottomMargin: Style.pagePad
                    clip: true

                    // The routed page. On each route change it crossfades in and rises 8 px, on the
                    // render thread (Animators), so the swap costs no main-thread work. The page is
                    // loaded by URL from its type name, so a new page lights up the moment its file
                    // lands; unknown routes fall through to the placeholder.
                    Item {
                        id: pageStack
                        width: parent.width
                        height: parent.height
                        readonly property string page: Router.current ? Router.current.page : "home"

                        Loader {
                            id: pageLoader
                            anchors.fill: parent
                            source: {
                                var m = { home: "HomePage", search: "SearchPage", library: "LibraryPage",
                                    playlist: "PlaylistPage", album: "AlbumPage", artist: "ArtistPage", list: "ListPage",
                                    radio: "RadioPage", settings: "SettingsPage" };
                                return m[pageStack.page] ? Qt.resolvedUrl("pages/" + m[pageStack.page] + ".qml") : "";
                            }
                        }
                        Loader {
                            anchors.fill: parent
                            active: pageLoader.status !== Loader.Ready
                            sourceComponent: placeholder
                        }

                        ParallelAnimation {
                            id: enter
                            OpacityAnimator { target: pageStack; from: 0; to: 1; duration: Tokens.durFastEffects; easing.type: Easing.OutCubic }
                            YAnimator { target: pageStack; from: Style.sp(2); to: 0; duration: Tokens.durFastEffects; easing.type: Easing.OutCubic }
                        }
                        onPageChanged: enter.restart()
                        Component.onCompleted: enter.start()
                        // A provider switch swaps the catalogue under every page (a YouTube
                        // library is not a Spotify one), so the content re-enters the way a
                        // route change does while the room's light tweens to the new colour.
                        Connections {
                            target: Playback
                            function onProviderChanged(): void { enter.restart(); }
                        }
                    }
                }

                NowPlaying {
                    anchors.fill: parent
                    nowPlayingOpen: app.nowPlayingOpen
                    queueOpen: app.queueOpen
                    lyricsOpen: app.lyricsOpen
                    onTabRequested: (tab) => app.npOpenTab(tab)
                    onCloseRequested: app.npClose()
                }
            }

            RightPanel {
                Layout.fillHeight: true
                open: app.panelShown
                tab: app.panelTab
                onTabRequested: (tab) => app.panelOpenTab(tab)
                onCloseRequested: app.panelOpen = false
            }
        }

        PlayerBar {
            Layout.fillWidth: true
            visible: !!Playback.now
            panelOpen: app.panelShown
            panelTab: app.panelTab
            nowPlayingOpen: app.nowPlayingOpen
            onToggleQueue: app.panelToggleTab("queue")
            onToggleLyrics: app.panelToggleTab("lyrics")
            onToggleNowPlaying: app.npToggle()
            onSoundClicked: app.soundOpen = true
        }
    }

    Component {
        id: placeholder
        Item {
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

    // The Sound dialog: a self-contained modal (its own spec-9 snapshot blur) that only exists while
    // open. The file may not exist yet — a string source only errors when the Loader activates, so
    // this stays inert until SoundWorker lands it; the only wiring is its close request.
    Loader {
        id: soundLoader
        anchors.fill: parent
        active: app.soundOpen
        source: "chrome/SoundDialog.qml"
    }
    Connections {
        target: soundLoader.item
        ignoreUnknownSignals: true
        function onCloseRequested(): void { app.soundOpen = false; }
    }

    // The Ctrl+K command palette and its global shortcut. On top of everything, so it can't be clipped
    // and covers the whole frame while open.
    CommandPalette { id: palette }
    Shortcut {
        sequences: ["Ctrl+K"]
        context: Qt.WindowShortcut
        onActivated: palette.open = !palette.open
    }
    // Escape walks the overlays back: palette, then the Now Playing stage. Keyboard actions get
    // no animation of their own; the stage's own fade is all.
    Shortcut {
        sequences: ["Escape"]
        context: Qt.WindowShortcut
        enabled: palette.open || app.nowPlayingOpen
        onActivated: {
            if (palette.open) palette.open = false;
            else app.npClose();
        }
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

            // ── YouTube Music ────────────────────────────────────────────────────────────
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
                    name: "youtube-music"
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

            Rectangle {
                id: ytAction
                Layout.fillWidth: true
                implicitHeight: Style.sp(9)
                radius: Style.radius
                color: ytHover.hovered ? Tokens.tint5 : "transparent"
                readonly property bool signedIn: !!(Playback.auth && Playback.auth.signedIn)
                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(2)
                    spacing: Style.sp(2)
                    Icon {
                        name: ytAction.signedIn ? "close" : "account"
                        size: Style.fs.md
                        color: ytAction.signedIn ? Style.alert : Tokens.ink
                    }
                    Text {
                        Layout.fillWidth: true
                        text: ytAction.signedIn ? "Sign out" : "Sign in with Google"
                        color: ytAction.signedIn ? Style.alert : Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                    }
                }
                HoverHandler { id: ytHover }
                MouseArea {
                    anchors.fill: parent
                    cursorShape: Qt.PointingHandCursor
                    onClicked: {
                        accountMenu.visible = false;
                        Daemon.call(ytAction.signedIn ? "sign_out" : "sign_in").catch(() => {});
                    }
                }
            }

            Hairline { Layout.fillWidth: true }

            // ── Spotify ──────────────────────────────────────────────────────────────────
            RowLayout {
                Layout.fillWidth: true
                Layout.margins: Style.sp(1)
                spacing: Style.sp(2)
                Icon {
                    name: "spotify"
                    size: Style.fs.lg
                    color: (Playback.spotify && Playback.spotify.signedIn) ? Style.providerColor : Tokens.inkMuted
                }
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 0
                    Text {
                        text: (Playback.spotify && Playback.spotify.signedIn && Playback.spotify.name)
                            ? Playback.spotify.name : "Not signed in"
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        font.weight: Font.Medium
                        elide: Text.ElideRight
                        Layout.fillWidth: true
                    }
                    Text {
                        text: (Playback.spotify && Playback.spotify.signedIn) ? "Spotify" : "Premium required"
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        elide: Text.ElideRight
                        Layout.fillWidth: true
                    }
                }
            }

            Rectangle {
                id: spAction
                Layout.fillWidth: true
                implicitHeight: Style.sp(9)
                radius: Style.radius
                color: spHover.hovered ? Tokens.tint5 : "transparent"
                readonly property bool signedIn: !!(Playback.spotify && Playback.spotify.signedIn)
                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(2)
                    spacing: Style.sp(2)
                    Icon {
                        name: spAction.signedIn ? "close" : "spotify"
                        size: Style.fs.md
                        color: spAction.signedIn ? Style.alert : Tokens.ink
                    }
                    Text {
                        Layout.fillWidth: true
                        text: spAction.signedIn ? "Sign out" : "Sign in to Spotify"
                        color: spAction.signedIn ? Style.alert : Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                    }
                }
                HoverHandler { id: spHover }
                MouseArea {
                    anchors.fill: parent
                    cursorShape: Qt.PointingHandCursor
                    onClicked: {
                        accountMenu.visible = false;
                        if (spAction.signedIn) Playback.spotifySignOut().catch(() => {});
                        else Playback.spotifySignIn().catch(() => {});
                    }
                }
            }
        }
    }

    // Listen Together session sheet: a foreground overlay toggled from the title bar. It anchors the
    // whole frame and is visible only while open.
    ListenTogether { id: listenTogether }

}
