pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"
import "../lib/playback.js" as PB

// Synced lyrics, ported from ui/src/lib/components/LyricsView.svelte. It self-fetches get_lyrics on
// track change (guarded against stale responses by requestedId), lays the timed lines in a reused
// ListView, and follows the song: the active line is binary-searched from a local clock and centred
// with an animated contentY, click-to-seek jumps both playback and the view, and a 67 ms word timer
// karaoke-fills the active line. The timer is the one place this view spends CPU, so it runs only
// while the surface is visible, playback is not paused, and the track actually carries word timing —
// close the panel or pause and it stops (verified in Task 6 / Task 9). Manual scrolling pauses the
// auto-follow for five seconds. `compact` shrinks the type and drops the footer for the mini player.
Item {
    id: root

    property bool compact: false

    // --- fetched state ----------------------------------------------------------------------
    property var lyrics: null
    property bool loading: false
    property string requestedId: ""

    // Per-track manual timing correction (ms; positive = lyrics appear later). Kept in memory and
    // reset on every track change — a listening preference, not YouTube metadata.
    property int offsetMs: 0

    readonly property bool synced: !!(root.lyrics && root.lyrics.synced)
    readonly property bool instrumental: !!(root.lyrics && root.lyrics.instrumental)
    readonly property bool hasWordTiming: {
        if (!root.synced || !root.lyrics.lines)
            return false;
        for (var i = 0; i < root.lyrics.lines.length; i++) {
            var w = root.lyrics.lines[i].words;
            if (w && w.length)
                return true;
        }
        return false;
    }

    // The timed cue index, rebuilt only when the document changes; a long lyric is then binary-
    // searched against the clock instead of linearly rescanned 15 times a second.
    readonly property var timedLines: {
        var out = [];
        if (!root.synced || !root.lyrics.lines)
            return out;
        for (var i = 0; i < root.lyrics.lines.length; i++) {
            var t = root.lyrics.lines[i].time_ms;
            if (t !== undefined && t !== null)
                out.push({ index: i, time: t });
        }
        return out;
    }

    // --- local clock ------------------------------------------------------------------------
    // The shared transport samples position at ~4 Hz; the word timer interpolates between samples
    // at 15 Hz so the active word fills smoothly, and resyncs to every real sample.
    property real interpolatedPos: Playback.position
    property real clockBase: 0
    property real clockAt: 0
    readonly property real posMs: root.interpolatedPos * 1000 - root.offsetMs

    readonly property int activeIndex: {
        var currentMs = root.posMs;
        var lines = root.timedLines;
        var lo = 0, hi = lines.length - 1, answer = -1;
        while (lo <= hi) {
            var mid = (lo + hi) >> 1;
            if (lines[mid].time <= currentMs) {
                answer = lines[mid].index;
                lo = mid + 1;
            } else {
                hi = mid - 1;
            }
        }
        return answer;
    }

    // --- auto-follow / manual scroll --------------------------------------------------------
    property bool manualScroll: false
    property bool hasScrolled: false

    function fmtOffset(ms) {
        if (!ms)
            return "0.0s";
        return (ms > 0 ? "+" : "") + (ms / 1000).toFixed(1) + "s";
    }

    // --- fetch ------------------------------------------------------------------------------
    function maybeFetch(force) {
        var now = Playback.now;
        if (!root.visible || !now) {
            if (!now) {
                root.requestedId = "";
                root.lyrics = null;
                root.loading = false;
            }
            return;
        }
        if (!force && now.videoId === root.requestedId)
            return;
        var id = now.videoId;
        root.requestedId = id;
        root.offsetMs = 0;
        root.loading = true;
        root.lyrics = null;
        var q = Playback.queue;
        var cur = (q && q.items) ? q.items[q.currentIndex] : null;
        Daemon.call("get_lyrics", {
            videoId: id,
            title: now.title,
            artists: now.artists,
            album: (cur && cur.album) ? cur.album : undefined,
            duration: PB.durationToSeconds(now.duration)
        }).then((l) => {
            if (root.requestedId !== id)
                return;
            root.lyrics = l;
            root.loading = false;
            root.hasScrolled = false;
            root.manualScroll = false;
        }).catch(() => {
            if (root.requestedId !== id)
                return;
            root.lyrics = null;
            root.loading = false;
        });
    }

    onVisibleChanged: root.maybeFetch(false)
    Component.onCompleted: root.maybeFetch(false)

    Connections {
        target: Playback
        function onNowChanged() { root.maybeFetch(false); }
        // Resync the local clock to every authoritative transport sample.
        function onPositionChanged() {
            root.clockBase = Playback.position;
            root.clockAt = Date.now();
            if (!wordTimer.running)
                root.interpolatedPos = Playback.position;
        }
        function onPausedChanged() {
            if (Playback.paused)
                root.interpolatedPos = Playback.position;
        }
    }

    // The word clock: the only recurring work this view does, gated exactly per the plan.
    Timer {
        id: wordTimer
        interval: 67
        repeat: true
        running: root.visible && !Playback.paused && root.synced && root.hasWordTiming
        onRunningChanged: {
            root.clockBase = Playback.position;
            root.clockAt = Date.now();
        }
        onTriggered: root.interpolatedPos = root.clockBase + (Date.now() - root.clockAt) / 1000
    }

    // Centre the active line, gliding after the first landing (which jumps).
    NumberAnimation {
        id: scrollAnim
        target: view
        property: "contentY"
        duration: Style.motion.swap
        easing.type: Easing.OutCubic
    }
    function follow(i, animate) {
        if (i < 0 || view.count === 0)
            return;
        var prev = view.contentY;
        view.positionViewAtIndex(i, ListView.Center);
        var goal = view.contentY;
        view.contentY = prev;
        if (animate) {
            scrollAnim.stop();
            scrollAnim.from = prev;
            scrollAnim.to = goal;
            scrollAnim.start();
        } else {
            scrollAnim.stop();
            view.contentY = goal;
        }
    }
    onActiveIndexChanged: {
        if (root.manualScroll || root.activeIndex < 0)
            return;
        root.follow(root.activeIndex, root.hasScrolled);
        root.hasScrolled = true;
    }
    Timer {
        id: manualTimer
        interval: 5000
        onTriggered: {
            root.manualScroll = false;
            if (root.activeIndex >= 0)
                root.follow(root.activeIndex, true);
        }
    }
    function pauseFollow() {
        root.manualScroll = true;
        manualTimer.restart();
    }
    function returnToCurrent() {
        manualTimer.stop();
        root.manualScroll = false;
        if (root.activeIndex >= 0)
            root.follow(root.activeIndex, true);
    }
    function seekToLine(line) {
        if (line.time_ms === undefined || line.time_ms === null)
            return;
        var secs = Math.max(0, (line.time_ms + root.offsetMs) / 1000);
        Playback.position = secs;      // jump the view along with the seek
        Playback.seek(secs);
        manualTimer.stop();
        root.manualScroll = false;
    }

    // --- layout -----------------------------------------------------------------------------
    ColumnLayout {
        anchors.fill: parent
        spacing: 0

        // Synced / plain lines -----------------------------------------------------------
        ListView {
            id: view
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: !root.loading && !root.instrumental && !!root.lyrics && !!root.lyrics.lines && root.lyrics.lines.length > 0
            clip: true
            reuseItems: true
            cacheBuffer: Math.max(0, Math.round(height * 1.5))
            boundsBehavior: Flickable.StopAtBounds
            // Breathing room so the active cue can sit in the reading zone rather than the edge.
            header: Item { width: 1; height: view.height * 0.34 }
            footer: Item { width: 1; height: view.height * 0.34 }
            model: (root.lyrics && root.lyrics.lines) ? root.lyrics.lines : []

            onMovementStarted: root.pauseFollow()

            delegate: Item {
                id: lineWrap
                required property var modelData
                required property int index
                width: view.width
                implicitHeight: lineCol.implicitHeight + Style.sp(root.compact ? 1 : 2.5)

                readonly property bool isActive: root.synced && lineWrap.index === root.activeIndex
                readonly property bool isPast: root.synced && lineWrap.index < root.activeIndex
                readonly property bool wordMode: lineWrap.isActive && !!(lineWrap.modelData.words && lineWrap.modelData.words.length)
                readonly property int lineFs: root.compact
                    ? (lineWrap.isActive ? Style.fs.lg : Style.fs.md)
                    : (lineWrap.isActive ? Style.fs.xl : Style.fs.lg)
                // Sung lines settle to 0.4, upcoming to 0.6, the active line full; the viewport haze
                // then fades every line by its distance from the reading centre.
                readonly property real baseOpacity: !root.synced ? 0.85
                    : lineWrap.isActive ? 1 : lineWrap.isPast ? 0.4 : 0.6
                // The active word is the last one whose start has passed: earlier words are sung ink,
                // it lights in the accent, later words wait muted.
                readonly property int activeWord: {
                    if (!lineWrap.wordMode)
                        return -1;
                    var ws = lineWrap.modelData.words, p = root.posMs, idx = -1;
                    for (var i = 0; i < ws.length; i++) {
                        if (ws[i].start_ms <= p)
                            idx = i;
                        else
                            break;
                    }
                    return idx;
                }
                // A centred reading column, capped at 560 on the wide stage, snug in the mini.
                readonly property real colW: root.compact
                    ? Math.max(1, lineWrap.width - Style.sp(4))
                    : Math.min(lineWrap.width - Style.sp(8), Style.sp(140))

                ColumnLayout {
                    id: lineCol
                    x: (lineWrap.width - lineCol.width) / 2
                    width: lineWrap.colW
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: Style.sp(0.5)
                    // Reading-centre haze: full at the middle line, falling to nothing at the edges.
                    opacity: {
                        var mid = lineWrap.y + lineWrap.height / 2 - view.contentY;
                        var d = view.height > 0 ? Math.min(1, Math.abs(mid - view.height / 2) / view.height) : 0;
                        return lineWrap.baseOpacity * (1 - Math.pow(d, 0.45));
                    }

                    // Plain line (no word timing, or a non-active line): ink, dimmed by the opacity.
                    Text {
                        Layout.fillWidth: true
                        visible: !lineWrap.wordMode
                        text: lineWrap.modelData.text ? lineWrap.modelData.text : "♪"
                        color: Tokens.ink
                        font.family: Style.fontDisplay
                        font.pixelSize: lineWrap.lineFs
                        font.weight: Font.Medium
                        wrapMode: Text.WordWrap
                        lineHeight: 1.2
                    }

                    // Active line with word timing: the karaoke fill driven by root.posMs.
                    Flow {
                        Layout.fillWidth: true
                        visible: lineWrap.wordMode
                        spacing: Style.sp(1)
                        Repeater {
                            model: lineWrap.wordMode ? lineWrap.modelData.words : []
                            delegate: Text {
                                id: word
                                required property int index
                                required property var modelData
                                text: word.modelData.text ? String(word.modelData.text).replace(/\s+$/, "") : ""
                                color: word.index === lineWrap.activeWord ? Style.accent
                                    : word.index < lineWrap.activeWord ? Tokens.ink
                                    : Qt.rgba(Tokens.inkMuted.r, Tokens.inkMuted.g, Tokens.inkMuted.b, 0.72)
                                font.family: Style.fontDisplay
                                font.pixelSize: lineWrap.lineFs
                                font.weight: Font.Medium
                                Behavior on color { ColorAnimation { duration: Style.motion.snap } }
                            }
                        }
                    }

                    // Translation, if the provider supplied one.
                    Text {
                        Layout.fillWidth: true
                        visible: !!lineWrap.modelData.translation
                        text: lineWrap.modelData.translation ? lineWrap.modelData.translation : ""
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: root.compact ? Style.fs.xs : Style.fs.sm
                        font.italic: true
                        wrapMode: Text.WordWrap
                    }
                }

                MouseArea {
                    anchors.fill: parent
                    enabled: root.synced
                    cursorShape: root.synced ? Qt.PointingHandCursor : Qt.ArrowCursor
                    onClicked: root.seekToLine(lineWrap.modelData)
                }
            }

            // "Jump back to the current line" while auto-follow is paused.
            Rectangle {
                anchors.top: parent.top
                anchors.horizontalCenter: parent.horizontalCenter
                anchors.topMargin: Style.sp(2)
                visible: root.manualScroll && root.synced && root.activeIndex >= 0 && !root.compact
                z: 5
                implicitWidth: currentLabel.implicitWidth + Style.sp(4)
                implicitHeight: Style.sp(7)
                radius: height / 2
                color: Tokens.paperLift
                border.width: 1
                border.color: Tokens.lineStrong
                Text {
                    id: currentLabel
                    anchors.centerIn: parent
                    text: "CURRENT LINE"
                    color: Tokens.inkDim
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.xs
                    font.letterSpacing: 1.2
                }
                MouseArea {
                    anchors.fill: parent
                    cursorShape: Qt.PointingHandCursor
                    onClicked: root.returnToCurrent()
                }
            }
        }

        // Resolving state ----------------------------------------------------------------
        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: root.loading
            Item { Layout.fillHeight: true }
            ColumnLayout {
                Layout.alignment: Qt.AlignHCenter
                Layout.maximumWidth: Style.sp(120)
                spacing: Style.sp(1)
                Text {
                    text: "// RESOLVING LYRICS"
                    color: Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.xs
                    font.letterSpacing: 1.2
                }
                Text {
                    Layout.fillWidth: true
                    text: Playback.now && Playback.now.title ? Playback.now.title : "Current track"
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.md
                    font.weight: Font.Medium
                    elide: Text.ElideRight
                }
                Text {
                    Layout.fillWidth: true
                    text: Playback.now && Playback.now.artists ? Playback.now.artists : "Waiting for metadata"
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    elide: Text.ElideRight
                }
            }
            Item { Layout.fillHeight: true }
        }

        // Empty / instrumental state -----------------------------------------------------
        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            visible: !root.loading && (root.instrumental || !root.lyrics || !root.lyrics.lines || root.lyrics.lines.length === 0)
            Item { Layout.fillHeight: true }
            ColumnLayout {
                Layout.alignment: Qt.AlignHCenter
                Layout.maximumWidth: Style.sp(120)
                spacing: Style.sp(1.5)
                Icon {
                    Layout.alignment: Qt.AlignHCenter
                    name: "mic"
                    size: Style.fs.xl
                    color: Tokens.inkFaint
                }
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    text: root.instrumental ? "Instrumental track." : "No lyrics found for this track."
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.md
                    font.weight: Font.Medium
                }
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    Layout.fillWidth: true
                    horizontalAlignment: Text.AlignHCenter
                    text: root.instrumental
                        ? "There are no vocal lines to follow for this recording."
                        : ((Playback.now && Playback.now.title ? Playback.now.title : "Current track")
                            + " · " + (Playback.now && Playback.now.artists ? Playback.now.artists : "Unknown artist"))
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    wrapMode: Text.WordWrap
                }
                Rectangle {
                    Layout.alignment: Qt.AlignHCenter
                    visible: !root.instrumental
                    implicitWidth: retryRow.implicitWidth + Style.sp(4)
                    implicitHeight: Style.sp(8)
                    radius: Style.radius
                    color: retryHover.hovered ? Tokens.tint5 : "transparent"
                    border.width: 1
                    border.color: Tokens.line
                    RowLayout {
                        id: retryRow
                        anchors.centerIn: parent
                        spacing: Style.sp(1.5)
                        Icon { name: "loading"; size: Style.fs.sm; color: Tokens.inkDim }
                        Text {
                            text: "Retry lookup"
                            color: Tokens.inkDim
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.sm
                        }
                    }
                    HoverHandler { id: retryHover }
                    MouseArea {
                        anchors.fill: parent
                        cursorShape: Qt.PointingHandCursor
                        onClicked: root.maybeFetch(true)
                    }
                }
            }
            Item { Layout.fillHeight: true }
        }

        // Footer: attribution + timing offset (synced only, non-compact) -----------------
        Hairline { Layout.fillWidth: true; visible: lyricsFooter.visible }
        RowLayout {
            id: lyricsFooter
            Layout.fillWidth: true
            Layout.leftMargin: Style.sp(3)
            Layout.rightMargin: Style.sp(3)
            Layout.topMargin: Style.sp(1.5)
            Layout.bottomMargin: Style.sp(1.5)
            spacing: Style.sp(2)
            visible: !root.compact && !!root.lyrics && !root.loading
            Text {
                Layout.fillWidth: true
                text: {
                    var s = (root.lyrics && root.lyrics.source) ? root.lyrics.source : "";
                    if (!s)
                        return "";
                    return s.indexOf("Source:") === 0 ? s : ("Lyrics from " + s);
                }
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.xs
                elide: Text.ElideRight
            }
            RowLayout {
                visible: root.synced
                spacing: Style.sp(1)
                Text {
                    text: "Timing " + root.fmtOffset(root.offsetMs)
                    color: Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.xs
                }
                Repeater {
                    model: [{ label: "−0.5", d: -500 }, { label: "Reset", d: 0 }, { label: "+0.5", d: 500 }]
                    delegate: Rectangle {
                        id: offBtn
                        required property var modelData
                        implicitWidth: offLabel.implicitWidth + Style.sp(3)
                        implicitHeight: Style.sp(6)
                        radius: Style.radius
                        color: offHover.hovered ? Tokens.tint5 : "transparent"
                        Text {
                            id: offLabel
                            anchors.centerIn: parent
                            text: offBtn.modelData.label
                            color: Tokens.inkDim
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.xs
                        }
                        HoverHandler { id: offHover }
                        MouseArea {
                            anchors.fill: parent
                            cursorShape: Qt.PointingHandCursor
                            onClicked: {
                                if (offBtn.modelData.d === 0)
                                    root.offsetMs = 0;
                                else
                                    root.offsetMs = Math.max(-5000, Math.min(5000, root.offsetMs + offBtn.modelData.d));
                            }
                        }
                    }
                }
            }
        }
    }
}
