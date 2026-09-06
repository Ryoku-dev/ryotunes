import QtQuick
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The transient message layer. Playback.toast(message, kind) surfaces the daemon's
// playback-error / playback-notice / cover-error / lt-notice strings; the pill fades in at the
// bottom of the surface and a single self-stopping timer clears it. The timer only exists while a
// message is up, so it never contributes to idle wakeups.
Item {
    id: root
    anchors.fill: parent

    property string message: ""
    property string kind: "info"
    readonly property bool showing: opacity > 0

    Connections {
        target: Playback
        function onToast(message, kind) {
            root.message = message;
            root.kind = kind || "info";
            pill.opacity = 1;
            life.restart();
        }
    }

    Timer { id: life; interval: 3400; onTriggered: pill.opacity = 0 }

    Rectangle {
        id: pill
        opacity: 0
        visible: opacity > 0
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.bottom
        anchors.bottomMargin: Style.sp(24)
        implicitWidth: row.implicitWidth + Style.sp(8)
        implicitHeight: row.implicitHeight + Style.sp(5)
        radius: Style.radius
        color: Tokens.paperLift
        border.width: 1
        border.color: Tokens.lineStrong
        Behavior on opacity { NumberAnimation { duration: Style.motion.move; easing.type: Easing.OutQuad } }

        Row {
            id: row
            anchors.centerIn: parent
            spacing: Style.sp(2)
            Icon {
                anchors.verticalCenter: parent.verticalCenter
                size: Style.fs.md
                name: root.kind === "error" ? "alert" : root.kind === "success" ? "check-circle" : "info"
                color: root.kind === "error" ? Style.alert : root.kind === "success" ? Tokens.sun : Tokens.inkMuted
            }
            Text {
                anchors.verticalCenter: parent.verticalCenter
                text: root.message
                color: Tokens.ink
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
            }
        }

        // Dismiss on click.
        MouseArea { anchors.fill: parent; onClicked: pill.opacity = 0 }
    }
}
