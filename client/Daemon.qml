pragma Singleton
import QtQuick
import Quickshell
import Quickshell.Io

// One connection to ryotunesd. Requests carry an id and resolve a Promise; lines without
// an id are events. A dropped connection is retried every 2 s; once a subscription is asked
// for it is re-sent on every reconnect, and every pending request is rejected on a drop so no
// page waits forever.
Singleton {
    id: root

    readonly property string socketPath: (Quickshell.env("XDG_RUNTIME_DIR") || "/tmp") + "/ryotunes/ryotunesd.sock"
    readonly property bool connected: !!root.sock && root.sock.connected
    property int protocol: 0
    property string daemonVersion: ""

    // A daemon event (no id): name plus its payload, fanned out to Playback and any surface.
    signal event(string name, var data)
    // The `subscribe` reply: the full { playback, queue, settings, auth } snapshot, delivered on
    // the first subscribe and again after every reconnect so Playback resynchronises each time.
    signal snapshot(var data)

    property int nextId: 1
    property var pending: ({})
    // Whether a subscription has been asked for; drives the re-subscribe on reconnect.
    property bool wantSubscribe: false
    property var sock: null

    function reconnect() {
        if (root.connected) return;
        // Quickshell retains the native socket after an initial connection error; assigning
        // connected=true again does not reconnect it. A fresh Socket also clears stale framing.
        if (root.sock) root.sock.destroy();
        root.sock = socketComponent.createObject(root);
        root.sock.connected = true;
    }
    Component.onCompleted: root.reconnect()

    function call(method, params) {
        return new Promise((resolve, reject) => {
            if (!root.connected) {
                reject({ code: "disconnected", message: "ryotunesd is not connected" });
                return;
            }
            const id = root.nextId++;
            root.pending[id] = { resolve, reject };
            sock.write(JSON.stringify({ id, method, params: params === undefined ? null : params }) + "\n");
            sock.flush();
        });
    }

    // Ask the daemon for a hello (version handshake) and the event stream with its opening
    // snapshot. Idempotent: sets the intent, subscribes now if connected, and the socket re-runs
    // it on every future reconnect. Returns the subscribe Promise for the immediate caller.
    function subscribeAll() {
        root.wantSubscribe = true;
        return root._subscribe();
    }

    function _subscribe() {
        if (!root.connected)
            return Promise.reject({ code: "disconnected", message: "ryotunesd is not connected" });
        root.call("hello").then(h => { root.protocol = h.protocol; root.daemonVersion = h.daemon; });
        const p = root.call("subscribe", { events: ["*"] });
        p.then(s => root.snapshot(s));
        return p;
    }

    function handleLine(line) {
        let msg;
        try { msg = JSON.parse(line); } catch (e) { return; }
        if (msg.event !== undefined) {
            root.event(msg.event, msg.data);
            return;
        }
        const p = root.pending[msg.id];
        if (!p) return;
        delete root.pending[msg.id];
        if (msg.error) p.reject(msg.error); else p.resolve(msg.result);
    }

    Component {
        id: socketComponent
        Socket {
            id: connection
            path: root.socketPath
            parser: SplitParser { onRead: line => root.handleLine(line) }
            onConnectionStateChanged: {
                if (root.sock !== connection) return;
                if (connection.connected) {
                    if (root.wantSubscribe) root._subscribe();
                } else {
                    for (const id in root.pending) root.pending[id].reject({ code: "disconnected", message: "connection lost" });
                    root.pending = {};
                }
            }
        }
    }
    Timer {
        id: retry
        interval: 2000
        running: !root.connected
        repeat: true
        onTriggered: root.reconnect()
    }
}
