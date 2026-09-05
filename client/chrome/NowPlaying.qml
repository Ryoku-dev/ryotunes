pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import QtQuick.Effects
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

    // App's coupling flags, one-way in; writes go back as signals so App owns the mutation.
    property bool nowPlayingOpen: false
    property bool queueOpen: false
    property bool lyricsOpen: false
    signal tabRequested(string tab)
    signal closeRequested()

    readonly property bool open: root.nowPlayingOpen || root.queueOpen || root.lyricsOpen

    visible: root.open && !!Playback.now

    // The current queue item, for the album meta the now-playing snapshot does not carry.
    readonly property var nowItem: {
        var q = Playback.queue;
        if (!q || !q.items || !q.items.length)
            return null;
        var i = q.currentIndex >= 0 ? q.currentIndex : 0;
        return q.items.length > i ? q.items[i] : null;
    }

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
        strength: 0.6
    }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: Style.sp(6)
        spacing: Style.sp(4)

        RowLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: Style.sp(8)

            // ── artwork + track (left) ──────────────────────────────────────────────────
            ColumnLayout {
                Layout.fillHeight: true
                Layout.fillWidth: false
                Layout.preferredWidth: Math.min(Style.sp(120), root.width * 0.42)
                spacing: Style.sp(4)

                Item {
                    id: artHolder
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    readonly property int artPx: Math.max(Style.sp(40),
                        Math.min(Style.sp(120), Math.min(artHolder.width, artHolder.height)))

                    // A small static source of the cover, tinted to the accent and blurred once into
                    // a glow behind the art. It re-renders only when the source or the accent change
                    // — i.e. on track change — so the wash costs nothing per frame.
                    Image {
                        id: glowSource
                        anchors.centerIn: parent
                        width: 64
                        height: 64
                        source: (Playback.now && Playback.now.thumbnail) ? Style.thumb(Playback.now.thumbnail, 64) : ""
                        sourceSize: Qt.size(64, 64)
                        fillMode: Image.PreserveAspectCrop
                        asynchronous: true
                        cache: true
                        visible: false
                    }
                    MultiEffect {
                        anchors.centerIn: parent
                        width: artHolder.artPx * 1.12
                        height: artHolder.artPx * 1.12
                        source: glowSource
                        visible: Style.blurEnabled && glowSource.status === Image.Ready
                        blurEnabled: true
                        blur: 1.0
                        blurMax: 64
                        colorization: 1.0
                        colorizationColor: Style.accent
                        brightness: 0.1
                        opacity: 0.5
                    }

                    Artwork {
                        id: bigArt
                        anchors.centerIn: parent
                        px: artHolder.artPx
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
                }

                // title xl Fraunces · artist sm · meta micro
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(0.5)
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
                            if (root.nowItem && root.nowItem.album)
                                parts.push(String(root.nowItem.album));
                            if (Playback.duration > 0)
                                parts.push(Style.fmtTime(Playback.duration));
                            return parts.join("  ·  ").toUpperCase();
                        }
                        color: Tokens.inkFaint
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        font.letterSpacing: Style.trackMicro
                        elide: Text.ElideRight
                    }
                }
            }

            // ── lyrics stage (right) ────────────────────────────────────────────────────
            LyricsPanel {
                Layout.fillWidth: true
                Layout.fillHeight: true
            }
        }

        // ── spectrum ribbon (foot) ──────────────────────────────────────────────────────
        Item {
            Layout.fillWidth: true
            Layout.preferredHeight: Style.sp(20)
            RU.SpectrumField {
                anchors.fill: parent
                levels: Spectrum.levels
                energy: Spectrum.energy
                style: "wave"
                ramp: [Style.accent, Tokens.ink]
                boxX: 0
                boxY: 0
                boxW: 1
                boxH: 1
                grow: "center"
                opacity: Spectrum.analysing ? 1 : 0.35
                Behavior on opacity { NumberAnimation { duration: Style.motion.swap } }
            }
        }
    }

    // Close: collapse the stage back to the player bar.
    IconButton {
        anchors.top: parent.top
        anchors.right: parent.right
        anchors.margins: Style.sp(4)
        icon: "chevron-down"
        iconSize: Style.fs.lg
        diameter: Style.sp(9)
        onClicked: root.closeRequested()
    }
}
