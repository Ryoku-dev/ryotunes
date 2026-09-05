pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../lib/ids.js" as Ids

// The Home hero's listening console, the QML twin of RyokuMusicDeck.svelte: artwork with a grid
// wash and an activity meter on the left, the running readout (PLAYBACK / STREAM), the track,
// QUEUE / LEVEL / SPEED, a hairline progress and the transport on the right. The meter advances
// from the playback clock (four steps per second) rather than a timer, so it sleeps with playback.
Rectangle {
    id: root

    signal openNowPlaying(string tab)

    implicitHeight: Style.sp(37)
    radius: Style.radius
    color: Tokens.paper
    border.width: 1
    border.color: Tokens.line
    clip: true

    readonly property var now: Playback.now
    readonly property bool live: !!now && !Playback.paused
    readonly property string sourceLabel: !now ? "READY"
        : Ids.isLocalId(now.videoId) ? "LOCAL"
        : (now.streamClient === "cache" ? "CACHE" : "STREAM")
    readonly property int queueLeft: Math.max(0, Playback.queue.items.length - Playback.queue.currentIndex - 1)
    readonly property real progress: Playback.duration > 0
        ? Math.min(1, Math.max(0, Playback.position / Playback.duration)) : 0
    readonly property int meterPhase: Math.floor(Playback.position * 4)

    function level(i) {
        if (!root.live) return 0.12;
        var a = Math.sin(root.meterPhase * 0.79 + i * 1.73);
        var b = Math.sin(root.meterPhase * 0.47 + i * 2.31 + 1.2);
        return Math.min(0.96, 0.18 + Math.abs(a * 0.46 + b * 0.28));
    }

    // Corner ticks, the deck's registration marks.
    Rectangle { x: -1; y: Style.sp(2); width: Style.sp(2); height: 1; color: Tokens.lineStrong; z: 5 }
    Rectangle { x: Style.sp(2); y: -1; width: 1; height: Style.sp(2); color: Tokens.lineStrong; z: 5 }
    Rectangle { x: -1; y: parent.height - Style.sp(2); width: Style.sp(2); height: 1; color: Tokens.lineStrong; z: 5 }

    RowLayout {
        anchors.fill: parent
        spacing: 0

        // --- art -------------------------------------------------------------------------
        Item {
            id: art
            Layout.fillHeight: true
            Layout.preferredWidth: Math.round(root.width * 0.41)
            Layout.minimumWidth: Style.sp(42)
            clip: true

            Rectangle { anchors.fill: parent; color: Tokens.paperLift }

            Image {
                id: cover
                anchors.fill: parent
                source: root.now && root.now.thumbnail ? Style.thumb(root.now.thumbnail, 384) : ""
                sourceSize: Qt.size(384, 384)
                fillMode: Image.PreserveAspectCrop
                asynchronous: true
                cache: true
                opacity: status === Image.Ready ? 1 : 0
                Behavior on opacity { NumberAnimation { duration: Style.motion.slow } }
            }
            // Bottom wash so the meter reads over any artwork.
            Rectangle {
                anchors.fill: parent
                visible: cover.status === Image.Ready
                gradient: Gradient {
                    GradientStop { position: 0.42; color: "transparent" }
                    GradientStop { position: 1.0; color: Qt.rgba(Tokens.paper.r, Tokens.paper.g, Tokens.paper.b, 0.7) }
                }
            }
            // The grid, on artwork and on the idle plate alike.
            Canvas {
                id: grid
                anchors.fill: parent
                opacity: cover.status === Image.Ready ? 0.16 : 1
                onPaint: {
                    var ctx = getContext("2d");
                    ctx.clearRect(0, 0, width, height);
                    var step = Math.round(28 * Style.uiScale);
                    ctx.strokeStyle = Tokens.lineSoft;
                    ctx.lineWidth = 1;
                    ctx.beginPath();
                    for (var x = 0.5; x < width; x += step) { ctx.moveTo(x, 0); ctx.lineTo(x, height); }
                    for (var y = 0.5; y < height; y += step) { ctx.moveTo(0, y); ctx.lineTo(width, y); }
                    ctx.stroke();
                }
                onWidthChanged: requestPaint()
                onHeightChanged: requestPaint()
                Connections { target: Tokens; function onInkChanged() { grid.requestPaint(); } }
            }
            // Idle: the orbit rings and a note.
            Item {
                id: orbit
                anchors.centerIn: art
                visible: cover.status !== Image.Ready
                width: Style.sp(22); height: width
                Repeater {
                    model: [0, Style.sp(3.5), Style.sp(7.5)]
                    delegate: Rectangle {
                        required property var modelData
                        required property int index
                        anchors.centerIn: orbit
                        width: orbit.width - modelData * 2; height: width
                        radius: width / 2
                        color: "transparent"
                        border.width: 1
                        border.color: index === 0 ? Tokens.line : Tokens.lineSoft
                    }
                }
                Icon { anchors.centerIn: orbit; name: "music"; size: Style.sp(7); color: Tokens.inkMuted }
            }

            Text {
                x: Style.sp(2.25); y: Style.sp(2)
                text: "// LIVE"
                color: Qt.rgba(Tokens.ink.r, Tokens.ink.g, Tokens.ink.b, 0.7)
                font.family: Style.fontMono
                font.pixelSize: Style.fs.xs
                font.letterSpacing: 1.5
            }

            // The activity meter.
            Row {
                id: wave
                anchors { left: art.left; right: art.right; bottom: art.bottom; margins: Style.sp(2.25) }
                height: Style.sp(7)
                spacing: 2
                readonly property real barW: (width - spacing * 11) / 12
                Repeater {
                    model: 12
                    delegate: Item {
                        required property int index
                        id: bar
                        width: wave.barW; height: wave.height
                        Rectangle {
                            anchors.bottom: bar.bottom
                            width: bar.width
                            height: Math.max(1, bar.height * root.level(bar.index))
                            color: Qt.rgba(Tokens.ink.r, Tokens.ink.g, Tokens.ink.b, 0.78)
                            opacity: root.live ? 0.72 : 0.52
                            Behavior on height { NumberAnimation { duration: 180 } }
                        }
                    }
                }
            }

            MouseArea {
                anchors.fill: parent
                enabled: !!root.now
                cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
                onClicked: root.openNowPlaying("queue")
            }
            Rectangle { anchors.right: parent.right; width: 1; height: parent.height; color: Tokens.line }
        }

        // --- copy ------------------------------------------------------------------------
        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            Layout.margins: Style.sp(3)
            Layout.topMargin: Style.sp(2.75)
            Layout.bottomMargin: Style.sp(2.5)
            spacing: 0

            RowLayout {
                Layout.fillWidth: true
                spacing: Style.sp(1.75)
                Text { text: "PLAYBACK"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1.35 }
                Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; color: Tokens.lineSoft }
                Text { text: root.sourceLabel; color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1.35 }
            }

            // playing
            ColumnLayout {
                visible: !!root.now
                Layout.fillWidth: true
                Layout.fillHeight: true
                spacing: 0

                Item {
                    Layout.fillWidth: true
                    Layout.topMargin: Style.sp(2)
                    implicitHeight: trackCol.implicitHeight
                    ColumnLayout {
                        id: trackCol
                        width: parent.width
                        spacing: Style.sp(0.75)
                        Text {
                            Layout.fillWidth: true
                            text: root.now ? root.now.title : ""
                            color: trackHover.hovered ? Tokens.inkDim : Tokens.ink
                            font.family: Tokens.display
                            font.pixelSize: Style.fs.xl
                            elide: Text.ElideRight
                        }
                        Text {
                            Layout.fillWidth: true
                            text: root.now ? root.now.artists : ""
                            color: Tokens.inkMuted
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.sm
                            elide: Text.ElideRight
                        }
                    }
                    HoverHandler { id: trackHover; cursorShape: Qt.PointingHandCursor }
                    TapHandler { onTapped: root.openNowPlaying("queue") }
                }

                Rectangle { Layout.fillWidth: true; Layout.topMargin: Style.sp(2.25); Layout.preferredHeight: 1; color: Tokens.lineSoft }
                RowLayout {
                    Layout.fillWidth: true
                    Layout.topMargin: Style.sp(2)
                    spacing: Style.sp(2)
                    Repeater {
                        model: [
                            { k: "QUEUE", v: String(root.queueLeft) },
                            { k: "LEVEL", v: Math.round(Playback.volume) + "%" },
                            // The native client has no tempo control yet; the daemon plays at 1x.
                            { k: "SPEED", v: "1.00×" }
                        ]
                        delegate: ColumnLayout {
                            id: readout
                            required property var modelData
                            Layout.fillWidth: true
                            spacing: 1
                            Text { text: readout.modelData.k; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1.15 }
                            Text { text: readout.modelData.v; color: Tokens.inkDim; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; font.weight: Font.Medium }
                        }
                    }
                }

                RowLayout {
                    Layout.fillWidth: true
                    Layout.topMargin: Style.sp(2)
                    spacing: Style.sp(1.75)
                    Text { Layout.preferredWidth: Style.sp(8); text: Style.fmtTime(Playback.position); color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs }
                    Item {
                        Layout.fillWidth: true
                        Layout.preferredHeight: 3
                        Rectangle { anchors.verticalCenter: parent.verticalCenter; width: parent.width; height: 1; color: Tokens.lineStrong }
                        Rectangle { y: 0; width: parent.width * root.progress; height: 3; color: Tokens.ink }
                    }
                    Text { Layout.preferredWidth: Style.sp(8); horizontalAlignment: Text.AlignRight; text: Style.fmtTime(Playback.duration); color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs }
                }

                Item { Layout.fillHeight: true }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(1.25)
                    IconButton { icon: "previous"; iconSize: Style.fs.md; diameter: Style.sp(6.25); tip: "Previous"; onClicked: Playback.prev() }
                    IconButton { icon: Playback.paused ? "play" : "pause"; iconSize: Style.fs.md; diameter: Style.sp(6.25); primary: true; tip: Playback.paused ? "Play" : "Pause"; onClicked: Playback.togglePause() }
                    IconButton { icon: "next"; iconSize: Style.fs.md; diameter: Style.sp(6.25); tip: "Next"; onClicked: Playback.next() }
                    Item { Layout.fillWidth: true }
                    Rectangle {
                        implicitWidth: queueRow.implicitWidth + Style.sp(3)
                        implicitHeight: Style.sp(6.25)
                        radius: Style.radius
                        color: queueHover.hovered ? Tokens.tint10 : "transparent"
                        border.width: 1
                        border.color: queueHover.hovered ? Tokens.lineStrong : Tokens.line
                        RowLayout {
                            id: queueRow
                            anchors.centerIn: parent
                            spacing: Style.sp(1.25)
                            Icon { name: "queue"; size: Style.fs.md; color: queueHover.hovered ? Tokens.ink : Tokens.inkMuted }
                            Text { text: "QUEUE"; color: queueHover.hovered ? Tokens.ink : Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1 }
                        }
                        HoverHandler { id: queueHover; cursorShape: Qt.PointingHandCursor }
                        TapHandler { onTapped: root.openNowPlaying("queue") }
                    }
                }
            }

            // idle
            ColumnLayout {
                visible: !root.now
                Layout.fillWidth: true
                Layout.fillHeight: true
                spacing: 0
                Item { Layout.fillHeight: true }
                Text { text: "音"; color: Tokens.inkFaint; font.family: Tokens.jp; font.pixelSize: Style.fs.hero * 0.9 }
                Text { Layout.topMargin: Style.sp(1.75); text: "Ready to listen."; color: Tokens.inkDim; font.family: Tokens.display; font.pixelSize: Style.fs.lg }
                Text {
                    Layout.topMargin: Style.sp(1)
                    Layout.fillWidth: true
                    text: "Search, open your library, or pick up a recent session."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    wrapMode: Text.WordWrap
                }
                Item { Layout.fillHeight: true }
            }
        }
    }
}
