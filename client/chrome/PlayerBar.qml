pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"
import "../lib/ids.js" as Ids

// The persistent transport (spec section 3): three zones with the transport centred on the WINDOW.
// The left (now playing) and right (tools + volume) zones flow in a full-width RowLayout with a fixed
// 280 px each; the transport + seek is a separate column anchored to the bar's horizontal centre, so
// it sits at the window's midline whatever the side content does. It holds no truth: every control is
// a Playback method (a daemon call) and every readout is Playback state, save the two sanctioned
// optimisms — the held seek thumb (Playback.seekDrag) and the volume slider while dragged
// (Playback.volDrag). Queue / Lyrics select App's right panel; expand raises the Now Playing stage;
// the Sound dialog opens from the tools. The wave is gone: the seek is a hairline with an accent fill.
Rectangle {
    id: root

    signal toggleQueue()
    signal toggleLyrics()
    signal toggleNowPlaying()
    signal soundClicked()

    // App state, one-way in: the right panel's open flag and active tab, and the stage's open flag.
    property bool panelOpen: false
    property string panelTab: "queue"
    property bool nowPlayingOpen: false

    // Volume level to return to when un-muting (mute is just volume 0).
    property int preMute: 100

    readonly property var now: Playback.now
    readonly property bool live: !Playback.paused && !!Playback.now
    readonly property bool hasYouTubeTrack: !!Playback.now
        && !Ids.isLocalId(Playback.now.videoId) && !Ids.isRadioId(Playback.now.videoId)
    readonly property bool isRadioNow: !!Playback.now && Ids.isRadioId(Playback.now.videoId)
    readonly property bool scNow: !!Playback.now && String(Playback.now.videoId).startsWith("sc:")
    readonly property bool autoplayTrack: {
        var q = Playback.queue;
        var cur = (q && q.items) ? q.items[q.currentIndex] : null;
        return !!(cur && cur.autoplay && Playback.now && cur.video_id === Playback.now.videoId);
    }

    // The fixed side zone width (280) and the outer margin, shared by the flow layout and the centre's
    // width clamp so the transport never collides with the side content on a narrow window.
    readonly property int zoneW: Style.sp(70)
    readonly property int edge: Style.sp(6)

    implicitHeight: Style.playerBarH
    color: Qt.rgba(Tokens.paper.r, Tokens.paper.g, Tokens.paper.b, 0.9)

    function toggleLike() {
        var n = Playback.now;
        if (!n || !root.hasYouTubeTrack)
            return;
        var next = Playback.rating === "like" ? "indifferent" : "like";
        Playback.rating = next;
        Daemon.call("rate", { videoId: n.videoId, rating: next })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not rate", "error"));
    }

    function toggleMute() {
        var muted = Playback.volume === 0;
        if (!muted)
            root.preMute = Playback.volume;
        var v = muted ? (root.preMute || 100) : 0;
        Playback.volume = v;
        Playback.setVolume(v);
        Daemon.call("set_setting", { key: "volume", value: String(v) }).catch(() => {});
    }

    function nudgeVolume(delta) {
        var v = Math.max(0, Math.min(100, Playback.volume + delta));
        Playback.volume = v;
        Playback.setVolume(v);
        volSettle.restart();
    }

    // Persist the level once a run of wheel notches settles (one fsync per gesture, like the Svelte).
    Timer {
        id: volSettle
        interval: 400
        onTriggered: Daemon.call("set_setting", { key: "volume", value: String(Playback.volume) }).catch(() => {})
    }

    Hairline { anchors.top: parent.top; width: parent.width; height: 1 }

    // ── side zones (left now-playing, right tools) flowing across the bar ───────────────────
    RowLayout {
        anchors.fill: parent
        anchors.leftMargin: root.edge
        anchors.rightMargin: root.edge
        spacing: Style.sp(4)

        // now playing (left, 280)
        RowLayout {
            Layout.preferredWidth: root.zoneW
            Layout.fillWidth: false
            spacing: Style.sp(3)

            Artwork {
                url: root.now && root.now.thumbnail ? root.now.thumbnail : ""
                px: Style.sp(12)
            }

            ColumnLayout {
                Layout.fillWidth: true
                spacing: 2
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(1.5)
                    Text {
                        Layout.fillWidth: true
                        text: root.now && root.now.title ? root.now.title : "Nothing playing"
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        font.weight: Font.Medium
                        elide: Text.ElideRight
                    }
                    Text {
                        visible: !!Playback.pendingVideoId
                        text: "RESOLVING"
                        color: Tokens.inkFaint
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.xs
                        font.letterSpacing: 1
                    }
                    Icon {
                        visible: root.autoplayTrack
                        name: "infinity"
                        size: Style.fs.sm
                        color: Tokens.inkMuted
                    }
                }
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(1)
                    // The provider tell for the playing track: spotify glyph for spotify: ids, the
                    // YouTube Music glyph otherwise.
                    Icon {
                        visible: !!root.now
                        name: root.now ? (String(root.now.videoId).startsWith("sc:") ? "soundcloud"
                            : String(root.now.videoId).startsWith("spotify:") ? "spotify" : "youtube-music") : "youtube-music"
                        size: Style.sp(3.5)
                        color: Tokens.inkMuted
                    }
                    Text {
                        Layout.fillWidth: true
                        text: root.now && root.now.artists ? root.now.artists : ""
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        elide: Text.ElideRight
                    }
                }
            }

            IconButton {
                visible: root.hasYouTubeTrack
                icon: "heart"
                iconSize: Style.fs.md
                diameter: Style.sp(8)
                active: Playback.rating === "like"
                iconColor: Playback.rating === "like" ? Style.accent : Tokens.inkMuted
                onClicked: root.toggleLike()
            }
        }

        // the gap the centred transport floats over
        Item { Layout.fillWidth: true; Layout.fillHeight: true }

        // tools + volume (right, 280)
        RowLayout {
            Layout.preferredWidth: root.zoneW
            Layout.fillWidth: false
            layoutDirection: Qt.RightToLeft
            spacing: Style.sp(1)

            IconButton {
                icon: "expand"
                iconSize: Style.fs.lg
                diameter: Style.sp(8)
                active: root.nowPlayingOpen
                onClicked: root.toggleNowPlaying()
            }
            IconButton {
                icon: "queue"
                iconSize: Style.fs.lg
                diameter: Style.sp(8)
                active: root.panelOpen && root.panelTab === "queue"
                onClicked: root.toggleQueue()
            }
            IconButton {
                icon: "mic"
                iconSize: Style.fs.lg
                diameter: Style.sp(8)
                enabled: !root.isRadioNow
                active: root.panelOpen && root.panelTab === "lyrics"
                onClicked: root.toggleLyrics()
            }
            IconButton {
                icon: "sound"
                iconSize: Style.fs.lg
                diameter: Style.sp(8)
                onClicked: root.soundClicked()
            }

            Item { Layout.fillWidth: true }

            // volume group
            RowLayout {
                Layout.alignment: Qt.AlignVCenter
                spacing: Style.sp(1)
                layoutDirection: Qt.LeftToRight

                IconButton {
                    icon: Playback.volume === 0 ? "volume-mute" : "volume"
                    iconSize: Style.fs.md
                    diameter: Style.sp(8)
                    onClicked: root.toggleMute()
                }
                Slider {
                    id: volSlider
                    Layout.preferredWidth: Style.sp(24)
                    Layout.alignment: Qt.AlignVCenter
                    fillColor: Tokens.ink
                    from: 0
                    to: 100
                    value: Playback.volume
                    onPressedChanged: Playback.volDrag = pressed
                    onMoved: (v) => {
                        var iv = Math.round(v);
                        Playback.volume = iv;
                        Playback.setVolume(iv);
                    }
                    onCommitted: (v) => {
                        var iv = Math.round(v);
                        Playback.volume = iv;
                        Playback.setVolume(iv);
                        Daemon.call("set_setting", { key: "volume", value: String(iv) }).catch(() => {});
                        Playback.volDrag = false;
                    }
                    WheelHandler {
                        acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
                        onWheel: (e) => root.nudgeVolume(e.angleDelta.y > 0 ? 5 : -5)
                    }
                }
            }
        }
    }

    // ── transport + seek, anchored to the WINDOW's midline (max 480 wide) ───────────────────
    ColumnLayout {
        anchors.horizontalCenter: parent.horizontalCenter
        anchors.verticalCenter: parent.verticalCenter
        width: Math.min(Style.sp(120), root.width - 2 * (root.zoneW + root.edge + Style.sp(4)))
        spacing: Style.sp(1)

        RowLayout {
            Layout.alignment: Qt.AlignHCenter
            spacing: Style.sp(1)
            IconButton {
                icon: "shuffle"
                iconSize: Style.fs.md
                diameter: Style.sp(8)
                active: !!(Playback.queue && Playback.queue.shuffle)
                onClicked: Playback.toggleShuffle()
            }
            IconButton {
                icon: "previous"
                iconSize: Style.fs.lg
                diameter: Style.sp(8)
                onClicked: Playback.prev()
            }
            IconButton {
                icon: Playback.paused ? "play" : "pause"
                iconSize: Style.fs.lg
                diameter: Style.sp(10)
                primary: true
                onClicked: Playback.togglePause()
            }
            IconButton {
                icon: "next"
                iconSize: Style.fs.lg
                diameter: Style.sp(8)
                onClicked: Playback.next()
            }
            IconButton {
                icon: (Playback.queue && Playback.queue.repeat === "one") ? "repeat-one" : "repeat"
                iconSize: Style.fs.md
                diameter: Style.sp(8)
                active: !!(Playback.queue && Playback.queue.repeat && Playback.queue.repeat !== "off")
                onClicked: Playback.cycleRepeat()
            }
        }

        RowLayout {
            Layout.fillWidth: true
            spacing: Style.sp(2)
            Text {
                text: Style.fmtTime(Playback.shownPosition)
                color: Tokens.inkFaint
                font.family: Style.fontMono
                font.pixelSize: Style.fs.xs
            }
            Slider {
                id: seekSlider
                visible: !root.scNow
                Layout.fillWidth: true
                Layout.alignment: Qt.AlignVCenter
                thickness: 2
                handleSize: Style.sp(2.5)
                trackColor: Tokens.lineStrong
                fillColor: root.live ? Style.accent : Tokens.ink
                from: 0
                to: Playback.duration > 0 ? Playback.duration : 1
                value: Playback.shownPosition
                onMoved: (v) => Playback.seekDrag = v
                onCommitted: (v) => { Playback.seek(v); Playback.seekDrag = NaN; }
            }
            // A SoundCloud track swaps the hairline seek for the 28 px waveform (played in orange).
            Waveform {
                visible: root.scNow
                Layout.fillWidth: true
                Layout.alignment: Qt.AlignVCenter
                Layout.preferredHeight: Style.sp(7)
                samples: Playback.waveform || []
            }
            Text {
                text: Style.fmtTime(Playback.duration)
                color: Tokens.inkFaint
                font.family: Style.fontMono
                font.pixelSize: Style.fs.xs
            }
        }
    }
}
