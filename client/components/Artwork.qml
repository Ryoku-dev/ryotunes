pragma ComponentBehavior: Bound
import QtQuick
import Ryoku.Ui.Singletons
import "../"

// Track/album/artist artwork with the neutral placeholder every absent or failed thumbnail lands
// on. The URL is rewritten by Style.thumb() to the exact pixel size drawn (2x for crispness) so the
// CDN returns a small image and the decode/cache stays bounded.
//
// Rounded corners cost one layer, not three: the Image renders into its own layer and a tiny
// fragment shader (shaders/rounded.frag) clips it to a rounded rectangle from a signed-distance
// field. The previous MultiEffect mask needed the image layer, a mask layer and the effect's own
// pass per thumbnail, and measured 20% of a core scrolling a 700-track list.
Item {
    id: root

    property string url: ""
    property int px: 48
    property bool round: false
    property string placeholderIcon: "music"
    property int glyphSize: Math.round(px * 0.42)

    width: px
    height: px

    property real cornerRadius: root.round ? width / 2 : Style.radius

    Rectangle {
        id: plate
        anchors.fill: parent
        radius: root.cornerRadius
        color: Tokens.paperLift
        border.width: 1
        border.color: Tokens.lineSoft

        Icon {
            anchors.centerIn: parent
            visible: img.status !== Image.Ready
            name: root.placeholderIcon
            size: root.glyphSize
            color: Tokens.inkFaint
        }
    }

    Image {
        id: img
        anchors.fill: parent
        source: root.url ? Style.thumb(root.url, Math.round(root.px * 2)) : ""
        sourceSize: Qt.size(Math.round(root.px * 2), Math.round(root.px * 2))
        fillMode: Image.PreserveAspectCrop
        asynchronous: true
        cache: true
        visible: status === Image.Ready
        layer.enabled: true
        layer.effect: ShaderEffect {
            // The layer's pixel size: the item size times the window's device pixel ratio, which
            // is what the SDF needs so the radius is in the same units as the texture.
            property vector2d size: Qt.vector2d(img.width * img.Screen.devicePixelRatio, img.height * img.Screen.devicePixelRatio)
            property real radius: root.cornerRadius * img.Screen.devicePixelRatio
            fragmentShader: Qt.resolvedUrl("../shaders/rounded.frag.qsb")
        }
    }

    Rectangle {
        anchors.fill: parent
        radius: root.cornerRadius
        color: "transparent"
        border.width: 1
        border.color: Tokens.lineSoft
        visible: img.visible
    }
}
