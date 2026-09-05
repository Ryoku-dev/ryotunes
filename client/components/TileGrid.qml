pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"

// A compact tile grid (spec section 4): 56 px artwork tiles with a name in md over a kind in sm,
// three columns when wide and two when narrow. Home's Pinned block (Personal picks / shortcuts) is
// its first user. Each tile emits `activated(item)`; when `removable`, a hover X emits
// `removed(id)`.
Item {
    id: root

    property var model: []
    property bool removable: false
    signal activated(var item)
    signal removed(string id)

    // Three across when there is room, else two (the framed content never reaches the reference's
    // 1200 px window, so the break is on the grid's own width).
    readonly property int columns: root.width >= Style.sp(200) ? 3 : 2

    implicitHeight: grid.implicitHeight

    GridLayout {
        id: grid
        width: parent.width
        columns: root.columns
        columnSpacing: Style.sp(4)
        rowSpacing: Style.sp(2)

        Repeater {
            model: root.model
            delegate: Rectangle {
                id: tile
                required property var modelData
                readonly property bool round: tile.modelData && tile.modelData.kind === "artist"
                Layout.fillWidth: true
                Layout.preferredWidth: 1
                implicitHeight: Style.sp(14) + Style.sp(3)
                radius: Style.radius
                color: tileHover.hovered ? Tokens.tint5 : "transparent"

                RowLayout {
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    anchors.leftMargin: Style.sp(1.5)
                    anchors.rightMargin: Style.sp(1.5)
                    spacing: Style.sp(2)

                    Artwork {
                        url: tile.modelData && tile.modelData.thumbnail ? tile.modelData.thumbnail : ""
                        px: Style.sp(14)
                        round: tile.round
                        placeholderIcon: tile.round ? "user" : "music"
                    }
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: 0
                        Text {
                            Layout.fillWidth: true
                            text: tile.modelData ? tile.modelData.title : ""
                            color: Tokens.ink
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.md
                            font.weight: Font.Medium
                            elide: Text.ElideRight
                        }
                        Text {
                            Layout.fillWidth: true
                            text: (tile.modelData && tile.modelData.subtitle) ? tile.modelData.subtitle
                                : (tile.modelData ? tile.modelData.kind : "")
                            color: Tokens.inkMuted
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.sm
                            elide: Text.ElideRight
                            textFormat: Text.PlainText
                        }
                    }
                }

                IconButton {
                    visible: root.removable && tileHover.hovered
                    anchors.top: parent.top
                    anchors.right: parent.right
                    icon: "close"
                    iconSize: Style.fs.xs
                    diameter: Style.sp(5)
                    onClicked: root.removed(tile.modelData.id)
                }

                HoverHandler { id: tileHover }
                MouseArea {
                    anchors.fill: parent
                    cursorShape: Qt.PointingHandCursor
                    onClicked: root.activated(tile.modelData)
                }
            }
        }
    }
}
