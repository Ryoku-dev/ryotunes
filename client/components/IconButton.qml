import QtQuick
import Ryoku.Ui.Singletons
import "../"

// A square icon button: hover and pressed tints from Tokens, a disabled dim, and an `active` state
// that pins the "on" ink like the Svelte transport toggles (shuffle/repeat/like). `primary` is the
// one filled control (play/pause), drawn on the bone plate so it keeps contrast on any palette.
Item {
    id: root

    property string icon: ""
    property int iconSize: 18
    property int diameter: Style.sp(8)
    property bool active: false
    property bool primary: false
    // A hairline ring, for a button that sits beside a Btn in an action row.
    property bool outlined: false
    // The hover label, and the name assistive tech reads. It was declared here
    // from the start and never drawn, so every tip: in the app set a property
    // nothing rendered.
    property string tip: ""
    // Draw the label under the button instead of over it, for a row that sits
    // against the top of the window where there is no room above.
    property bool tipBelow: false
    property color iconColor: root.primary ? Tokens.inkOnBone
        : !root.enabled ? Tokens.inkFaint
        : root.active ? Tokens.ink
        : Tokens.inkMuted

    readonly property alias hovered: hover.hovered
    signal clicked()

    implicitWidth: diameter
    implicitHeight: diameter
    opacity: enabled ? 1 : 0.45

    Rectangle {
        anchors.fill: parent
        radius: Style.radius
        color: root.primary ? Tokens.bone
            : ma.pressed ? Tokens.tint16
            : hover.hovered ? Tokens.tint10
            : "transparent"
        border.width: root.outlined ? 1 : 0
        border.color: hover.hovered ? Tokens.lineStrong : Tokens.line
        Behavior on color { ColorAnimation { duration: Style.motion.snap } }
    }

    Icon {
        anchors.centerIn: parent
        name: root.icon
        size: root.iconSize
        color: root.iconColor
    }

    // Hover label. One line, elided; fades unless reduced motion is asked for.
    Rectangle {
        id: tipPlate
        visible: opacity > 0
        opacity: (hover.hovered && root.tip !== "") ? 1 : 0
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.top: root.tipBelow ? parent.bottom : undefined
        anchors.bottom: root.tipBelow ? undefined : parent.top
        anchors.topMargin: Style.sp(1.5)
        anchors.bottomMargin: Style.sp(1.5)
        implicitWidth: tipText.width + Style.sp(3)
        implicitHeight: tipText.implicitHeight + Style.sp(2)
        radius: Style.radius
        color: Tokens.paperLift
        border.width: 1
        border.color: Tokens.lineStrong
        z: 100
        Behavior on opacity { enabled: !Tokens.reduceMotion; NumberAnimation { duration: Style.motion.snap } }
        Text {
            id: tipText
            anchors.centerIn: parent
            width: Math.min(Style.sp(56), implicitWidth)
            text: root.tip
            textFormat: Text.PlainText
            color: Tokens.ink
            font.family: Style.fontUi
            font.pixelSize: Style.fs.xs
            elide: Text.ElideRight
        }
    }

    Accessible.role: Accessible.Button
    Accessible.name: root.tip
    Accessible.onPressAction: root.clicked()

    HoverHandler { id: hover; enabled: root.enabled }
    MouseArea {
        id: ma
        anchors.fill: parent
        enabled: root.enabled
        cursorShape: Qt.PointingHandCursor
        onClicked: root.clicked()
    }
}
