pragma Singleton
import QtQuick
import Quickshell

// The client's side of the diagnostics record. Every failure the UI can see — a rejected RPC,
// a lost daemon connection, a playback stop, a QML binding error — goes through root.error()/
// root.warn(). While the daemon is reachable the entry is forwarded straight to `log_client_error`
// (it lands in the same ring + file as daemon entries, tagged `client`); while it is not, entries
// buffer here (bounded) and flush on the next successful connection, because "the daemon died"
// is exactly the failure whose evidence must survive. A short window around each burst collapses
// repeats of the same message so a retry loop cannot flood the log.
Singleton {
    id: root

    readonly property int maxBuffer: 200
    property var buffer: []

    // Last-forwarded key + time, for the repeat window.
    property string lastKey: ""
    property real lastAt: 0

    Connections {
        target: Daemon
        function onConnectedChanged(): void {
            if (Daemon.connected)
                root.flush();
        }
        function onCallFailed(level, target, message) {
            root._record(level, target, message);
        }
    }

    // Every failure the user actually sees passes through the toast layer: local validation
    // ("sign in first"), daemon events (cover-error, playback stops) and surfaced RPC errors.
    // Recording them means a screenshot of a toast is never the only evidence of a bug.
    Connections {
        target: Playback
        function onToast(message, kind) {
            if (kind === "error")
                root._record("error", "toast", message);
        }
    }
    function warn(target, message) {
        root._record("warn", target, message);
    }
    function error(target, message) {
        root._record("error", target, message);
    }

    function _record(level, target, message) {
        var text = String(message === undefined ? "" : message);
        if (text === "")
            text = "unknown failure";
        // Collapse an exact repeat of the same level/target/message inside 3 s.
        var key = level + "\u0000" + target + "\u0000" + text;
        var now = Date.now();
        if (key === root.lastKey && now - root.lastAt < 3000)
            return;
        root.lastKey = key;
        root.lastAt = now;

        var entry = { level: level, target: target, message: text };
        if (Daemon.connected) {
            Daemon.call("log_client_error", entry).catch(function () {
                root._buffer(entry);
            });
        } else {
            root._buffer(entry);
        }
    }

    function _buffer(entry) {
        var next = root.buffer.slice();
        next.push(entry);
        if (next.length > root.maxBuffer)
            next = next.slice(next.length - root.maxBuffer);
        root.buffer = next;
    }

    // Deliver everything buffered while the daemon was down. One failure to post stops the
    // flush (the rest stay buffered for the next connection); entries carry their own text, so
    // a daemon that is flapping still accumulates them locally.
    function flush() {
        if (root.buffer.length === 0)
            return;
        var pending = root.buffer;
        root.buffer = [];
        var chain = Promise.resolve();
        for (var i = 0; i < pending.length; i++) {
            (function (entry) {
                chain = chain.then(function () {
                    return Daemon.call("log_client_error", entry);
                }).catch(function () {
                    root._buffer(entry);
                });
            })(pending[i]);
        }
    }
}
