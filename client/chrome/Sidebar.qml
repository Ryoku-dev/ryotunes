pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui as RU
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The navigation rail (spec section 3, eye-candy pass): a bordered brand card (力 tile + RYOTUNES /
// RYOKU // MUSIC eyebrow + a collapse button), the three route groups (Discover / Collection /
// System) as tracked mono micro eyebrows over hairlines, 40 px nav rows with an icon and a body
// label whose active state is a paper-lift fill plus a 2 px accent left border (hover is a faint
// tint), the Library group expanding to its five tabs while you are in it, a Playlists group with a
// full-width outlined "New playlist" button over the user's library playlists (from get_library,
// merged with any pinned playlists), and a RYOKU // MUSIC ··· LIVE register at the foot. The active
// route is read from Router.current; a click routes there. `open` collapses the rail: the layout
// slot animates to zero while a fixed-width glass body anchored to the right slides off to the left,
// so the content never reflows mid-slide. The collapse button raises `collapseRequested` (App owns
// the sidebarOpen truth), the same door the title bar's rail toggle opens.
Rectangle {
    id: root

    property bool open: true
    signal collapseRequested()

    readonly property string activePage: Router.current ? Router.current.page : "home"
    readonly property string activeTab: (Router.current && Router.current.params && Router.current.params.tab)
        ? Router.current.params.tab : "songs"

    // The user's library playlists (get_library, kind "playlist" — includes the smart On Repeat /
    // Recently Played cards and device playlists), merged with any pinned playlists from the shared
    // personal store, deduped by id. Reloaded on (re)connect and after a create.
    property var libPlaylists: []
    readonly property var playlists: {
        var seen = ({}), out = [];
        for (var i = 0; i < root.libPlaylists.length; i++) {
            var p = root.libPlaylists[i];
            if (p && p.id && !seen[p.id]) { seen[p.id] = true; out.push(p); }
        }
        var picks = Personal.picks;
        for (var j = 0; j < picks.length; j++) {
            var q = picks[j];
            if (q && q.kind === "playlist" && q.id && !seen[q.id]) { seen[q.id] = true; out.push(q); }
        }
        return out;
    }
    function loadPlaylists() {
        Daemon.call("get_library")
            .then((r) => { root.libPlaylists = (r || []).filter((i) => i && i.kind === "playlist"); })
            .catch(() => { root.libPlaylists = []; });
    }
    // create_playlist exists in crates/ryotunesd/src/methods.rs (returns the new id); make one and
    // open it so the user can name and fill it. No dialog lives in the rail.
    function newPlaylist() {
        Daemon.call("create_playlist", { title: "New playlist" })
            .then((id) => {
                root.loadPlaylists();
                Playback.toast("Playlist created", "success");
                if (id) Router.push("playlist", { id: id, title: "New playlist" });
            })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not create", "error"));
    }
    Component.onCompleted: if (Daemon.connected) root.loadPlaylists();
    Connections {
        target: Daemon
        function onSnapshot(snap): void { root.loadPlaylists(); }
        // A device-playlist mutation (a track added or removed, the cover set or reset) can change a
        // playlist's auto cover; re-read the rail so its tiles follow.
        function onEvent(name: string, data: var): void {
            if (name === "library-changed")
                root.loadPlaylists();
        }
    }

    color: "transparent"
    implicitWidth: root.open ? Style.sidebarW : 0
    clip: true
    Behavior on implicitWidth { NumberAnimation { duration: Style.motion.slow; easing.type: Easing.OutCubic } }

    // A top-level nav row: 40 px, icon + body label + optional kana seal. Active = paper-lift fill +
    // a 2 px accent left border; hover = a faint tint.
    component NavRow: Rectangle {
        id: nr
        property string label: ""
        property string icon: ""
        property string kana: ""
        property int badge: 0
        property bool current: false
        signal activated()
        activeFocusOnTab: enabled && visible
        Accessible.role: Accessible.Button
        Accessible.name: nr.label
        Accessible.onPressAction: nr.activated()
        Keys.onReturnPressed: nr.activated()
        Keys.onSpacePressed: nr.activated()
        border.width: activeFocus ? 2 : 0
        border.color: Tokens.ink

        Layout.fillWidth: true
        implicitHeight: Style.sp(10)
        radius: Style.radius
        color: nr.current ? Tokens.paperLift : navHover.hovered ? Tokens.tint5 : "transparent"
        Behavior on color { ColorAnimation { duration: Style.motion.snap } }

        // The 2 px accent left border of the active row (state, per the eye-candy brief).
        Rectangle {
            visible: nr.current
            anchors.left: parent.left
            anchors.verticalCenter: parent.verticalCenter
            width: Style.sp(0.5)
            height: parent.height - Style.sp(4)
            radius: width / 2
            color: Style.accent
        }

        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: Style.sp(3)
            anchors.rightMargin: Style.sp(3)
            spacing: Style.sp(3)
            Icon {
                name: nr.icon
                size: Style.fs.lg
                color: nr.current ? Tokens.ink : Tokens.inkMuted
            }
            Text {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                text: nr.label
                color: nr.current ? Tokens.ink : Tokens.inkDim
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
                font.weight: nr.current ? Font.Medium : Font.Normal
                elide: Text.ElideRight
            }
            Rectangle {
                visible: nr.badge > 0
                Layout.alignment: Qt.AlignVCenter
                implicitHeight: Style.sp(4.5)
                implicitWidth: Math.max(Style.sp(4.5), badgeText.implicitWidth + Style.sp(2))
                radius: height / 2
                color: Style.accentSoft
                Text {
                    id: badgeText
                    anchors.centerIn: parent
                    text: nr.badge > 99 ? "99+" : String(nr.badge)
                    color: Style.accent
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.weight: Font.DemiBold
                }
            }
            Text {
                visible: Style.decorRich && nr.kana !== ""
                text: nr.kana
                color: nr.current ? Tokens.inkMuted : Tokens.inkFaint
                opacity: nr.current ? 0.9 : 0.6
                font.family: Tokens.jp
                font.pixelSize: Style.fs.sm
            }
        }
        HoverHandler { id: navHover }
        MouseArea {
            anchors.fill: parent
            cursorShape: Qt.PointingHandCursor
            onClicked: nr.activated()
        }
    }

    // A section register: number, tracked mono micro label, a hairline to the right and (rich) seal.
    component SectionLabel: RowLayout {
        id: sl
        property string num: ""
        property string label: ""
        property string seal: ""
        Layout.fillWidth: true
        Layout.leftMargin: Style.sp(3)
        Layout.rightMargin: Style.sp(3)
        Layout.topMargin: Style.sp(3)
        Layout.bottomMargin: Style.sp(1)
        spacing: Style.sp(2)
        Text {
            text: sl.num
            color: Tokens.inkFaint
            font.family: Style.fontMono
            font.pixelSize: Style.fs.micro
            font.letterSpacing: Style.trackMicro
        }
        Text {
            text: sl.label
            color: Tokens.inkMuted
            font.family: Style.fontMono
            font.pixelSize: Style.fs.micro
            font.letterSpacing: Style.trackMicro
        }
        Hairline { Layout.fillWidth: true; soft: true }
        Text {
            visible: Style.decorRich && sl.seal !== ""
            text: sl.seal
            color: Tokens.inkFaint
            font.family: Tokens.jp
            font.pixelSize: Style.fs.sm
        }
    }

    // An indented Library tab row (32 px), shown only while the Library is the active route.
    component SubRow: Rectangle {
        id: sr
        property string label: ""
        property string tab: ""
        property bool current: false
        signal activated()

        Layout.fillWidth: true
        implicitHeight: Style.sp(8)
        radius: Style.radius
        color: subHover.hovered ? Tokens.tint5 : "transparent"

        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: Style.sp(7)
            anchors.rightMargin: Style.sp(3)
            spacing: Style.sp(2)
            Rectangle {
                Layout.alignment: Qt.AlignVCenter
                Layout.preferredWidth: Style.sp(1)
                Layout.preferredHeight: sr.current ? Style.sp(3.5) : Style.sp(1)
                radius: Style.sp(0.5)
                color: sr.current ? Tokens.ink : Tokens.lineStrong
                Behavior on Layout.preferredHeight { NumberAnimation { duration: Style.motion.snap } }
            }
            Text {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                text: sr.label
                color: sr.current ? Tokens.ink : Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                font.weight: sr.current ? Font.Medium : Font.Normal
                elide: Text.ElideRight
            }
        }
        HoverHandler { id: subHover }
        MouseArea {
            anchors.fill: parent
            cursorShape: Qt.PointingHandCursor
            onClicked: sr.activated()
        }
    }

    // A playlist row: a 40 px art tile, the title and its "N songs" sub, over a faint hover tint.
    component PlaylistRow: Rectangle {
        id: plr
        property var item: null
        signal activated()

        Layout.fillWidth: true
        implicitHeight: Style.sp(12)
        radius: Style.radius
        color: plHover.hovered ? Tokens.tint5 : "transparent"
        Behavior on color { ColorAnimation { duration: Style.motion.snap } }

        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: Style.sp(2)
            anchors.rightMargin: Style.sp(2)
            spacing: Style.sp(2)
            Artwork {
                url: (plr.item && plr.item.thumbnail) ? plr.item.thumbnail : ""
                px: Style.sp(10)
                placeholderIcon: "playlist"
            }
            ColumnLayout {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                spacing: 0
                Text {
                    Layout.fillWidth: true
                    Layout.minimumWidth: 0
                    text: plr.item ? plr.item.title : ""
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    font.weight: Font.Medium
                    elide: Text.ElideRight
                }
                Text {
                    Layout.fillWidth: true
                    Layout.minimumWidth: 0
                    text: (plr.item && plr.item.subtitle) ? plr.item.subtitle : "Playlist"
                    color: Tokens.inkMuted
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
                    elide: Text.ElideRight
                    textFormat: Text.PlainText
                }
            }
        }
        HoverHandler { id: plHover }
        MouseArea {
            anchors.fill: parent
            cursorShape: Qt.PointingHandCursor
            onClicked: plr.activated()
        }
    }

    // The frosted glass body, full width, anchored right so a shrinking slot slides it off the left.
    Rectangle {
        id: glass
        width: Style.sidebarW
        // The registration sheet behind the rail, the way the Hub's NavRail sits on one: the print
        // texture rides the chrome, never the content.
        RU.Reg { anchors.fill: parent; visible: Style.decorRich; opacity: 0.7 }
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        color: Qt.rgba(Tokens.paper.r, Tokens.paper.g, Tokens.paper.b, 0.9)

        Hairline { anchors.right: parent.right; width: 1; height: parent.height }

        ColumnLayout {
            anchors.fill: parent
            anchors.margins: Style.sp(2)
            spacing: Style.sp(1)

            // --- brand card -----------------------------------------------------------------
            Rectangle {
                Layout.fillWidth: true
                Layout.bottomMargin: Style.sp(2)
                implicitHeight: Style.sp(14)
                radius: Style.radiusCard
                color: "transparent"
                border.width: 1
                border.color: Tokens.lineSoft

                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(1)
                    spacing: Style.sp(2)

                    Rectangle {
                        Layout.preferredWidth: Style.sp(10)
                        Layout.preferredHeight: Style.sp(10)
                        radius: Style.radius
                        color: Tokens.paperLift
                        border.width: 1
                        border.color: Tokens.lineSoft
                        Text {
                            anchors.centerIn: parent
                            text: "力"
                            color: Tokens.ink
                            font.family: Tokens.jp
                            font.pixelSize: Style.fs.xl
                        }
                    }
                    ColumnLayout {
                        Layout.fillWidth: true
                        Layout.minimumWidth: 0
                        spacing: 1
                        Text {
                            Layout.fillWidth: true
                            text: "RYOTUNES"
                            color: Tokens.ink
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.md
                            font.weight: Font.DemiBold
                            font.letterSpacing: 2
                            elide: Text.ElideRight
                        }
                        Text {
                            Layout.fillWidth: true
                            text: "RYOKU // MUSIC"
                            color: Tokens.inkFaint
                            font.family: Style.fontMono
                            font.pixelSize: Style.fs.micro
                            font.letterSpacing: Style.trackMicro
                            elide: Text.ElideRight
                        }
                    }
                    IconButton {
                        icon: "rail-collapse"
                        iconSize: Style.fs.lg
                        diameter: Style.sp(8)
                        outlined: true
                        tip: "Collapse"
                        onClicked: root.collapseRequested()
                    }
                }
            }

            // --- scrolling nav body ---------------------------------------------------------
            Flickable {
                Layout.fillWidth: true
                Layout.fillHeight: true
                contentHeight: nav.implicitHeight
                clip: true
                boundsBehavior: Flickable.StopAtBounds

                ColumnLayout {
                    id: nav
                    width: parent.width
                    spacing: Style.sp(1)

                    // DISCOVER
                    SectionLabel { num: "01"; label: "DISCOVER"; seal: "聴" }
                    NavRow { label: "Home"; icon: "home"; kana: "聴"; current: root.activePage === "home"; onActivated: if (root.activePage !== "home") Router.push("home") }
                    NavRow { label: "Search"; icon: "search"; kana: "探"; current: root.activePage === "search"; onActivated: if (root.activePage !== "search") Router.push("search") }
                    NavRow { label: "Radio"; icon: "radio"; kana: "波"; current: root.activePage === "radio"; onActivated: if (root.activePage !== "radio") Router.push("radio") }

                    // COLLECTION
                    SectionLabel { num: "02"; label: "COLLECTION"; seal: "蔵" }
                    NavRow { label: "Library"; icon: "library"; kana: "蔵"; current: root.activePage === "library"; onActivated: if (root.activePage !== "library") Router.push("library", { tab: "songs" }) }
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(0.5)
                        visible: root.activePage === "library"
                        SubRow { label: "Songs"; tab: "songs"; current: root.activeTab === "songs"; onActivated: Router.replace("library", { tab: "songs" }) }
                        SubRow { label: "Albums"; tab: "albums"; current: root.activeTab === "albums"; onActivated: Router.replace("library", { tab: "albums" }) }
                        SubRow { label: "Artists"; tab: "artists"; current: root.activeTab === "artists"; onActivated: Router.replace("library", { tab: "artists" }) }
                        SubRow { label: "Playlists"; tab: "playlists"; current: root.activeTab === "playlists"; onActivated: Router.replace("library", { tab: "playlists" }) }
                        SubRow { label: "Local"; tab: "local"; current: root.activeTab === "local"; onActivated: Router.replace("library", { tab: "local" }) }
                    }
                    NavRow {
                        label: "Downloads"; icon: "download"; kana: "保"
                        badge: Downloads.activeCount + Downloads.queuedCount
                        current: root.activePage === "downloads"
                        onActivated: if (root.activePage !== "downloads") Router.push("downloads")
                    }

                    // SYSTEM
                    SectionLabel { num: "03"; label: "SYSTEM"; seal: "設" }
                    NavRow { label: "Settings"; icon: "settings"; kana: "設"; current: root.activePage === "settings"; onActivated: if (root.activePage !== "settings") Router.push("settings") }

                    // PLAYLISTS
                    SectionLabel { num: "04"; label: "PLAYLISTS"; seal: "列" }
                    Btn {
                        Layout.fillWidth: true
                        Layout.topMargin: Style.sp(1)
                        Layout.bottomMargin: Style.sp(1)
                        text: "New playlist"
                        icon: "add"
                        onClicked: root.newPlaylist()
                    }
                    Repeater {
                        model: root.playlists
                        delegate: PlaylistRow {
                            id: plRow
                            required property var modelData
                            item: plRow.modelData
                            onActivated: {
                                if (!plRow.modelData)
                                    return;
                                Router.push("playlist", { id: plRow.modelData.id, title: plRow.modelData.title });
                            }
                        }
                    }
                }
            }

            // --- LIVE register --------------------------------------------------------------
            RowLayout {
                Layout.fillWidth: true
                Layout.topMargin: Style.sp(1)
                spacing: Style.sp(2)
                Text {
                    text: "RYOKU // MUSIC"
                    color: Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
                }
                Hairline { Layout.fillWidth: true; soft: true }
                Rectangle {
                    Layout.alignment: Qt.AlignVCenter
                    Layout.preferredWidth: Style.sp(2)
                    Layout.preferredHeight: Style.sp(2)
                    radius: Style.sp(1)
                    color: Style.ambient ? Style.accent : Tokens.inkFaint
                }
                Text {
                    text: "LIVE"
                    color: Style.ambient ? Tokens.inkDim : Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
                }
            }
        }
    }
}
