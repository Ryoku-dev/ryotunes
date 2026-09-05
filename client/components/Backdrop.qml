pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Effects
import Ryoku.Ui.Singletons
import "../"

// The playing cover as the room's light: a 64 px thumbnail stretched over this item, blurred once
// (one MultiEffect pass per track change; static between), cross-faded A/B on track change, and
// faded toward paper at the edges so it reads as a bloom rather than a poster. `strength` is the
// wash's opacity. Under Style.ambient the bloom drifts (scale 1.0-1.06 over 12 s), stepped by a
// 10 fps timer so an idle window never renders per vsync; paused, hidden or power-saver: still.
Item {
    id: root

    property real strength: 0.22
    // Where the light comes from, as fractions of the item; the edge fade is centred there.
    property real focusX: 0.72
    property real focusY: 0.18
    property bool drift: true

    readonly property string url: (Playback.now && Playback.now.thumbnail) ? Style.thumb(Playback.now.thumbnail, 64) : ""
    property int front: 0
    readonly property bool paperDark: (Tokens.paper.r + Tokens.paper.g + Tokens.paper.b) / 3 < 0.5

    onUrlChanged: {
        var back = root.front === 0 ? layerB : layerA;
        back.src = root.url;
    }
    Component.onCompleted: layerA.src = root.url

    component Wash: Item {
        id: wash
        anchors.fill: parent
        property string src: ""
        property bool shown: false
        readonly property bool ready: img.status === Image.Ready
        opacity: shown ? root.strength : 0
        Behavior on opacity { NumberAnimation { duration: Tokens.durSlowEffects; easing.type: Easing.OutCubic } }
        Image {
            id: img
            anchors.fill: parent
            source: wash.src
            sourceSize: Qt.size(64, 64)
            fillMode: Image.PreserveAspectCrop
            asynchronous: true
            cache: true
            visible: false
            onStatusChanged: if (status === Image.Ready) {
                // The newest loaded layer takes the front; the other fades out behind it.
                root.front = wash === layerA ? 0 : 1;
                layerA.shown = root.front === 0;
                layerB.shown = root.front === 1;
            }
        }
        MultiEffect {
            anchors.fill: parent
            source: img
            visible: wash.ready && Style.blurEnabled
            blurEnabled: true
            blur: 1.0
            blurMax: 64
            saturation: root.paperDark ? 0.15 : -0.1
            brightness: root.paperDark ? 0.05 : 0.1
            scale: driftScale.value
            transformOrigin: Item.Center
        }
    }
    Wash { id: layerA }
    Wash { id: layerB }

    // The bloom's edge: paper, transparent at the focus, painted once per resize.
    Canvas {
        id: fade
        anchors.fill: parent
        onWidthChanged: requestPaint()
        onHeightChanged: requestPaint()
        onPaint: {
            var c = getContext("2d");
            c.clearRect(0, 0, width, height);
            var p = Tokens.paper;
            var g = c.createRadialGradient(width * root.focusX, height * root.focusY, 0, width * root.focusX, height * root.focusY, Math.max(width, height) * 0.9);
            g.addColorStop(0, Qt.rgba(p.r, p.g, p.b, 0));
            g.addColorStop(0.55, Qt.rgba(p.r, p.g, p.b, 0.55));
            g.addColorStop(1, Qt.rgba(p.r, p.g, p.b, 0.95));
            c.fillStyle = g;
            c.fillRect(0, 0, width, height);
        }
        Connections { target: Tokens; function onPaperChanged() { fade.requestPaint(); } }
    }

    // Ambient drift: a 12 s sine on the wash's scale, sampled at 10 fps.
    QtObject {
        id: driftScale
        property real phase: 0
        readonly property real value: 1.03 + 0.03 * Math.sin(phase)
    }
    Timer {
        interval: 100
        repeat: true
        running: root.drift && root.visible && Style.ambient
        onTriggered: driftScale.phase = (driftScale.phase + (2 * Math.PI) / 120) % (2 * Math.PI)
    }
}
