pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Quickshell
import Quickshell.Io
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The settings surface, ported from ui/src/lib/components/SettingsDialog.svelte (plus the account,
// local-folder and playlist-transfer controls the Svelte app spreads across its titlebar and
// library). Every UI_SETTINGS key the daemon accepts round-trips through set_setting; the theme
// mode drives Style.themeMode, which pins Tokens to the local light/dark palette so the whole chrome
// re-renders. File choices use zenity (the same picker the library Local tab already adopted; the
// daemon dropped its Tauri dialogs for an explicit path). Nothing here polls: discord status is
// fetched on open and after a toggle, never on an interval, honouring the no-idle-timer rule.
//
// Visual layer: a left section rail (unchanged navigation + lazy-load data flow) beside a scrolling
// body of two-column setting rows (label md + description sm | control) grouped under SectionHeadings,
// per spec section 5. The page fills the padded rect App gives it — no outer margins, no opaque paper
// base (App draws the Backdrop + paper@0.88 behind it); local card fills only.
Item {
    id: page

    property string section: "general"
    property var settings: ({})
    property var clients: []
    property bool loaded: false
    property string discordStatus: "disabled"
    property var folders: []
    property bool foldersLoaded: false
    property var identities: []
    property bool clearing: false
    property bool importing: false
    property bool exporting: false

    // Editable-field mirrors, seeded on load so typing never fights the settings binding.
    property string proxyInput: ""
    property string discordNameInput: "Ryotunes"
    property bool forkOpen: false
    property string forkName: ""

    // Software updates (About). Never polled — a check runs only on the button. The daemon dispatches
    // requests concurrently and imposes no RPC timeout, so the long install call simply resolves when
    // it is done; no background job protocol is needed.
    property var updateInfo: null
    property string updateStatus: "idle" // idle | checking | installing | installed | error
    property string updateError: ""
    property string installedVersion: ""
    property bool showReleaseNotes: false
    // The client (APP) version ships as a plain-text file in the QML package, stamped per release.
    // Kept separate from the daemon handshake version so a stale daemon stays visible after an update.
    property string appVersion: ""

    readonly property var qualities: [
        { id: "LOW", l: "Low" },
        { id: "AUTO", l: "Auto" },
        { id: "HIGH", l: "High" }
    ]
    readonly property var uiScales: [80, 90, 100, 110, 120, 130, 140]
    readonly property var sections: [
        { k: "general", l: "General", jp: "全般" },
        { k: "playback", l: "Playback", jp: "再生" },
        { k: "downloads", l: "Downloads", jp: "保存" },
        { k: "data", l: "Data & storage", jp: "保存" },
        { k: "account", l: "Account", jp: "鍵" },
        { k: "local", l: "Local music", jp: "音源" },
        { k: "playlists", l: "Playlists", jp: "転送" },
        { k: "about", l: "About", jp: "力" }
    ]

    Component.onCompleted: {
        page.load();
        var requested = Router.current && Router.current.params ? Router.current.params.section : "";
        if (page.sections.some((s) => s.k === requested)) page.selectSection(requested);
    }

    // --- data ------------------------------------------------------------------------------
    function load() {
        Promise.all([
            Daemon.call("get_settings").catch(() => ({})),
            Daemon.call("get_stream_clients").catch(() => [])
        ]).then((res) => {
            page.settings = res[0] || ({});
            page.clients = res[1] || [];
            page.proxyInput = page.settings.proxy || "";
            page.discordNameInput = (page.settings.discord_presence_name || "").trim() || "Ryotunes";
            page.loaded = true;
            page.loadDiscord();
        }).catch((e) => { page.loaded = true; Playback.toast((e && e.message) ? e.message : String(e), "error"); });
    }

    function selectSection(k) {
        page.section = k;
        scroll.contentY = 0;
        if (k === "local" && !page.foldersLoaded) page.scanFolders();
        if (k === "account") page.loadIdentities();
        if (k === "general") page.loadDiscord();
    }

    // --- appearance (skin) -----------------------------------------------------------------
    function chooseSkin(id) { Prefs.skin = id; Prefs.save(); }
    function skinMode(m) { return (m && m["default"] === "light") ? "light" : "dark"; }
    function skinSwatches(entry) {
        var mode = page.skinMode(entry.manifest);
        var p = Object.assign({}, Skin.fallback.modes[mode], Skin.palette(mode, entry.manifest));
        return [p.paper, p.paperLift, p.ink, p.sun, p.bone];
    }
    function skinBadge(entry) { return entry.generated ? "GENERATED" : entry.source.toUpperCase(); }
    function kebab(s) {
        return (s || "").toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
    }
    function createSkin() {
        var id = page.kebab(page.forkName);
        if (!id) { Playback.toast("Name the new skin first", "error"); return; }
        var path = Skin.forkCurrent(id);
        Prefs.skin = id; Prefs.save();
        Quickshell.execDetached(["xdg-open", path]);
        page.forkOpen = false; page.forkName = "";
        Playback.toast("Created skin “" + id + "” — opening it to edit", "success");
    }

    function applyLocal(key, value) {
        var s = ({});
        for (var k in page.settings) s[k] = page.settings[k];
        s[key] = value;
        page.settings = s;
        // Keep the chrome that binds Playback.settings (the titlebar's Discord light) coherent.
        var ps = ({});
        for (var k2 in Playback.settings) ps[k2] = Playback.settings[k2];
        ps[key] = value;
        Playback.settings = ps;
    }
    function setSetting(key, value) {
        page.applyLocal(key, value);
        return Daemon.call("set_setting", { key: key, value: value })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not save setting", "error"));
    }

    function loadDiscord() {
        Daemon.call("discord_status")
            .then((d) => { if (d && d.status) page.discordStatus = d.status; })
            .catch(() => {});
    }
    function setDiscord(on) {
        page.setSetting("discord_rpc", on ? "true" : "false").then(() => page.loadDiscord());
    }
    function saveDiscordName() {
        var value = page.discordNameInput.trim() || "Ryotunes";
        var n = value.length;
        if (n < 2 || n > 128) {
            Playback.toast("Discord presence title must be between 2 and 128 characters", "error");
            return;
        }
        page.discordNameInput = value;
        page.setSetting("discord_presence_name", value)
            .then(() => Playback.toast("Discord now shows “Listening to " + value + "”", "success"));
    }
    function setAutostart(on) {
        var prev = page.settings.autostart;
        page.applyLocal("autostart", on ? "true" : "false");
        Daemon.call("set_setting", { key: "autostart", value: on ? "true" : "false" })
            .catch((e) => { page.applyLocal("autostart", prev || "false"); Playback.toast((e && e.message) ? e.message : "Could not change autostart", "error"); });
    }
    function setQuality(q) {
        page.setSetting("quality", q)
            .then(() => Daemon.call("clear_caches").catch(() => {}))
            .then(() => Playback.toast("Audio quality updated", "success"));
    }
    function saveProxy() {
        var value = page.proxyInput.trim();
        page.setSetting("proxy", value)
            .then(() => { page.proxyInput = value; Playback.toast("Proxy saved — restart to apply", "success"); });
    }
    function clearCaches() {
        page.clearing = true;
        Daemon.call("clear_caches")
            .then(() => { page.clearing = false; Playback.toast("Caches cleared", "success"); })
            .catch((e) => { page.clearing = false; Playback.toast((e && e.message) ? e.message : String(e), "error"); });
    }

    // --- software updates ------------------------------------------------------------------
    function checkUpdates() {
        if (page.updateStatus === "checking" || page.updateStatus === "installing") return;
        page.updateStatus = "checking";
        page.updateError = "";
        page.showReleaseNotes = false;
        Daemon.call("check_for_updates")
            .then((info) => { page.updateInfo = info || null; page.updateStatus = "idle"; })
            .catch((e) => { page.updateError = (e && e.message) ? e.message : String(e); page.updateStatus = "error"; });
    }
    function installUpdate() {
        if (page.updateStatus === "installing") return;
        var info = page.updateInfo;
        if (!info || !info.latestVersion || !info.available || !info.canInstall) return;
        var version = info.latestVersion;
        page.updateStatus = "installing";
        page.updateError = "";
        Daemon.call("install_update", { version: version })
            .then((res) => { page.installedVersion = (res && res.version) ? res.version : version; page.updateStatus = "installed"; })
            .catch((e) => { page.updateError = (e && e.message) ? e.message : String(e); page.updateStatus = "error"; });
    }
    function updateMessage() {
        if (page.updateStatus === "checking") return "Checking for a new version…";
        if (page.updateStatus === "installing") return "Downloading and verifying the update, then asking for administrator approval. Keep Ryotunes open until it finishes.";
        if (page.updateStatus === "installed") return "Ryotunes " + page.installedVersion + " is installed. Quit Ryotunes and open it again to finish — closing the window isn't enough; the background service reloads only on a full restart.";
        if (page.updateStatus === "error") return "Update failed: " + page.updateError;
        var info = page.updateInfo;
        if (!info) return "";
        if (info.latestVersion === null || info.latestVersion === undefined) return "No release has been published yet.";
        if (info.available) {
            var msg = "Version " + info.latestVersion + " is available.";
            if (!info.canInstall && info.unsupportedReason) msg += " " + info.unsupportedReason;
            return msg;
        }
        return "You're on the latest version.";
    }
    function openUrl(url) { Quickshell.execDetached(["xdg-open", url]); }

    function clientDisabled(name) {
        return (page.settings.disabled_stream_clients || "").split(",").map((s) => s.trim()).filter(Boolean).indexOf(name) >= 0;
    }
    function toggleClient(name) {
        var set = (page.settings.disabled_stream_clients || "").split(",").map((s) => s.trim()).filter(Boolean);
        var i = set.indexOf(name);
        if (i >= 0) set.splice(i, 1); else set.push(name);
        page.setSetting("disabled_stream_clients", set.join(","));
    }

    // --- account ---------------------------------------------------------------------------
    function loadIdentities() {
        Daemon.call("get_account_identities")
            .then((rows) => { page.identities = rows || []; })
            .catch(() => { page.identities = []; });
    }
    function switchAccount(key) {
        Daemon.call("switch_account", { selectionKey: key })
            .then(() => { page.loadIdentities(); Playback.toast("Account switched", "success"); })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not switch account", "error"));
    }

    // --- local folders ---------------------------------------------------------------------
    function scanFolders() {
        page.foldersLoaded = true;
        Daemon.call("get_local_library")
            .then((l) => { page.folders = (l && l.folders) ? l.folders : []; })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not scan local music", "error"));
    }
    function addFolder(path) {
        if (!path) return;
        Daemon.call("add_local_folder", { path: path })
            .then((l) => { if (l && l.folders) page.folders = l.folders; })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not add folder", "error"));
    }
    function removeFolder(path) {
        Daemon.call("remove_local_folder", { path: path })
            .then((l) => { page.folders = (l && l.folders) ? l.folders : []; })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not remove folder", "error"));
    }

    // --- playlist transfer -----------------------------------------------------------------
    function doImport(path) {
        page.importing = true;
        Daemon.call("import_playlist_file", { path: path })
            .then((transfer) => {
                if (!transfer || !transfer.items || !transfer.items.length) {
                    page.importing = false;
                    Playback.toast("That playlist file has no tracks.", "error");
                    return;
                }
                return Daemon.call("create_playlist", { title: transfer.title }).then((id) => {
                    var chain = Promise.resolve();
                    var added = 0;
                    for (var i = 0; i < transfer.items.length; i++) {
                        (function (song) {
                            chain = chain.then(() => Daemon.call("add_to_playlist", { playlistId: id, videoId: song.video_id })
                                .then((ok) => { if (ok) added++; })
                                .catch(() => {}));
                        })(transfer.items[i]);
                    }
                    return chain.then(() => {
                        page.importing = false;
                        Playback.toast("Imported " + added + (added === 1 ? " song" : " songs"), "success");
                    });
                });
            })
            .catch((e) => { page.importing = false; Playback.toast((e && e.message) ? e.message : "Could not import", "error"); });
    }
    function doExport(path) {
        var items = (Playback.queue && Playback.queue.items) ? Playback.queue.items : [];
        if (!items.length) return;
        var title = (Playback.queue && Playback.queue.sourceName) ? Playback.queue.sourceName : "Ryotunes Queue";
        page.exporting = true;
        Daemon.call("export_playlist_file", { title: title, items: items, path: path })
            .then(() => { page.exporting = false; Playback.toast("Queue exported", "success"); })
            .catch((e) => { page.exporting = false; Playback.toast((e && e.message) ? e.message : "Could not export", "error"); });
    }

    function discordLabel(s) {
        return s === "connected" ? "Connected"
            : s === "connecting" ? "Connecting…"
            : s === "unavailable" ? "Discord not running / unavailable"
            : "Disabled";
    }

    // The client version file shipped beside the QML. blockLoading makes it ready on open; watching it
    // means a package upgrade updates the APP row live while the daemon row stays on its handshake
    // version until Ryotunes is fully restarted.
    FileView {
        id: versionFile
        path: Quickshell.shellDir + "/version"
        blockLoading: true
        watchChanges: true
        printErrors: false
        onLoaded: page.appVersion = (versionFile.text() || "").trim()
        onFileChanged: reload()
    }

    // --- pickers (zenity; the daemon expects an explicit path) ------------------------------
    Process {
        id: folderPicker
        command: ["zenity", "--file-selection", "--directory", "--title=Choose a music folder"]
        stdout: StdioCollector {
            id: folderOut
            onStreamFinished: { var p = folderOut.text.trim(); if (p) page.addFolder(p); }
        }
    }
    Process {
        id: importPicker
        command: ["zenity", "--file-selection", "--title=Import a playlist file"]
        stdout: StdioCollector {
            id: importOut
            onStreamFinished: { var p = importOut.text.trim(); if (p) page.doImport(p); }
        }
    }
    Process {
        id: exportPicker
        command: ["zenity", "--file-selection", "--save", "--confirm-overwrite", "--title=Export the queue", "--filename=ryotunes-queue.json"]
        stdout: StdioCollector {
            id: exportOut
            onStreamFinished: { var p = exportOut.text.trim(); if (p) page.doExport(p); }
        }
    }

    // A two-column on/off setting row: label + description in a fillWidth column, the shared Toggle
    // pinned to the right. Unidirectional like the component — renders `value`, emits `flipped`, the
    // host writes the daemon setting and the binding feeds the new value back.
    component ToggleRow: RowLayout {
        id: toggleRow
        property string title: ""
        property string desc: ""
        property string note: ""
        property bool value: false
        signal flipped(bool on)

        Layout.fillWidth: true
        spacing: Style.sp(4)

        ColumnLayout {
            Layout.fillWidth: true
            spacing: 1
            Text {
                Layout.fillWidth: true
                text: toggleRow.title
                color: Tokens.ink
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
                font.weight: Font.Medium
                wrapMode: Text.WordWrap
            }
            Text {
                Layout.fillWidth: true
                visible: toggleRow.desc !== ""
                text: toggleRow.desc
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                wrapMode: Text.WordWrap
            }
            Text {
                visible: toggleRow.note !== ""
                text: toggleRow.note
                color: Tokens.inkFaint
                font.family: Style.fontMono
                font.pixelSize: Style.fs.xs
            }
        }
        Toggle {
            Layout.alignment: Qt.AlignVCenter
            checked: toggleRow.value
            onToggled: (v) => toggleRow.flipped(v)
        }
    }

    RowLayout {
        anchors.fill: parent
        spacing: 0

        // --- section rail -------------------------------------------------------------------
        // Frosted over the App backdrop: transparent so the paper@0.88 + bloom read through; only the
        // right hairline and the active bone plate carry weight.
        Rectangle {
            Layout.preferredWidth: Style.sp(46)
            Layout.fillHeight: true
            color: "transparent"
            Hairline { anchors.right: parent.right; width: 1; height: parent.height }

            ColumnLayout {
                anchors.fill: parent
                anchors.margins: Style.sp(3)
                spacing: Style.sp(1)

                ColumnLayout {
                    Layout.fillWidth: true
                    Layout.bottomMargin: Style.sp(2)
                    spacing: 1
                    Text {
                        text: "力 // SETTINGS"
                        color: Tokens.inkDim
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        font.weight: Font.DemiBold
                        font.letterSpacing: 1
                    }
                    Text {
                        text: "RYOTUNES // 設定"
                        color: Tokens.inkFaint
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.xs
                        font.letterSpacing: 1
                    }
                }

                Repeater {
                    model: page.sections
                    delegate: Rectangle {
                        id: railItem
                        required property var modelData
                        readonly property bool current: page.section === railItem.modelData.k
                        Layout.fillWidth: true
                        implicitHeight: Style.sp(9)
                        radius: Style.radius
                        color: railItem.current ? Tokens.bone : railHover.hovered ? Tokens.tint10 : "transparent"

                        activeFocusOnTab: true
                        Accessible.role: Accessible.Button
                        Accessible.name: railItem.modelData.l + " settings"
                        Accessible.onPressAction: page.selectSection(railItem.modelData.k)
                        Keys.onReturnPressed: page.selectSection(railItem.modelData.k)
                        Keys.onSpacePressed: page.selectSection(railItem.modelData.k)
                        border.width: activeFocus ? 2 : 0
                        border.color: Tokens.ink
                        RowLayout {
                            anchors.fill: parent
                            anchors.leftMargin: Style.sp(2.5)
                            anchors.rightMargin: Style.sp(2.5)
                            spacing: Style.sp(2)
                            Text {
                                Layout.fillWidth: true
                                text: (railItem.current ? "// " : "") + railItem.modelData.l
                                color: railItem.current ? Tokens.inkOnBone : Tokens.inkDim
                                font.family: Style.fontUi
                                font.pixelSize: Style.fs.md
                                font.weight: Font.Medium
                                elide: Text.ElideRight
                            }
                            Text {
                                text: railItem.modelData.jp
                                color: railItem.current ? Tokens.inkOnBone : Tokens.inkFaint
                                opacity: railItem.current ? 0.8 : 0.55
                                font.family: Tokens.jp
                                font.pixelSize: Style.fs.sm
                            }
                        }
                        HoverHandler { id: railHover }
                        MouseArea {
                            anchors.fill: parent
                            cursorShape: Qt.PointingHandCursor
                            onClicked: page.selectSection(railItem.modelData.k)
                        }
                    }
                }
                Item { Layout.fillHeight: true }
            }
        }

        // --- section body -------------------------------------------------------------------
        Item {
            Layout.fillWidth: true
            Layout.fillHeight: true

            Text {
                anchors.centerIn: parent
                visible: !page.loaded
                text: "Loading…"
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
            }

            Flickable {
                id: scroll
                anchors.fill: parent
                visible: page.loaded
                clip: true
                contentWidth: width
                boundsBehavior: Flickable.StopAtBounds
                contentHeight: {
                    var h = 0;
                    if (page.section === "general") h = generalCol.implicitHeight;
                    else if (page.section === "playback") h = playbackCol.implicitHeight;
                    else if (page.section === "data") h = dataCol.implicitHeight;
                    else if (page.section === "downloads") h = downloadSettingsLoader.item ? downloadSettingsLoader.item.implicitHeight : 0;
                    else if (page.section === "account") h = accountCol.implicitHeight;
                    else if (page.section === "local") h = localCol.implicitHeight;
                    else if (page.section === "playlists") h = playlistsCol.implicitHeight;
                    else h = aboutCol.implicitHeight;
                    return h + Style.sp(16);
                }

                // ─────────────────────────── GENERAL ───────────────────────────
                ColumnLayout {
                    id: generalCol
                    visible: page.section === "general"
                    anchors { left: parent.left; right: parent.right; top: parent.top; leftMargin: Style.sp(6); rightMargin: Style.sp(6); topMargin: Style.sp(4) }
                    spacing: Style.sp(3)

                    // Appearance ------------------------------------------------------------
                    SectionHeading { Layout.fillWidth: true; title: "Appearance"; mark: "表示" }

                    // Skin — Prefs.skin picks the palette source (see Skin.qml); clicking a card
                    // writes it and saves, and Skin re-applies through Tokens so the chrome repaints.
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(1)
                        Text { Layout.fillWidth: true; text: "Skin"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium; wrapMode: Text.WordWrap }
                        Text {
                            Layout.fillWidth: true
                            text: "The palette, type, motion and decor Ryotunes wears. System follows your Ryoku desktop; install more from RyoStore, or drop a folder in ~/.config/ryotunes/skins to add your own."
                            color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                        }

                        Flow {
                            Layout.fillWidth: true
                            Layout.topMargin: Style.sp(2)
                            spacing: Style.sp(3)

                            SkinCard {
                                title: "System"
                                author: "Follow the Ryoku desktop"
                                swatches: [Tokens.paper, Tokens.paperLift, Tokens.ink, Tokens.sun, Tokens.bone]
                                active: Skin.followSystem
                                onClicked: page.chooseSkin("system")
                            }
                            Repeater {
                                model: Skin.all
                                delegate: SkinCard {
                                    required property var modelData
                                    title: modelData.name
                                    author: modelData.author !== "" ? modelData.author : "unknown"
                                    swatches: page.skinSwatches(modelData)
                                    badge: page.skinBadge(modelData)
                                    active: !Skin.followSystem && Skin.id === modelData.id
                                    onClicked: page.chooseSkin(modelData.id)
                                }
                            }
                        }

                        // Status: the last load/parse problem in the accent, or the fell-back mode note.
                        Text {
                            Layout.fillWidth: true
                            Layout.topMargin: Style.sp(1)
                            visible: Skin.error !== ""
                            text: Skin.error
                            color: Style.accent; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                        }
                        Text {
                            Layout.fillWidth: true
                            Layout.topMargin: Style.sp(1)
                            visible: Skin.error === "" && Skin.modeMissing
                            text: "This skin has no " + Skin.mode + " mode; showing its " + (Skin.mode === "dark" ? "light" : "dark") + "."
                            color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                        }

                        Flow {
                            Layout.fillWidth: true
                            Layout.topMargin: Style.sp(2)
                            spacing: Style.sp(2)
                            // RyoStore's `ryotunes-skins` category, when the store is on this box.
                            Btn { text: "Get more skins"; icon: "download"; primary: true; visible: Skin.storeAvailable; onClicked: Quickshell.execDetached(["ryostore", "open", "ryotunes-skins"]) }
                            Btn { text: "Open skins folder"; icon: "library"; onClicked: Quickshell.execDetached(["xdg-open", Skin.userDir]) }
                            Btn { text: "New skin from current"; icon: "add"; onClicked: page.forkOpen = !page.forkOpen }
                            Btn { text: "Reload"; icon: "repeat"; onClicked: Skin.reload() }
                            Btn { text: "Skins guide"; icon: "link"; onClicked: Quickshell.execDetached(["xdg-open", "https://github.com/ryoku-dev/ryotunes/blob/main/docs/SKINS.md"]) }
                        }

                        // Fork the painted palette into a new user skin — kebab-cased id, opened to edit.
                        RowLayout {
                            Layout.fillWidth: true
                            Layout.topMargin: Style.sp(1)
                            visible: page.forkOpen
                            spacing: Style.sp(2)
                            Rectangle {
                                Layout.fillWidth: true
                                Layout.maximumWidth: Style.sp(80)
                                implicitHeight: Style.sp(9)
                                radius: Style.radius
                                color: Tokens.paperLift
                                border.width: 1
                                border.color: forkField.activeFocus ? Tokens.lineStrong : Tokens.line
                                TextInput {
                                    id: forkField
                                    anchors.fill: parent
                                    anchors.leftMargin: Style.sp(2)
                                    anchors.rightMargin: Style.sp(2)
                                    verticalAlignment: TextInput.AlignVCenter
                                    clip: true
                                    color: Tokens.ink
                                    font.family: Style.fontUi
                                    font.pixelSize: Style.fs.md
                                    text: page.forkName
                                    onTextChanged: page.forkName = text
                                    onAccepted: page.createSkin()
                                    Text {
                                        anchors.verticalCenter: parent.verticalCenter
                                        visible: forkField.text.length === 0
                                        text: "my-skin-name"
                                        color: Tokens.inkFaint
                                        font: forkField.font
                                    }
                                }
                            }
                            Btn { text: "Create"; primary: true; onClicked: page.createSkin() }
                        }
                    }

                    // Theme → Prefs.themeMode (+ save); Skin pins Tokens to the mode, so the chrome re-themes.
                    RowLayout {
                        Layout.fillWidth: true
                        Layout.topMargin: Style.sp(2)
                        spacing: Style.sp(4)
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 1
                            Text { Layout.fillWidth: true; text: "Theme"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium; wrapMode: Text.WordWrap }
                            Text {
                                Layout.fillWidth: true
                                text: "Follow the desktop automatically, or pin Ryotunes to the skin's light or dark mode."
                                color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                            }
                        }
                        RowLayout {
                            Layout.alignment: Qt.AlignVCenter
                            spacing: Style.sp(2)
                            Repeater {
                                model: [ { m: "system", l: "System" }, { m: "light", l: "Light" }, { m: "dark", l: "Dark" } ]
                                delegate: Chip {
                                    required property var modelData
                                    text: modelData.l
                                    active: Prefs.themeMode === modelData.m
                                    onClicked: { Prefs.themeMode = modelData.m; Prefs.save(); }
                                }
                            }
                        }
                    }

                    // Decor → Prefs.decor (+ save). "skin" follows the skin's own level; rich/calm override it.
                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(4)
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 1
                            Text { Layout.fillWidth: true; text: "Decor"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium; wrapMode: Text.WordWrap }
                            Text {
                                Layout.fillWidth: true
                                text: "Skin default keeps the skin's own level; Rich layers grain, register crosses and kana seals over the paper; Calm keeps it plain."
                                color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                            }
                        }
                        RowLayout {
                            Layout.alignment: Qt.AlignVCenter
                            spacing: Style.sp(2)
                            Repeater {
                                model: [ { m: "skin", l: "Skin default" }, { m: "rich", l: "Rich" }, { m: "calm", l: "Calm" } ]
                                delegate: Chip {
                                    required property var modelData
                                    text: modelData.l
                                    active: Prefs.decor === modelData.m
                                    onClicked: { Prefs.decor = modelData.m; Prefs.save(); }
                                }
                            }
                        }
                    }

                    // Session ---------------------------------------------------------------
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "Session"; mark: "全般" }

                    ToggleRow {
                        title: "Watch history"
                        desc: (Playback.auth && Playback.auth.signedIn) ? "Register completed plays in your YouTube Music history." : "Sign in to register completed plays in your YouTube Music history."
                        value: page.settings.enable_history !== "false"
                        onFlipped: (on) => page.setSetting("enable_history", on ? "true" : "false")
                    }
                    ToggleRow {
                        title: "Close to tray"
                        desc: "Closing the window keeps music playing in the background."
                        value: page.settings.close_to_tray !== "false"
                        onFlipped: (on) => page.setSetting("close_to_tray", on ? "true" : "false")
                    }
                    ToggleRow {
                        title: "Low resource mode"
                        desc: "Disable speculative stream warming and reduce automatic Home/network work and decorative motion."
                        value: page.settings.low_resource_mode === "true"
                        onFlipped: (on) => page.setSetting("low_resource_mode", on ? "true" : "false")
                    }
                    ToggleRow {
                        title: "Start on login"
                        desc: "Launch Ryotunes automatically when you log in."
                        value: page.settings.autostart === "true"
                        onFlipped: (on) => page.setAutostart(on)
                    }

                    // Interface scale — a wide chip set, stacked under the label.
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(1)
                        Text { text: "Interface scale"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium }
                        Text {
                            Layout.fillWidth: true
                            text: "Preferred renderer scale, persisted for every Ryotunes client on this account."
                            color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                        }
                        Flow {
                            Layout.fillWidth: true
                            Layout.topMargin: Style.sp(1)
                            spacing: Style.sp(2)
                            Repeater {
                                model: page.uiScales
                                delegate: Chip {
                                    required property var modelData
                                    text: modelData + "%"
                                    active: Number(page.settings.ui_scale || "110") === modelData
                                    onClicked: page.setSetting("ui_scale", String(modelData))
                                }
                            }
                        }
                    }

                    // Discord ---------------------------------------------------------------
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "Discord"; mark: "接続" }

                    ToggleRow {
                        title: "Discord rich presence"
                        desc: "Show what you're listening to on your Discord profile through the local Discord client."
                        note: "Status: " + page.discordLabel(page.discordStatus)
                        value: page.settings.discord_rpc === "true"
                        onFlipped: (on) => page.setDiscord(on)
                    }

                    // Discord presence title — a text field, stacked.
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(1)
                        Text { text: "Discord presence title"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium }
                        Text {
                            Layout.fillWidth: true
                            text: "The text Discord renders as “Listening to …”."
                            color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                        }
                        RowLayout {
                            Layout.topMargin: Style.sp(1)
                            Layout.fillWidth: true
                            spacing: Style.sp(2)
                            Rectangle {
                                Layout.preferredWidth: Style.sp(70)
                                implicitHeight: Style.sp(9)
                                radius: Style.radius
                                color: Tokens.paperLift
                                border.width: 1
                                border.color: discordField.activeFocus ? Tokens.lineStrong : Tokens.line
                                TextInput {
                                    id: discordField
                                    anchors.fill: parent
                                    anchors.leftMargin: Style.sp(2)
                                    anchors.rightMargin: Style.sp(2)
                                    verticalAlignment: TextInput.AlignVCenter
                                    clip: true
                                    maximumLength: 128
                                    color: Tokens.ink
                                    font.family: Style.fontUi
                                    font.pixelSize: Style.fs.md
                                    text: page.discordNameInput
                                    onTextChanged: page.discordNameInput = text
                                    onAccepted: page.saveDiscordName()
                                    Text {
                                        anchors.verticalCenter: parent.verticalCenter
                                        visible: discordField.text.length === 0
                                        text: "Ryotunes"
                                        color: Tokens.inkFaint
                                        font: discordField.font
                                    }
                                }
                            }
                            Pill { label: "Save"; enabled: page.discordNameInput.trim().length > 0; onClicked: page.saveDiscordName() }
                            Pill { label: "Reset"; enabled: page.discordNameInput !== "Ryotunes"; onClicked: { page.discordNameInput = "Ryotunes"; page.saveDiscordName(); } }
                        }
                        Text {
                            text: "Preview: Listening to " + (page.discordNameInput.trim() || "Ryotunes")
                            color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.xs
                        }
                    }
                }

                // ─────────────────────────── PLAYBACK ───────────────────────────
                ColumnLayout {
                    id: playbackCol
                    visible: page.section === "playback"
                    anchors { left: parent.left; right: parent.right; top: parent.top; leftMargin: Style.sp(6); rightMargin: Style.sp(6); topMargin: Style.sp(4) }
                    spacing: Style.sp(3)

                    // Sound — the Ryotunes-only effect chain; opens the Sound dialog.
                    SectionHeading { Layout.fillWidth: true; title: "Sound"; mark: "音響" }

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(4)
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 1
                            Text { Layout.fillWidth: true; text: "Sound"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium; wrapMode: Text.WordWrap }
                            Text {
                                Layout.fillWidth: true
                                text: "Shape playback with tempo, pitch, reverb, bass and stereo width — and one-tap presets like Slowed + Reverb."
                                color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                            }
                        }
                        Btn {
                            Layout.alignment: Qt.AlignVCenter
                            text: "Open"
                            icon: "sound"
                            onClicked: Playback.soundRequested()
                        }
                    }

                    // Engine ----------------------------------------------------------------
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "Engine"; mark: "再生" }

                    // Audio quality
                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(4)
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 1
                            Text { Layout.fillWidth: true; text: "Audio quality"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium; wrapMode: Text.WordWrap }
                            Text {
                                Layout.fillWidth: true
                                text: "Preferred stream quality when resolving a track. Changing it clears cached URLs."
                                color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                            }
                        }
                        RowLayout {
                            Layout.alignment: Qt.AlignVCenter
                            spacing: Style.sp(2)
                            Repeater {
                                model: page.qualities
                                delegate: Chip {
                                    required property var modelData
                                    text: modelData.l
                                    active: (page.settings.quality || "HIGH") === modelData.id
                                    onClicked: page.setQuality(modelData.id)
                                }
                            }
                        }
                    }

                    ToggleRow {
                        title: "Autoplay"
                        desc: "Keep the music going with similar songs when your queue ends."
                        value: page.settings.autoplay !== "false"
                        onFlipped: (on) => page.setSetting("autoplay", on ? "true" : "false")
                    }
                    ToggleRow {
                        title: "Prevent duplicate tracks in queue"
                        desc: "Adding a track already queued moves it instead of adding a second copy."
                        value: page.settings.prevent_duplicates === "true"
                        onFlipped: (on) => page.setSetting("prevent_duplicates", on ? "true" : "false")
                    }
                    ToggleRow {
                        title: "Word-by-word lyrics"
                        desc: "Ask lyrics-api.boidu.dev first for per-word timings. Turning this off keeps your listening off that service; line-by-line lyrics still work."
                        value: page.settings.lyrics_boidu !== "false"
                        onFlipped: (on) => page.setSetting("lyrics_boidu", on ? "true" : "false")
                    }

                    // Stream clients --------------------------------------------------------
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "Stream clients"; mark: "詳細" }
                    Text {
                        Layout.fillWidth: true
                        text: "Advanced — turn a client off to skip it when resolving streams."
                        color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                    }
                    Repeater {
                        model: page.clients
                        delegate: RowLayout {
                            id: clientRow
                            required property var modelData
                            Layout.fillWidth: true
                            spacing: Style.sp(4)
                            Text {
                                Layout.fillWidth: true
                                text: clientRow.modelData
                                color: Tokens.inkDim
                                font.family: Style.fontMono
                                font.pixelSize: Style.fs.sm
                            }
                            Toggle {
                                Layout.alignment: Qt.AlignVCenter
                                checked: !page.clientDisabled(clientRow.modelData)
                                onToggled: () => page.toggleClient(clientRow.modelData)
                            }
                        }
                    }
                }

                Loader {
                    id: downloadSettingsLoader
                    active: page.section === "downloads"
                    visible: active
                    anchors { left: parent.left; right: parent.right; top: parent.top; leftMargin: Style.sp(6); rightMargin: Style.sp(6); topMargin: Style.sp(4) }
                    source: "DownloadSettings.qml"
                }

                // ─────────────────────────── DATA ───────────────────────────
                ColumnLayout {
                    id: dataCol
                    visible: page.section === "data"
                    anchors { left: parent.left; right: parent.right; top: parent.top; leftMargin: Style.sp(6); rightMargin: Style.sp(6); topMargin: Style.sp(4) }
                    spacing: Style.sp(3)

                    // Network ---------------------------------------------------------------
                    SectionHeading { Layout.fillWidth: true; title: "Network"; mark: "経路" }

                    // Proxy — a text field, stacked.
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(1)
                        Text { text: "Proxy"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium }
                        Text {
                            Layout.fillWidth: true
                            text: "HTTP or HTTPS proxy for all YouTube traffic. Takes effect on restart."
                            color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                        }
                        RowLayout {
                            Layout.topMargin: Style.sp(1)
                            Layout.fillWidth: true
                            spacing: Style.sp(2)
                            Rectangle {
                                Layout.fillWidth: true
                                Layout.maximumWidth: Style.sp(120)
                                implicitHeight: Style.sp(9)
                                radius: Style.radius
                                color: Tokens.paperLift
                                border.width: 1
                                border.color: proxyField.activeFocus ? Tokens.lineStrong : Tokens.line
                                TextInput {
                                    id: proxyField
                                    anchors.fill: parent
                                    anchors.leftMargin: Style.sp(2)
                                    anchors.rightMargin: Style.sp(2)
                                    verticalAlignment: TextInput.AlignVCenter
                                    clip: true
                                    color: Tokens.ink
                                    font.family: Style.fontUi
                                    font.pixelSize: Style.fs.md
                                    text: page.proxyInput
                                    onTextChanged: page.proxyInput = text
                                    onAccepted: page.saveProxy()
                                    Text {
                                        anchors.verticalCenter: parent.verticalCenter
                                        visible: proxyField.text.length === 0
                                        text: "http://host:port (blank = none)"
                                        color: Tokens.inkFaint
                                        font: proxyField.font
                                    }
                                }
                            }
                            Pill { label: "Save"; onClicked: page.saveProxy() }
                        }
                    }

                    // Storage ---------------------------------------------------------------
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "Storage"; mark: "保存" }

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(4)
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 1
                            Text { Layout.fillWidth: true; text: "Cache"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium; wrapMode: Text.WordWrap }
                            Text {
                                Layout.fillWidth: true
                                text: "Clear cached stream URLs and downloaded audio bytes."
                                color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                            }
                        }
                        Pill {
                            Layout.alignment: Qt.AlignVCenter
                            label: page.clearing ? "Clearing…" : "Clear caches"
                            icon: "close"
                            enabled: !page.clearing
                            onClicked: page.clearCaches()
                        }
                    }
                }

                // ─────────────────────────── ACCOUNT ───────────────────────────
                ColumnLayout {
                    id: accountCol
                    visible: page.section === "account"
                    anchors { left: parent.left; right: parent.right; top: parent.top; leftMargin: Style.sp(6); rightMargin: Style.sp(6); topMargin: Style.sp(4) }
                    spacing: Style.sp(3)

                    SectionHeading { Layout.fillWidth: true; title: "Account"; mark: "鍵" }

                    // current identity
                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(3)
                        Artwork {
                            visible: !!(Playback.auth && Playback.auth.avatar)
                            url: (Playback.auth && Playback.auth.avatar) ? Playback.auth.avatar : ""
                            px: Style.sp(12)
                            round: true
                            placeholderIcon: "account"
                        }
                        Icon {
                            visible: !(Playback.auth && Playback.auth.avatar)
                            name: "account"; size: Style.fs.hero; color: Tokens.inkMuted
                        }
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 1
                            Text {
                                text: (Playback.auth && Playback.auth.signedIn && Playback.auth.name) ? Playback.auth.name : "Not signed in"
                                color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.lg; font.weight: Font.DemiBold
                                elide: Text.ElideRight; Layout.fillWidth: true
                            }
                            Text {
                                text: (Playback.auth && Playback.auth.signedIn) ? "YouTube Music" : "Sign in to sync your library"
                                color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm
                            }
                        }
                        Pill {
                            label: (Playback.auth && Playback.auth.signedIn) ? "Sign out" : "Sign in with Google"
                            icon: (Playback.auth && Playback.auth.signedIn) ? "close" : "account"
                            primary: !(Playback.auth && Playback.auth.signedIn)
                            onClicked: {
                                var out = !!(Playback.auth && Playback.auth.signedIn);
                                Daemon.call(out ? "sign_out" : "sign_in").catch((e) => Playback.toast((e && e.message) ? e.message : String(e), "error"));
                            }
                        }
                    }

                    // ── Spotify ─────────────────────────────────────────────────────────
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "Spotify"; mark: "緑" }

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(3)
                        Icon {
                            name: "spotify"
                            size: Style.fs.hero
                            color: (Playback.spotify && Playback.spotify.signedIn) ? Style.providerColor : Tokens.inkMuted
                        }
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 1
                            Text {
                                text: (Playback.spotify && Playback.spotify.signedIn && Playback.spotify.name) ? Playback.spotify.name : "Not signed in"
                                color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.lg; font.weight: Font.DemiBold
                                elide: Text.ElideRight; Layout.fillWidth: true
                            }
                            Text {
                                readonly property var sp: Playback.spotify || ({})
                                text: sp.signedIn ? "Connected \u00b7 Premium"
                                    : (sp.error ? sp.error
                                    : (sp.flow === "browser" ? "Waiting for your browser\u2026"
                                    : "Not connected \u00b7 Premium is required for playback."))
                                color: sp.error ? Style.accent : Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm
                                wrapMode: Text.WordWrap; Layout.fillWidth: true
                            }
                        }
                        Pill {
                            label: (Playback.spotify && Playback.spotify.signedIn) ? "Sign out" : "Sign in to Spotify"
                            icon: (Playback.spotify && Playback.spotify.signedIn) ? "close" : "spotify"
                            primary: !(Playback.spotify && Playback.spotify.signedIn)
                            onClicked: {
                                var out = !!(Playback.spotify && Playback.spotify.signedIn);
                                if (out) Playback.spotifySignOut().catch((e) => Playback.toast((e && e.message) ? e.message : String(e), "error"));
                                else Playback.spotifySignIn().catch((e) => Playback.toast((e && e.message) ? e.message : String(e), "error"));
                            }
                        }
                    }

                    // ── SoundCloud ──────────────────────────────────────────────────────
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "SoundCloud"; mark: "雲" }

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(3)
                        Icon {
                            name: "soundcloud"
                            size: Style.fs.hero
                            color: Playback.provider === "soundcloud" ? Style.providerColors.soundcloud : Tokens.inkMuted
                        }
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: 1
                            Text {
                                text: "Listening as a guest"
                                color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.lg; font.weight: Font.DemiBold
                                elide: Text.ElideRight; Layout.fillWidth: true
                            }
                            Text {
                                text: "No account needed \u00b7 SoundCloud's public catalogue plays for everyone."
                                color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm
                                wrapMode: Text.WordWrap; Layout.fillWidth: true
                            }
                        }
                    }

                    // switch account
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "Channels"; mark: "選択"; visible: page.identities.length > 0 }
                    ColumnLayout {
                        Layout.fillWidth: true
                        visible: page.identities.length > 0
                        spacing: Style.sp(1)
                        Text {
                            Layout.fillWidth: true
                            text: "Pick which YouTube channel or brand account this session acts as."
                            color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                        }
                        Repeater {
                            model: page.identities
                            delegate: RowLayout {
                                id: idRow
                                required property var modelData
                                Layout.fillWidth: true
                                Layout.topMargin: Style.sp(1)
                                spacing: Style.sp(2)
                                Artwork {
                                    url: idRow.modelData.thumbnail || ""
                                    px: Style.sp(8)
                                    round: true
                                    placeholderIcon: "account"
                                }
                                ColumnLayout {
                                    Layout.fillWidth: true
                                    spacing: 0
                                    Text {
                                        Layout.fillWidth: true
                                        text: idRow.modelData.name + (idRow.modelData.selected ? "  ·  CURRENT" : "")
                                        color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; elide: Text.ElideRight
                                    }
                                    Text {
                                        visible: !!idRow.modelData.handle
                                        text: idRow.modelData.handle || ""
                                        color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs
                                    }
                                }
                                Pill {
                                    label: "Use"
                                    enabled: !idRow.modelData.selected
                                    onClicked: page.switchAccount(idRow.modelData.selectionKey)
                                }
                            }
                        }
                    }
                    Text {
                        visible: page.identities.length === 0
                        text: (Playback.auth && Playback.auth.signedIn) ? "This account has a single channel." : "Sign in to see the channels on this account."
                        color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.sm
                    }
                }

                // ─────────────────────────── LOCAL MUSIC ───────────────────────────
                ColumnLayout {
                    id: localCol
                    visible: page.section === "local"
                    anchors { left: parent.left; right: parent.right; top: parent.top; leftMargin: Style.sp(6); rightMargin: Style.sp(6); topMargin: Style.sp(4) }
                    spacing: Style.sp(3)

                    SectionHeading { Layout.fillWidth: true; title: "Watched folders"; mark: "音源" }
                    Text {
                        Layout.fillWidth: true
                        text: "Folders Ryotunes watches for files on disk. The daemon rescans them on demand."
                        color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                    }

                    RowLayout {
                        Layout.topMargin: Style.sp(1)
                        spacing: Style.sp(2)
                        Pill { label: "Add folder"; icon: "add"; primary: true; onClicked: folderPicker.running = true }
                        Pill { label: "Rescan"; icon: "on-repeat"; onClicked: page.scanFolders() }
                        Item { Layout.fillWidth: true }
                    }

                    Text {
                        visible: !page.folders.length
                        Layout.topMargin: Style.sp(1)
                        text: "No folders yet. Add the one your music sits in."
                        color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm
                    }

                    Repeater {
                        model: page.folders
                        delegate: RowLayout {
                            id: folderRow
                            required property var modelData
                            Layout.fillWidth: true
                            Layout.topMargin: Style.sp(1)
                            spacing: Style.sp(2)
                            Rectangle {
                                Layout.fillWidth: true
                                implicitHeight: Style.sp(9)
                                radius: Style.radius
                                color: Tokens.paperLift
                                border.width: 1
                                border.color: Tokens.line
                                RowLayout {
                                    anchors.fill: parent
                                    anchors.leftMargin: Style.sp(2.5)
                                    anchors.rightMargin: Style.sp(1)
                                    spacing: Style.sp(2)
                                    Icon { name: "music"; size: Style.fs.sm; color: Tokens.inkMuted }
                                    Text {
                                        Layout.fillWidth: true
                                        text: folderRow.modelData
                                        color: Tokens.inkDim
                                        font.family: Style.fontMono
                                        font.pixelSize: Style.fs.sm
                                        elide: Text.ElideMiddle
                                    }
                                    IconButton {
                                        icon: "close"
                                        iconSize: Style.fs.sm
                                        diameter: Style.sp(7)
                                        onClicked: page.removeFolder(folderRow.modelData)
                                    }
                                }
                            }
                        }
                    }
                }

                // ─────────────────────────── PLAYLISTS ───────────────────────────
                ColumnLayout {
                    id: playlistsCol
                    visible: page.section === "playlists"
                    anchors { left: parent.left; right: parent.right; top: parent.top; leftMargin: Style.sp(6); rightMargin: Style.sp(6); topMargin: Style.sp(4) }
                    spacing: Style.sp(3)

                    // Import ----------------------------------------------------------------
                    SectionHeading { Layout.fillWidth: true; title: "Import a playlist"; mark: "転送" }
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(1)
                        Text {
                            Layout.fillWidth: true
                            text: "Choose a Ryotunes playlist file; its tracks land in a new library playlist. Sign in first."
                            color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                        }
                        Pill {
                            Layout.topMargin: Style.sp(1)
                            label: page.importing ? "Importing…" : "Import from file"
                            icon: "add"
                            enabled: !page.importing && !!(Playback.auth && Playback.auth.signedIn)
                            onClicked: importPicker.running = true
                        }
                    }

                    // Export ----------------------------------------------------------------
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "Export the current queue"; mark: "出力" }
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(1)
                        Text {
                            Layout.fillWidth: true
                            text: "Write the tracks now in your queue to a portable .json file."
                            color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                        }
                        Pill {
                            Layout.topMargin: Style.sp(1)
                            label: page.exporting ? "Exporting…" : "Export to file"
                            icon: "playlist"
                            enabled: !page.exporting && !!(Playback.queue && Playback.queue.items && Playback.queue.items.length)
                            onClicked: exportPicker.running = true
                        }
                    }
                }

                // ─────────────────────────── ABOUT ───────────────────────────
                ColumnLayout {
                    id: aboutCol
                    visible: page.section === "about"
                    anchors { left: parent.left; right: parent.right; top: parent.top; leftMargin: Style.sp(6); rightMargin: Style.sp(6); topMargin: Style.sp(4) }
                    spacing: Style.sp(2)

                    SectionHeading { Layout.fillWidth: true; title: "About"; mark: "力" }
                    Text {
                        Layout.fillWidth: true
                        text: "A focused Ryoku desktop music instrument: your YouTube Music library, local media, queue, lyrics and playback engine in one paper-and-ink surface."
                        color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.md; wrapMode: Text.WordWrap
                    }
                    GridLayout {
                        Layout.topMargin: Style.sp(2)
                        columns: 2
                        columnSpacing: Style.sp(6)
                        rowSpacing: Style.sp(1)
                        Text { text: "APP"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs }
                        Text { text: page.appVersion || "—"; color: Tokens.inkDim; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; font.weight: Font.Medium }
                        Text { text: "DAEMON"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs }
                        Text { text: Daemon.daemonVersion || "—"; color: Tokens.inkDim; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; font.weight: Font.Medium }
                        Text { text: "ENGINE"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs }
                        Text { text: "RUST + MPV"; color: Tokens.inkDim; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; font.weight: Font.Medium }
                        Text { text: "CLIENT"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs }
                        Text { text: "QUICKSHELL / QML"; color: Tokens.inkDim; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; font.weight: Font.Medium }
                    }

                    // --- software updates ---
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "Software updates"; mark: "新" }
                    Text {
                        Layout.fillWidth: true
                        text: "You're on " + (page.appVersion || Daemon.daemonVersion || "this build") + ". Ryotunes checks GitHub only when you ask — nothing runs in the background."
                        color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                    }
                    RowLayout {
                        Layout.topMargin: Style.sp(1)
                        spacing: Style.sp(2)
                        Btn {
                            text: page.updateStatus === "checking" ? "Checking…" : "Check for new version"
                            icon: "repeat"
                            enabled: page.updateStatus !== "checking" && page.updateStatus !== "installing" && page.updateStatus !== "installed"
                            onClicked: page.checkUpdates()
                        }
                        Btn {
                            text: "View changelog"
                            icon: "link"
                            onClicked: page.openUrl((page.updateInfo && page.updateInfo.releaseUrl) ? page.updateInfo.releaseUrl : "https://github.com/ryoku-dev/ryotunes/releases")
                        }
                        Btn {
                            visible: !!(page.updateInfo && page.updateInfo.available && page.updateInfo.latestVersion && page.updateInfo.canInstall) && page.updateStatus !== "installed"
                            primary: true
                            text: page.updateStatus === "installing" ? "Installing…" : ("Update to " + (page.updateInfo ? page.updateInfo.latestVersion : ""))
                            icon: "download"
                            enabled: page.updateStatus !== "installing" && page.updateStatus !== "checking"
                            onClicked: page.installUpdate()
                        }
                    }
                    Text {
                        Layout.fillWidth: true
                        Layout.topMargin: Style.sp(1)
                        visible: text.length > 0
                        text: page.updateMessage()
                        color: page.updateStatus === "error" ? Tokens.alert
                            : (page.updateStatus === "idle" && page.updateInfo && page.updateInfo.available) ? Tokens.sun
                            : Tokens.inkMuted
                        font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                        textFormat: Text.PlainText
                        Accessible.role: Accessible.StaticText
                        Accessible.name: text
                    }
                    Btn {
                        visible: !!(page.updateInfo && page.updateInfo.notes)
                        Layout.topMargin: Style.sp(1)
                        text: page.showReleaseNotes ? "Hide release notes" : "Show release notes"
                        onClicked: page.showReleaseNotes = !page.showReleaseNotes
                    }
                    Rectangle {
                        visible: page.showReleaseNotes && !!(page.updateInfo && page.updateInfo.notes)
                        Layout.fillWidth: true
                        Layout.topMargin: Style.sp(1)
                        Layout.preferredHeight: Math.min(notesText.implicitHeight + Style.sp(4), Style.sp(60))
                        color: "transparent"
                        border.width: 1
                        border.color: Tokens.line
                        radius: Style.radius
                        Flickable {
                            anchors.fill: parent
                            anchors.margins: Style.sp(2)
                            clip: true
                            contentWidth: width
                            contentHeight: notesText.implicitHeight
                            Text {
                                id: notesText
                                width: parent.width
                                text: (page.updateInfo && page.updateInfo.notes) ? page.updateInfo.notes : ""
                                color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; wrapMode: Text.WordWrap; textFormat: Text.PlainText
                            }
                        }
                    }

                    // --- made by ---
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "Made by"; mark: "人" }
                    Text {
                        Layout.fillWidth: true
                        text: "Ryotunes is built by two developers. Open their GitHub profiles:"
                        color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                    }
                    RowLayout {
                        Layout.topMargin: Style.sp(1)
                        spacing: Style.sp(2)
                        Btn { text: "ashmitvoid"; icon: "link"; onClicked: page.openUrl("https://github.com/ashmitvoid") }
                        Btn { text: "neur0map"; icon: "link"; onClicked: page.openUrl("https://github.com/neur0map") }
                    }

                    // --- open source ---
                    SectionHeading { Layout.fillWidth: true; Layout.topMargin: Style.sp(3); title: "Open source"; mark: "源" }
                    Text {
                        Layout.fillWidth: true
                        text: "Ryotunes is a fork of LiMusic, released under the GNU General Public License v3.0 or later."
                        color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                    }
                    RowLayout {
                        Layout.topMargin: Style.sp(1)
                        spacing: Style.sp(2)
                        Btn { text: "LiMusic upstream"; icon: "link"; onClicked: page.openUrl("https://github.com/SimoHypers/limusic") }
                        Btn { text: "GPL-3.0-or-later"; icon: "link"; onClicked: page.openUrl("https://www.gnu.org/licenses/gpl-3.0.html") }
                    }
                }
            }
        }
    }
}
