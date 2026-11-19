import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"

// The music-server gate a page shows in place of its content while Navidrome is selected and no
// session exists. Unlike the other catalogues there is nothing to browse signed out, so the
// button walks to Settings → Account, where the address and password form lives.
ColumnLayout {
    id: root
    spacing: Style.sp(3)

    readonly property var nd: Playback.navidrome || ({})
    readonly property string failure: root.nd.error || ""

    Icon {
        Layout.alignment: Qt.AlignHCenter
        name: "navidrome"
        size: Style.sp(10)
        color: Style.providerColor
    }
    Text {
        Layout.alignment: Qt.AlignHCenter
        text: "Connect your music server"
        color: Tokens.ink
        font.family: Style.fontUi
        font.pixelSize: Style.fs.lg
        font.weight: Font.DemiBold
    }
    Btn {
        Layout.alignment: Qt.AlignHCenter
        text: "Open account settings"
        primary: true
        onClicked: Router.push("settings", { section: "account" })
    }
    Text {
        Layout.alignment: Qt.AlignHCenter
        Layout.maximumWidth: Style.sp(110)
        horizontalAlignment: Text.AlignHCenter
        wrapMode: Text.WordWrap
        text: root.failure
            ? root.failure
            : "Navidrome and other Subsonic servers live on your own machine or network. Ryotunes reads the address, the username and the password once, then keeps only a token."
        color: root.failure ? Style.accent : Tokens.inkMuted
        font.family: Style.fontUi
        font.pixelSize: Style.fs.sm
    }
}
