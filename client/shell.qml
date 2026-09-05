pragma ComponentBehavior: Bound
import QtQuick
import Quickshell
import Quickshell.Io
import Quickshell.Wayland
import Ryoku.Ui.Singletons
import "mini"
import "components"

ShellRoot {
    id: shellRoot

    // Ask for the version handshake, the event stream and its opening snapshot as soon as the
    // config is up. subscribeAll() is idempotent and re-subscribes on every reconnect, so a single
    // call here covers a daemon that is already up, one that starts later, and one that restarts.
    Component.onCompleted: {
        Daemon.subscribeAll();
        Style.applyPrefs();
    }

    // Dev hooks (all optional, unset in normal use): RYOTUNES_WINDOW_TITLE renames the toplevel so
    // a test instance can be told apart from the daily one; RYOTUNES_SCREEN pins the layer
    // surfaces to an output (a headless test output, for instance); RYOTUNES_CTL opens a control
    // socket that takes one command per line: `nav <page> [json]`, `show`, `mini on|off`,
    // `np <queue|lyrics|off>`, `panel on|off`, `decor rich|calm`, `theme system|light|dark`.
    readonly property string devTitle: Quickshell.env("RYOTUNES_WINDOW_TITLE") || ""
    readonly property string devScreen: Quickshell.env("RYOTUNES_SCREEN") || ""
    readonly property string devCtl: Quickshell.env("RYOTUNES_CTL") || ""
    readonly property var devScreenObj: {
        if (!shellRoot.devScreen) return null;
        var list = Quickshell.screens;
        for (var i = 0; i < list.length; i++) if (list[i].name === shellRoot.devScreen) return list[i];
        return null;
    }

    function ctl(cmd, arg, rest) {
        if (cmd === "nav") {
            var params = {};
            try { params = rest ? JSON.parse(rest) : {}; } catch (e) { params = {}; }
            Router.push(arg, params);
        } else if (cmd === "show") shellRoot.present();
        else if (cmd === "mini") appRoot.miniOpen = arg === "on";
        else if (cmd === "np") { if (arg === "off") appRoot.npClose(); else appRoot.npOpenTab(arg); }
        else if (cmd === "panel") appRoot.panelOpen = arg === "on";
        else if (cmd === "decor") { Prefs.decor = arg; Prefs.save(); }
        else if (cmd === "theme") { Prefs.themeMode = arg; Prefs.save(); }
        else if (cmd === "provider") Playback.setProvider(arg);
    }

    SocketServer {
        active: shellRoot.devCtl !== ""
        path: shellRoot.devCtl
        handler: Socket {
            parser: SplitParser {
                onRead: (line) => {
                    var parts = line.trim().split(" ");
                    shellRoot.ctl(parts[0], parts[1] || "", parts.slice(2).join(" "));
                }
            }
        }
    }

    FloatingWindow {
        id: win
        title: shellRoot.devTitle || "Ryotunes"
        color: Tokens.paper
        minimumSize: Qt.size(900, 620)
        // Open at the Tauri app's daily-driver geometry (about 86% x 82% of the monitor, capped at
        // 1600 x 1000) rather than the minimum; the Ryoku rule floats and centres it.
        implicitWidth: Math.max(900, Math.min(1600, Math.round((win.screen ? win.screen.width : 1600) * 0.86)))
        implicitHeight: Math.max(620, Math.min(1000, Math.round((win.screen ? win.screen.height : 1000) * 0.82)))

        // Honour this monitor's Interface scale through Tokens, the same way the Hub does; the app's
        // own sp()/type scale ride on it.
        Binding {
            target: Tokens
            property: "uiScale"
            value: Tokens.uiScaleFor(win.screen && win.screen.name ? win.screen.name : "")
        }
        App { id: appRoot; anchors.fill: parent }
    }

    // The mini player: the Tauri widget's compact 724 x 356 geometry (always on top, skip-taskbar,
    // bottom-right of the work area, draggable, position remembered), as a layer-shell surface.
    // The surface spans the work area (exclusive zone 0 keeps it out from under the bar and dock)
    // and is transparent and input-masked everywhere except the widget, which is an item dragged
    // inside it: a surface that moves itself under the pointer feeds its own drag deltas back and
    // flies off, an item in a fixed surface does not. Opening it hides the main window the way the
    // Tauri app hibernated it; the maximize button brings the main window back.
    PanelWindow {
        id: miniWin
        visible: appRoot.miniOpen
        screen: shellRoot.devScreenObj
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
                    appRoot.miniOpen = false;
                    shellRoot.present();
                }
                onDragEnded: {
                    Prefs.miniRight = Math.round(miniWin.width - miniBox.width - miniBox.x);
                    Prefs.miniBottom = Math.round(miniWin.height - miniBox.height - miniBox.y);
                    Prefs.save();
                }
            }
        }
    }

    Connections {
        target: appRoot
        function onMiniOpenChanged(): void {
            if (appRoot.miniOpen)
                win.visible = false;
        }
    }

    // "Come back" from two directions: the daemon's `show` event (tray Show, a second `ryotunes` /
    // `ryotunesd` launch, the desktop keybind) and `qs -p ... ipc call window show`. A closed
    // FloatingWindow is hidden, not destroyed, so this process stays subscribed and the daemon's
    // show reaches it here rather than spawning a second client.
    function present(): void {
        appRoot.miniOpen = false;
        // After a compositor close (Super+Q) the toplevel is gone but Quickshell leaves
        // `visible` at true, so assigning true again is a no-op; drop it first to remap.
        win.visible = false;
        win.visible = true;
    }
    Connections {
        target: Daemon
        function onEvent(name: string, data: var): void {
            if (name === "show")
                shellRoot.present();
        }
    }
    IpcHandler {
        target: "window"
        function show(): void { shellRoot.present(); }
        function mini(): void { appRoot.miniOpen = !appRoot.miniOpen; }
    }
}
