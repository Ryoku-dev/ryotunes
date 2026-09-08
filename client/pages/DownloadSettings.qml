pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Quickshell.Io
import Ryoku.Ui.Singletons
import "../"
import "../components"

ColumnLayout {
    id: root
    spacing: Style.sp(4)

    property var saved: null
    property string pathInput: ""
    property string formatInput: "original"
    property int workersInput: 1
    property bool organizeInput: true
    property bool metadataInput: true
    property bool loading: false
    property bool saving: false
    property string error: ""
    readonly property bool dirty: !!saved && (pathInput !== saved.path || formatInput !== saved.format
        || workersInput !== saved.workers || organizeInput !== saved.organizeByArtist || metadataInput !== saved.embedMetadata)

    function accept(settings) {
        root.saved = settings;
        root.pathInput = settings.path;
        root.formatInput = settings.format;
        root.workersInput = settings.workers;
        root.organizeInput = settings.organizeByArtist;
        root.metadataInput = settings.embedMetadata;
    }
    function load() {
        root.loading = true;
        root.error = "";
        Daemon.call("get_download_settings").then((settings) => {
            root.accept(settings);
            root.loading = false;
        }).catch((e) => { root.loading = false; root.error = e.message || String(e); });
    }
    function save() {
        if (!root.saved || root.saving || !Daemon.connected) return;
        root.saving = true;
        root.error = "";
        Daemon.call("set_download_settings", { settings: {
            path: root.pathInput.trim(), format: root.formatInput, workers: root.workersInput,
            organizeByArtist: root.organizeInput, embedMetadata: root.metadataInput
        } }).then((settings) => {
            root.accept(settings);
            root.saving = false;
            Playback.toast("Download preferences saved", "success");
        }).catch((e) => { root.saving = false; root.error = e.message || String(e); });
    }
    Component.onCompleted: root.load()
    Connections {
        target: Daemon
        function onConnectedChanged(): void {
            if (Daemon.connected && !root.saved && !root.loading) root.load();
        }
    }

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
    component Choice: Chip {
        id: choice
        property string accessibleLabel: text
        activeFocusOnTab: enabled && visible
        Accessible.role: Accessible.RadioButton
        Accessible.name: accessibleLabel
        Accessible.checkable: true
        Accessible.checked: active
        Accessible.onPressAction: choice.clicked()
        Keys.onSpacePressed: (event) => { choice.clicked(); event.accepted = true; }
        Keys.onReturnPressed: (event) => { choice.clicked(); event.accepted = true; }
        Rectangle {
            anchors.fill: parent
            anchors.margins: -2
            radius: height / 2
            color: "transparent"
            border.width: choice.activeFocus ? 2 : 0
            border.color: Tokens.ink
        }
    }

    SectionHeading { Layout.fillWidth: true; title: "Downloads"; mark: "保存" }
    Text {
        Layout.fillWidth: true
        text: "Save a track with the arrow beside its heart. Downloads continue when you close the window."
        color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.md; wrapMode: Text.WordWrap
    }
    RowLayout {
        Layout.fillWidth: true
        Action { text: "Queue & history"; icon: "download"; onClicked: Router.push("downloads") }
        Action {
            text: "Open download folder"; enabled: !!root.saved && Daemon.connected
            onClicked: Daemon.call("open_download_folder").catch((e) => { root.error = e.message || String(e); })
        }
    }
    Text {
        Layout.fillWidth: true
        visible: root.loading || !Daemon.connected || root.error !== ""
        text: !Daemon.connected ? "Reconnect to Ryotunes to change download preferences."
            : root.error || "Loading download preferences…"
        color: root.error ? Style.accent : Tokens.inkMuted
        font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
        Accessible.role: Accessible.StaticText
        Accessible.name: text
    }
    Action { visible: !root.saved && !root.loading; enabled: Daemon.connected; text: "Try again"; onClicked: root.load() }

    ColumnLayout {
        Layout.fillWidth: true
        visible: !!root.saved
        enabled: !root.saving && Daemon.connected
        spacing: Style.sp(4)

        ColumnLayout {
            Layout.fillWidth: true
            spacing: Style.sp(1.5)
            Text { text: "Download folder"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium }
            Text {
                Layout.fillWidth: true
                text: "An absolute path, or ~/ for your home folder. Missing folders are created when needed."
                color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
            }
            RowLayout {
                Layout.fillWidth: true
                spacing: Style.sp(2)
                Rectangle {
                    Layout.fillWidth: true
                    implicitHeight: Style.ctlH
                    radius: Style.radius
                    color: Tokens.paperLift
                    border.width: folderField.activeFocus ? 2 : 1
                    border.color: folderField.activeFocus ? Tokens.ink : Tokens.line
                    TextInput {
                        id: folderField
                        anchors.fill: parent
                        anchors.leftMargin: Style.sp(2)
                        anchors.rightMargin: Style.sp(2)
                        verticalAlignment: TextInput.AlignVCenter
                        clip: true
                        selectByMouse: true
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        text: root.pathInput
                        onTextChanged: root.pathInput = text
                        onAccepted: root.save()
                        Accessible.name: "Download folder"
                    }
                }
                Action { text: "Browse…"; enabled: !folderPicker.running; onClicked: folderPicker.running = true }
            }
        }

        ColumnLayout {
            Layout.fillWidth: true
            spacing: Style.sp(1.5)
            Text { text: "Audio format"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium }
            Flow {
                Layout.fillWidth: true
                spacing: Style.sp(2)
                Repeater {
                    model: [{ key: "original", label: "Original" }, { key: "mp3", label: "MP3" }, { key: "opus", label: "Opus" }]
                    delegate: Choice {
                        required property var modelData
                        text: modelData.label
                        accessibleLabel: "Audio format: " + text
                        active: root.formatInput === modelData.key
                        onClicked: root.formatInput = modelData.key
                    }
                }
            }
            Text {
                Layout.fillWidth: true
                text: root.formatInput === "original" ? "Best available source audio without lossy conversion. Fastest and lightest."
                    : root.formatInput === "mp3" ? "MP3 for broad compatibility. Conversion targets 320 kbps but cannot improve the source quality."
                    : "Opus for compact, efficient audio. Conversion targets 160 kbps; existing Opus audio is kept without re-encoding."
                color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
            }
        }

        ColumnLayout {
            Layout.fillWidth: true
            spacing: Style.sp(1.5)
            Text { text: "Simultaneous downloads"; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium }
            RowLayout {
                spacing: Style.sp(2)
                Repeater {
                    model: [1, 2, 3, 4]
                    delegate: Choice {
                        required property int modelData
                        text: String(modelData)
                        accessibleLabel: modelData + (modelData === 1 ? " download worker" : " download workers")
                        active: root.workersInput === modelData
                        onClicked: root.workersInput = modelData
                    }
                }
            }
            Text {
                Layout.fillWidth: true
                text: "1 recommended for quieter listening. At most 4 workers; extra tracks wait in the queue. Lowering the limit lets running downloads finish."
                color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
            }
        }

        Repeater {
            model: [
                { key: "organize", title: "Artist subfolders", desc: "Keep each artist’s downloads together inside your download folder." },
                { key: "metadata", title: "Embed cover art & lyrics", desc: "Write track info, cover art and lyrics into the audio file, and save matching cover and lyrics files next to it. Artwork and lyrics not already in the download are fetched from free providers on a best-effort basis." }
            ]
            delegate: RowLayout {
                id: option
                required property var modelData
                Layout.fillWidth: true
                spacing: Style.sp(4)
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 2
                    Text { Layout.fillWidth: true; text: option.modelData.title; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.Medium; wrapMode: Text.WordWrap }
                    Text { Layout.fillWidth: true; text: option.modelData.desc; color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap }
                }
                Toggle {
                    id: toggle
                    checked: option.modelData.key === "organize" ? root.organizeInput : root.metadataInput
                    onToggled: (value) => { if (option.modelData.key === "organize") root.organizeInput = value; else root.metadataInput = value; }
                    activeFocusOnTab: enabled && visible
                    Accessible.role: Accessible.CheckBox
                    Accessible.name: option.modelData.title
                    Accessible.checkable: true
                    Accessible.checked: checked
                    Accessible.onToggleAction: toggle.toggled(!toggle.checked)
                    Keys.onSpacePressed: (event) => { toggle.toggled(!toggle.checked); event.accepted = true; }
                    Rectangle {
                        anchors.fill: parent; anchors.margins: -3; radius: height / 2
                        color: "transparent"; border.width: toggle.activeFocus ? 2 : 0; border.color: Tokens.ink
                    }
                }
            }
        }
    }
    RowLayout {
        visible: !!root.saved
        Layout.fillWidth: true
        spacing: Style.sp(3)
        Action { text: root.saving ? "Saving…" : "Save preferences"; primary: true; enabled: root.dirty && !root.saving && Daemon.connected; onClicked: root.save() }
        Text { Layout.fillWidth: true; text: root.dirty ? "Unsaved changes" : "Saved · file preferences apply to new downloads"; color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap }
    }
    Hairline { Layout.fillWidth: true; height: 1 }
    Text {
        Layout.fillWidth: true
        text: "YouTube and SoundCloud tracks download from their source. Spotify tracks use a YouTube search match, which may be a different version. Local files are already on disk; live radio cannot be downloaded. Protected or unavailable tracks show an error. Cover and lyrics lookups send track details, including name and artist, to external providers. Requires yt-dlp and FFmpeg. Only download music you have permission to save."
        color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
    }
    Process {
        id: folderPicker
        command: ["zenity", "--file-selection", "--directory", "--title=Choose a download folder"]
        stdout: StdioCollector { id: folderOutput }
        onExited: (exitCode, exitStatus) => {
            if (exitCode === 0 && folderOutput.text.trim()) root.pathInput = folderOutput.text.trim();
        }
    }
}
