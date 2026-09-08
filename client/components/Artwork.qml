pragma ComponentBehavior: Bound
import QtQuick
import Ryoku.Ui.Singletons
import "../"

// Track/album/artist/playlist artwork with the neutral placeholder every absent or failed thumbnail
// lands on. The URL is rewritten by Style.thumb() to the exact pixel size drawn (2x for crispness)
// so the CDN returns a small image and the decode/cache stays bounded.
//
// Rounded corners cost one layer, not three: the Image renders into its own layer and a tiny
// fragment shader (shaders/rounded.frag) clips it to a rounded rectangle from a signed-distance
// field. The previous MultiEffect mask needed the image layer, a mask layer and the effect's own
// pass per thumbnail, and measured 20% of a core scrolling a 700-track list.
//
// One thumbnail string draws two ways. An ordinary URL — a real cover, a custom override, a
// provider image — is the single Image above. The internal collage contract is the literal marker
// "ryotunes-collage:" followed by a JSON array of exactly four nonempty URL strings; it renders as a
// 2x2 mosaic. The marker is produced only inside the app (a local playlist with four distinct song
// covers) and never leaves it. Anything that fails to parse — a truncated marker, the wrong shape —
// falls back to the placeholder rather than the broken source.
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

    // A marker URL is one the daemon addressed to this component; only a well-formed one becomes a
    // collage. Splitting the two lets a malformed marker skip the single Image (which would try, and
    // fail, to load the marker text as a source) and land on the placeholder instead.
    readonly property bool markerUrl: root.url.lastIndexOf("ryotunes-collage:", 0) === 0
    readonly property var collageUrls: root.parseCollage(root.url)
    readonly property bool isCollage: root.collageUrls !== null

    // Parse once per url change (a plain binding, so no image status feeds back into it): the array
    // of four covers, or null when this is an ordinary URL or a malformed marker.
    function parseCollage(u) {
        if (u.lastIndexOf("ryotunes-collage:", 0) !== 0)
            return null;
        try {
            var arr = JSON.parse(u.substring(17));
            if (Array.isArray(arr) && arr.length === 4) {
                for (var i = 0; i < 4; i++)
                    if (typeof arr[i] !== "string" || arr[i].length === 0)
                        return null;
                return arr;
            }
        } catch (e) {}
        return null;
    }

    Rectangle {
        id: plate
        anchors.fill: parent
        radius: root.cornerRadius
        color: Tokens.paperLift
        border.width: 1
        border.color: Tokens.lineSoft

        Icon {
            anchors.centerIn: parent
            visible: !root.isCollage && img.status !== Image.Ready
            name: root.placeholderIcon
            size: root.glyphSize
            color: Tokens.inkFaint
        }
    }

    Image {
        id: img
        anchors.fill: parent
        source: (root.url && !root.markerUrl) ? Style.thumb(root.url, Math.round(root.px * 2)) : ""
        sourceSize: Qt.size(Math.round(root.px * 2), Math.round(root.px * 2))
        fillMode: Image.PreserveAspectCrop
        asynchronous: true
        cache: true
        visible: !root.isCollage && status === Image.Ready
        layer.enabled: true
        layer.effect: ShaderEffect {
            // The layer's pixel size: the item size times the window's device pixel ratio, which
            // is what the SDF needs so the radius is in the same units as the texture.
            property vector2d size: Qt.vector2d(img.width * img.Screen.devicePixelRatio, img.height * img.Screen.devicePixelRatio)
            property real radius: root.cornerRadius * img.Screen.devicePixelRatio
            fragmentShader: Qt.resolvedUrl("../shaders/rounded.frag.qsb")
        }
    }

    // The 2x2 mosaic, built only while a well-formed marker is present. All four tiles render into
    // one layer, so the group takes a single rounded-clip SDF pass — the same cost as the single
    // Image, not four. The Loader keeps the same delegates while one collage swaps for another (the
    // model is a constant 4 and only the sources rebind), so nothing here recreates itself on load.
    Loader {
        anchors.fill: parent
        active: root.isCollage
        visible: root.isCollage
        sourceComponent: collage
    }

    Component {
        id: collage
        Item {
            id: grid
            anchors.fill: parent
            layer.enabled: true
            layer.effect: ShaderEffect {
                property vector2d size: Qt.vector2d(grid.width * grid.Screen.devicePixelRatio, grid.height * grid.Screen.devicePixelRatio)
                property real radius: root.cornerRadius * grid.Screen.devicePixelRatio
                fragmentShader: Qt.resolvedUrl("../shaders/rounded.frag.qsb")
            }

            // Integer halves with complementary far edges: the two columns/rows meet with no seam
            // and no overhang at any width.
            readonly property int half: Math.floor(grid.width / 2)

            Repeater {
                model: 4
                Image {
                    required property int index
                    x: (index % 2 === 0) ? 0 : grid.half
                    y: (index < 2) ? 0 : grid.half
                    width: (index % 2 === 0) ? grid.half : (grid.width - grid.half)
                    height: (index < 2) ? grid.half : (grid.height - grid.half)
                    // Covers are square (thumb() squares Google URLs), so a tile fills its quadrant
                    // edge to edge; a rare non-square cover fits whole, the plate showing behind it,
                    // rather than being cropped.
                    source: root.collageUrls ? Style.thumb(root.collageUrls[index], Math.round(root.px)) : ""
                    sourceSize: Qt.size(Math.round(root.px), Math.round(root.px))
                    fillMode: Image.PreserveAspectFit
                    asynchronous: true
                    cache: true
                }
            }
        }
    }

    Rectangle {
        anchors.fill: parent
        radius: root.cornerRadius
        color: "transparent"
        border.width: 1
        border.color: Tokens.lineSoft
        visible: img.visible || root.isCollage
    }
}
