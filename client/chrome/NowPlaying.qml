pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import Ryoku.Ui as RU
import "../"
import "../components"

// The Now Playing stage (spec section 3): the playing surface at full size, artwork-first. It
// replaces the content area and the right panel when open. The cover fills the left column over a
// 0.6 Backdrop wash, lit by a static accent glow that re-renders only on track change; the lyrics
// stage fills the right column; a Spectrum ribbon runs along the foot while a track plays. It holds
// no state beyond the play/pause flash on the artwork — App owns the open state and the active tab
// through nowPlayingOpen / queueOpen / lyricsOpen, and this surface asks for changes through
// tabRequested / closeRequested so the player-bar toggles stay in lockstep. The Queue lives in
// App's right panel; the stage is the immersive lyrics view.
Item {
    id: root
    // The 0.6 backdrop blurs past its own bounds; clip so the wash never leaks over the title bar
    // or the rail.
    clip: true

    // App's coupling flags, one-way in; writes go back as signals so App owns the mutation.
    property bool nowPlayingOpen: false
    property bool queueOpen: false
    property bool lyricsOpen: false
    signal tabRequested(string tab)
    signal closeRequested()

    readonly property bool open: root.nowPlayingOpen || root.queueOpen || root.lyricsOpen

    // The stage is present while open with a track; it fades in/out (opacity only) so collapsing
    // reads as a soft dismissal — 200 ms enter, faster 150 ms exit, both OutCubic.
    readonly property bool shown: root.open && !!Playback.now
    visible: opacity > 0
    opacity: root.shown ? 1 : 0
    Behavior on opacity {
        NumberAnimation { duration: root.shown ? 200 : 150; easing.type: Easing.OutCubic }
    }

    // The current queue item, for the album meta the now-playing snapshot does not carry.
    readonly property var nowItem: {
        var q = Playback.queue;
        if (!q || !q.items || !q.items.length)
            return null;
        var i = q.currentIndex >= 0 ? q.currentIndex : 0;
        return q.items.length > i ? q.items[i] : null;
    }

    // A SoundCloud track shows its plays / likes / genre in the meta line and its waveform under
    // the title, the Orange look at full size.
    readonly property bool scNow: !!Playback.now && String(Playback.now.videoId).startsWith("sc:")

    // The spectrum ribbon claims the analyser only while the stage is on screen.
    onVisibleChanged: Spectrum.claim("stage", root.visible)
    Component.onCompleted: Spectrum.claim("stage", root.visible)

    // --- artwork play/pause flash -----------------------------------------------------------
    property string flash: ""
    Timer { id: flashTimer; interval: 240; onTriggered: root.flash = "" }
    function toggle() {
        root.flash = Playback.paused ? "play" : "pause";
        flashTimer.restart();
        Playback.togglePause();
    }

    // Opaque base + click swallow, so the routed page behind never takes a stray click.
    Rectangle {
        anchors.fill: parent
        color: Tokens.paper
        MouseArea { anchors.fill: parent }
    }

    // The playing cover as the room's light, stronger here than on a page (spec 1).
    Backdrop {
        anchors.fill: parent
        strength: 0.24
    }

    ColumnLayout {
        id: stage
        anchors.fill: parent
        anchors.margins: Style.sp(12)
        anchors.topMargin: Style.sp(8)
        spacing: Style.sp(6)

        // ── header: eyebrow left, the Collapse pill right (in flow, so nothing sits under it) ──
        RowLayout {
            Layout.fillWidth: true
            spacing: Style.sp(4)
            Text {
                Layout.fillWidth: true
                text: (root.scNow ? "SOUNDCLOUD" : (Playback.provider === "spotify" ? "SPOTIFY" : "YOUTUBE MUSIC")) + "  \u00b7  NOW PLAYING"
                color: Tokens.inkFaint
                font.family: Style.fontMono
                font.pixelSize: Style.fs.micro
                font.letterSpacing: Style.trackMicro
            }
            Rectangle {
                id: collapsePill
                implicitWidth: collapseRow.implicitWidth + Style.sp(6)
                implicitHeight: Style.sp(9)
                radius: height / 2
                color: collapseHover.hovered ? Tokens.tint5 : Tokens.paperLift
                border.width: 1
                border.color: collapseHover.hovered ? Tokens.lineStrong : Tokens.line
                Behavior on color { ColorAnimation { duration: Style.motion.snap } }
                Behavior on border.color { ColorAnimation { duration: Style.motion.snap } }

                RowLayout {
                    id: collapseRow
                    anchors.centerIn: parent
                    spacing: Style.sp(1.5)
                    Text {
                        text: "Collapse"
                        color: Tokens.inkDim
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        font.weight: Font.Medium
                    }
                    Icon { name: "chevron-down"; size: Style.fs.md; color: Tokens.inkDim }
                }

                HoverHandler { id: collapseHover }
                MouseArea {
                    anchors.fill: parent
                    cursorShape: Qt.PointingHandCursor
                    onClicked: root.closeRequested()
                }
            }
        }

        // ── body: art + track (left, top-aligned) | lyrics (right), tops on one line ──────
        RowLayout {
            id: body
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: Style.sp(12)

            ColumnLayout {
                id: artCol
                Layout.fillHeight: true
                Layout.fillWidth: false
                Layout.preferredWidth: Math.min(Style.sp(110), Math.round(root.width * 0.34))
                spacing: Style.sp(5)
                // The cover is as wide as the column unless the height says otherwise: the title
                // block (~3 lines) must always fit beneath it.
                readonly property int artPx: Math.max(Style.sp(40),
                    Math.min(width, body.height - Style.sp(30)))

                Artwork {
                    id: bigArt
                    Layout.alignment: Qt.AlignTop | Qt.AlignLeft
                    Layout.preferredWidth: artCol.artPx
                    Layout.preferredHeight: artCol.artPx
                    px: artCol.artPx
                    cornerRadius: Style.radiusCard
                    url: (Playback.now && Playback.now.thumbnail) ? Playback.now.thumbnail : ""
                    placeholderIcon: "music"

                    MouseArea {
                        anchors.fill: parent
                        cursorShape: Qt.PointingHandCursor
                        onClicked: root.toggle()
                    }

                    // Flash the action taken, over the cover, so the click visibly did something.
                    Rectangle {
                        anchors.centerIn: parent
                        visible: root.flash !== ""
                        width: Style.sp(14)
                        height: width
                        radius: width / 2
                        color: Qt.rgba(0, 0, 0, 0.55)
                        Icon {
                            anchors.centerIn: parent
                            name: root.flash === "play" ? "play" : "pause"
                            size: Style.fs.xl
                            color: "#ffffff"
                        }
                    }
                }

                // title xl Fraunces · artist sm · meta micro
                ColumnLayout {
                    Layout.fillWidth: true
                    Layout.maximumWidth: artCol.artPx
                    spacing: Style.sp(1)
                    Text {
                        Layout.fillWidth: true
                        text: (Playback.now && Playback.now.title) ? Playback.now.title : ""
                        color: Tokens.ink
                        font.family: Style.fontDisplay
                        font.pixelSize: Style.fs.xl
                        maximumLineCount: 2
                        wrapMode: Text.WordWrap
                        elide: Text.ElideRight
                    }
                    Text {
                        Layout.fillWidth: true
                        text: (Playback.now && Playback.now.artists) ? Playback.now.artists : ""
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        elide: Text.ElideRight
                    }
                    Text {
                        Layout.fillWidth: true
                        visible: text !== ""
                        text: {
                            var parts = [];
                            if (root.scNow) {
                                var n = Playback.now;
                                var p = Style.fmtCount(n.plays);
                                if (p) parts.push(p + " PLAYS");
                                var l = Style.fmtCount(n.likes);
                                if (l) parts.push(l + " LIKES");
                                if (n.genre) parts.push(String(n.genre).toUpperCase());
                            } else if (root.nowItem && root.nowItem.album) {
                                parts.push(String(root.nowItem.album).toUpperCase());
                            }
                            if (Playback.duration > 0)
                                parts.push(Style.fmtTime(Playback.duration));
                            return parts.join("  \u00b7  ");
                        }
                        color: Tokens.inkFaint
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        font.letterSpacing: Style.trackMicro
                        elide: Text.ElideRight
                    }
                }

                Item { Layout.fillHeight: true }
            }

            LyricsPanel {
                Layout.fillWidth: true
                Layout.fillHeight: true
            }
        }

        // ── foot: ONE band. SoundCloud's own waveform is the seek bar; everything else gets the
        // live spectrum ribbon. Never both.
        Item {
            Layout.fillWidth: true
            Layout.preferredHeight: Style.sp(14)

            Waveform {
                anchors.fill: parent
                visible: root.scNow
                samples: Playback.waveform || []
            }
            RU.SpectrumField {
                anchors.fill: parent
                visible: !root.scNow
                levels: Spectrum.levels
                energy: Spectrum.energy
                style: "wave"
                ramp: [Style.accent, Tokens.ink]
                boxX: 0
                boxY: 0
                boxW: 1
                boxH: 1
                grow: "center"
                opacity: Spectrum.analysing ? 0.6 : 0.25
                Behavior on opacity { NumberAnimation { duration: Style.motion.swap } }
            }
        }
    }
}
