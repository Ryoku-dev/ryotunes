pragma ComponentBehavior: Bound
import QtQuick
import Ryoku.Ui.Singletons
import "../"

// The SoundCloud seek bar (the Orange look): the track's 240-sample waveform drawn as thin bars with
// a mirrored reflection, the played portion in the provider colour and the rest in inkFaint. It is a
// seek control — click or drag scrubs through Playback.seek(seconds) — and hovering shows a time
// tooltip with a scrub line. Rendered on a Canvas (240 rects, repainted only when the samples, the
// played fraction, the colours or the size change), so it costs nothing per frame. Height is 56 on
// the Now Playing stage and Home card; the player bar sizes it to 28. The one animated property is
// opacity, so a missing waveform fades rather than pops when it resolves.
Item {
    id: root

    // The 0..100 amplitude samples the daemon reports (get_waveform); 240 of them by contract, but
    // any length is resampled to `count` bars.
    property var samples: []
    // What the bar split reads from; defaults to the live transport (the held thumb while dragging).
    property real position: Playback.shownPosition
    property real duration: Playback.duration
    property color playedColor: Style.providerColor
    property color restColor: Tokens.inkFaint
    property bool seekable: true

    readonly property int count: 240
    readonly property real fraction: root.duration > 0
        ? Math.max(0, Math.min(1, root.position / root.duration)) : 0
    // Normalise against the loudest sample so a quiet track still fills the height.
    readonly property real peak: {
        var s = root.samples, m = 1;
        if (s)
            for (var i = 0; i < s.length; i++)
                if (s[i] > m)
                    m = s[i];
        return m;
    }

    implicitHeight: 56

    // Only appears once samples exist; the fade is the sole animated property.
    opacity: (root.samples && root.samples.length > 0) ? 1 : 0
    visible: opacity > 0
    Behavior on opacity { NumberAnimation { duration: Style.motion.swap } }

    onSamplesChanged: cv.requestPaint()
    onFractionChanged: cv.requestPaint()
    onPeakChanged: cv.requestPaint()
    onPlayedColorChanged: cv.requestPaint()
    onRestColorChanged: cv.requestPaint()

    Canvas {
        id: cv
        anchors.fill: parent
        onWidthChanged: cv.requestPaint()
        onHeightChanged: cv.requestPaint()
        onPaint: {
            var ctx = cv.getContext("2d");
            var w = cv.width, h = cv.height;
            ctx.clearRect(0, 0, w, h);
            var s = root.samples;
            if (!s || !s.length || w <= 0 || h <= 0)
                return;
            var n = root.count;
            var slot = w / n;
            var barW = Math.max(1, slot * 0.66);          // 2 px bar / 1 px gap at full width
            var baseY = h * 0.70;                          // 70 % above the line, 30 % reflection
            var topH = h * 0.70, botH = h * 0.30;
            var played = root.fraction * n;
            for (var i = 0; i < n; i++) {
                var idx = Math.floor(i * s.length / n);
                var a = Math.max(0.05, s[idx] / root.peak);
                var bx = i * slot + (slot - barW) / 2;
                var mh = a * topH, bh = a * botH;
                ctx.fillStyle = i < played ? root.playedColor : root.restColor;
                ctx.globalAlpha = 1;
                ctx.fillRect(bx, baseY - mh, barW, mh);
                ctx.globalAlpha = 0.35;
                ctx.fillRect(bx, baseY, barW, bh);
            }
            ctx.globalAlpha = 1;
        }
    }

    // Scrub line under the pointer while hovering the bar.
    Rectangle {
        visible: root.seekable && seek.containsMouse && seek.hoverX >= 0
        x: Math.max(0, Math.min(root.width - 1, seek.hoverX))
        width: 1
        height: parent.height
        color: Qt.rgba(Tokens.ink.r, Tokens.ink.g, Tokens.ink.b, 0.35)
    }

    // The time tooltip, floated above the pointer.
    Rectangle {
        id: tip
        visible: root.seekable && seek.containsMouse && seek.hoverX >= 0 && root.duration > 0
        x: Math.max(0, Math.min(root.width - width, seek.hoverX - width / 2))
        y: -height - Style.sp(1)
        implicitWidth: tipText.implicitWidth + Style.sp(2)
        implicitHeight: tipText.implicitHeight + Style.sp(1)
        radius: Style.radius - 2
        color: Tokens.bone
        Text {
            id: tipText
            anchors.centerIn: parent
            text: Style.fmtTime((seek.hoverX / Math.max(1, root.width)) * root.duration)
            color: Tokens.inkOnBone
            font.family: Style.fontMono
            font.pixelSize: Style.fs.xs
        }
    }

    MouseArea {
        id: seek
        anchors.fill: parent
        enabled: root.seekable
        hoverEnabled: true
        cursorShape: root.seekable ? Qt.PointingHandCursor : Qt.ArrowCursor
        property real hoverX: -1
        function frac(x) { return Math.max(0, Math.min(1, x / Math.max(1, root.width))); }
        onPositionChanged: (m) => {
            hoverX = m.x;
            if (pressed)
                Playback.seekDrag = seek.frac(m.x) * root.duration;
        }
        onExited: hoverX = -1
        onPressed: (m) => {
            hoverX = m.x;
            Playback.seekDrag = seek.frac(m.x) * root.duration;
        }
        onReleased: (m) => {
            var secs = seek.frac(m.x) * root.duration;
            Playback.seek(secs);
            Playback.seekDrag = NaN;
        }
        onCanceled: Playback.seekDrag = NaN
    }
}
