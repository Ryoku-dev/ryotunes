import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"

// The Spotify sign-in gate a page shows in place of its content while the provider is Spotify
// and no Premium session exists. It reflects the OAuth flow: idle, waiting on the browser, or
// the daemon's failure reason (a free account, a cancelled page) - so a click always visibly
// leads somewhere.
ColumnLayout {
    id: root
    spacing: Style.sp(3)

    readonly property var sp: Playback.spotify || ({})
    readonly property bool waiting: root.sp.flow === "browser"
    readonly property string failure: root.sp.error || ""

    Icon {
        Layout.alignment: Qt.AlignHCenter
        name: "spotify"
        size: Style.sp(10)
        color: Style.providerColor
    }
    Btn {
        Layout.alignment: Qt.AlignHCenter
        text: root.waiting ? "Waiting for your browser\u2026" : (root.failure ? "Try another account" : "Sign in to Spotify")
        primary: true
        onClicked: Playback.spotifySignIn().catch(() => {})
    }
    Text {
        Layout.alignment: Qt.AlignHCenter
        Layout.maximumWidth: Style.sp(110)
        horizontalAlignment: Text.AlignHCenter
        wrapMode: Text.WordWrap
        text: root.failure
            ? root.failure + "\nSpotify streams only to Premium accounts; YouTube Music plays without one."
            : (root.waiting ? "Finish the authorization in the tab that opened, then come back here."
                            : "Premium is required for playback.")
        color: root.failure ? Style.accent : Tokens.inkMuted
        font.family: Style.fontUi
        font.pixelSize: Style.fs.sm
    }
}
