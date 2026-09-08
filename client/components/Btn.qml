import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"

// The text button: 36 px, radius 8, md 500. `primary` is the one filled control on a surface
// (bone on paper, Ryoku's inverse pair); everything else is a hairline ghost. Press scales 0.97,
// hover lifts the border, both on Tokens' snap timing.
Item {
    id: root

    property string text: ""
    property string icon: ""
    property bool primary: false
    signal clicked()

    implicitWidth: row.implicitWidth + Style.sp(4) * 2
    implicitHeight: Style.ctlH
    opacity: enabled ? 1 : 0.45
    activeFocusOnTab: true
    Keys.onReturnPressed: event => { if (root.enabled) root.clicked(); event.accepted = true; }
    Keys.onSpacePressed: event => { if (root.enabled) root.clicked(); event.accepted = true; }

    Rectangle {
        anchors.fill: parent
        radius: Style.radius
        color: root.primary ? Tokens.bone : (press.pressed ? Tokens.tint16 : hover.hovered ? Tokens.tint10 : "transparent")
        border.width: root.activeFocus ? 2 : root.primary ? 0 : 1
        border.color: root.primary && root.activeFocus ? Tokens.inkOnBone
            : root.activeFocus || hover.hovered ? Tokens.lineStrong : Tokens.line
        scale: press.pressed ? 0.97 : 1
        Behavior on scale { NumberAnimation { duration: Style.motion.snap; easing.type: Easing.OutCubic } }
        Behavior on color { ColorAnimation { duration: Style.motion.snap } }
        Behavior on border.color { ColorAnimation { duration: Style.motion.snap } }
        RowLayout {
            id: row
            anchors.centerIn: parent
            spacing: Style.sp(2)
            Icon {
                visible: root.icon !== ""
                name: root.icon
                size: Style.fs.md
                color: root.primary ? Tokens.inkOnBone : Tokens.ink
            }
            Text {
                visible: root.text !== ""
                text: root.text
                color: root.primary ? Tokens.inkOnBone : Tokens.ink
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
                font.weight: Font.Medium
            }
        }
    }
    HoverHandler { id: hover; cursorShape: Qt.PointingHandCursor }
    TapHandler { id: press; onTapped: root.clicked() }
    Accessible.role: Accessible.Button
    Accessible.name: root.text
    Accessible.onPressAction: { if (root.enabled) root.clicked(); }
}
