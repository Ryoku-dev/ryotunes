pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui as RU
import "../"

// A shelf (spec section 4): a SectionHeading over a horizontally flicking row of MediaCards, gap 16,
// each card revealed with a Ryoku.Ui.Entrance stagger. Fed either a feed `section`
// ({ title, items, moreBrowseId, moreParams }) or explicit `title`/`items`. A section with a
// moreBrowseId routes "See all" into its list page; otherwise the host handles `moreClicked`.
ColumnLayout {
    id: root

    property var section: null
    property string title: (section && section.title) ? section.title : ""
    property var items: (section && section.items) ? section.items : []
    property bool more: !!(section && section.moreBrowseId)
    property string mark: ""
    signal moreClicked()

    spacing: Style.sp(4)

    function seeAll() {
        if (root.section && root.section.moreBrowseId)
            Router.push("list", {
                id: root.section.moreBrowseId,
                title: root.section.title,
                params: root.section.moreParams
            });
        else
            root.moreClicked();
    }

    SectionHeading {
        Layout.fillWidth: true
        title: root.title
        mark: root.mark
        more: root.more
        onMoreClicked: root.seeAll()
    }

    Flickable {
        Layout.fillWidth: true
        implicitHeight: rail.implicitHeight
        contentWidth: rail.implicitWidth
        contentHeight: rail.implicitHeight
        flickableDirection: Flickable.HorizontalFlick
        boundsBehavior: Flickable.StopAtBounds
        clip: true

        Row {
            id: rail
            spacing: Style.sp(4)
            Repeater {
                model: root.items
                delegate: Item {
                    id: cell
                    required property var modelData
                    required property int index
                    implicitWidth: entrance.implicitWidth
                    implicitHeight: entrance.implicitHeight
                    RU.Entrance {
                        id: entrance
                        index: cell.index
                        MediaCard {
                            item: cell.modelData
                            cardWidth: Style.cardW
                        }
                    }
                }
            }
        }
    }
}
