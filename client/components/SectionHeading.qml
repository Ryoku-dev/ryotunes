import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"

// A shelf / section header: the title in lg 500, a hairline rule filling the dead space, an optional
// kana `mark` at the right when the decor is rich, and an optional "See all" affordance. The kind
// icon and the "//" mark of the first pass are gone — one editorial heading, per spec section 4.
RowLayout {
    id: root

    property string title: ""
    // A kana glyph drawn at the far right when Style.decorRich (a section seal); "" hides it.
    property string mark: ""
    property bool more: false
    signal moreClicked()

    spacing: Style.sp(3)

    Text {
        Layout.maximumWidth: Style.sp(100)
        text: root.title
        color: Tokens.ink
        font.family: Style.fontUi
        font.pixelSize: Style.fs.lg
        font.weight: Font.Medium
        elide: Text.ElideRight
    }

    Rectangle {
        Layout.fillWidth: true
        Layout.preferredHeight: 1
        Layout.alignment: Qt.AlignVCenter
        gradient: Gradient {
            orientation: Gradient.Horizontal
            GradientStop { position: 0.0; color: Tokens.line }
            GradientStop { position: 0.7; color: Tokens.lineSoft }
            GradientStop { position: 1.0; color: "transparent" }
        }
    }

    // "See all"
    Item {
        visible: root.more
        implicitWidth: moreRow.implicitWidth
        implicitHeight: moreRow.implicitHeight
        Layout.alignment: Qt.AlignVCenter
        Row {
            id: moreRow
            spacing: Style.sp(1)
            Text {
                anchors.verticalCenter: parent.verticalCenter
                text: "SEE ALL"
                color: seeHover.hovered ? Tokens.ink : Tokens.inkMuted
                font.family: Style.fontMono
                font.pixelSize: Style.fs.micro
                font.letterSpacing: Style.trackMicro
            }
            Icon {
                anchors.verticalCenter: parent.verticalCenter
                name: "arrow-right"
                size: Style.fs.sm
                color: seeHover.hovered ? Tokens.ink : Tokens.inkMuted
            }
        }
        HoverHandler { id: seeHover }
        MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: root.moreClicked() }
    }

    // kana seal (rich decor only)
    Text {
        visible: Style.decorRich && root.mark !== ""
        Layout.alignment: Qt.AlignVCenter
        text: root.mark
        color: Tokens.inkFaint
        font.family: Tokens.jp
        font.pixelSize: Style.fs.sm
    }
}
