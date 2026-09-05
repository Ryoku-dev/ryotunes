pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"

// The library Insights tab, ported from ListeningInsights.svelte: on-demand local listening stats
// per period (no background polling, per the performance budget). listening_stats(period) once per
// selection; a request counter drops a stale period's answer. The summary numbers render as stat
// cells (tracked micro label + Fraunces value); the top artists / tracks sit under section rules.
Item {
    id: root

    property string period: "week"
    property var stats: null
    property bool loading: true
    property string errorMsg: ""
    property int request: 0
    property bool started: false

    readonly property bool ready: !root.loading && root.errorMsg === "" && root.stats !== null

    readonly property var overview: root.stats ? [
        { label: "PLAYS", value: String(root.stats.plays) },
        { label: "KNOWN DURATION", value: root.fmtDuration(root.stats.knownDurationSeconds) }
    ] : []

    function ensureLoaded() {
        if (root.started)
            return;
        root.started = true;
        root.load("week");
    }
    function load(p) {
        root.period = p;
        root.loading = true;
        root.errorMsg = "";
        var seq = ++root.request;
        Daemon.call("listening_stats", { period: p })
            .then((s) => { if (seq === root.request) { root.stats = s; root.loading = false; } })
            .catch((e) => { if (seq === root.request) { root.errorMsg = (e && e.message) ? e.message : String(e); root.loading = false; } });
    }
    function fmtDuration(secs) {
        var s = Math.max(0, Math.round(secs || 0));
        var h = Math.floor(s / 3600);
        var m = Math.floor((s % 3600) / 60);
        return h > 0 ? (h + "h " + m + "m") : (m + "m");
    }

    Component.onCompleted: root.ensureLoaded()

    Flickable {
        anchors.fill: parent
        topMargin: Style.sp(2)
        bottomMargin: Style.sp(20)
        clip: true
        contentWidth: width
        contentHeight: col.implicitHeight
        boundsBehavior: Flickable.StopAtBounds

        ColumnLayout {
            id: col
            width: parent.width
            spacing: Style.sp(5)

            // period selector
            Row {
                spacing: Style.sp(2)
                Repeater {
                    model: [{ k: "day", l: "Day" }, { k: "week", l: "Week" }, { k: "month", l: "Month" }]
                    delegate: Chip {
                        required property var modelData
                        text: modelData.l
                        active: root.period === modelData.k
                        onClicked: root.load(modelData.k)
                    }
                }
            }

            Text {
                visible: root.loading || root.errorMsg !== ""
                text: root.loading ? "Reading listening history…" : root.errorMsg
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
            }

            // overview — the summary stats as responsive stat cells
            ColumnLayout {
                Layout.fillWidth: true
                visible: root.ready
                spacing: Style.sp(3)
                SectionHeading { Layout.fillWidth: true; title: "Overview" }
                Flow {
                    Layout.fillWidth: true
                    spacing: Style.sp(3)
                    Repeater {
                        model: root.overview
                        delegate: Rectangle {
                            id: statCell
                            required property var modelData
                            implicitWidth: Math.max(Style.sp(44), cellCol.implicitWidth + Style.sp(8))
                            implicitHeight: cellCol.implicitHeight + Style.sp(8)
                            radius: Style.radiusCard
                            color: Tokens.tint5
                            border.width: 1
                            border.color: Tokens.lineSoft
                            ColumnLayout {
                                id: cellCol
                                anchors.left: parent.left
                                anchors.top: parent.top
                                anchors.margins: Style.sp(4)
                                spacing: Style.sp(2)
                                Text {
                                    text: statCell.modelData.label
                                    color: Tokens.inkMuted
                                    font.family: Style.fontMono
                                    font.pixelSize: Style.fs.micro
                                    font.letterSpacing: Style.trackMicro
                                }
                                Text {
                                    text: statCell.modelData.value
                                    color: Tokens.ink
                                    font.family: Style.fontDisplay
                                    font.pixelSize: Style.fs.xl
                                }
                            }
                        }
                    }
                }
            }

            // top artists
            ColumnLayout {
                Layout.fillWidth: true
                visible: root.ready
                spacing: Style.sp(2)
                SectionHeading { Layout.fillWidth: true; title: "Top artists" }
                Repeater {
                    model: (root.stats && root.stats.topArtists) ? root.stats.topArtists : []
                    delegate: RowLayout {
                        id: artistRow
                        required property var modelData
                        required property int index
                        Layout.fillWidth: true
                        spacing: Style.sp(3)
                        Text { text: String(artistRow.index + 1).padStart(2, "0"); color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.sm }
                        Text { Layout.fillWidth: true; text: artistRow.modelData.name; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; elide: Text.ElideRight }
                        Text { text: artistRow.modelData.plays + " plays"; color: Tokens.inkDim; font.family: Style.fontMono; font.pixelSize: Style.fs.sm }
                    }
                }
                Text {
                    visible: !!(root.stats && (!root.stats.topArtists || !root.stats.topArtists.length))
                    text: "Play some music and this fills itself in."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                }
            }

            // top tracks
            ColumnLayout {
                Layout.fillWidth: true
                visible: root.ready
                spacing: Style.sp(2)
                SectionHeading { Layout.fillWidth: true; title: "Top tracks" }
                Repeater {
                    model: (root.stats && root.stats.topTracks) ? root.stats.topTracks : []
                    delegate: RowLayout {
                        id: trackRow
                        required property var modelData
                        required property int index
                        Layout.fillWidth: true
                        spacing: Style.sp(3)
                        Text { text: String(trackRow.index + 1).padStart(2, "0"); color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.sm }
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 0
                            Text { Layout.fillWidth: true; text: trackRow.modelData.title; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; elide: Text.ElideRight }
                            Text { Layout.fillWidth: true; text: trackRow.modelData.artists; color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; elide: Text.ElideRight }
                        }
                        Text { text: trackRow.modelData.plays + " plays"; color: Tokens.inkDim; font.family: Style.fontMono; font.pixelSize: Style.fs.sm }
                    }
                }
                Text {
                    visible: !!(root.stats && (!root.stats.topTracks || !root.stats.topTracks.length))
                    text: "Nothing recorded for this period yet."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                }
            }
        }
    }
}
