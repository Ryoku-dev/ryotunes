pragma Singleton
import QtQuick
import Quickshell
import Quickshell.Io

// The playback spectrum: cava on the PipeWire monitor, 40 bands at 30 fps, the Ryoku shell's
// AudioBars pattern. A surface that draws it claims the feed with claim(id, true) while it is
// visible and releases with claim(id, false); the analyser runs only while at least one surface
// claims it AND Style.live holds (something is playing). Power-saver no longer gates it: a
// visualizer the user is looking at is content, not decoration.
// levels settle flat when frames stop (silence, a restart gap, the gate closing).
Singleton {
    id: root

    property var owners: []
    readonly property bool claimed: owners.length > 0
    function claim(id, on) {
        var next = owners.filter(o => o !== id);
        if (on) next.push(id);
        owners = next;
    }

    readonly property bool analysing: root.claimed && Style.live
    readonly property int bands: 40
    readonly property int fps: 30

    property var levels: root.flat()
    property real energy: 0
    property real lastReadMs: 0

    function flat() {
        var a = [];
        for (var i = 0; i < root.bands; i++) a.push(0);
        return a;
    }

    Process {
        id: cava
        command: ["sh", "-c", "command -v cava >/dev/null 2>&1 || exit 0; cfg=\"${XDG_RUNTIME_DIR:-/tmp}/ryotunes-cava.conf\"; printf '%s\\n' '[general]' 'framerate = " + root.fps + "' 'bars = " + root.bands + "' '' '[input]' 'method = pipewire' 'source = auto' '' '[output]' 'method = raw' 'raw_target = /dev/stdout' 'data_format = ascii' 'ascii_max_range = 100' 'channels = mono' 'mono_option = average' '' '[smoothing]' 'noise_reduction = 45' > \"$cfg\"; exec cava -p \"$cfg\""]
        // Bound, never assigned (an imperative write would detach it from the gate).
        running: root.analysing && !cava.backoff
        property bool backoff: false
        stdout: SplitParser {
            splitMarker: "\n"
            onRead: (line) => root.readBars(line)
        }
        onExited: if (root.analysing) {
            cava.backoff = true;
            restartTimer.restart();
        }
    }
    Timer { id: restartTimer; interval: 1200; onTriggered: cava.backoff = false }

    Timer {
        interval: 120
        running: root.analysing
        repeat: true
        onTriggered: if (Date.now() - root.lastReadMs > 260) {
            root.levels = root.flat();
            root.energy = 0;
        }
    }
    onAnalysingChanged: {
        levels = flat();
        energy = 0;
        if (analysing) lastReadMs = 0;
    }

    function readBars(line) {
        var t = line.trim();
        if (!t) return;
        var parts = t.split(/[;\s]+/);
        if (parts.length < root.bands) return;
        var out = [], sum = 0;
        for (var i = 0; i < root.bands; i++) {
            var n = parseInt(parts[i]);
            var v = isNaN(n) ? 0 : Math.max(0, Math.min(1, n / 100));
            out.push(v); sum += v;
        }
        root.levels = out;
        root.energy = sum / root.bands;
        root.lastReadMs = Date.now();
    }
}
