pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../lib/collections.js" as Collections
import "../"
import "../components"

// The download manager surface (routed as "downloads", reached from the rail and the download
// button beside the like heart). It renders the shared Downloads mirror — the daemon owns every
// job, this page just reflects it and calls back through the singleton — split into a live queue
// (downloading / waiting, each cancellable with its progress) and a history (finished files with an
// open-folder action, and failures with their error and a retry). Nothing polls: Downloads updates
// from the subscribe snapshot and the downloads-changed event, and every row rebinds on its own.
// Clearing history removes terminal records only; saved files are never touched.
Item {
    id: page

    readonly property var jobs: Downloads.jobs

    // Live jobs (downloading before queued), then terminal jobs newest-first for the history.
    readonly property var queue: {
        var out = [];
        for (var i = 0; i < page.jobs.length; i++) {
            var j = page.jobs[i];
            if (j && (j.status === "downloading" || j.status === "queued"))
                out.push(j);
        }
        out.sort((a, b) => {
            if (a.status !== b.status)
                return a.status === "downloading" ? -1 : 1;
            return (a.createdAt || 0) - (b.createdAt || 0);
        });
        return out;
    }
    readonly property var history: {
        var out = [];
        for (var i = 0; i < page.jobs.length; i++) {
            var j = page.jobs[i];
            if (j && (j.status === "completed" || j.status === "failed" || j.status === "cancelled"))
                out.push(j);
        }
        out.sort((a, b) => ((b.finishedAt || b.createdAt || 0) - (a.finishedAt || a.createdAt || 0)));
        return out;
    }

    // Collapse jobs that share a collection label into one card; single tracks stay flat rows
    // (Collections.groupJobs). The card sits where its first job sorted, so queue/history order
    // is preserved.
    readonly property var queueGroups: Collections.groupJobs(page.queue)
    readonly property var historyGroups: Collections.groupJobs(page.history)
    readonly property string folderPath: (Downloads.settings && Downloads.settings.path) ? Downloads.settings.path : ""

    Component.onCompleted: {
        if (Daemon.connected) {
            Downloads.refresh();
            Downloads.loadSettings();
        }
    }
    Connections {
        target: Daemon
        function onConnectedChanged(): void {
            if (Daemon.connected) {
                Downloads.refresh();
                Downloads.loadSettings();
            }
        }
    }

    // A focusable text button (Btn) with a keyboard focus ring, matching the settings page.
    component Action: Btn {
        id: action
        activeFocusOnTab: enabled && visible
        Keys.onSpacePressed: (event) => { action.clicked(); event.accepted = true; }
        Keys.onReturnPressed: (event) => { action.clicked(); event.accepted = true; }
        Accessible.onPressAction: action.clicked()
        Rectangle {
            anchors.fill: parent
            anchors.margins: -2
            radius: Style.radius
            color: "transparent"
            border.width: action.activeFocus ? 2 : 0
            border.color: Tokens.ink
        }
    }

    // A focusable icon action for a row (cancel / open / retry).
    component IconAction: IconButton {
        id: iact
        property string a11y: ""
        activeFocusOnTab: enabled && visible
        Accessible.role: Accessible.Button
        Accessible.name: iact.a11y
        Accessible.onPressAction: iact.clicked()
        Keys.onSpacePressed: (event) => { iact.clicked(); event.accepted = true; }
        Keys.onReturnPressed: (event) => { iact.clicked(); event.accepted = true; }
        Rectangle {
            anchors.fill: parent
            anchors.margins: -2
            radius: Style.radius
            color: "transparent"
            border.width: iact.activeFocus ? 2 : 0
            border.color: Tokens.ink
        }
    }

    // One compact download row: artwork, title, source + artists, a status/progress line, and the
    // action for its state. Handles every status so the queue and history reuse it.
    component JobRow: Rectangle {
        id: jr
        property var job: null
        readonly property string status: jr.job ? String(jr.job.status) : ""
        readonly property int progress: (jr.job && jr.job.progress) ? Math.max(0, Math.min(100, jr.job.progress)) : 0
        readonly property string fileName: (jr.job && jr.job.filePath) ? String(jr.job.filePath).split("/").pop() : ""

        Layout.fillWidth: true
        implicitHeight: Math.max(Style.sp(15), rowLay.implicitHeight + Style.sp(4))
        radius: Style.radius
        color: rowHover.hovered ? Tokens.tint5 : "transparent"
        Behavior on color { ColorAnimation { duration: Style.motion.snap } }
        HoverHandler { id: rowHover }

        RowLayout {
            id: rowLay
            anchors.fill: parent
            anchors.leftMargin: Style.sp(2)
            anchors.rightMargin: Style.sp(2)
            spacing: Style.sp(3)

            Artwork {
                Layout.alignment: Qt.AlignVCenter
                url: (jr.job && jr.job.thumbnail) ? jr.job.thumbnail : ""
                px: Style.sp(11)
            }

            ColumnLayout {
                Layout.fillWidth: true
                Layout.minimumWidth: 0
                Layout.alignment: Qt.AlignVCenter
                spacing: Style.sp(0.5)

                Text {
                    Layout.fillWidth: true
                    text: (jr.job && jr.job.title) ? jr.job.title : "Untitled"
                    textFormat: Text.PlainText
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.md
                    font.weight: Font.Medium
                    elide: Text.ElideRight
                }
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(1.5)
                    Text {
                        text: (jr.job && jr.job.source) ? jr.job.source : ""
                        textFormat: Text.PlainText
                        visible: text !== ""
                        color: Tokens.inkFaint
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        font.letterSpacing: Style.trackMicro
                        elide: Text.ElideRight
                        Layout.maximumWidth: Style.sp(60)
                    }
                    Text {
                        Layout.fillWidth: true
                        Layout.minimumWidth: 0
                        visible: !!(jr.job && jr.job.artists)
                        text: (jr.job && jr.job.artists) ? jr.job.artists : ""
                        textFormat: Text.PlainText
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        elide: Text.ElideRight
                    }
                }

                // status / progress line
                RowLayout {
                    Layout.fillWidth: true
                    Layout.topMargin: Style.sp(0.5)
                    spacing: Style.sp(2)
                    visible: jr.status !== ""

                    // progress track (downloading only)
                    Rectangle {
                        visible: jr.status === "downloading"
                        Layout.fillWidth: true
                        Layout.maximumWidth: Style.sp(60)
                        Layout.alignment: Qt.AlignVCenter
                        implicitHeight: Style.sp(1)
                        radius: height / 2
                        color: Tokens.lineSoft
                        Rectangle {
                            height: parent.height
                            width: parent.width * jr.progress / 100
                            radius: height / 2
                            color: Style.accent
                            Behavior on width {
                                enabled: !Tokens.reduceMotion
                                NumberAnimation { duration: Style.motion.move; easing.type: Easing.OutCubic }
                            }
                        }
                    }
                    Text {
                        textFormat: Text.PlainText
                        text: {
                            switch (jr.status) {
                            case "downloading": return jr.progress + "%";
                            case "queued": return "Waiting in queue";
                            case "completed": return jr.fileName !== "" ? jr.fileName : "Saved";
                            case "failed": return (jr.job && jr.job.error) ? jr.job.error : "Download failed";
                            case "cancelled": return "Cancelled";
                            default: return "";
                            }
                        }
                        color: jr.status === "failed" ? Style.alert : Tokens.inkMuted
                        font.family: jr.status === "downloading" ? Style.fontMono : Style.fontUi
                        font.pixelSize: Style.fs.sm
                        elide: Text.ElideRight
                        maximumLineCount: 2
                        wrapMode: jr.status === "failed" ? Text.WordWrap : Text.NoWrap
                        Layout.fillWidth: jr.status !== "downloading"
                        Layout.minimumWidth: 0
                    }
                    Item { visible: jr.status === "downloading"; Layout.fillWidth: true }
                }

                // Non-fatal metadata notes (missing cover/lyrics, or a tag that could not be
                // written) for a completed download. The audio still saved, so these read as a
                // muted aside rather than an error.
                Text {
                    readonly property var warns: (jr.status === "completed" && jr.job && jr.job.warnings) ? jr.job.warnings : []
                    Layout.fillWidth: true
                    Layout.topMargin: Style.sp(0.5)
                    visible: warns.length > 0
                    textFormat: Text.PlainText
                    text: warns.join(" \u00b7 ")
                    color: Tokens.inkFaint
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    wrapMode: Text.WordWrap
                    maximumLineCount: 3
                    elide: Text.ElideRight
                    Accessible.role: Accessible.StaticText
                    Accessible.name: text
                }
            }

            // trailing action for this state
            IconAction {
                Layout.alignment: Qt.AlignVCenter
                visible: jr.status === "downloading" || jr.status === "queued"
                icon: "close"
                iconSize: Style.fs.md
                diameter: Style.sp(9)
                a11y: "Cancel download"
                onClicked: if (jr.job) Downloads.cancel(jr.job.id)
            }
            IconAction {
                Layout.alignment: Qt.AlignVCenter
                visible: jr.status === "completed"
                icon: "folder"
                iconSize: Style.fs.md
                diameter: Style.sp(9)
                a11y: "Open file location"
                onClicked: if (jr.job) Downloads.open(jr.job.id)
            }
            IconAction {
                Layout.alignment: Qt.AlignVCenter
                visible: jr.status === "failed" || jr.status === "cancelled"
                icon: "loading"
                iconSize: Style.fs.md
                diameter: Style.sp(9)
                a11y: "Retry download"
                onClicked: if (jr.job) Downloads.retry(jr.job.id)
            }
        }
    }

    // One album/playlist batch collapsed into a single card: cover, kind, aggregate progress,
    // and an expandable list of its per-track rows. A collection is what the user asked for,
    // so the queue shows one card per collection instead of a hundred anonymous rows.
    component CollectionCard: Rectangle {
        id: cc
        property var group: null
        readonly property var jobs: cc.group ? cc.group.jobs : []
        readonly property string label: cc.group ? cc.group.label : ""
        readonly property bool open: Downloads.openCollections[label] === true
        readonly property var stats: Collections.stats(cc.jobs)
        readonly property int doneCount: cc.stats.done
        readonly property int waitingCount: cc.stats.waiting
        readonly property int movingCount: cc.stats.moving
        readonly property int failedCount: cc.stats.failed
        readonly property int cancelledCount: cc.stats.cancelled
        readonly property bool active: cc.movingCount + cc.waitingCount > 0
        readonly property int percent: cc.stats.percent
        readonly property string summary: {
            var parts = [];
            if (movingCount > 0)
                parts.push("Downloading " + doneCount + " of " + jobs.length);
            else if (doneCount > 0)
                parts.push(doneCount + " saved");
            if (waitingCount > 0)
                parts.push(waitingCount + " waiting");
            if (failedCount > 0)
                parts.push(failedCount + " failed");
            if (cancelledCount > 0)
                parts.push(cancelledCount + " cancelled");
            return parts.join(" \u00b7 ");
        }
        // The first artwork the batch carried; album pages always have one.
        readonly property string cover: {
            for (var i = 0; i < jobs.length; i++)
                if (jobs[i].thumbnail)
                    return jobs[i].thumbnail;
            return "";
        }

        implicitHeight: body.implicitHeight + Style.sp(4)
        radius: Style.radius
        color: cardHover.hovered ? Tokens.tint5 : "transparent"
        Behavior on color { ColorAnimation { duration: Style.motion.snap } }
        HoverHandler { id: cardHover }

        ColumnLayout {
            id: body
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.margins: Style.sp(2)
            spacing: Style.sp(1)

            RowLayout {
                id: header
                Layout.fillWidth: true
                spacing: Style.sp(3)

                Artwork {
                    Layout.alignment: Qt.AlignVCenter
                    url: cc.cover
                    px: Style.sp(11)
                }
                ColumnLayout {
                    Layout.fillWidth: true
                    Layout.minimumWidth: 0
                    spacing: Style.sp(0.5)
                    Text {
                        text: (cc.group && cc.group.kind === "album") ? "Album" : "Playlist"
                        textFormat: Text.PlainText
                        color: Tokens.inkFaint
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        font.letterSpacing: Style.trackMicro
                    }
                    Text {
                        Layout.fillWidth: true
                        text: cc.label
                        textFormat: Text.PlainText
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        font.weight: Font.Medium
                        elide: Text.ElideRight
                    }
                    // aggregate progress while anything is still moving
                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(2)
                        visible: cc.active
                        Rectangle {
                            Layout.fillWidth: true
                            Layout.maximumWidth: Style.sp(60)
                            Layout.alignment: Qt.AlignVCenter
                            implicitHeight: Style.sp(1)
                            radius: height / 2
                            color: Tokens.lineSoft
                            Rectangle {
                                height: parent.height
                                width: parent.width * cc.percent / 100
                                radius: height / 2
                                color: Style.accent
                                Behavior on width {
                                    enabled: !Tokens.reduceMotion
                                    NumberAnimation { duration: Style.motion.move; easing.type: Easing.OutCubic }
                                }
                            }
                        }
                        Text {
                            text: cc.percent + "%"
                            textFormat: Text.PlainText
                            color: Tokens.inkMuted
                            font.family: Style.fontMono
                            font.pixelSize: Style.fs.sm
                        }
                    }
                    Text {
                        Layout.fillWidth: true
                        text: cc.summary
                        textFormat: Text.PlainText
                        visible: text !== ""
                        color: (cc.failedCount + cc.cancelledCount) > 0 ? Style.alert : Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        elide: Text.ElideRight
                    }
                }

                // Reveal the collection folder (any completed file sits in it).
                IconAction {
                    Layout.alignment: Qt.AlignVCenter
                    visible: cc.doneCount > 0
                    icon: "folder"
                    iconSize: Style.fs.md
                    diameter: Style.sp(9)
                    a11y: "Open " + cc.label + " folder"
                    onClicked: {
                        for (var i = 0; i < cc.jobs.length; i++)
                            if (cc.jobs[i].status === "completed") {
                                Downloads.open(cc.jobs[i].id);
                                return;
                            }
                    }
                }
                // Cancel every unfinished track of this collection.
                IconAction {
                    Layout.alignment: Qt.AlignVCenter
                    visible: cc.active
                    icon: "close"
                    iconSize: Style.fs.md
                    diameter: Style.sp(9)
                    a11y: "Cancel " + cc.label + " downloads"
                    onClicked: {
                        for (var i = 0; i < cc.jobs.length; i++)
                            if (cc.jobs[i].status === "downloading" || cc.jobs[i].status === "queued")
                                Downloads.cancel(cc.jobs[i].id);
                    }
                }
                IconAction {
                    Layout.alignment: Qt.AlignVCenter
                    icon: cc.open ? "chevron-down" : "chevron-right"
                    iconSize: Style.fs.md
                    diameter: Style.sp(9)
                    a11y: (cc.open ? "Collapse " : "Expand ") + cc.label
                    onClicked: Downloads.toggleCollection(cc.label)
                }
            }

            // Expanded: the per-track rows, indented under their collection.
            ColumnLayout {
                id: rows
                Layout.fillWidth: true
                Layout.leftMargin: Style.sp(4)
                visible: cc.open
                spacing: 0
                Repeater {
                    model: cc.jobs
                    delegate: JobRow {
                        required property var modelData
                        job: modelData
                    }
                }
            }
        }
    }

    // One entry of the grouped queue/history list: a collection card when the batch shares a
    // label, a flat JobRow for a single track.
    component GroupRow: Loader {
        required property var entry
        Layout.fillWidth: true
        sourceComponent: entry && entry.group ? collectionCard : jobRow
        readonly property var group: entry ? entry.group : null
        readonly property var job: entry ? entry.job : null
        Component {
            id: collectionCard
            CollectionCard { group: parent.group }
        }
        Component {
            id: jobRow
            JobRow { job: parent.job }
        }
    }
    Flickable {
        id: scroll
        anchors.fill: parent
        contentWidth: width
        contentHeight: content.implicitHeight
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        bottomMargin: Style.sp(20)

        ColumnLayout {
            id: content
            width: scroll.width
            spacing: Style.sp(4)

            // --- header -------------------------------------------------------------------
            ColumnLayout {
                Layout.fillWidth: true
                Layout.topMargin: Style.sp(2)
                spacing: Style.sp(1)
                Text {
                    Layout.fillWidth: true
                    text: "Downloads"
                    color: Tokens.ink
                    font.family: Style.fontDisplay
                    font.pixelSize: Style.fs.title
                    elide: Text.ElideRight
                }
                Text {
                    Layout.fillWidth: true
                    text: page.folderPath !== "" ? "Saving to " + page.folderPath
                        : "Save a track with the download arrow beside its heart in the player."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    elide: Text.ElideRight
                }
            }

            RowLayout {
                Layout.fillWidth: true
                spacing: Style.sp(2)
                Action {
                    text: "Open folder"
                    icon: "folder"
                    enabled: Daemon.connected
                    onClicked: Downloads.openFolder()
                }
                Action {
                    text: "Clear history"
                    icon: "trash"
                    enabled: Daemon.connected && page.history.length > 0
                    onClicked: Downloads.clearHistory()
                }
                Item { Layout.fillWidth: true }
                Action {
                    text: "Settings"
                    icon: "settings"
                    onClicked: Router.push("settings", { section: "downloads" })
                }
            }

            // disconnected banner (state may be stale until reconnect)
            Rectangle {
                Layout.fillWidth: true
                visible: !Daemon.connected
                implicitHeight: dcRow.implicitHeight + Style.sp(4)
                radius: Style.radius
                color: Tokens.tint5
                border.width: 1
                border.color: Tokens.line
                RowLayout {
                    id: dcRow
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(3)
                    anchors.rightMargin: Style.sp(3)
                    spacing: Style.sp(2)
                    Icon { name: "alert"; size: Style.fs.md; color: Style.alert }
                    Text {
                        Layout.fillWidth: true
                        text: "Disconnected from Ryotunes. This list will update when it reconnects. Interrupted downloads can be retried."
                        color: Tokens.inkDim
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        wrapMode: Text.WordWrap
                    }
                }
            }

            Text {
                Layout.fillWidth: true
                visible: Daemon.connected && (Downloads.error !== "" || (!Downloads.loaded && Downloads.loading))
                text: Downloads.error || "Loading downloads…"
                color: Downloads.error ? Style.alert : Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                wrapMode: Text.WordWrap
            }
            Action {
                visible: Daemon.connected && Downloads.error !== ""
                enabled: !Downloads.loading
                text: "Try again"
                onClicked: Downloads.refresh()
            }

            Hairline { Layout.fillWidth: true; height: 1 }

            // --- big empty state (no jobs at all) -----------------------------------------
            ColumnLayout {
                Layout.fillWidth: true
                Layout.topMargin: Style.sp(10)
                visible: page.jobs.length === 0 && Downloads.loaded && Downloads.error === ""
                spacing: Style.sp(2)
                Icon {
                    Layout.alignment: Qt.AlignHCenter
                    name: "download"
                    size: Style.fs.hero
                    color: Tokens.inkFaint
                }
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    text: Daemon.connected ? "No downloads yet" : "Not connected"
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.lg
                    font.weight: Font.Medium
                }
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    Layout.maximumWidth: Style.sp(120)
                    horizontalAlignment: Text.AlignHCenter
                    text: Daemon.connected
                        ? "Save a track with the download arrow beside its heart in the player. Downloads keep running when you close the window."
                        : "Your download history will update when Ryotunes reconnects."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.md
                    wrapMode: Text.WordWrap
                }
            }

            // --- queue --------------------------------------------------------------------
            ColumnLayout {
                Layout.fillWidth: true
                visible: page.jobs.length > 0
                spacing: Style.sp(2)

                SectionHeading {
                    Layout.fillWidth: true
                    title: page.queue.length > 0 ? "Queue \u00b7 " + page.queue.length : "Queue"
                    mark: "待"
                }
                Text {
                    Layout.fillWidth: true
                    visible: page.queue.length === 0
                    text: "Nothing downloading right now."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                }
                Repeater {
                    model: page.queueGroups
                    delegate: GroupRow {
                        required property var modelData
                        entry: modelData
                    }
                }
            }

            // --- history ------------------------------------------------------------------
            ColumnLayout {
                Layout.fillWidth: true
                Layout.topMargin: Style.sp(2)
                visible: page.jobs.length > 0
                spacing: Style.sp(2)

                SectionHeading {
                    Layout.fillWidth: true
                    title: page.history.length > 0 ? "History \u00b7 " + page.history.length : "History"
                    mark: "録"
                }
                Text {
                    Layout.fillWidth: true
                    text: "Latest 200 downloads. Clearing history keeps your music files."
                    color: Tokens.inkFaint
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    wrapMode: Text.WordWrap
                }
                Text {
                    Layout.fillWidth: true
                    visible: page.history.length === 0
                    text: "Finished downloads will appear here."
                    color: Tokens.inkMuted
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                }
                Repeater {
                    model: page.historyGroups
                    delegate: GroupRow {
                        required property var modelData
                        entry: modelData
                    }
                }
            }
        }
    }
}
