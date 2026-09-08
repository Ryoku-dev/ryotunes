pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import QtQuick.Effects
import Ryoku.Ui.Singletons
import "../"
import "../lib/personal.js" as P
import "../lib/playlist-links.js" as Links

// The Home "Add shortcut" picker. Instead of sending the user off to search, this raises a modal
// over Home with three ways to pin a shortcut: the recents/library the user already has, a title
// filter that falls through to a daemon search, and a paste-a-YouTube-playlist-link field for the
// playlists that only exist as a URL (not in search, not in the library). A pasted link is resolved
// to a `VL…` browse id (lib/playlist-links.js) and previewed through `get_playlist` — never by
// fetching the URL — so the tile carries the playlist's real title and cover and reopens/plays it.
//
// Public surface (the parent anchors it over the Home page bounds): `opened`, `open()`, `closed()`.
// It stays instantiated and hides itself when closed. Every async reply is guarded by a sequence
// token and the open flag, so a stale search or link fetch that lands after a re-query, a close, or
// a reopen is dropped.
Item {
    id: root

    anchors.fill: parent
    visible: root.opened
    z: 250

    property bool opened: false
    signal closed()

    // The sibling page content to snapshot-blur behind the card (spec §9); unset → scrim only.
    property Item blurSource: null

    // The filter box, and the daemon results it falls through to.
    property string query: ""
    property var library: []
    property bool libLoaded: false
    property var remote: []
    property bool searching: false
    property int searchSeq: 0
    property string searchError: ""
    property int librarySeq: 0

    // The paste-a-link field and its previewed target.
    property string linkInput: ""
    property var linkPreview: null
    property string linkError: ""
    property bool linkLoading: false
    property int linkSeq: 0

    // The grid is a fixed size; a suggestion never evicts a hand-added tile, so at capacity the
    // picker refuses with feedback rather than silently dropping the stalest shortcut.
    readonly property int capacity: P.MAX_PICKS || 18
    readonly property bool full: Personal.picks.length >= root.capacity

    // Recents (newest first, already minus current shortcuts) plus the user's library playlists,
    // deduped — the offline-friendly candidates shown before a search runs.
    readonly property var recents: root.opened ? Personal.recent(40) : []
    readonly property var localCandidates: {
        var out = [];
        var seen = ({});
        var add = function (it) {
            if (!it || !it.id || seen[it.id])
                return;
            seen[it.id] = true;
            out.push(it);
        };
        var r = root.recents;
        for (var i = 0; i < r.length; i++)
            add(r[i]);
        for (var j = 0; j < root.library.length; j++)
            add(root.library[j]);
        return out;
    }

    // What the list shows: with no query, the local candidates; with one, local title matches first
    // then the daemon results, deduped and always minus what is already a shortcut.
    readonly property var candidates: {
        var q = root.query.trim().toLowerCase();
        var picks = Personal.picks;
        var isPick = function (id) {
            for (var k = 0; k < picks.length; k++)
                if (picks[k].id === id)
                    return true;
            return false;
        };
        var out = [];
        var seen = ({});
        var add = function (it) {
            if (!it || !it.id || seen[it.id] || isPick(it.id))
                return;
            seen[it.id] = true;
            out.push(it);
        };
        var local = root.localCandidates;
        for (var i = 0; i < local.length; i++) {
            var it = local[i];
            if (!q || (String(it.title || "").toLowerCase().indexOf(q) >= 0)
                || (String(it.subtitle || "").toLowerCase().indexOf(q) >= 0))
                add(it);
        }
        if (q) {
            for (var j = 0; j < root.remote.length; j++)
                add(root.remote[j]);
        }
        return out.slice(0, 40);
    }

    function open() {
        root.query = "";
        root.remote = [];
        root.searching = false;
        root.searchSeq++;
        root.linkInput = "";
        root.linkPreview = null;
        root.linkError = "";
        root.linkLoading = false;
        root.linkSeq++;
        root.opened = true;
        if (root.blurSource)
            snap.scheduleUpdate();
        root.loadLibrary();
        Qt.callLater(function () { filterField.forceActiveFocus(); });
    }

    function close() {
        // Bump the guards so any in-flight reply that lands after close is ignored.
        root.searchSeq++;
        root.linkSeq++;
        root.opened = false;
        root.librarySeq++;
        searchTimer.stop();
        linkTimer.stop();
        root.closed();
    }

    function loadLibrary() {
        var seq = ++root.librarySeq;
        var provider = Playback.provider;
        root.library = [];
        Daemon.call("get_library")
            .then(function (items) {
                if (seq !== root.librarySeq || !root.opened || provider !== Playback.provider)
                    return;
                root.library = (items || []).filter(function (i) { return i && i.id && i.kind; });
            })
            .catch(function () {
                if (seq === root.librarySeq && root.opened)
                    root.searchError = "Your library could not be loaded. Search or paste a public playlist link.";
            });
    }

    function runSearch() {
        if (!root.opened)
            return;
        root.searchError = "";
        var q = root.query.trim().replace(/\s+/g, " ");
        var seq = ++root.searchSeq;
        var prov = Playback.provider;
        if (q.length < 2) {
            root.remote = [];
            root.searching = false;
            return;
        }
        root.searching = true;
        Daemon.call("search_all", { query: q })
            .then(function (res) {
                if (seq !== root.searchSeq || !root.opened || Playback.provider !== prov)
                    return;
                var flat = [];
                var pushAll = function (arr) {
                    if (!arr)
                        return;
                    for (var i = 0; i < arr.length; i++) {
                        var it = arr[i];
                        if (it && it.kind === "song" && it.isVideo)
                            continue;
                        if (it && it.id)
                            flat.push(it);
                    }
                };
                pushAll(res ? res.top : null);
                pushAll(res ? res.songs : null);
                pushAll(res ? res.albums : null);
                pushAll(res ? res.artists : null);
                pushAll(res ? res.playlists : null);
                root.remote = flat;
                root.searching = false;
            })
            .catch(function () {
                if (seq !== root.searchSeq || !root.opened || Playback.provider !== prov)
                    return;
                root.remote = [];
                root.searchError = "Search could not be loaded. Try again or paste a playlist link.";
                root.searching = false;
            });
    }

    function isShortcut(id) {
        var picks = Personal.picks;
        for (var i = 0; i < picks.length; i++)
            if (picks[i].id === id)
                return true;
        return false;
    }

    function pickFrom(it) {
        var item = {};
        for (var key in it)
            item[key] = it[key];
        return item;
    }

    function addCandidate(it) {
        if (!Personal.hydrated) {
            Playback.toast("Your listening library is still loading", "info");
            return;
        }
        if (!it || !it.id)
            return;
        if (root.isShortcut(it.id)) {
            Playback.toast("Already a shortcut", "info");
            return;
        }
        if (root.full) {
            Playback.toast("Shortcuts are full — remove one first", "info");
            return;
        }
        if (Personal.addPick(root.pickFrom(it)))
            Playback.toast("Pinned " + (it.title || "shortcut"), "success");
    }

    // --- pasted link ---------------------------------------------------------------------------
    function resolveLink() {
        if (!root.opened)
            return;
        var parsed = Links.parsePlaylistLink(root.linkInput);
        root.linkPreview = null;
        root.linkError = "";
        var seq = ++root.linkSeq;
        if (parsed.reason === "empty") {
            root.linkLoading = false;
            return;
        }
        if (!parsed.ok) {
            root.linkLoading = false;
            root.linkError = parsed.message;
            return;
        }
        root.linkLoading = true;
        Daemon.call("get_playlist", { id: parsed.id })
            .then(function (p) {
                if (seq !== root.linkSeq || !root.opened)
                    return;
                root.linkLoading = false;
                if (!p || !p.title) {
                    root.linkError = "That playlist is private, empty or unavailable.";
                    return;
                }
                root.linkPreview = {
                    id: parsed.id,
                    kind: "playlist",
                    title: p.title,
                    subtitle: (p && p.subtitle) ? p.subtitle : "",
                    thumbnail: (p && (p.cover || p.thumbnail)) ? (p.cover || p.thumbnail) : ""
                };
            })
            .catch(function (e) {
                if (seq !== root.linkSeq || !root.opened)
                    return;
                root.linkLoading = false;
                root.linkError = (e && e.message) ? e.message
                    : "That playlist could not be opened. It may be private or unavailable.";
            });
    }

    function addLink() {
        if (!Personal.hydrated) {
            Playback.toast("Your listening library is still loading", "info");
            return;
        }
        var t = root.linkPreview;
        if (!t)
            return;
        if (root.isShortcut(t.id)) {
            Playback.toast("Already a shortcut", "info");
            return;
        }
        if (root.full) {
            Playback.toast("Shortcuts are full — remove one first", "info");
            return;
        }
        Personal.addPick(root.pickFrom(t));
        Playback.toast("Pinned " + t.title, "success");
        root.linkInput = "";
        root.linkPreview = null;
        root.linkError = "";
    }

    onLinkInputChanged: {
        root.linkSeq++;
        root.linkLoading = false;
        root.linkPreview = null;
        root.linkError = "";
        if (root.opened)
            linkTimer.restart();
    }
    onQueryChanged: {
        root.searchSeq++;
        root.remote = [];
        root.searching = false;
        if (root.opened)
            searchTimer.restart();
    }
    Connections {
        target: Playback
        function onProviderChanged(): void {
            if (root.opened)
                root.open();
        }
    }

    Timer { id: searchTimer; interval: 250; onTriggered: root.runSearch() }
    Timer { id: linkTimer; interval: 300; onTriggered: root.resolveLink() }

    Keys.onEscapePressed: (event) => { root.close(); event.accepted = true; }

    // snapshot blur of the page behind, captured once on open (spec §9)
    ShaderEffectSource {
        id: snap
        anchors.fill: parent
        visible: false
        sourceItem: root.blurSource
        live: false
        hideSource: false
        recursive: false
    }
    MultiEffect {
        anchors.fill: parent
        visible: root.blurSource !== null && Style.blurEnabled
        source: snap
        blurEnabled: true
        blur: 1.0
        blurMax: 32
        autoPaddingEnabled: false
    }

    // dismiss layer + scrim
    MouseArea {
        anchors.fill: parent
        onClicked: root.close()
    }
    Rectangle {
        anchors.fill: parent
        color: "#000000"
        opacity: 0.5
    }

    Rectangle {
        id: card
        anchors.centerIn: parent
        width: Math.min(root.width - Style.sp(12), Style.sp(150))
        implicitHeight: sheet.implicitHeight + Style.sp(12)
        height: Math.min(implicitHeight, root.height - Style.sp(12))
        radius: Style.radiusCard
        color: Tokens.paper
        border.width: 1
        border.color: Tokens.line

        // swallow clicks so they don't reach the dismiss layer
        MouseArea { anchors.fill: parent }

        ColumnLayout {
            id: sheet
            anchors.fill: parent
            anchors.margins: Style.sp(6)
            spacing: Style.sp(3)

            // header
            RowLayout {
                Layout.fillWidth: true
                spacing: Style.sp(2)
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(0.5)
                    Text {
                        text: "// PIN / SHORTCUT"
                        color: Tokens.inkFaint
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        font.letterSpacing: Style.trackMicro
                    }
                    Text {
                        text: "Add a shortcut"
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.lg
                        font.weight: Font.DemiBold
                    }
                }
                IconButton {
                    icon: "close"
                    iconSize: Style.fs.md
                    diameter: Style.sp(8)
                    tip: "Close"
                    onClicked: root.close()
                }
            }

            Text {
                visible: root.full
                Layout.fillWidth: true
                text: "Shortcuts are full (" + root.capacity + "). Remove one on Home to add another."
                color: Style.alert
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                wrapMode: Text.WordWrap
            }

            // filter / search field
            Rectangle {
                Layout.fillWidth: true
                implicitHeight: Style.sp(10)
                radius: Style.radius
                color: Tokens.paperLift
                border.width: 1
                border.color: filterField.activeFocus ? Tokens.lineStrong : Tokens.line
                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(2)
                    spacing: Style.sp(2)
                    Icon { name: "search"; size: Style.fs.md; color: Tokens.inkMuted }
                    TextInput {
                        id: filterField
                        Layout.fillWidth: true
                        verticalAlignment: TextInput.AlignVCenter
                        clip: true
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        text: root.query
                        onTextChanged: root.query = text
                        onAccepted: root.runSearch()
                        Keys.onEscapePressed: (event) => { root.close(); event.accepted = true; }
                        Text {
                            anchors.verticalCenter: parent.verticalCenter
                            visible: filterField.text.length === 0
                            text: "Filter recents & library, or search…"
                            color: Tokens.inkFaint
                            font: filterField.font
                        }
                    }
                    Text {
                        visible: root.searching
                        text: "…"
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                    }
                }
            }

            // candidate list
            Text {
                visible: root.candidates.length === 0 && !root.searching
                Layout.fillWidth: true
                text: root.searchError || (root.query.trim().length > 0
                    ? "No matches. Try a different search, or paste a playlist link below."
                    : "Play something and it shows up here, or search above.")
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                wrapMode: Text.WordWrap
            }
            ListView {
                id: candidateList
                Layout.fillWidth: true
                Layout.fillHeight: true
                Layout.minimumHeight: 0
                Layout.preferredHeight: Math.min(contentHeight, Style.sp(72))
                visible: root.candidates.length > 0
                clip: true
                reuseItems: true
                boundsBehavior: Flickable.StopAtBounds
                model: root.candidates
                spacing: Style.sp(1)
                delegate: Rectangle {
                    id: candRow
                    required property var modelData
                    readonly property bool round: candRow.modelData && candRow.modelData.kind === "artist"
                    readonly property bool pinned: root.isShortcut(candRow.modelData.id)
                    width: candidateList.width
                    implicitHeight: Style.sp(12)
                    radius: Style.radius
                    color: candHover.hovered ? Tokens.tint5 : "transparent"
                    RowLayout {
                        anchors.fill: parent
                        anchors.leftMargin: Style.sp(2)
                        anchors.rightMargin: Style.sp(2)
                        spacing: Style.sp(3)
                        Artwork {
                            Layout.alignment: Qt.AlignVCenter
                            url: candRow.modelData.thumbnail ? candRow.modelData.thumbnail : ""
                            px: Style.sp(9)
                            round: candRow.round
                            placeholderIcon: candRow.round ? "user"
                                : candRow.modelData.kind === "song" ? "music" : "playlist"
                        }
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: Style.sp(0.5)
                            Text {
                                Layout.fillWidth: true
                                text: candRow.modelData.title ? candRow.modelData.title : ""
                                color: Tokens.ink
                                font.family: Style.fontUi
                                font.pixelSize: Style.fs.md
                                font.weight: Font.Medium
                                elide: Text.ElideRight
                            }
                            Text {
                                Layout.fillWidth: true
                                text: candRow.modelData.subtitle ? candRow.modelData.subtitle
                                    : (candRow.modelData.kind ? candRow.modelData.kind : "")
                                color: Tokens.inkMuted
                                font.family: Style.fontUi
                                font.pixelSize: Style.fs.sm
                                elide: Text.ElideRight
                                textFormat: Text.PlainText
                            }
                        }
                        // kind tag
                        Rectangle {
                            Layout.alignment: Qt.AlignVCenter
                            visible: !!candRow.modelData.kind
                            implicitHeight: Style.sp(6)
                            implicitWidth: kindLabel.implicitWidth + Style.sp(4)
                            radius: height / 2
                            color: "transparent"
                            border.width: 1
                            border.color: Tokens.line
                            Text {
                                id: kindLabel
                                anchors.centerIn: parent
                                text: candRow.modelData.kind ? String(candRow.modelData.kind).toUpperCase() : ""
                                color: Tokens.inkFaint
                                font.family: Style.fontMono
                                font.pixelSize: Style.fs.micro
                                font.letterSpacing: Style.trackMicro
                            }
                        }
                        Btn {
                            text: candRow.pinned ? "Added" : "Add"
                            icon: candRow.pinned ? "" : "add"
                            enabled: !candRow.pinned && !root.full
                            onClicked: root.addCandidate(candRow.modelData)
                        }
                    }
                    HoverHandler { id: candHover }
                }
            }

            // divider
            Rectangle { Layout.fillWidth: true; implicitHeight: 1; color: Tokens.lineSoft }

            // paste-a-link
            Text {
                text: "Or paste a YouTube playlist link"
                color: Tokens.inkDim
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                font.weight: Font.Medium
            }
            RowLayout {
                Layout.fillWidth: true
                spacing: Style.sp(2)
                Rectangle {
                    Layout.fillWidth: true
                    implicitHeight: Style.sp(10)
                    radius: Style.radius
                    color: Tokens.paperLift
                    border.width: 1
                    border.color: linkField.activeFocus ? Tokens.lineStrong : Tokens.line
                    RowLayout {
                        anchors.fill: parent
                        anchors.leftMargin: Style.sp(2)
                        anchors.rightMargin: Style.sp(2)
                        spacing: Style.sp(2)
                        Icon { name: "link"; size: Style.fs.md; color: Tokens.inkMuted }
                        TextInput {
                            id: linkField
                            Layout.fillWidth: true
                            verticalAlignment: TextInput.AlignVCenter
                            clip: true
                            color: Tokens.ink
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.md
                            text: root.linkInput
                            onTextChanged: root.linkInput = text
                            onAccepted: { if (root.linkPreview) root.addLink(); else root.resolveLink(); }
                            Keys.onEscapePressed: (event) => { root.close(); event.accepted = true; }
                            Text {
                                anchors.verticalCenter: parent.verticalCenter
                                visible: linkField.text.length === 0
                                text: "music.youtube.com/playlist?list=…"
                                color: Tokens.inkFaint
                                font: linkField.font
                            }
                        }
                    }
                }
                Btn {
                    text: "Add"
                    icon: "add"
                    primary: true
                    enabled: !!root.linkPreview && !root.isShortcut(root.linkPreview.id) && !root.full
                    onClicked: root.addLink()
                }
            }

            // link readout / preview / error
            Text {
                visible: root.linkLoading
                text: "Resolving playlist…"
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
            }
            Text {
                visible: root.linkError !== ""
                Layout.fillWidth: true
                text: root.linkError
                color: Style.alert
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                wrapMode: Text.WordWrap
            }
            Rectangle {
                Layout.fillWidth: true
                visible: !!root.linkPreview
                implicitHeight: Style.sp(12)
                radius: Style.radius
                color: Tokens.paperLift
                border.width: 1
                border.color: Tokens.line
                RowLayout {
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(2)
                    spacing: Style.sp(3)
                    Artwork {
                        Layout.alignment: Qt.AlignVCenter
                        url: root.linkPreview && root.linkPreview.thumbnail ? root.linkPreview.thumbnail : ""
                        px: Style.sp(9)
                        placeholderIcon: "playlist"
                    }
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(0.5)
                        Text {
                            Layout.fillWidth: true
                            text: root.linkPreview ? root.linkPreview.title : ""
                            color: Tokens.ink
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.md
                            font.weight: Font.Medium
                            elide: Text.ElideRight
                        }
                        Text {
                            Layout.fillWidth: true
                            text: root.linkPreview && root.linkPreview.subtitle ? root.linkPreview.subtitle : "Playlist"
                            color: Tokens.inkMuted
                            font.family: Style.fontUi
                            font.pixelSize: Style.fs.sm
                            elide: Text.ElideRight
                            textFormat: Text.PlainText
                        }
                    }
                    Text {
                        visible: root.linkPreview && root.isShortcut(root.linkPreview.id)
                        text: "Added"
                        color: Tokens.inkFaint
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                    }
                }
            }
        }
    }
}
