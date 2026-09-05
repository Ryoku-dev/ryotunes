pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"
import "../lib/browse.js" as Browse

// The navigation rail (spec section 3): the masthead register, the three route groups (Discover /
// Collection / System) as micro labels over hairlines, 40 px nav rows with an icon, a body label and
// a kana seal at the rich decor level, the Library group expanding to its five tabs while you are in
// it, the PINNED shortcuts from the shared personal store as 40 px art rows, and the edition tag at
// the foot. The active route is read from Router.current; a click routes there. `open` collapses the
// rail: the layout slot animates to zero while a fixed-width glass body anchored to the right slides
// off to the left, so the content never reflows mid-slide.
Rectangle {
    id: root

    property bool open: true
    readonly property string activePage: Router.current ? Router.current.page : "home"
    readonly property string activeTab: (Router.current && Router.current.params && Router.current.params.tab)
        ? Router.current.params.tab : "songs"

    color: "transparent"
    implicitWidth: root.open ? Style.sidebarW : 0
    clip: true
    Behavior on implicitWidth { NumberAnimation { duration: Style.motion.slow; easing.type: Easing.OutCubic } }

    // A top-level nav row: 40 px, icon + body label + optional kana seal, bone plate when current.
    component NavRow: Rectangle {
        id: nr
        property string label: ""
        property string icon: ""
        property string kana: ""
        property bool current: false
        signal activated()

        Layout.fillWidth: true
        implicitHeight: Style.sp(10)
        radius: Style.radius
        color: nr.current ? Tokens.bone : navHover.hovered ? Tokens.tint10 : "transparent"
        Behavior on color { ColorAnimation { duration: Style.motion.snap } }

        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: Style.sp(2.5)
            anchors.rightMargin: Style.sp(2.5)
            spacing: Style.sp(3)
            Icon {
                name: nr.icon
                size: Style.fs.lg
                color: nr.current ? Tokens.inkOnBone : Tokens.inkMuted
            }
            Text {
                Layout.fillWidth: true
                text: (nr.current ? "// " : "") + nr.label
                color: nr.current ? Tokens.inkOnBone : Tokens.inkDim
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
                font.weight: Font.Medium
                elide: Text.ElideRight
            }
            Text {
                visible: Style.decorRich && nr.kana !== ""
                text: nr.kana
                color: nr.current ? Tokens.inkOnBone : Tokens.inkFaint
                opacity: nr.current ? 0.75 : 0.55
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

    // A section register: number, tracked label, hairline and (rich) seal.
    component SectionLabel: RowLayout {
        id: sl
        property string num: ""
        property string label: ""
        property string seal: ""
        Layout.fillWidth: true
        Layout.topMargin: Style.sp(1.5)
        Layout.bottomMargin: Style.sp(0.5)
        spacing: Style.sp(2)
        Text {
            text: sl.num
            color: Tokens.inkFaint
            font.family: Style.fontMono
            font.pixelSize: Style.fs.xs
        }
        Text {
            text: sl.label
            color: Tokens.inkMuted
            font.family: Style.fontUi
            font.pixelSize: Style.fs.xs
            font.weight: Font.DemiBold
            font.letterSpacing: 1.75
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
            anchors.rightMargin: Style.sp(2.5)
            spacing: Style.sp(2)
            Rectangle {
                Layout.alignment: Qt.AlignVCenter
                Layout.preferredWidth: Style.sp(1)
                Layout.preferredHeight: sr.current ? Style.sp(3.5) : Style.sp(1)
                radius: Style.sp(0.5)
                color: sr.current ? Style.accent : Tokens.lineStrong
                Behavior on Layout.preferredHeight { NumberAnimation { duration: Style.motion.snap } }
            }
            Text {
                Layout.fillWidth: true
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

    // The frosted glass body, full width, anchored right so a shrinking slot slides it off the left.
    Rectangle {
        id: glass
        width: Style.sidebarW
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        color: Qt.rgba(Tokens.paper.r, Tokens.paper.g, Tokens.paper.b, 0.9)

        Hairline { anchors.right: parent.right; width: 1; height: parent.height }

        ColumnLayout {
            anchors.fill: parent
            anchors.margins: Style.sp(2.5)
            spacing: Style.sp(1)

            // --- masthead register ----------------------------------------------------------
            RowLayout {
                Layout.fillWidth: true
                Layout.bottomMargin: Style.sp(2)
                spacing: Style.sp(2.5)
                Text {
                    text: "力"
                    color: Tokens.inkDim
                    font.family: Tokens.jp
                    font.pixelSize: Style.fs.xl
                }
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 1
                    Text {
                        text: "RYOTUNES"
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        font.weight: Font.DemiBold
                        font.letterSpacing: 2
                    }
                    Text {
                        text: "RYOKU // MUSIC"
                        color: Tokens.inkFaint
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.xs
                        font.letterSpacing: 1.2
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

                    // SYSTEM
                    SectionLabel { num: "03"; label: "SYSTEM"; seal: "設" }
                    NavRow { label: "Settings"; icon: "settings"; kana: "設"; current: root.activePage === "settings"; onActivated: if (root.activePage !== "settings") Router.push("settings") }

                    // PINNED shortcuts
                    SectionLabel { visible: Personal.picks.length > 0; num: "留"; label: "PINNED"; seal: "留" }
                    Repeater {
                        model: Personal.picks
                        delegate: Rectangle {
                            id: pin
                            required property var modelData
                            readonly property bool round: pin.modelData && pin.modelData.kind === "artist"
                            Layout.fillWidth: true
                            implicitHeight: Style.sp(11)
                            radius: Style.radius
                            color: pinHover.hovered ? Tokens.tint5 : "transparent"

                            RowLayout {
                                anchors.fill: parent
                                anchors.leftMargin: Style.sp(1.5)
                                anchors.rightMargin: Style.sp(1.5)
                                spacing: Style.sp(2)
                                Artwork {
                                    url: pin.modelData && pin.modelData.thumbnail ? pin.modelData.thumbnail : ""
                                    px: Style.sp(10)
                                    round: pin.round
                                    placeholderIcon: pin.round ? "user"
                                        : (pin.modelData && pin.modelData.kind === "album") ? "cd"
                                        : (pin.modelData && pin.modelData.kind === "playlist") ? "playlist" : "music"
                                }
                                ColumnLayout {
                                    Layout.fillWidth: true
                                    spacing: 0
                                    Text {
                                        Layout.fillWidth: true
                                        text: pin.modelData ? pin.modelData.title : ""
                                        color: Tokens.ink
                                        font.family: Style.fontUi
                                        font.pixelSize: Style.fs.sm
                                        font.weight: Font.Medium
                                        elide: Text.ElideRight
                                    }
                                    Text {
                                        Layout.fillWidth: true
                                        text: (pin.modelData && pin.modelData.subtitle) ? pin.modelData.subtitle
                                            : (pin.modelData ? pin.modelData.kind : "")
                                        color: Tokens.inkMuted
                                        font.family: Style.fontUi
                                        font.pixelSize: Style.fs.xs
                                        elide: Text.ElideRight
                                        textFormat: Text.PlainText
                                    }
                                }
                            }
                            HoverHandler { id: pinHover }
                            MouseArea {
                                anchors.fill: parent
                                cursorShape: Qt.PointingHandCursor
                                onClicked: {
                                    var it = pin.modelData;
                                    if (!it)
                                        return;
                                    if (it.kind === "song")
                                        Playback.play(Browse.asSong(it));
                                    else
                                        Router.push(it.kind, { id: it.id, title: it.title });
                                    Personal.touchPick(it.id);
                                }
                            }
                        }
                    }
                }
            }

            // --- edition register (dead-space ornament, per the design language) ------------
            RowLayout {
                Layout.fillWidth: true
                Layout.topMargin: Style.sp(1)
                spacing: Style.sp(1.5)
                Hairline { Layout.fillWidth: true; soft: true }
                Text {
                    text: "ED. 力 // NATIVE"
                    color: Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.xs
                    font.letterSpacing: 1.2
                }
            }
        }
    }
}
