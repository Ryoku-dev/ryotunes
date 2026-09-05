pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"
import "../lib/browse.js" as Browse

// Home's Pinned block, ported from Shortcuts.svelte: the user's pinned items as a 56 px TileGrid,
// plus a trailing "add" tile. `picks` comes from the shared Personal store (the daemon's personal
// blob), so a tile pinned in the Tauri app or a second client shows here too; `removed` drops it
// through Personal.removePick. Each tile plays or opens; the add tile routes into Search, where a
// card's menu pins it.
ColumnLayout {
    id: root

    property var picks: []
    signal removed(string id)

    spacing: Style.sp(4)

    SectionHeading {
        Layout.fillWidth: true
        title: "Pinned"
        mark: "選"
    }

    TileGrid {
        Layout.fillWidth: true
        model: root.picks
        removable: true
        onRemoved: (id) => root.removed(id)
        onActivated: (it) => {
            if (it.kind === "song")
                Playback.play(Browse.asSong(it));
            else
                Router.push(it.kind, { id: it.id, title: it.title });
        }
    }

    // add tile
    Rectangle {
        Layout.preferredWidth: Style.sp(52)
        Layout.preferredHeight: Style.sp(12)
        radius: Style.radius
        color: addHover.hovered ? Tokens.tint5 : "transparent"
        border.width: 1
        border.color: Tokens.line
        RowLayout {
            anchors.centerIn: parent
            spacing: Style.sp(2)
            Icon { name: "add"; size: Style.fs.md; color: Tokens.inkMuted }
            Text {
                text: "Add a shortcut"
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
            }
        }
        HoverHandler { id: addHover }
        MouseArea {
            anchors.fill: parent
            cursorShape: Qt.PointingHandCursor
            onClicked: Router.push("search")
        }
    }
}
