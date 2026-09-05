import QtQuick
import Ryoku.Ui.Singletons
import "../"

// A one-dimensional drag control (volume, and the seek overlay with visuals off). A 2 px track with
// an accent fill; the 12 px handle appears only on hover or while held, so the resting control is a
// quiet hairline (spec section 4). Strictly unidirectional: it never stores its own value. It reads
// `value` (bound by the parent to the model) and only emits `moved`/`committed`; the parent updates
// the model, and the binding feeds the new value straight back, so an incoming daemon echo can never
// fight the pointer. Qt grabs the pointer on press, so `committed` fires even when the release lands
// outside the window.
Item {
    id: root

    property real from: 0
    property real to: 1
    property real value: 0
    property bool visualTrack: true
    property bool showHandle: true
    property int thickness: 2
    property int handleSize: 12
    property color trackColor: Tokens.lineStrong
    property color fillColor: Style.accent

    readonly property bool pressed: ma.pressed
    readonly property real pct: to > from ? Math.max(0, Math.min(1, (value - from) / (to - from))) : 0
    readonly property bool showKnob: root.visualTrack && root.showHandle && (hover.hovered || ma.pressed)

    signal moved(real v)
    signal committed(real v)

    implicitWidth: 120
    implicitHeight: Math.max(handleSize, thickness) + Style.sp(2)

    function _valueAt(x) {
        var t = width > 0 ? Math.max(0, Math.min(1, x / width)) : 0;
        return from + t * (to - from);
    }

    Rectangle {
        visible: root.visualTrack
        anchors.verticalCenter: parent.verticalCenter
        width: parent.width
        height: root.thickness
        radius: height / 2
        color: root.trackColor
    }
    Rectangle {
        visible: root.visualTrack
        anchors.verticalCenter: parent.verticalCenter
        width: parent.width * root.pct
        height: root.thickness
        radius: height / 2
        color: root.fillColor
    }
    Rectangle {
        visible: root.showKnob
        width: root.handleSize
        height: root.handleSize
        radius: height / 2
        color: root.fillColor
        anchors.verticalCenter: parent.verticalCenter
        x: parent.width * root.pct - width / 2
    }

    HoverHandler { id: hover }
    MouseArea {
        id: ma
        anchors.fill: parent
        anchors.topMargin: -Style.sp(2)
        anchors.bottomMargin: -Style.sp(2)
        preventStealing: true
        cursorShape: Qt.PointingHandCursor
        onPressed: (m) => root.moved(root._valueAt(m.x))
        onPositionChanged: (m) => { if (ma.pressed) root.moved(root._valueAt(m.x)); }
        onReleased: (m) => root.committed(root._valueAt(m.x))
    }
}
