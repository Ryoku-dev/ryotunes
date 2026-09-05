pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui as RU
import Ryoku.Ui.Singletons
import "../"
import "../lib/ids.js" as Ids

// The Home hero's listening console (spec section 4): a fixed 560 x 160 card with a square 160 px
// artwork on the left and, over its foot, the shared Spectrum drawn as bars ramped from the
// artwork accent into ink — claimed with id "deck" while the deck is visible, so cava only runs
// when this surface is on screen and something is playing. The right column carries the
// PLAYBACK / STREAM readout, the track title in xl Fraunces, the QUEUE / LEVEL / SPEED figures, a
// hairline progress and the transport.
Rectangle {
    id: root

    signal openNowPlaying(string tab)

    implicitWidth: Style.sp(140)    // 560
    implicitHeight: Style.sp(40)    // 160
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
    readonly property string speedLabel: Playback.audioFx.speed.toFixed(2) + "×"

    // The deck claims the spectrum feed only while it is visible (Spectrum runs it only when a
    // claim is live AND Style.ambient holds — playing, motion on, not power-saver).
    function claim(on) { Spectrum.claim("deck", on); }
    Component.onCompleted: root.claim(root.visible)
    Component.onDestruction: root.claim(false)
    onVisibleChanged: root.claim(root.visible)

    RowLayout {
        anchors.fill: parent
        spacing: 0

        // --- art (square 160) + spectrum foot -------------------------------------------
        Item {
            id: art
            Layout.preferredWidth: Style.sp(40)
            Layout.fillHeight: true
            clip: true

            Rectangle { anchors.fill: parent; color: Tokens.paperLift }

            Image {
                id: cover
                anchors.fill: parent
                source: root.now && root.now.thumbnail ? Style.thumb(root.now.thumbnail, 320) : ""
                sourceSize: Qt.size(320, 320)
                fillMode: Image.PreserveAspectCrop
                asynchronous: true
                cache: true
                opacity: status === Image.Ready ? 1 : 0
                Behavior on opacity { NumberAnimation { duration: Style.motion.slow } }
            }

            // idle: orbit rings and a note
            Item {
                id: orbit
                anchors.centerIn: art
                visible: cover.status !== Image.Ready
                width: Style.sp(20); height: width
                Repeater {
                    model: [0, Style.sp(3), Style.sp(6.5)]
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
                Icon { anchors.centerIn: orbit; name: "music"; size: Style.sp(6); color: Tokens.inkMuted }
            }

            // bottom wash so the bars read over any artwork
            Rectangle {
                anchors.fill: parent
                visible: cover.status === Image.Ready
                gradient: Gradient {
                    GradientStop { position: 0.45; color: "transparent" }
                    GradientStop { position: 1.0; color: Qt.rgba(Tokens.paper.r, Tokens.paper.g, Tokens.paper.b, 0.75) }
                }
            }

            // the spectrum, ramped from the cover accent into ink
            RU.SpectrumField {
                anchors.fill: parent
                levels: Spectrum.levels
                energy: Spectrum.energy
                fade: root.live ? 1 : 0
                ramp: [Style.accent, Tokens.ink]
                style: "bars"
                boxX: 0; boxY: 0.52; boxW: 1; boxH: 0.48
                grow: "up"
                thickness: 0.5
                glow: 0.45
                Behavior on fade { NumberAnimation { duration: Style.motion.slow } }
            }

            // LIVE tag, its dot breathing under ambient motion
            Row {
                x: Style.sp(2); y: Style.sp(2)
                spacing: Style.sp(1)
                Rectangle {
                    anchors.verticalCenter: parent.verticalCenter
                    width: Style.sp(1.5); height: width; radius: height / 2
                    color: Style.accent
                    opacity: 0.9
                    SequentialAnimation on opacity {
                        running: Style.ambient
                        loops: Animation.Infinite
                        alwaysRunToEnd: true
                        NumberAnimation { from: 0.9; to: 0.3; duration: 500; easing.type: Easing.InOutSine }
                        NumberAnimation { from: 0.3; to: 0.9; duration: 500; easing.type: Easing.InOutSine }
                    }
                }
                Text {
                    anchors.verticalCenter: parent.verticalCenter
                    text: "LIVE"
                    color: Qt.rgba(Tokens.ink.r, Tokens.ink.g, Tokens.ink.b, 0.8)
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
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

        // --- copy (readout, title, figures, progress, transport) ------------------------
        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            Layout.margins: Style.sp(2)
            spacing: 0

            // PLAYBACK / STREAM
            RowLayout {
                Layout.fillWidth: true
                spacing: Style.sp(1.5)
                Text { text: "PLAYBACK"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
                Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; color: Tokens.lineSoft }
                Text { text: root.sourceLabel; color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
            }

            // playing
            ColumnLayout {
                visible: !!root.now
                Layout.fillWidth: true
                Layout.fillHeight: true
                spacing: 0

                Text {
                    Layout.fillWidth: true
                    Layout.topMargin: Style.sp(1.5)
                    text: root.now ? root.now.title : ""
                    color: trackHover.hovered ? Tokens.inkDim : Tokens.ink
                    font.family: Tokens.display
                    font.pixelSize: Style.fs.xl
                    elide: Text.ElideRight
                    HoverHandler { id: trackHover; cursorShape: Qt.PointingHandCursor }
                    TapHandler { onTapped: root.openNowPlaying("queue") }
                }
                Text {
                    Layout.fillWidth: true
                    text: root.now ? root.now.artists : ""
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    elide: Text.ElideRight
                }

                Item { Layout.fillHeight: true; Layout.preferredHeight: 1 }

                // QUEUE / LEVEL / SPEED
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(2)
                    Repeater {
                        model: [
                            { k: "QUEUE", v: String(root.queueLeft) },
                            { k: "LEVEL", v: Math.round(Playback.volume) + "%" },
                            { k: "SPEED", v: root.speedLabel }
                        ]
                        delegate: ColumnLayout {
                            id: readout
                            required property var modelData
                            Layout.fillWidth: true
                            spacing: 1
                            Text { text: readout.modelData.k; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
                            Text { text: readout.modelData.v; color: Tokens.inkDim; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; font.weight: Font.Medium }
                        }
                    }
                }

                // hairline progress
                RowLayout {
                    Layout.fillWidth: true
                    Layout.topMargin: Style.sp(1.5)
                    spacing: Style.sp(1.5)
                    Text { Layout.preferredWidth: Style.sp(8); text: Style.fmtTime(Playback.position); color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs }
                    Item {
                        Layout.fillWidth: true
                        Layout.preferredHeight: 2
                        Rectangle { anchors.verticalCenter: parent.verticalCenter; width: parent.width; height: 2; radius: 1; color: Tokens.lineStrong }
                        Rectangle { anchors.verticalCenter: parent.verticalCenter; width: parent.width * root.progress; height: 2; radius: 1; color: Style.accent }
                    }
                    Text { Layout.preferredWidth: Style.sp(8); horizontalAlignment: Text.AlignRight; text: Style.fmtTime(Playback.duration); color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs }
                }

                // transport
                RowLayout {
                    Layout.fillWidth: true
                    Layout.topMargin: Style.sp(1.25)
                    spacing: Style.sp(1)
                    IconButton { icon: "previous"; iconSize: Style.fs.md; diameter: Style.sp(6); tip: "Previous"; onClicked: Playback.prev() }
                    IconButton { icon: Playback.paused ? "play" : "pause"; iconSize: Style.fs.md; diameter: Style.sp(6); primary: true; tip: Playback.paused ? "Play" : "Pause"; onClicked: Playback.togglePause() }
                    IconButton { icon: "next"; iconSize: Style.fs.md; diameter: Style.sp(6); tip: "Next"; onClicked: Playback.next() }
                    Item { Layout.fillWidth: true }
                    IconButton { icon: "queue"; iconSize: Style.fs.md; diameter: Style.sp(6); outlined: true; tip: "Queue"; onClicked: root.openNowPlaying("queue") }
                }
            }

            // idle
            ColumnLayout {
                visible: !root.now
                Layout.fillWidth: true
                Layout.fillHeight: true
                spacing: 0
                Item { Layout.fillHeight: true }
                Text { text: "音"; color: Tokens.inkFaint; font.family: Tokens.jp; font.pixelSize: Style.fs.xl }
                Text { Layout.topMargin: Style.sp(1); text: "Ready to listen."; color: Tokens.inkDim; font.family: Tokens.display; font.pixelSize: Style.fs.lg }
                Text {
                    Layout.topMargin: Style.sp(0.5)
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
