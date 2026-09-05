pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The persistent right column (Queue | Lyrics), the everyday companion to the routed page. Unlike the
// Now Playing stage (which takes the whole content), this is a docked 340 px panel App keeps open on a
// wide window. It owns no truth: the active tab is App's `tab`, written back through `tabRequested`,
// and the close chevron raises `closeRequested` so App clears `panelOpen`. Both panels stay
// instantiated across a tab switch and gate their timers on `visible`, so the lyrics keep their scroll
// and the queue its position, and neither spends CPU while the panel is closed. The column slides: its
// layout slot (implicitWidth) animates 0<->panelW while a fixed-width glass body anchored to the left
// is revealed/hidden by the clip, so the queue never reflows mid-slide.
Item {
    id: root

    property bool open: true
    property string tab: "queue"
    signal tabRequested(string tab)
    signal closeRequested()

    implicitWidth: root.open ? Style.panelW : 0
    clip: true
    Behavior on implicitWidth { NumberAnimation { duration: Style.motion.slow; easing.type: Easing.OutCubic } }

    // The glass body: full panel width, anchored to the left edge so a shrinking slot slides it off to
    // the right. Tokens.paper at 0.9 alpha reads as frosted glass over the content bloom beside it.
    Rectangle {
        id: glass
        width: Style.panelW
        anchors.left: parent.left
        anchors.top: parent.top
        anchors.bottom: parent.bottom
        color: Qt.rgba(Tokens.paper.r, Tokens.paper.g, Tokens.paper.b, 0.9)

        Hairline { anchors.left: parent.left; width: 1; height: parent.height }

        ColumnLayout {
            anchors.fill: parent
            anchors.leftMargin: 1
            spacing: 0

            // --- header: segmented Queue | Lyrics + close chevron -------------------------------
            Item {
                Layout.fillWidth: true
                Layout.preferredHeight: Style.titleBarH

                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(3)
                    anchors.rightMargin: Style.sp(2)
                    spacing: Style.sp(2)

                    Rectangle {
                        id: seg
                        Layout.fillWidth: true
                        Layout.preferredHeight: Style.sp(8)
                        Layout.alignment: Qt.AlignVCenter
                        radius: Style.radius
                        color: Tokens.paperLift
                        border.width: 1
                        border.color: Tokens.line

                        readonly property real pad: Style.sp(0.75)
                        readonly property real half: (width - seg.pad * 2) / 2

                        // The sliding indicator, travelling under the labels on the selector timing.
                        Rectangle {
                            id: indicator
                            y: seg.pad
                            x: seg.pad + (root.tab === "lyrics" ? seg.half : 0)
                            width: seg.half
                            height: parent.height - seg.pad * 2
                            radius: Style.radius - 1
                            color: Tokens.bone
                            Behavior on x { NumberAnimation { duration: Style.motion.move; easing.type: Easing.OutCubic } }
                        }

                        RowLayout {
                            anchors.fill: parent
                            anchors.margins: seg.pad
                            spacing: 0
                            Repeater {
                                model: [
                                    { key: "queue", label: "Queue", icon: "queue" },
                                    { key: "lyrics", label: "Lyrics", icon: "mic" }
                                ]
                                delegate: Item {
                                    id: tabBtn
                                    required property var modelData
                                    readonly property bool selected: root.tab === tabBtn.modelData.key
                                    Layout.fillWidth: true
                                    Layout.fillHeight: true
                                    RowLayout {
                                        anchors.centerIn: parent
                                        spacing: Style.sp(1.5)
                                        Icon {
                                            name: tabBtn.modelData.icon
                                            size: Style.fs.sm
                                            color: tabBtn.selected ? Tokens.inkOnBone : Tokens.inkMuted
                                        }
                                        Text {
                                            text: tabBtn.modelData.label
                                            color: tabBtn.selected ? Tokens.inkOnBone : Tokens.inkMuted
                                            font.family: Style.fontUi
                                            font.pixelSize: Style.fs.sm
                                            font.weight: Font.Medium
                                        }
                                    }
                                    MouseArea {
                                        anchors.fill: parent
                                        cursorShape: Qt.PointingHandCursor
                                        onClicked: root.tabRequested(tabBtn.modelData.key)
                                    }
                                }
                            }
                        }
                    }

                    IconButton {
                        icon: "chevron-right"
                        iconSize: Style.fs.lg
                        diameter: Style.sp(8)
                        onClicked: root.closeRequested()
                    }
                }

                Hairline { anchors.bottom: parent.bottom; width: parent.width; height: 1 }
            }

            // --- body: both panels alive, shown by tab ------------------------------------------
            Item {
                Layout.fillWidth: true
                Layout.fillHeight: true

                QueuePanel {
                    anchors.fill: parent
                    visible: root.open && root.tab === "queue"
                }
                LyricsPanel {
                    anchors.fill: parent
                    visible: root.open && root.tab === "lyrics"
                    compact: false
                }
            }
        }
    }
}
