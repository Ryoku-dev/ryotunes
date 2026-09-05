import QtQuick
import Ryoku.Ui.Singletons
import "../"

// A mood/filter pill from the Home chip rail: a 32 px fully-rounded pill, outlined at rest and
// filled with the bone plate when it is the active filter, so the one selected chip is the only
// solid thing above the feed (per the Svelte chipClass and spec section 4).
Rectangle {
    id: root

    property string text: ""
    property bool active: false
    signal clicked()

    implicitHeight: Style.sp(8)
    implicitWidth: label.implicitWidth + Style.sp(8)
    radius: height / 2
    color: active ? Tokens.bone : chipHover.hovered ? Tokens.tint10 : "transparent"
    border.width: 1
    border.color: active ? Tokens.bone : chipHover.hovered ? Tokens.lineStrong : Tokens.line
    Behavior on color { ColorAnimation { duration: Style.motion.snap } }

    Text {
        id: label
        anchors.centerIn: parent
        text: root.text
        color: root.active ? Tokens.inkOnBone : Tokens.inkMuted
        font.family: Style.fontUi
        font.pixelSize: Style.fs.md
        font.weight: Font.Medium
    }

    HoverHandler { id: chipHover }
    MouseArea {
        anchors.fill: parent
        cursorShape: Qt.PointingHandCursor
        onClicked: root.clicked()
    }
}
