pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import QtQuick.Effects
import Ryoku.Ui.Singletons
import "../"

// "Edit playlist" details, ported from EditPlaylistDialog.svelte: name, description and visibility.
// edit_playlist_details merges only the fields sent, so an untouched one is never overwritten; the
// page reloads on save to pick up whatever YouTube accepted. A centred modal card over a one-shot
// snapshot blur of the page behind (spec §9) under a 0.5 scrim. Data flows are unchanged.
Item {
    id: root

    anchors.fill: parent
    z: 210

    property Item blurSource: null
    property string playlistId: ""
    property string initialName: ""
    property string initialDescription: ""
    property bool initialPublic: false
    property bool saving: false

    signal closed()
    signal saved()

    property string nameText: initialName
    property string descText: initialDescription
    property bool isPublic: initialPublic

    Component.onCompleted: { if (root.blurSource) snap.scheduleUpdate(); }

    function submit() {
        if (root.saving || !root.nameText.trim())
            return;
        root.saving = true;
        Daemon.call("edit_playlist_details", {
            playlistId: root.playlistId,
            name: root.nameText.trim(),
            description: root.descText,
            public: root.isPublic
        }).then(() => { root.saving = false; root.saved(); })
            .catch((e) => { root.saving = false; Playback.toast((e && e.message) ? e.message : "Could not save", "error"); });
    }

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

    MouseArea { anchors.fill: parent; onClicked: root.closed() }
    Rectangle { anchors.fill: parent; color: "#000000"; opacity: 0.5 }

    Rectangle {
        anchors.centerIn: parent
        width: Style.sp(100)
        implicitHeight: col.implicitHeight + Style.sp(12)
        height: implicitHeight
        radius: Style.radiusCard
        color: Tokens.paper
        border.width: 1
        border.color: Tokens.line
        MouseArea { anchors.fill: parent }

        ColumnLayout {
            id: col
            anchors.fill: parent
            anchors.margins: Style.sp(6)
            spacing: Style.sp(3)

            Text {
                text: "Edit playlist"
                color: Tokens.ink
                font.family: Style.fontUi
                font.pixelSize: Style.fs.lg
                font.weight: Font.DemiBold
            }

            // name
            Rectangle {
                Layout.fillWidth: true
                implicitHeight: Style.sp(10)
                radius: Style.radius
                color: Tokens.paperLift
                border.width: 1
                border.color: nameField.activeFocus ? Tokens.lineStrong : Tokens.line
                TextInput {
                    id: nameField
                    anchors.fill: parent
                    anchors.leftMargin: Style.sp(2)
                    anchors.rightMargin: Style.sp(2)
                    verticalAlignment: TextInput.AlignVCenter
                    clip: true
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.md
                    text: root.nameText
                    focus: true
                    Component.onCompleted: nameField.forceActiveFocus()
                    onTextChanged: root.nameText = text
                    Text {
                        anchors.verticalCenter: parent.verticalCenter
                        visible: nameField.text.length === 0
                        text: "Playlist name"
                        color: Tokens.inkFaint
                        font: nameField.font
                    }
                }
            }

            // description
            Rectangle {
                Layout.fillWidth: true
                implicitHeight: Style.sp(24)
                radius: Style.radius
                color: Tokens.paperLift
                border.width: 1
                border.color: descField.activeFocus ? Tokens.lineStrong : Tokens.line
                TextEdit {
                    id: descField
                    anchors.fill: parent
                    anchors.margins: Style.sp(2)
                    clip: true
                    wrapMode: TextEdit.Wrap
                    color: Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.sm
                    text: root.descText
                    onTextChanged: root.descText = text
                    Text {
                        visible: descField.text.length === 0
                        text: "Description"
                        color: Tokens.inkFaint
                        font: descField.font
                    }
                }
            }

            // public toggle
            RowLayout {
                Layout.fillWidth: true
                spacing: Style.sp(3)
                Toggle {
                    checked: root.isPublic
                    onToggled: (v) => root.isPublic = v
                }
                Text {
                    Layout.fillWidth: true
                    text: root.isPublic ? "Public" : "Private"
                    color: Tokens.inkDim
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.md
                }
            }

            RowLayout {
                Layout.alignment: Qt.AlignRight
                spacing: Style.sp(2)
                Pill { label: "Cancel"; onClicked: root.closed() }
                Pill { label: "Save"; primary: true; enabled: !root.saving && root.nameText.trim().length > 0; onClicked: root.submit() }
            }
        }
    }
}
