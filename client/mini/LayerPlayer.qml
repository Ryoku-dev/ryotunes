pragma ComponentBehavior: Bound
import QtQuick
import Quickshell
import Quickshell.Wayland
import "../"
import "../components"

PanelWindow {
    id: miniWin
    signal maximize()
    visible: false
    color: "transparent"
    anchors { top: true; bottom: true; left: true; right: true }
    exclusiveZone: 0
    WlrLayershell.layer: WlrLayer.Top
    WlrLayershell.namespace: "ryotunes-mini"
    WlrLayershell.keyboardFocus: WlrKeyboardFocus.None
    mask: Region { x: miniBox.x; y: miniBox.y; width: miniBox.width; height: miniBox.height }

    Item {
        id: miniBox
        width: 724
        height: 356
        ArtAccent {}
        // Offsets from the work area's bottom-right corner, clamped so the widget stays on
        // screen whatever the monitor.
        x: Math.max(0, Math.min(miniWin.width - width, miniWin.width - width - Prefs.miniRight))
        y: Math.max(0, Math.min(miniWin.height - height, miniWin.height - height - Prefs.miniBottom))

        MiniPlayer {
            anchors.fill: parent
            active: miniWin.visible
            dragTarget: miniBox
            dragMaxX: Math.max(0, miniWin.width - miniBox.width)
            dragMaxY: Math.max(0, miniWin.height - miniBox.height)
            onMaximize: {
                miniWin.maximize();
            }
            onDragEnded: {
                Prefs.miniRight = Math.round(miniWin.width - miniBox.width - miniBox.x);
                Prefs.miniBottom = Math.round(miniWin.height - miniBox.height - miniBox.y);
                Prefs.save();
            }
        }
    }
}
