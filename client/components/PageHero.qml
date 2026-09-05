pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"

// A page's head, Sonora's fixed art-plus-title block in Ryoku's type: 168 px art (a circle for
// artists), then the tracked eyebrow (ALBUM / PLAYLIST / ARTIST), the Fraunces title, a meta line
// and the action row (Play, like, more). Flat paper behind it: the print texture rides the chrome
// (the nav rail), never the content (docs/ui-ux.md).
Item {
    id: root

    property string eyebrow: ""
    property string title: ""
    property string meta: ""
    property string art: ""
    property bool round: false
    property string placeholderIcon: "music"
    property string primaryLabel: "Play"
    property bool primaryPlaying: false
    property bool likeable: false
    property bool liked: false
    property bool showMore: true
    signal primary()
    signal like()
    signal more()

    implicitHeight: Math.max(Style.heroArt, copy.implicitHeight) + Style.sp(4)


    RowLayout {
        anchors.fill: parent
        anchors.bottomMargin: Style.sp(4)
        spacing: Style.sp(6)

        Artwork {
            Layout.preferredWidth: Style.heroArt
            Layout.preferredHeight: Style.heroArt
            Layout.alignment: Qt.AlignVCenter
            url: root.art
            px: Style.heroArt
            round: root.round
            placeholderIcon: root.placeholderIcon
            cornerRadius: root.round ? Style.heroArt / 2 : Style.radiusCard
        }

        ColumnLayout {
            id: copy
            Layout.fillWidth: true
            Layout.alignment: Qt.AlignVCenter
            spacing: Style.sp(2)

            Text {
                visible: root.eyebrow !== ""
                text: root.eyebrow.toUpperCase()
                color: Tokens.inkFaint
                font.family: Style.fontMono
                font.pixelSize: Style.fs.micro
                font.letterSpacing: Style.trackMicro
            }
            Text {
                Layout.fillWidth: true
                text: root.title
                color: Tokens.ink
                font.family: Style.fontDisplay
                font.pixelSize: Style.fs.title
                lineHeight: 1.05
                wrapMode: Text.Wrap
                maximumLineCount: 2
                elide: Text.ElideRight
            }
            Text {
                visible: root.meta !== ""
                Layout.fillWidth: true
                text: root.meta
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                elide: Text.ElideRight
            }
            RowLayout {
                Layout.topMargin: Style.sp(2)
                spacing: Style.sp(2)
                Btn {
                    primary: true
                    icon: root.primaryPlaying ? "pause" : "play"
                    text: root.primaryPlaying ? "Pause" : root.primaryLabel
                    onClicked: root.primary()
                }
                IconButton {
                    visible: root.likeable
                    icon: "heart"
                    iconSize: Style.fs.md
                    diameter: Style.ctlH
                    outlined: true
                    active: root.liked
                    iconColor: root.liked ? Style.accent : Tokens.inkMuted
                    tip: root.liked ? "Remove from library" : "Save to library"
                    onClicked: root.like()
                }
                IconButton {
                    visible: root.showMore
                    icon: "more"
                    iconSize: Style.fs.md
                    diameter: Style.ctlH
                    outlined: true
                    tip: "More"
                    onClicked: root.more()
                }
            }
        }
    }
}
