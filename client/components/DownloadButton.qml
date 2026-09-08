pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Shapes
import Ryoku.Ui.Singletons
import "../"

// The one-click "save this track" control that sits beside the like heart. It reads the shared
// Downloads mirror for the playing track's job and reflects its state on the same plate the other
// transport buttons use: a download arrow to start, a progress ring while it runs, a check when it
// lands, an alert to retry a failure, and a muted arrow with an explanatory tooltip when the track
// (local file, live radio) can't be saved. One click enqueues immediately (Downloads.enqueue toasts
// the confirmation); clicking an in-flight or finished track opens the Downloads page instead, and a
// failed/cancelled one retries. It is a focusable, labelled control in its own right — Space/Return
// activate it, Tab reaches it — so it never depends on rewriting the shared IconButton.
Item {
    id: root

    // The track to act on; defaults to whatever is playing (Playback.now shape).
    property var track: Playback.now
    property int diameter: Style.sp(8)
    property int iconSize: Style.fs.md

    // The job for this track, reactive to the Downloads queue (jobFor reads Downloads.jobs).
    readonly property var job: Downloads.jobFor(root.track ? root.track.videoId : "")
    readonly property string status: root.job ? String(root.job.status) : ""
    readonly property bool downloadable: Downloads.canDownload(root.track)
    readonly property int progress: (root.job && root.job.progress) ? Math.max(0, Math.min(100, root.job.progress)) : 0
    readonly property bool busy: root.status === "downloading" || root.status === "queued"

    readonly property string iconName: root.status === "completed" ? "check-circle"
        : root.status === "failed" ? "alert"
        : "download"
    readonly property color tint: root.status === "completed" ? Style.accent
        : root.status === "failed" ? Style.alert
        : root.busy ? Tokens.ink
        : root.downloadable ? Tokens.inkMuted
        : Tokens.inkFaint

    // The hover tooltip line and the assistive-tech name for the current state.
    readonly property string hint: {
        switch (root.status) {
        case "completed": return "Downloaded \u00b7 open downloads";
        case "downloading": return "Downloading " + root.progress + "% \u00b7 open downloads";
        case "queued": return "Queued \u00b7 open downloads";
        case "failed": return (root.job && root.job.error ? "Failed: " + root.job.error : "Download failed") + " \u00b7 retry";
        case "cancelled": return "Cancelled \u00b7 retry";
        default: return Downloads.reason(root.track);
        }
    }
    readonly property string a11yName: {
        switch (root.status) {
        case "completed": return "Downloaded. Open downloads.";
        case "downloading": return "Downloading, " + root.progress + " percent. Open downloads.";
        case "queued": return "Queued for download. Open downloads.";
        case "failed": return "Download failed. Retry.";
        case "cancelled": return "Download cancelled. Retry.";
        default: return root.downloadable ? "Download this track." : Downloads.reason(root.track);
        }
    }

    function activate() {
        var t = root.track;
        if (!t || !t.videoId)
            return;
        var j = root.job;
        if (j) {
            if (j.status === "completed" || j.status === "downloading" || j.status === "queued")
                Router.push("downloads");
            else
                Downloads.retry(j.id);   // failed or cancelled reuse the record
            return;
        }
        if (!root.downloadable) {
            Playback.toast(Downloads.reason(t), "info");
            return;
        }
        Downloads.enqueue(t);
    }

    implicitWidth: diameter
    implicitHeight: diameter
    opacity: (root.downloadable || root.job) ? 1 : 0.55
    activeFocusOnTab: true

    Accessible.role: Accessible.Button
    Accessible.name: root.a11yName
    Accessible.description: root.hint
    Accessible.onPressAction: root.activate()
    Keys.onSpacePressed: (e) => { root.activate(); e.accepted = true; }
    Keys.onReturnPressed: (e) => { root.activate(); e.accepted = true; }
    Keys.onEnterPressed: (e) => { root.activate(); e.accepted = true; }

    // The plate, matching IconButton's hover/press tints so it reads as one of the transport row.
    Rectangle {
        anchors.fill: parent
        radius: Style.radius
        color: ma.pressed ? Tokens.tint16
            : hover.hovered ? Tokens.tint10
            : "transparent"
        Behavior on color { ColorAnimation { duration: Style.motion.snap } }
    }

    // The keyboard focus ring.
    Rectangle {
        anchors.fill: parent
        anchors.margins: -2
        radius: Style.radius
        color: "transparent"
        border.width: root.activeFocus ? 2 : 0
        border.color: Tokens.ink
    }

    Icon {
        anchors.centerIn: parent
        name: root.iconName
        size: root.iconSize
        color: root.tint
    }

    // The progress ring while queued (empty accent track) or downloading (accent arc to progress).
    Shape {
        id: ring
        anchors.centerIn: parent
        visible: root.busy
        width: root.diameter
        height: root.diameter
        preferredRendererType: Shape.CurveRenderer
        readonly property real cx: width / 2
        readonly property real cy: height / 2
        readonly property real rad: root.diameter * 0.36

        ShapePath {
            strokeColor: Style.accentSoft
            strokeWidth: 2
            fillColor: "transparent"
            PathAngleArc { centerX: ring.cx; centerY: ring.cy; radiusX: ring.rad; radiusY: ring.rad; startAngle: 0; sweepAngle: 360 }
        }
        ShapePath {
            strokeColor: Style.accent
            strokeWidth: 2
            fillColor: "transparent"
            capStyle: ShapePath.RoundCap
            PathAngleArc {
                centerX: ring.cx; centerY: ring.cy; radiusX: ring.rad; radiusY: ring.rad
                startAngle: -90
                sweepAngle: 360 * root.progress / 100
            }
        }
    }

    // Hover/focus tooltip. One line, elided; fades unless reduced motion is asked for.
    Rectangle {
        id: tip
        visible: opacity > 0
        opacity: ((hover.hovered || root.activeFocus) && root.hint !== "") ? 1 : 0
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.bottom: parent.top
        anchors.bottomMargin: Style.sp(1.5)
        implicitWidth: tipText.width + Style.sp(3)
        implicitHeight: tipText.implicitHeight + Style.sp(2)
        radius: Style.radius
        color: Tokens.paperLift
        border.width: 1
        border.color: Tokens.lineStrong
        z: 100
        Behavior on opacity { enabled: !Tokens.reduceMotion; NumberAnimation { duration: Style.motion.snap } }
        Text {
            id: tipText
            anchors.centerIn: parent
            width: Math.min(Style.sp(56), implicitWidth)
            text: root.hint
            textFormat: Text.PlainText
            color: Tokens.ink
            font.family: Style.fontUi
            font.pixelSize: Style.fs.xs
            elide: Text.ElideRight
        }
    }

    HoverHandler { id: hover }
    MouseArea {
        id: ma
        anchors.fill: parent
        cursorShape: Qt.PointingHandCursor
        onClicked: root.activate()
    }
}
