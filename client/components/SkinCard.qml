pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"

// One entry in the Settings skin picker: a 200 px card carrying the skin's five-swatch strip
// (paper, paperLift, ink, sun, bone from its default mode — the live Tokens roles for the System
// card), its name, its author in tracked mono, a source badge (SHIPPED / USER / DEV / GENERATED)
// and, on the active skin, a check. Purely presentational: the host feeds `swatches`/`badge` and
// handles the pick through `clicked`.
Rectangle {
    id: root

    property string title: ""
    property string author: ""
    property var swatches: []          // five colour values, left→right
    property string badge: ""          // "" hides it; GENERATED wins over the source
    property bool active: false
    signal clicked()

    implicitWidth: Style.sp(50)        // 200 px at scale 1
    implicitHeight: col.implicitHeight + Style.sp(6)
    radius: Style.radiusCard
    color: root.active ? Tokens.tint5 : cardHover.hovered ? Tokens.tint5 : "transparent"
    border.width: 1
    border.color: root.active ? Tokens.sun : cardHover.hovered ? Tokens.lineStrong : Tokens.line
    Behavior on border.color { ColorAnimation { duration: Style.motion.snap } }
    Behavior on color { ColorAnimation { duration: Style.motion.snap } }

    ColumnLayout {
        id: col
        anchors { left: parent.left; right: parent.right; top: parent.top; margins: Style.sp(3) }
        spacing: Style.sp(2)

        // The palette strip: five equal blocks, clipped into the control radius.
        Rectangle {
            id: strip
            Layout.fillWidth: true
            implicitHeight: Style.sp(9)
            radius: Style.radius
            clip: true
            color: Tokens.line
            Row {
                anchors.fill: parent
                Repeater {
                    model: 5
                    delegate: Rectangle {
                        required property int index
                        width: strip.width / 5
                        height: strip.height
                        color: (root.swatches && root.swatches.length > index) ? root.swatches[index] : Tokens.line
                    }
                }
            }
        }

        RowLayout {
            Layout.fillWidth: true
            spacing: Style.sp(2)
            Text {
                Layout.fillWidth: true
                text: root.title
                color: Tokens.ink
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
                font.weight: Font.Medium
                elide: Text.ElideRight
            }
            Icon {
                visible: root.active
                name: "check-circle"
                size: Style.fs.md
                color: Tokens.sun
            }
        }

        RowLayout {
            Layout.fillWidth: true
            spacing: Style.sp(2)
            Text {
                Layout.fillWidth: true
                text: root.author
                color: Tokens.inkFaint
                font.family: Style.fontMono
                font.pixelSize: Style.fs.micro
                font.letterSpacing: Style.trackMicro
                elide: Text.ElideRight
            }
            Rectangle {
                visible: root.badge !== ""
                implicitHeight: Style.sp(4)
                implicitWidth: badgeText.implicitWidth + Style.sp(3)
                radius: height / 2
                color: "transparent"
                border.width: 1
                border.color: root.badge === "GENERATED" ? Style.accent : Tokens.line
                Text {
                    id: badgeText
                    anchors.centerIn: parent
                    text: root.badge
                    color: root.badge === "GENERATED" ? Style.accent : Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
                }
            }
        }
    }

    HoverHandler { id: cardHover }
    MouseArea {
        anchors.fill: parent
        cursorShape: Qt.PointingHandCursor
        onClicked: root.clicked()
    }
}
