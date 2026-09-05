pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"
import "../chrome"
import "../lib/ids.js" as Ids

// The compact player, the Tauri build's 724 x 356 widget (MiniPlayer.svelte, ryo-mini-v2): a full-
// height artwork column on the left with the track name over its foot, and on the right a head
// (brand, Now / Lyrics / Queue tabs, like, maximize), the body for the chosen tab and a footer with
// the transport and the volume. It lives in shell.qml's layer-shell PanelWindow; like every surface
// it holds no truth, it renders Playback and calls Playback. The lyrics word-timer is gated on the
// widget's real visibility (`active`) AND the lyrics tab, so a hidden mini never wakes it.
Item {
    id: root

    // True only while the widget is mapped; gates the lyrics timer off when hidden.
    property bool active: true
    // Raised when the user asks to return to the full window.
    signal maximize()
    // The item the header and artwork drag (the widget's box inside its layer surface), with the
    // box's travel bounds; dragEnded lets the window persist where it landed.
    property Item dragTarget: null
    property real dragMaxX: 0
    property real dragMaxY: 0
    signal dragEnded()

    property string view: "now"   // "now" | "lyrics" | "queue"
    property int preMute: 100

    readonly property var now: Playback.now
    readonly property bool hasYouTubeTrack: !!Playback.now
        && !Ids.isLocalId(Playback.now.videoId) && !Ids.isRadioId(Playback.now.videoId)
    readonly property var nextUp: {
        var q = Playback.queue;
        return (q && q.items) ? (q.items[q.currentIndex + 1] || null) : null;
    }
    readonly property int queueLeft: Math.max(0, Playback.queue.items.length - Playback.queue.currentIndex - 1)
    readonly property real radius: 16
    readonly property int artW: 232

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
    function mix(a, b, t) { return Qt.rgba(a.r + (b.r - a.r) * t, a.g + (b.g - a.g) * t, a.b + (b.b - a.b) * t, 1); }

    // The whole widget is clipped to a 16 px rounded rectangle by one shader layer, so the
    // artwork column and the panel share the same corners on a transparent layer surface.
    Item {
        id: panel
        anchors.fill: parent
        layer.enabled: true
        layer.effect: ShaderEffect {
            property vector2d size: Qt.vector2d(panel.width * panel.Screen.devicePixelRatio, panel.height * panel.Screen.devicePixelRatio)
            property real radius: root.radius * panel.Screen.devicePixelRatio
            fragmentShader: Qt.resolvedUrl("../shaders/rounded.frag.qsb")
        }

        // Paper tinted 3% toward the accent, the accent's glow ellipse off the top-left.
        Rectangle { anchors.fill: parent; color: root.mix(Tokens.paper, Style.accent, 0.03) }
        Canvas {
            id: glow
            anchors.fill: parent
            opacity: 0.68
            onPaint: {
                var c = getContext("2d");
                c.clearRect(0, 0, width, height);
                var g = c.createRadialGradient(width * 0.32, -30, 0, width * 0.32, -30, 300);
                var a = Style.accent;
                g.addColorStop(0, Qt.rgba(a.r, a.g, a.b, 0.11));
                g.addColorStop(0.7, Qt.rgba(a.r, a.g, a.b, 0));
                g.addColorStop(1, Qt.rgba(a.r, a.g, a.b, 0));
                c.fillStyle = g;
                c.fillRect(0, 0, width, height);
            }
            Connections { target: Style; function onAccentChanged() { glow.requestPaint(); } }
        }

        // ── artwork column ─────────────────────────────────────────────────────────────
        Item {
            id: art
            width: root.artW
            height: parent.height

            Rectangle { anchors.fill: parent; color: Tokens.paperLift }
            Image {
                id: cover
                anchors.fill: parent
                source: (root.now && root.now.thumbnail) ? Style.thumb(root.now.thumbnail, 480) : ""
                sourceSize: Qt.size(480, 480)
                fillMode: Image.PreserveAspectCrop
                asynchronous: true
                cache: true
                opacity: status === Image.Ready ? 1 : 0
                Behavior on opacity { NumberAnimation { duration: Tokens.durDefaultEffects } }
            }
            Icon {
                anchors.centerIn: parent
                visible: cover.status !== Image.Ready
                name: "music"
                size: 40
                color: Tokens.inkFaint
            }
            // Foot gradient so the copy reads over any cover.
            Rectangle {
                anchors.fill: parent
                gradient: Gradient {
                    GradientStop { position: 0.30; color: Qt.rgba(0, 0, 0, 0.01) }
                    GradientStop { position: 1.0; color: Qt.rgba(0, 0, 0, 0.72) }
                }
            }
            // "// LIVE" pill
            Rectangle {
                x: 16; y: 15
                width: liveText.implicitWidth + 16
                height: 20
                radius: 10
                color: Qt.rgba(7 / 255, 7 / 255, 8 / 255, 0.48)
                border.width: 1
                border.color: Qt.rgba(1, 1, 1, 0.18)
                Text {
                    id: liveText
                    anchors.centerIn: parent
                    text: (!!root.now && !Playback.paused) ? "// LIVE" : "// PAUSED"
                    color: Qt.rgba(1, 1, 1, 0.88)
                    font.family: Style.fontMono
                    font.pixelSize: 8
                    font.weight: Font.DemiBold
                    font.letterSpacing: 1.2
                }
            }
            ColumnLayout {
                anchors { left: parent.left; right: parent.right; bottom: parent.bottom; margins: 17 }
                anchors.bottomMargin: 18
                spacing: 5
                Text {
                    Layout.fillWidth: true
                    text: (root.now && root.now.title) ? root.now.title : "Nothing playing"
                    color: "white"
                    font.family: Style.fontUi
                    font.pixelSize: 14
                    font.weight: Font.DemiBold
                    font.letterSpacing: -0.14
                    elide: Text.ElideRight
                }
                Text {
                    Layout.fillWidth: true
                    text: (root.now && root.now.artists) ? root.now.artists : "Ryotunes is ready"
                    color: Qt.rgba(1, 1, 1, 0.70)
                    font.family: Style.fontUi
                    font.pixelSize: 10
                    elide: Text.ElideRight
                }
            }
            // Tap the cover to play / pause (the full window's preview does the same); drag it to
            // move the widget.
            HoverHandler { cursorShape: Qt.PointingHandCursor }
            TapHandler { enabled: !!root.now; onTapped: Playback.togglePause() }
            DragHandler {
                target: root.dragTarget
                xAxis.minimum: 0
                xAxis.maximum: root.dragMaxX
                yAxis.minimum: 0
                yAxis.maximum: root.dragMaxY
                onActiveChanged: if (!active) root.dragEnded()
            }
            Rectangle { anchors.right: parent.right; width: 1; height: parent.height; color: Qt.rgba(Tokens.ink.r, Tokens.ink.g, Tokens.ink.b, 0.12) }
        }

        // ── main column ────────────────────────────────────────────────────────────────
        ColumnLayout {
            id: main
            anchors { left: art.right; right: parent.right; top: parent.top; bottom: parent.bottom }
            anchors.leftMargin: 19
            anchors.rightMargin: 18
            anchors.topMargin: 11
            anchors.bottomMargin: 13
            spacing: 0

            // head: brand | tabs | like, maximize -- and the drag grip for the whole widget
            Item {
                id: head
                Layout.fillWidth: true
                Layout.preferredHeight: 54

                DragHandler {
                    target: root.dragTarget
                    xAxis.minimum: 0
                    xAxis.maximum: root.dragMaxX
                    yAxis.minimum: 0
                    yAxis.maximum: root.dragMaxY
                    onActiveChanged: if (!active) root.dragEnded()
                }

                RowLayout {
                    anchors.fill: parent
                    anchors.bottomMargin: 9
                    spacing: 14

                    RowLayout {
                        spacing: 8
                        Rectangle { Layout.preferredWidth: 17; Layout.preferredHeight: 1; color: Tokens.inkDim; opacity: 0.58 }
                        Text {
                            text: "力 RYOTUNES"
                            color: Tokens.inkDim
                            font.family: Style.fontUi
                            font.pixelSize: 8
                            font.weight: Font.Bold
                            font.letterSpacing: 1.35
                        }
                    }

                    Item { Layout.fillWidth: true }

                    // tabs
                    Rectangle {
                        implicitWidth: tabRow.implicitWidth + 6
                        implicitHeight: 40
                        radius: 12
                        color: Qt.rgba(Tokens.paperLift.r, Tokens.paperLift.g, Tokens.paperLift.b, 0.78)
                        border.width: 1
                        border.color: Tokens.lineSoft
                        RowLayout {
                            id: tabRow
                            anchors.centerIn: parent
                            spacing: 7
                            Repeater {
                                model: [
                                    { key: "now", icon: "music", tip: "Now playing" },
                                    { key: "lyrics", icon: "mic", tip: "Lyrics" },
                                    { key: "queue", icon: "queue", tip: "Queue" }
                                ]
                                delegate: Rectangle {
                                    id: tab
                                    required property var modelData
                                    readonly property bool on: root.view === tab.modelData.key
                                    Layout.preferredWidth: 38
                                    Layout.preferredHeight: 34
                                    radius: 9
                                    color: tab.on ? Tokens.paperLift : (tabHover.hovered ? Tokens.tint5 : "transparent")
                                    border.width: tab.on ? 1 : 0
                                    border.color: Tokens.line
                                    Icon {
                                        anchors.centerIn: parent
                                        name: tab.modelData.icon
                                        size: 16
                                        color: tab.on ? Tokens.ink : Tokens.inkFaint
                                    }
                                    Rectangle {
                                        visible: tab.on
                                        anchors { left: parent.left; right: parent.right; bottom: parent.bottom; leftMargin: 14; rightMargin: 14; bottomMargin: 3 }
                                        height: 2
                                        radius: 2
                                        color: root.mix(Style.accent, Tokens.ink, 0.42)
                                        opacity: 0.78
                                    }
                                    HoverHandler { id: tabHover; cursorShape: Qt.PointingHandCursor }
                                    TapHandler { onTapped: root.view = tab.modelData.key }
                                }
                            }
                        }
                    }

                    Item { Layout.fillWidth: true }

                    RowLayout {
                        spacing: 6
                        IconButton {
                            visible: root.hasYouTubeTrack
                            icon: "heart"; iconSize: 16; diameter: 32
                            active: Playback.rating === "like"
                            iconColor: Playback.rating === "like" ? Style.accent : Tokens.inkMuted
                            tip: "Like"
                            onClicked: root.toggleLike()
                        }
                        IconButton { icon: "rail-expand"; iconSize: 16; diameter: 32; tip: "Open full Ryotunes"; onClicked: root.maximize() }
                    }
                }
                Rectangle {
                    anchors { left: parent.left; right: parent.right; bottom: parent.bottom }
                    height: 1
                    color: Tokens.lineSoft
                }
            }

            // body
            Item {
                Layout.fillWidth: true
                Layout.fillHeight: true

                // NOW
                ColumnLayout {
                    anchors.fill: parent
                    visible: root.view === "now"
                    spacing: 0

                    Item { Layout.fillHeight: true }
                    ColumnLayout {
                        Layout.fillWidth: true
                        Layout.leftMargin: 4
                        Layout.rightMargin: 4
                        spacing: 8
                        Text {
                            Layout.fillWidth: true
                            text: (root.now && root.now.title) ? root.now.title : "Nothing playing"
                            color: Tokens.ink
                            font.family: Tokens.display
                            font.pixelSize: 29
                            font.weight: Font.Normal
                            font.letterSpacing: -0.8
                            elide: Text.ElideRight
                        }
                        Text {
                            Layout.fillWidth: true
                            text: (root.now && root.now.artists) ? root.now.artists : "Ryotunes is ready"
                            color: Tokens.inkMuted
                            font.family: Style.fontUi
                            font.pixelSize: 11
                            font.weight: Font.Medium
                            elide: Text.ElideRight
                        }
                    }
                    Item { Layout.fillHeight: true }

                    // seek
                    RowLayout {
                        Layout.fillWidth: true
                        Layout.preferredHeight: 36
                        spacing: 10
                        Text {
                            Layout.preferredWidth: 36
                            text: Style.fmtTime(Playback.shownPosition)
                            color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: 8; font.weight: Font.DemiBold
                        }
                        Slider {
                            id: miniSeek
                            Layout.fillWidth: true
                            Layout.alignment: Qt.AlignVCenter
                            from: 0
                            to: Playback.duration > 0 ? Playback.duration : 1
                            value: Playback.shownPosition
                            fillColor: (!Playback.paused && !!Playback.now) ? Style.accent : Tokens.ink
                            onMoved: (v) => Playback.seekDrag = v
                            onCommitted: (v) => { Playback.seek(v); Playback.seekDrag = NaN; }
                        }
                        Text {
                            Layout.preferredWidth: 36
                            horizontalAlignment: Text.AlignRight
                            text: Style.fmtTime(Playback.duration)
                            color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: 8; font.weight: Font.DemiBold
                        }
                    }
                    // next
                    RowLayout {
                        Layout.fillWidth: true
                        Layout.preferredHeight: 30
                        spacing: 7
                        Icon { name: "queue"; size: 12; color: Tokens.inkFaint }
                        Text {
                            Layout.fillWidth: true
                            text: root.nextUp ? ("NEXT · " + root.nextUp.title) : "QUEUE · END"
                            color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: 8; font.weight: Font.Bold; font.letterSpacing: 0.55
                            elide: Text.ElideRight
                        }
                    }
                }

                // LYRICS
                ColumnLayout {
                    anchors.fill: parent
                    anchors.topMargin: 8
                    anchors.bottomMargin: 3
                    visible: root.view === "lyrics"
                    spacing: 0
                    RowLayout {
                        Layout.fillWidth: true
                        Layout.preferredHeight: 27
                        Layout.leftMargin: 5
                        Layout.rightMargin: 5
                        Text { text: "// LYRICS"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: 8; font.weight: Font.Bold; font.letterSpacing: 0.85 }
                        Item { Layout.fillWidth: true }
                        Text { text: "AUTO FOLLOW"; color: Tokens.inkFaint; opacity: 0.68; font.family: Style.fontMono; font.pixelSize: 8; font.weight: Font.Bold; font.letterSpacing: 0.85 }
                    }
                    LyricsPanel {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        compact: true
                        visible: root.view === "lyrics" && root.active
                    }
                }

                // QUEUE
                ColumnLayout {
                    anchors.fill: parent
                    anchors.topMargin: 8
                    anchors.bottomMargin: 3
                    visible: root.view === "queue"
                    spacing: 0
                    RowLayout {
                        Layout.fillWidth: true
                        Layout.preferredHeight: 27
                        Layout.leftMargin: 5
                        Layout.rightMargin: 5
                        Text { text: "// QUEUE"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: 8; font.weight: Font.Bold; font.letterSpacing: 0.85 }
                        Item { Layout.fillWidth: true }
                        Text { text: root.queueLeft + " NEXT"; color: Tokens.inkFaint; opacity: 0.68; font.family: Style.fontMono; font.pixelSize: 8; font.weight: Font.Bold; font.letterSpacing: 0.85 }
                    }
                    TrackList {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        items: (Playback.queue && Playback.queue.items) ? Playback.queue.items : []
                        menu: false
                        canAdd: false
                        onActivated: (i) => Playback.playIndex(i)
                    }
                }
            }

            // footer: transport | volume
            Item {
                Layout.fillWidth: true
                Layout.preferredHeight: 66
                Rectangle {
                    anchors { left: parent.left; right: parent.right; top: parent.top }
                    height: 1
                    color: Tokens.lineSoft
                }
                RowLayout {
                    anchors.fill: parent
                    anchors.topMargin: 11
                    spacing: 14

                    RowLayout {
                        spacing: 7
                        IconButton {
                            icon: "shuffle"; iconSize: 16; diameter: 32
                            active: !!(Playback.queue && Playback.queue.shuffle)
                            iconColor: active ? Tokens.bone : Tokens.inkMuted
                            onClicked: Playback.toggleShuffle()
                        }
                        IconButton { icon: "previous"; iconSize: 16; diameter: 32; onClicked: Playback.prev() }
                        IconButton { icon: Playback.paused ? "play" : "pause"; iconSize: 16; diameter: 32; primary: true; onClicked: Playback.togglePause() }
                        IconButton { icon: "next"; iconSize: 16; diameter: 32; onClicked: Playback.next() }
                        IconButton {
                            icon: (Playback.queue && Playback.queue.repeat === "one") ? "repeat-one" : "repeat"
                            iconSize: 16; diameter: 32
                            active: !!(Playback.queue && Playback.queue.repeat && Playback.queue.repeat !== "off")
                            iconColor: active ? Tokens.bone : Tokens.inkMuted
                            onClicked: Playback.cycleRepeat()
                        }
                    }

                    Item { Layout.fillWidth: true }

                    RowLayout {
                        spacing: 6
                        IconButton {
                            icon: Playback.volume === 0 ? "volume-mute" : "volume"
                            iconSize: 16; diameter: 32
                            onClicked: root.toggleMute()
                        }
                        Slider {
                            id: miniVol
                            Layout.preferredWidth: 96
                            Layout.alignment: Qt.AlignVCenter
                            from: 0
                            to: 100
                            value: Playback.volume
                            onPressedChanged: Playback.volDrag = pressed
                            onMoved: (v) => { var iv = Math.round(v); Playback.volume = iv; Playback.setVolume(iv); }
                            onCommitted: (v) => {
                                var iv = Math.round(v);
                                Playback.volume = iv;
                                Playback.setVolume(iv);
                                Daemon.call("set_setting", { key: "volume", value: String(iv) }).catch(() => {});
                                Playback.volDrag = false;
                            }
                        }
                    }
                }
            }
        }

        // The 1 px frame, inside the rounded clip.
        Rectangle {
            anchors.fill: parent
            radius: root.radius
            color: "transparent"
            border.width: 1
            border.color: Qt.rgba(Tokens.ink.r, Tokens.ink.g, Tokens.ink.b, 0.14)
        }
    }
}
