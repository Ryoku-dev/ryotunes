pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The top register (spec section 3): the rail toggle and history on the left, then a 280 px search
// field that routes to the search page, and the right cluster — account, Listen Together, Discord,
// the panel toggle, the Sound dialog and the mini window. Under Hyprland the compositor owns the
// window controls, so this bar carries no minimise/close. Discord is a plain setting toggle wired
// here; the rest raise signals the App owns.
Rectangle {
    id: root

    signal sidebarToggleClicked()
    signal panelToggleClicked()
    signal soundClicked()
    signal accountClicked(real gx, real gy)
    signal listenTogetherClicked()
    signal miniClicked()

    property bool sidebarOpen: true
    property bool panelOpen: true
    property bool discordOn: false

    implicitHeight: Style.titleBarH
    color: Tokens.paper

    Component.onCompleted: root.discordOn = !!(Playback.settings && Playback.settings.discord_rpc === "true")
    Connections {
        target: Playback
        function onSettingsChanged() {
            root.discordOn = !!(Playback.settings && Playback.settings.discord_rpc === "true");
        }
    }

    Hairline { anchors.bottom: parent.bottom; width: parent.width; height: 1 }

    RowLayout {
        anchors.fill: parent
        anchors.leftMargin: Style.sp(2)
        anchors.rightMargin: Style.sp(2)
        spacing: Style.sp(1)

        // --- rail toggle + history ----------------------------------------------------------
        IconButton {
            icon: "sidebar"
            iconSize: Style.fs.lg
            diameter: Style.sp(8)
            active: root.sidebarOpen
            onClicked: root.sidebarToggleClicked()
        }

        Rectangle { Layout.preferredWidth: 1; Layout.preferredHeight: Style.sp(4); Layout.alignment: Qt.AlignVCenter; color: Tokens.line }

        IconButton {
            icon: "arrow-left"
            iconSize: Style.fs.lg
            diameter: Style.sp(8)
            enabled: Router.canGoBack
            onClicked: Router.pop()
        }
        IconButton {
            icon: "arrow-right"
            iconSize: Style.fs.lg
            diameter: Style.sp(8)
            enabled: false
        }

        Item { Layout.fillWidth: true }

        // --- provider switch (YouTube Music | Spotify) --------------------------------------
        Rectangle {
            id: providerPill
            Layout.preferredHeight: Style.sp(8)
            Layout.alignment: Qt.AlignVCenter
            implicitWidth: providerRow.implicitWidth + Style.sp(1.5)
            radius: Style.radius
            color: Tokens.paperLift
            border.width: 1
            border.color: Tokens.line

            RowLayout {
                id: providerRow
                anchors.fill: parent
                anchors.margins: Style.sp(0.75)
                spacing: Style.sp(0.75)
                Repeater {
                    model: [
                        { key: "youtube", icon: "youtube-music" },
                        { key: "spotify", icon: "spotify" }
                    ]
                    delegate: Rectangle {
                        id: prov
                        required property var modelData
                        readonly property bool selected: Playback.provider === prov.modelData.key
                        Layout.preferredWidth: Style.sp(7)
                        Layout.fillHeight: true
                        radius: Style.radius - 1
                        // The selected provider wears its own colour as a soft plate, the one
                        // place the brand colours appear as chrome; the glyph stays ink.
                        readonly property color brand: prov.modelData.key === "spotify" ? "#1db954" : "#ff2d2d"
                        color: prov.selected ? Qt.rgba(prov.brand.r, prov.brand.g, prov.brand.b, 0.22)
                            : provHover.hovered ? Tokens.tint5 : "transparent"
                        border.width: prov.selected ? 1 : 0
                        border.color: Qt.rgba(prov.brand.r, prov.brand.g, prov.brand.b, 0.55)
                        Behavior on color { ColorAnimation { duration: Style.motion.snap } }
                        Icon {
                            anchors.centerIn: parent
                            name: prov.modelData.icon
                            size: Style.fs.lg
                            color: prov.selected ? prov.brand : Tokens.inkMuted
                        }
                        HoverHandler { id: provHover }
                        MouseArea {
                            anchors.fill: parent
                            cursorShape: Qt.PointingHandCursor
                            onClicked: if (!prov.selected) Playback.setProvider(prov.modelData.key)
                        }
                    }
                }
            }
        }

        // --- search field (routes to the search page) ---------------------------------------
        Rectangle {
            id: searchField
            Layout.preferredWidth: Style.sp(70)
            Layout.preferredHeight: Style.sp(8)
            Layout.alignment: Qt.AlignVCenter
            radius: Style.radius
            color: Tokens.paperLift
            border.width: 1
            border.color: searchInput.activeFocus ? Tokens.lineStrong : Tokens.line
            Behavior on border.color { ColorAnimation { duration: Style.motion.snap } }

            function submit() {
                var q = searchInput.text.trim();
                if (q === "")
                    return;
                Router.push("search", { q: q });
                searchInput.text = "";
                searchInput.focus = false;
            }

            RowLayout {
                anchors.fill: parent
                anchors.leftMargin: Style.sp(2.5)
                anchors.rightMargin: Style.sp(1.5)
                spacing: Style.sp(2)
                Icon { name: "search"; size: Style.fs.sm; color: Tokens.inkMuted }
                TextInput {
                    id: searchInput
                    Layout.fillWidth: true
                    verticalAlignment: TextInput.AlignVCenter
                    clip: true
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    onAccepted: searchField.submit()
                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        visible: searchInput.text.length === 0
                        text: "Search…"
                        color: Tokens.inkFaint
                        font: searchInput.font
                    }
                }
                IconButton {
                    visible: searchInput.text.length > 0
                    icon: "close"
                    iconSize: Style.fs.sm
                    diameter: Style.sp(6)
                    onClicked: searchInput.text = ""
                }
            }
        }

        // --- account ------------------------------------------------------------------------
        Item {
            id: account
            Layout.alignment: Qt.AlignVCenter
            implicitWidth: Style.sp(8)
            implicitHeight: Style.sp(8)

            Artwork {
                anchors.centerIn: parent
                visible: !!(Playback.auth && Playback.auth.signedIn && Playback.auth.avatar)
                url: (Playback.auth && Playback.auth.avatar) ? Playback.auth.avatar : ""
                px: Style.sp(6)
                round: true
                placeholderIcon: "account"
            }
            Icon {
                anchors.centerIn: parent
                visible: !(Playback.auth && Playback.auth.signedIn && Playback.auth.avatar)
                name: "account"
                size: Style.fs.lg
                color: (Playback.auth && Playback.auth.signedIn) ? Tokens.ink : Tokens.inkMuted
            }
            HoverHandler { id: accountHover }
            Rectangle {
                anchors.fill: parent
                radius: Style.radius
                z: -1
                color: accountHover.hovered ? Tokens.tint5 : "transparent"
            }
            MouseArea {
                anchors.fill: parent
                cursorShape: Qt.PointingHandCursor
                onClicked: {
                    var p = account.mapToItem(null, 0, account.height);
                    root.accountClicked(p.x, p.y);
                }
            }
        }

        Rectangle { Layout.preferredWidth: 1; Layout.preferredHeight: Style.sp(4); Layout.alignment: Qt.AlignVCenter; color: Tokens.line }

        IconButton {
            icon: "group"
            iconSize: Style.fs.lg
            diameter: Style.sp(8)
            active: !!(Playback.lt && Playback.lt.role && Playback.lt.role !== "none")
            onClicked: root.listenTogetherClicked()
        }
        IconButton {
            icon: "discord"
            iconSize: Style.fs.lg
            diameter: Style.sp(8)
            active: root.discordOn
            onClicked: {
                root.discordOn = !root.discordOn;
                Daemon.call("set_setting", { key: "discord_rpc", value: root.discordOn ? "true" : "false" })
                    .catch(() => { root.discordOn = !root.discordOn; });
            }
        }

        Rectangle { Layout.preferredWidth: 1; Layout.preferredHeight: Style.sp(4); Layout.alignment: Qt.AlignVCenter; color: Tokens.line }

        IconButton {
            icon: "panel"
            iconSize: Style.fs.lg
            diameter: Style.sp(8)
            active: root.panelOpen
            onClicked: root.panelToggleClicked()
        }
        IconButton {
            icon: "sound"
            iconSize: Style.fs.lg
            diameter: Style.sp(8)
            onClicked: root.soundClicked()
        }
        IconButton {
            icon: "minimize"
            iconSize: Style.fs.lg
            diameter: Style.sp(8)
            onClicked: root.miniClicked()
        }
    }
}
