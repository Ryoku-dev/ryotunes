import QtQuick
import Ryoku.Ui.Singletons
import "../"

// The provider's light: a red (YouTube Music) or green (Spotify) radial glow at `focusX/Y`, the one
// clear tell of which catalogue is on, tweened between the two on a switch. One static Canvas,
// repainted only on resize or a colour step; it sits above the paper glass so it reads.
Canvas {
    id: root

    property real focusX: 0.82
    property real focusY: 0.1
    property real strength: 0.28
    property real radius: 0.6

    property color glowColor: Style.providerColor
    Behavior on glowColor { ColorAnimation { duration: Tokens.durSlowEffects } }
    onGlowColorChanged: requestPaint()
    onWidthChanged: requestPaint()
    onHeightChanged: requestPaint()

    onPaint: {
        var c = getContext("2d");
        c.clearRect(0, 0, width, height);
        var fx = width * root.focusX, fy = height * root.focusY;
        var pc = root.glowColor;
        var g = c.createRadialGradient(fx, fy, 0, fx, fy, Math.max(width, height) * root.radius);
        g.addColorStop(0, Qt.rgba(pc.r, pc.g, pc.b, root.strength));
        g.addColorStop(0.35, Qt.rgba(pc.r, pc.g, pc.b, root.strength * 0.4));
        g.addColorStop(1, Qt.rgba(pc.r, pc.g, pc.b, 0));
        c.fillStyle = g;
        c.fillRect(0, 0, width, height);
    }
}
