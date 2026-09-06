pragma Singleton
import QtQuick
import Quickshell
import Quickshell.Io

// Client-local preferences: what the daemon has no UI_SETTINGS key for and no other client needs.
// One JSON file, written atomically on every change through the adapter (the Ryoku shell's
// shell.json pattern). Absent or malformed reads as the defaults below.
Singleton {
    id: root

    property alias skin: adapter.skin             // "system" | a skin id (see Skin.qml, docs/SKINS.md)
    property alias decor: adapter.decor           // "skin" | "rich" | "calm": the decor level, or the skin's own
    property alias themeMode: adapter.themeMode   // "system" | "light" | "dark"
    // The mini widget's offset from the screen's bottom-right corner, logical px.
    property alias miniRight: adapter.miniRight
    property alias miniBottom: adapter.miniBottom

    function save() { file.writeAdapter(); }

    FileView {
        id: file
        path: (Quickshell.env("XDG_CONFIG_HOME") || (Quickshell.env("HOME") + "/.config")) + "/ryotunes/client.json"
        blockLoading: true
        watchChanges: true
        printErrors: false
        atomicWrites: true
        onFileChanged: reload()
        JsonAdapter {
            id: adapter
            property string skin: "system"
            property string decor: "skin"
            property string themeMode: "system"
            property int miniRight: 24
            property int miniBottom: 24
        }
    }
}
