pragma ComponentBehavior: Bound
import QtQuick
import Ryoku.Ui.Singletons
import "../"

// A reused grid of MediaCards — the library's card tabs, a radio station wall, an artist's
// releases. A GridView with reuseItems and a bounded cache keeps a long collection to a couple of
// screenfuls of delegates. Columns = max(2, floor(width / (cardW + sp(4)))), per spec section 4.
Item {
    id: root

    property var model: []
    property bool loading: false
    property string emptyText: "Nothing here."
    property int pad: Style.pagePad

    readonly property int columns: Math.max(2, Math.floor(width / (Style.cardW + Style.sp(4))))

    GridView {
        id: grid
        anchors.fill: parent
        anchors.leftMargin: root.pad
        anchors.rightMargin: root.pad
        topMargin: Style.sp(4)
        bottomMargin: Style.sp(20)
        clip: true
        reuseItems: true
        cacheBuffer: Math.max(0, Math.round(height * 1.5))
        boundsBehavior: Flickable.StopAtBounds
        cellWidth: Math.floor(width / Math.max(1, root.columns))
        cellHeight: grid.cellWidth + Style.sp(16)
        model: root.loading ? [] : root.model

        delegate: Item {
            required property var modelData
            width: grid.cellWidth
            height: grid.cellHeight
            MediaCard {
                anchors.horizontalCenter: parent.horizontalCenter
                item: parent.modelData
                cardWidth: grid.cellWidth - Style.sp(4)
            }
        }
    }

    Text {
        anchors.centerIn: parent
        visible: root.loading || (root.model.length === 0)
        text: root.loading ? "Loading…" : root.emptyText
        color: Tokens.inkMuted
        font.family: Style.fontUi
        font.pixelSize: Style.fs.md
    }
}
