pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Quickshell
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The diagnostics surface (Settings ▸ Diagnostics): everything that went wrong, both halves of
// the app in one list. The daemon records its own warn/error tracing, every failed RPC, panics,
// and the client's forwarded failures into a ring + a rotating file; this page reads them back
// through `get_logs` with level/source/text filters. Refresh is manual (no polling, the same
// rule as software updates). "Open folder" reveals the file for attaching to a bug report;
// "Copy all" puts a plain-text dump on the clipboard for the same reason.
ColumnLayout {
    id: root
    spacing: Style.sp(3)

    property var log: null // the get_logs snapshot
    property bool loading: false
    property string error: ""
    property string level: "warn" // all | warn | error
    property string source: "all" // all | daemon | client
    property string query: ""
    readonly property var entries: (root.log && root.log.entries) ? root.log.entries : []

    readonly property var levels: [
        { k: "all", l: "Everything" },
        { k: "warn", l: "Warnings +" },
        { k: "error", l: "Errors" }
    ]
    readonly property var sources: [
        { k: "all", l: "Both" },
        { k: "daemon", l: "Daemon" },
        { k: "client", l: "App" }
    ]

    function fmtTime(ts) {
        if (!ts)
            return "";
        var d = new Date(ts);
        function p(n) { return (n < 10 ? "0" : "") + n; }
        return p(d.getMonth() + 1) + "-" + p(d.getDate()) + " " + p(d.getHours()) + ":" + p(d.getMinutes()) + ":" + p(d.getSeconds());
    }
    function fmtBytes(n) {
        if (!n)
            return "0 B";
        if (n < 1024)
            return n + " B";
        if (n < 1024 * 1024)
            return Math.round(n / 1024) + " KB";
        return (n / (1024 * 1024)).toFixed(1) + " MB";
    }

    function refresh() {
        root.loading = true;
        root.error = "";
        Daemon.call("get_logs", {
            level: root.level,
            source: root.source,
            query: root.query,
            limit: 500
        }).then((d) => {
            root.log = d;
            root.loading = false;
        }).catch((e) => {
            root.loading = false;
            root.error = (e && e.message) ? e.message : "Could not read the log";
        });
    }

    function copyAll() {
        var lines = [];
        for (var i = 0; i < root.entries.length; i++) {
            var e = root.entries[i];
            lines.push(new Date(e.ts).toISOString() + " [" + e.level + "] " + e.source + " " + e.target + ": " + e.message);
        }
        clip.text = lines.join("\n");
        clip.copy();
        Playback.toast(lines.length + " log lines copied", "success");
    }

    Component.onCompleted: root.refresh()
    TextEdit { id: clip; visible: false; width: 0; height: 0 }

    // ── header ────────────────────────────────────────────────────────────────────────────
    ColumnLayout {
        Layout.fillWidth: true
        spacing: Style.sp(1)
        Text {
            Layout.fillWidth: true
            text: "Diagnostics"
            color: Tokens.ink; font.family: Style.fontDisplay; font.pixelSize: Style.fs.lg
        }
        Text {
            Layout.fillWidth: true
            text: "Everything Ryotunes and its background service got wrong, newest first — including failures from before this session. Use it when something \"works for everyone else but me\"."
            color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
        }
    }

    // ── summary strip ─────────────────────────────────────────────────────────────────────
    RowLayout {
        Layout.fillWidth: true
        spacing: Style.sp(4)
        ColumnLayout {
            Layout.fillWidth: true
            spacing: 1
            Text {
                text: root.log ? (root.log.errors + " errors · " + root.log.warnings + " warnings recorded") : "—"
                color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium
            }
            Text {
                Layout.fillWidth: true
                text: root.log && root.log.path ? ("File: " + root.log.path + " (" + root.fmtBytes(root.log.fileBytes) + ")") : "Log file not available"
                color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs
                elide: Text.ElideMiddle
            }
        }
        Pill {
            Layout.alignment: Qt.AlignVCenter
            label: "Open folder"
            icon: "folder"
            onClicked: Daemon.call("open_logs_folder").catch((e) => Playback.toast((e && e.message) ? e.message : "Could not open the folder", "error"))
        }
        Pill {
            Layout.alignment: Qt.AlignVCenter
            label: "Copy all"
            icon: "link"
            enabled: root.entries.length > 0
            onClicked: root.copyAll()
        }
        Pill {
            Layout.alignment: Qt.AlignVCenter
            label: root.loading ? "Refreshing…" : "Refresh"
            icon: "loading"
            enabled: !root.loading
            onClicked: root.refresh()
        }
    }

    // ── filters ───────────────────────────────────────────────────────────────────────────
    RowLayout {
        Layout.fillWidth: true
        spacing: Style.sp(3)

        RowLayout {
            spacing: Style.sp(1)
            Repeater {
                model: root.levels
                delegate: Pill {
                    required property var modelData
                    label: modelData.l
                    active: root.level === modelData.k
                    onClicked: { root.level = modelData.k; root.refresh(); }
                }
            }
        }
        RowLayout {
            spacing: Style.sp(1)
            Repeater {
                model: root.sources
                delegate: Pill {
                    required property var modelData
                    label: modelData.l
                    active: root.source === modelData.k
                    onClicked: { root.source = modelData.k; root.refresh(); }
                }
            }
        }
        Rectangle {
            Layout.fillWidth: true
            Layout.minimumWidth: Style.sp(30)
            implicitHeight: Style.ctlH
            radius: Style.radius
            color: Tokens.paperLift
            border.width: searchField.activeFocus ? 2 : 1
            border.color: searchField.activeFocus ? Tokens.ink : Tokens.line
            TextInput {
                id: searchField
                anchors.fill: parent
                anchors.leftMargin: Style.sp(2)
                anchors.rightMargin: Style.sp(2)
                verticalAlignment: TextInput.AlignVCenter
                clip: true
                selectByMouse: true
                color: Tokens.ink
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                onAccepted: root.refresh()
                Accessible.name: "Search log messages"
            }
            Text {
                anchors.verticalCenter: parent.verticalCenter
                anchors.left: parent.left
                anchors.leftMargin: Style.sp(2)
                visible: searchField.text === ""
                text: "Search messages…"
                color: Tokens.inkFaint
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
            }
        }
        Pill {
            label: "Search"
            icon: "search"
            enabled: !root.loading
            onClicked: root.refresh()
        }
    }

    // ── body ──────────────────────────────────────────────────────────────────────────────
    Text {
        Layout.fillWidth: true
        visible: root.error !== ""
        text: root.error
        color: Style.alert; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
    }
    Text {
        Layout.fillWidth: true
        visible: !root.loading && root.error === "" && root.entries.length === 0
        text: "Nothing here. No failures have been recorded with these filters — that is good news."
        color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
    }

    // Bounded list: the page lives inside the settings scroller, so cap the rows and let the
    // filters narrow instead of growing the settings page to thousands of items.
    ColumnLayout {
        Layout.fillWidth: true
        spacing: 0
        Repeater {
            model: Math.min(root.entries.length, 200)
            delegate: RowLayout {
                id: row
                required property int index
                readonly property var entry: root.entries[index]
                Layout.fillWidth: true
                Layout.topMargin: Style.sp(1)
                spacing: Style.sp(2)

                Rectangle {
                    Layout.alignment: Qt.AlignTop
                    Layout.topMargin: Style.sp(0.5)
                    width: 8
                    height: 8
                    radius: 4
                    color: row.entry && row.entry.level === "error" ? Style.alert : Tokens.sun
                }
                Text {
                    Layout.alignment: Qt.AlignTop
                    text: root.fmtTime(row.entry ? row.entry.ts : 0)
                    color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs
                }
                Text {
                    Layout.alignment: Qt.AlignTop
                    text: row.entry ? row.entry.source + " " + row.entry.target : ""
                    color: Tokens.inkDim; font.family: Style.fontMono; font.pixelSize: Style.fs.xs
                    Layout.maximumWidth: Style.sp(46)
                    elide: Text.ElideMiddle
                }
                Text {
                    Layout.fillWidth: true
                    Layout.minimumWidth: 0
                    text: row.entry ? row.entry.message : ""
                    textFormat: Text.PlainText
                    color: Tokens.ink; font.family: Style.fontMono; font.pixelSize: Style.fs.xs
                    wrapMode: Text.WordWrap
                    maximumLineCount: 4
                    elide: Text.ElideRight
                    Accessible.role: Accessible.StaticText
                    Accessible.name: text
                }
            }
        }
    }
    Text {
        Layout.fillWidth: true
        visible: root.entries.length > 200
        text: "Showing the newest 200 of " + root.entries.length + " matching lines — narrow with the filters or open the file."
        color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
    }
}
