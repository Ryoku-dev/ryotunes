pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui as RU
import Ryoku.Ui.Singletons
import "../"
import "../lib/ids.js" as Ids

// The Home hero's Now Playing card (spec "Home hero"): a ~550 x 125 paper card with a square, full
// height artwork on the left (the shared Spectrum ramped over its foot while playing) and, on the
// right, the PLAYBACK / STREAM readout, the track title in Fraunces, the artist, three mono stat
// cells that surface Ryotunes' Sound effects (RATIO / SPEED / TEMPO), a tiny prev / play / next
// transport at the foot and a QUEUE pill that opens the Now Playing stage's queue tab. It only
// exists while something plays; HomePage hides it (and drops to a single column) on a narrow page,
// so it can never overlap the greeting column.
Rectangle {
    id: root

    // Asks the App to open the Now Playing stage on its queue tab (App wires
    // Playback.nowPlayingRequested -> npOpenTab, so this reaches the stage from anywhere).
    signal openQueue()

    implicitWidth: Style.sp(138)    // ~550
    implicitHeight: cardRow.implicitHeight
    radius: Style.radiusCard
    color: Tokens.paper
    border.width: 1
    border.color: Tokens.line
    clip: true

    readonly property var now: Playback.now
    readonly property bool live: !!now && !Playback.paused
    readonly property string sourceLabel: !now ? "READY"
        : Ids.isLocalId(now.videoId) ? "LOCAL"
        : (now.streamClient === "cache" ? "CACHE" : "STREAM")

    // The three figures showcase the Sound chain (spec section 7): the stereo width as a ratio, the
    // tempo multiplier, and the pitch shift in semitones — the state normal players never expose.
    readonly property string ratioLabel: Math.round(Playback.audioFx.width * 100) + "%"
    readonly property string speedLabel: Playback.audioFx.speed.toFixed(2) + "×"
    readonly property string tempoLabel: {
        var s = Math.round(Playback.audioFx.semitones);
        return (s > 0 ? "+" : "") + s + " st";
    }

    // A SoundCloud track shows its waveform and the plays / likes / genre line in place of the
    // Sound-effect figures, the Orange look on Home.
    readonly property bool scNow: !!root.now && String(root.now.videoId).startsWith("sc:")
    readonly property string scMeta: {
        var n = root.now;
        if (!n)
            return "";
        var parts = [];
        var p = Style.fmtCount(n.plays);
        if (p) parts.push(p + " plays");
        var l = Style.fmtCount(n.likes);
        if (l) parts.push(l + " likes");
        if (n.genre) parts.push(String(n.genre));
        return parts.join("  \u00b7  ");
    }

    // The card claims the spectrum feed only while it is on screen (Spectrum runs cava only when a
    // claim is live AND Style.live holds — something is playing).
    function claim(on) { Spectrum.claim("npcard", on); }
    Component.onCompleted: root.claim(root.visible)
    Component.onDestruction: root.claim(false)
    onVisibleChanged: root.claim(root.visible)

    RowLayout {
        id: cardRow
        anchors.fill: parent
        spacing: 0

        // --- art (square, full height) + spectrum foot -------------------------------------
        Item {
            id: art
            Layout.fillHeight: true
            Layout.preferredWidth: root.height
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

            // idle placeholder: a single note (the card is only shown while playing, so this only
            // flashes before the first cover decodes)
            Icon {
                anchors.centerIn: parent
                visible: cover.status !== Image.Ready
                name: "music"
                size: Style.sp(6)
                color: Tokens.inkMuted
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
                cursorShape: Qt.PointingHandCursor
                onClicked: root.openQueue()
            }
            Rectangle { anchors.right: parent.right; width: 1; height: parent.height; color: Tokens.line }
        }

        // --- readout, title, figures, transport --------------------------------------------
        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            Layout.leftMargin: Style.sp(3)
            Layout.rightMargin: Style.sp(3)
            Layout.topMargin: Style.sp(1.5)
            Layout.bottomMargin: Style.sp(1.5)
            spacing: Style.sp(0.5)

            // PLAYBACK ——— STREAM
            RowLayout {
                Layout.fillWidth: true
                spacing: Style.sp(1.5)
                Text { text: "PLAYBACK"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
                Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; color: Tokens.lineSoft }
                Text { text: root.sourceLabel; color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
            }

            Text {
                Layout.fillWidth: true
                Layout.topMargin: Style.sp(0.5)
                text: root.now ? root.now.title : ""
                color: titleHover.hovered ? Tokens.inkDim : Tokens.ink
                font.family: Style.fontDisplay
                font.pixelSize: Style.fs.xl
                elide: Text.ElideRight
                HoverHandler { id: titleHover; cursorShape: Qt.PointingHandCursor }
                TapHandler { onTapped: root.openQueue() }
            }
            Text {
                Layout.fillWidth: true
                text: root.now ? root.now.artists : ""
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                elide: Text.ElideRight
            }
            Text {
                Layout.fillWidth: true
                visible: root.scNow && root.scMeta !== ""
                text: root.scMeta
                color: Tokens.inkFaint
                font.family: Style.fontMono
                font.pixelSize: Style.fs.micro
                font.letterSpacing: Style.trackMicro
                elide: Text.ElideRight
            }

            // RATIO / SPEED / TEMPO (a SoundCloud track shows its waveform instead)
            RowLayout {
                visible: !root.scNow
                Layout.fillWidth: true
                spacing: Style.sp(2)
                Repeater {
                    model: [
                        { k: "RATIO", v: root.ratioLabel },
                        { k: "SPEED", v: root.speedLabel },
                        { k: "TEMPO", v: root.tempoLabel }
                    ]
                    delegate: ColumnLayout {
                        id: cell
                        required property var modelData
                        Layout.fillWidth: true
                        spacing: 1
                        Text { text: cell.modelData.k; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.micro; font.letterSpacing: Style.trackMicro }
                        Text { text: cell.modelData.v; color: Tokens.inkDim; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; font.weight: Font.Medium }
                    }
                }
            }

            // SoundCloud waveform, in the figures' place
            Waveform {
                visible: root.scNow
                Layout.fillWidth: true
                Layout.preferredHeight: 56
                samples: Playback.waveform || []
            }

            // transport (prev / play / next) + queue pill
            RowLayout {
                Layout.fillWidth: true
                Layout.topMargin: Style.sp(1)
                spacing: Style.sp(1)
                IconButton { icon: "previous"; iconSize: Style.fs.md; diameter: Style.sp(7); tip: "Previous"; onClicked: Playback.prev() }
                IconButton { icon: Playback.paused ? "play" : "pause"; iconSize: Style.fs.md; diameter: Style.sp(7); primary: true; tip: Playback.paused ? "Play" : "Pause"; onClicked: Playback.togglePause() }
                IconButton { icon: "next"; iconSize: Style.fs.md; diameter: Style.sp(7); tip: "Next"; onClicked: Playback.next() }
                Item { Layout.fillWidth: true }

                // QUEUE pill
                Rectangle {
                    Layout.alignment: Qt.AlignVCenter
                    implicitHeight: Style.sp(7)
                    implicitWidth: pillRow.implicitWidth + Style.sp(5)
                    radius: height / 2
                    color: pillHover.hovered ? Tokens.tint10 : "transparent"
                    border.width: 1
                    border.color: pillHover.hovered ? Tokens.lineStrong : Tokens.line
                    Behavior on color { ColorAnimation { duration: Style.motion.snap } }
                    Row {
                        id: pillRow
                        anchors.centerIn: parent
                        spacing: Style.sp(1.5)
                        Icon { anchors.verticalCenter: parent.verticalCenter; name: "queue"; size: Style.fs.sm; color: Tokens.inkMuted }
                        Text {
                            anchors.verticalCenter: parent.verticalCenter
                            text: "QUEUE"
                            color: Tokens.inkMuted
                            font.family: Style.fontMono
                            font.pixelSize: Style.fs.micro
                            font.letterSpacing: Style.trackMicro
                        }
                    }
                    HoverHandler { id: pillHover; cursorShape: Qt.PointingHandCursor }
                    MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: root.openQueue() }
                }
            }
        }
    }
}
