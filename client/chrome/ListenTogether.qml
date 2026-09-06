pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The Listen Together session sheet, ported from ui/src/lib/components/ListenTogether.svelte. A modal
// overlay over the whole app frame, bound to App's own `open` flag. It is a pure view of Playback.lt
// (the daemon's lt-state snapshot, applied in lib/playback.js) and drives every mutation through the
// lt_* daemon methods. An invite bundles the server URL and room code as `RYO~<base64(server|code)>`
// so a guest pastes one thing. No timers: the live session is entirely event-driven from the daemon.
Item {
    id: root
    anchors.fill: parent
    visible: root.open

    property bool open: false
    property string mode: "join"       // "join" | "host"
    property string uname: ""
    property string serverUrl: ""
    property string inviteInput: ""
    property bool busy: false

    readonly property var lt: Playback.lt
    readonly property bool inRoom: !!(root.lt && root.lt.role && root.lt.role !== "none")
    readonly property bool isHost: !!(root.lt && root.lt.role === "host")
    readonly property bool waiting: !!(root.lt && root.lt.requesting && root.lt.role === "none")
    readonly property string invite: root.makeInvite((root.lt && root.lt.serverUrl) ? root.lt.serverUrl : "",
        (root.lt && root.lt.roomCode) ? root.lt.roomCode : "")

    onOpenChanged: {
        if (root.open) {
            root.uname = (Playback.auth && Playback.auth.name) ? String(Playback.auth.name).trim() : "";
            root.serverUrl = (Playback.lt && Playback.lt.serverUrl) ? Playback.lt.serverUrl : "";
            root.inviteInput = "";
            root.busy = false;
        }
    }

    function makeInvite(server, code) { return "RYO~" + Qt.btoa(server + "|" + code); }
    function parseInvite(raw) {
        var s = raw.trim();
        if (s.indexOf("RYO~") === 0) {
            try {
                var parts = Qt.atob(s.slice(4)).split("|");
                return { server: parts[0] || "", code: (parts[1] || "").toUpperCase() };
            } catch (e) {
                return null;
            }
        }
        return { server: "", code: s.toUpperCase() };
    }

    function host() {
        if (!root.uname.trim()) { Playback.toast("Enter a name first", "error"); return; }
        var u = root.serverUrl.trim();
        if (!u) { Playback.toast("Enter your sync server URL", "error"); return; }
        if (!/^wss?:\/\//i.test(u)) { Playback.toast("Use a ws:// or wss:// sync server address", "error"); return; }
        root.busy = true;
        var cur = (root.lt && root.lt.serverUrl) ? root.lt.serverUrl : "";
        var p = (u !== cur) ? Daemon.call("lt_set_server_url", { url: u }) : Promise.resolve();
        p.then(() => Daemon.call("lt_create_room", { username: root.uname.trim() }))
            .then(() => { root.busy = false; })
            .catch((e) => { root.busy = false; Playback.toast((e && e.message) ? e.message : String(e), "error"); });
    }
    function join() {
        if (!root.uname.trim()) { Playback.toast("Enter a name first", "error"); return; }
        var parsed = root.parseInvite(root.inviteInput);
        if (!parsed || !parsed.code) { Playback.toast("Paste the invite code your friend sent", "error"); return; }
        var cur = (root.lt && root.lt.serverUrl) ? root.lt.serverUrl : "";
        var server = parsed.server || cur;
        if (!server) { Playback.toast("Paste the full invite from the host, it carries the server address", "error"); return; }
        if (!/^wss?:\/\//i.test(server)) { Playback.toast("The invite does not contain a valid sync server", "error"); return; }
        root.busy = true;
        var p = (server !== cur) ? Daemon.call("lt_set_server_url", { url: server }) : Promise.resolve();
        p.then(() => Daemon.call("lt_join_room", { code: parsed.code, username: root.uname.trim() }))
            .then(() => { root.busy = false; })
            .catch((e) => { root.busy = false; Playback.toast((e && e.message) ? e.message : String(e), "error"); });
    }
    function leave() { Daemon.call("lt_leave").catch(() => {}); }
    function copyInvite() {
        clip.text = root.invite;
        clip.selectAll();
        clip.copy();
        Playback.toast("Invite copied, send it to a friend", "success");
    }

    // Hidden holder that owns the system-clipboard copy of the invite string.
    TextEdit { id: clip; visible: false; width: 0; height: 0 }

    // Scrim over the (App-blurred) page; a click outside the sheet dismisses. The page behind is
    // frozen and blurred by App while a modal is open (spec 9's snapshot blur is a layer.live:false
    // pass on the content, owned by App).
    Rectangle { anchors.fill: parent; color: "#000000"; opacity: 0.45 }
    MouseArea { anchors.fill: parent; onClicked: root.open = false }

    Rectangle {
        id: sheet
        anchors.centerIn: parent
        width: Math.min(parent.width - Style.sp(12), Style.sp(170))
        height: Math.min(parent.height - Style.sp(12), Math.max(Style.sp(120), bodyCol.implicitHeight + head.height + Style.sp(6)))
        radius: Style.radiusCard
        color: Tokens.paperLift
        border.width: 1
        border.color: Tokens.lineStrong
        MouseArea { anchors.fill: parent }   // swallow clicks so the dismiss layer doesn't close it

        // header
        Item {
            id: head
            anchors { left: parent.left; right: parent.right; top: parent.top }
            height: Style.sp(16)
            ColumnLayout {
                anchors.fill: parent
                anchors.leftMargin: Style.sp(4)
                anchors.rightMargin: Style.sp(4)
                anchors.topMargin: Style.sp(3)
                spacing: 1
                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(2)
                    Text { text: "力"; color: Tokens.inkMuted; font.family: Tokens.jp; font.pixelSize: Style.fs.sm }
                    Text {
                        text: "SESSION / SYNC · LT-01"
                        color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1.5
                    }
                    Item { Layout.fillWidth: true }
                    IconButton { icon: "close"; iconSize: Style.fs.md; diameter: Style.sp(7); onClicked: root.open = false }
                }
                Text { text: "Listen Together"; color: Tokens.ink; font.family: Style.fontDisplay; font.pixelSize: Style.fs.lg }
            }
            Hairline { anchors { left: parent.left; right: parent.right; bottom: parent.bottom } }
        }

        Flickable {
            id: scroll
            anchors { left: parent.left; right: parent.right; top: head.bottom; bottom: parent.bottom }
            anchors.margins: Style.sp(4)
            clip: true
            contentWidth: width
            contentHeight: bodyCol.implicitHeight
            boundsBehavior: Flickable.StopAtBounds

            ColumnLayout {
                id: bodyCol
                anchors { left: parent.left; right: parent.right; top: parent.top }
                spacing: Style.sp(2)

                // ── waiting for host / connecting ──────────────────────────────────
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: root.waiting
                    spacing: Style.sp(1)
                    Text { text: "// SESSION / HANDSHAKE"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1.5 }
                    Text {
                        Layout.fillWidth: true
                        text: (root.lt && root.lt.status === "connecting") ? "Connecting to the sync transport." : "Waiting for host approval."
                        color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.DemiBold; wrapMode: Text.WordWrap
                    }
                    Text {
                        Layout.fillWidth: true
                        text: "The music engine stays local until the session is accepted."
                        color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; wrapMode: Text.WordWrap
                    }
                    Pill { Layout.topMargin: Style.sp(1); label: "Cancel request"; onClicked: root.leave() }
                }

                // ── setup (join / host) ────────────────────────────────────────────
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: !root.waiting && !root.inRoom
                    spacing: Style.sp(2)

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(2)
                        Chip { text: "01 JOIN"; active: root.mode === "join"; onClicked: root.mode = "join" }
                        Chip { text: "02 HOST"; active: root.mode === "host"; onClicked: root.mode = "host" }
                        Item { Layout.fillWidth: true }
                    }

                    // JOIN
                    ColumnLayout {
                        Layout.fillWidth: true
                        visible: root.mode === "join"
                        spacing: Style.sp(2)
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: Style.sp(1)
                            Text { text: "INVITE"; color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1 }
                            Text { Layout.fillWidth: true; text: "Paste the code your friend sent. It already carries the server address."; color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.xs; wrapMode: Text.WordWrap }
                            Rectangle {
                                Layout.fillWidth: true
                                implicitHeight: Style.sp(9)
                                radius: Style.radius
                                color: Tokens.paper
                                border.width: 1
                                border.color: inviteField.activeFocus ? Tokens.lineStrong : Tokens.line
                                TextInput {
                                    id: inviteField
                                    anchors.fill: parent
                                    anchors.leftMargin: Style.sp(2)
                                    anchors.rightMargin: Style.sp(2)
                                    verticalAlignment: TextInput.AlignVCenter
                                    clip: true
                                    color: Tokens.ink; font.family: Style.fontMono; font.pixelSize: Style.fs.md
                                    text: root.inviteInput
                                    onTextChanged: root.inviteInput = text
                                    onAccepted: root.join()
                                    Text { anchors.verticalCenter: parent.verticalCenter; visible: inviteField.text.length === 0; text: "RYO~…"; color: Tokens.inkFaint; font: inviteField.font }
                                }
                            }
                        }
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: Style.sp(1)
                            Text { text: "NAME"; color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1 }
                            Text { Layout.fillWidth: true; text: "Shown to the other listeners in this room."; color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.xs; wrapMode: Text.WordWrap }
                            Rectangle {
                                Layout.fillWidth: true
                                implicitHeight: Style.sp(9)
                                radius: Style.radius
                                color: Tokens.paper
                                border.width: 1
                                border.color: joinNameField.activeFocus ? Tokens.lineStrong : Tokens.line
                                TextInput {
                                    id: joinNameField
                                    anchors.fill: parent
                                    anchors.leftMargin: Style.sp(2)
                                    anchors.rightMargin: Style.sp(2)
                                    verticalAlignment: TextInput.AlignVCenter
                                    clip: true
                                    color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md
                                    text: root.uname
                                    onTextChanged: root.uname = text
                                    onAccepted: root.join()
                                    Text { anchors.verticalCenter: parent.verticalCenter; visible: joinNameField.text.length === 0; text: "Your name"; color: Tokens.inkFaint; font: joinNameField.font }
                                }
                            }
                        }
                        Pill { Layout.topMargin: Style.sp(1); label: root.busy ? "Connecting…" : "Join session"; icon: "group"; primary: true; enabled: !root.busy; onClicked: root.join() }
                    }

                    // HOST
                    ColumnLayout {
                        Layout.fillWidth: true
                        visible: root.mode === "host"
                        spacing: Style.sp(2)
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: Style.sp(1)
                            Text { text: "SYNC SERVER"; color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1 }
                            Text { Layout.fillWidth: true; text: "Your self-hosted WebSocket endpoint. It is remembered on this machine."; color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.xs; wrapMode: Text.WordWrap }
                            Rectangle {
                                Layout.fillWidth: true
                                implicitHeight: Style.sp(9)
                                radius: Style.radius
                                color: Tokens.paper
                                border.width: 1
                                border.color: serverField.activeFocus ? Tokens.lineStrong : Tokens.line
                                TextInput {
                                    id: serverField
                                    anchors.fill: parent
                                    anchors.leftMargin: Style.sp(2)
                                    anchors.rightMargin: Style.sp(2)
                                    verticalAlignment: TextInput.AlignVCenter
                                    clip: true
                                    color: Tokens.ink; font.family: Style.fontMono; font.pixelSize: Style.fs.md
                                    text: root.serverUrl
                                    onTextChanged: root.serverUrl = text
                                    onAccepted: root.host()
                                    Text { anchors.verticalCenter: parent.verticalCenter; visible: serverField.text.length === 0; text: "wss://relay.example.org/ws"; color: Tokens.inkFaint; font: serverField.font }
                                }
                            }
                        }
                        ColumnLayout {
                            Layout.fillWidth: true
                            spacing: Style.sp(1)
                            Text { text: "NAME"; color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1 }
                            Text { Layout.fillWidth: true; text: "The host identity other listeners will see."; color: Tokens.inkFaint; font.family: Style.fontUi; font.pixelSize: Style.fs.xs; wrapMode: Text.WordWrap }
                            Rectangle {
                                Layout.fillWidth: true
                                implicitHeight: Style.sp(9)
                                radius: Style.radius
                                color: Tokens.paper
                                border.width: 1
                                border.color: hostNameField.activeFocus ? Tokens.lineStrong : Tokens.line
                                TextInput {
                                    id: hostNameField
                                    anchors.fill: parent
                                    anchors.leftMargin: Style.sp(2)
                                    anchors.rightMargin: Style.sp(2)
                                    verticalAlignment: TextInput.AlignVCenter
                                    clip: true
                                    color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md
                                    text: root.uname
                                    onTextChanged: root.uname = text
                                    onAccepted: root.host()
                                    Text { anchors.verticalCenter: parent.verticalCenter; visible: hostNameField.text.length === 0; text: "Your name"; color: Tokens.inkFaint; font: hostNameField.font }
                                }
                            }
                        }
                        Pill { Layout.topMargin: Style.sp(1); label: root.busy ? "Starting…" : "Start session"; icon: "group"; primary: true; enabled: !root.busy; onClicked: root.host() }
                    }
                }

                // ── in room ────────────────────────────────────────────────────────
                ColumnLayout {
                    Layout.fillWidth: true
                    visible: root.inRoom
                    spacing: Style.sp(2)

                    // room state + invite
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(1)
                        RowLayout {
                            Layout.fillWidth: true
                            spacing: Style.sp(2)
                            Text { text: "// LIVE SESSION"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1.5 }
                            Item { Layout.fillWidth: true }
                            Text {
                                text: (root.isHost ? "HOST · " : "GUEST · ") + ((root.lt && root.lt.status) ? String(root.lt.status).toUpperCase() : "")
                                color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs
                            }
                        }
                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: Style.sp(9)
                            radius: Style.radius
                            color: Tokens.paper
                            border.width: 1
                            border.color: Tokens.line
                            RowLayout {
                                anchors.fill: parent
                                anchors.leftMargin: Style.sp(2.5)
                                anchors.rightMargin: Style.sp(1)
                                spacing: Style.sp(2)
                                Text {
                                    Layout.fillWidth: true
                                    text: root.invite
                                    color: Tokens.inkDim; font.family: Style.fontMono; font.pixelSize: Style.fs.sm; elide: Text.ElideRight
                                }
                                Pill { label: "Copy invite"; icon: "link"; onClicked: root.copyInvite() }
                            }
                        }
                        // now playing in the room
                        RowLayout {
                            Layout.fillWidth: true
                            Layout.topMargin: Style.sp(1)
                            visible: !!(root.lt && root.lt.currentTrack)
                            spacing: Style.sp(2)
                            Artwork {
                                url: (root.lt && root.lt.currentTrack && root.lt.currentTrack.thumbnail) ? root.lt.currentTrack.thumbnail : ""
                                px: Style.sp(12)
                                placeholderIcon: "music"
                            }
                            ColumnLayout {
                                Layout.fillWidth: true
                                spacing: 0
                                Text { text: "NOW PLAYING"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs }
                                Text {
                                    Layout.fillWidth: true
                                    text: (root.lt && root.lt.currentTrack) ? root.lt.currentTrack.title : ""
                                    color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; font.weight: Font.DemiBold; elide: Text.ElideRight
                                }
                                Text {
                                    Layout.fillWidth: true
                                    text: (root.lt && root.lt.currentTrack) ? root.lt.currentTrack.artist : ""
                                    color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.sm; elide: Text.ElideRight
                                }
                            }
                        }
                    }

                    // join requests (host only)
                    ColumnLayout {
                        Layout.fillWidth: true
                        visible: root.isHost && !!(root.lt && root.lt.pendingJoins && root.lt.pendingJoins.length)
                        spacing: Style.sp(1)
                        Text {
                            text: "// JOIN REQUESTS · " + ((root.lt && root.lt.pendingJoins) ? root.lt.pendingJoins.length : 0)
                            color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1.5
                        }
                        Repeater {
                            model: (root.lt && root.lt.pendingJoins) ? root.lt.pendingJoins : []
                            delegate: RowLayout {
                                id: joinRow
                                required property var modelData
                                Layout.fillWidth: true
                                spacing: Style.sp(2)
                                Text { text: "?"; color: Tokens.sun; font.family: Style.fontMono; font.pixelSize: Style.fs.md }
                                Text { Layout.fillWidth: true; text: joinRow.modelData.username; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; elide: Text.ElideRight }
                                IconButton { icon: "check-circle"; iconSize: Style.fs.md; diameter: Style.sp(7); onClicked: Daemon.call("lt_approve_join", { userId: joinRow.modelData.userId }).catch(() => {}) }
                                IconButton { icon: "close"; iconSize: Style.fs.md; diameter: Style.sp(7); onClicked: Daemon.call("lt_reject_join", { userId: joinRow.modelData.userId }).catch(() => {}) }
                            }
                        }
                    }

                    // listeners
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(1)
                        Text {
                            text: "// LISTENERS · " + ((root.lt && root.lt.users) ? root.lt.users.length : 0)
                            color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1.5
                        }
                        Repeater {
                            model: (root.lt && root.lt.users) ? root.lt.users : []
                            delegate: RowLayout {
                                id: userRow
                                required property var modelData
                                readonly property bool isMe: !!(root.lt && root.lt.myId && userRow.modelData.user_id === root.lt.myId)
                                Layout.fillWidth: true
                                spacing: Style.sp(2)
                                opacity: userRow.modelData.is_connected ? 1 : 0.5
                                Text { text: userRow.modelData.is_connected ? "●" : "○"; color: userRow.modelData.is_connected ? Tokens.sun : Tokens.inkFaint; font.pixelSize: Style.fs.sm }
                                Text {
                                    text: userRow.modelData.username + (userRow.isMe ? " · YOU" : "")
                                    color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; elide: Text.ElideRight
                                }
                                Text { text: userRow.modelData.is_host ? "HOST" : "LISTENER"; color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs }
                                Item { Layout.fillWidth: true }
                                IconButton {
                                    visible: root.isHost && !userRow.isMe
                                    icon: "account"; iconSize: Style.fs.sm; diameter: Style.sp(7)
                                    onClicked: Daemon.call("lt_transfer_host", { userId: userRow.modelData.user_id }).catch(() => {})
                                }
                                IconButton {
                                    visible: root.isHost && !userRow.isMe
                                    icon: "close"; iconSize: Style.fs.sm; diameter: Style.sp(7)
                                    onClicked: Daemon.call("lt_kick", { userId: userRow.modelData.user_id }).catch(() => {})
                                }
                            }
                        }
                    }

                    // suggestions (host only)
                    ColumnLayout {
                        Layout.fillWidth: true
                        visible: root.isHost && !!(root.lt && root.lt.suggestions && root.lt.suggestions.length)
                        spacing: Style.sp(1)
                        Text {
                            text: "// SUGGESTIONS · " + ((root.lt && root.lt.suggestions) ? root.lt.suggestions.length : 0)
                            color: Tokens.inkFaint; font.family: Style.fontMono; font.pixelSize: Style.fs.xs; font.letterSpacing: 1.5
                        }
                        Repeater {
                            model: (root.lt && root.lt.suggestions) ? root.lt.suggestions : []
                            delegate: RowLayout {
                                id: sugRow
                                required property var modelData
                                Layout.fillWidth: true
                                spacing: Style.sp(2)
                                ColumnLayout {
                                    Layout.fillWidth: true
                                    spacing: 0
                                    Text { Layout.fillWidth: true; text: sugRow.modelData.track.title; color: Tokens.ink; font.family: Style.fontUi; font.pixelSize: Style.fs.md; elide: Text.ElideRight }
                                    Text { Layout.fillWidth: true; text: sugRow.modelData.track.artist + " · from " + sugRow.modelData.from_username; color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.xs; elide: Text.ElideRight }
                                }
                                IconButton { icon: "check-circle"; iconSize: Style.fs.md; diameter: Style.sp(7); onClicked: Daemon.call("lt_approve_suggestion", { id: sugRow.modelData.id }).catch(() => {}) }
                                IconButton { icon: "close"; iconSize: Style.fs.md; diameter: Style.sp(7); onClicked: Daemon.call("lt_reject_suggestion", { id: sugRow.modelData.id }).catch(() => {}) }
                            }
                        }
                    }

                    // footer
                    RowLayout {
                        Layout.fillWidth: true
                        Layout.topMargin: Style.sp(1)
                        spacing: Style.sp(2)
                        Text {
                            text: "SYNC · " + ((root.lt && root.lt.status) ? String(root.lt.status).toUpperCase() : "")
                            color: Tokens.inkMuted; font.family: Style.fontMono; font.pixelSize: Style.fs.xs
                        }
                        Item { Layout.fillWidth: true }
                        Pill { visible: !root.isHost; label: "Re-sync"; icon: "on-repeat"; onClicked: Daemon.call("lt_request_sync").catch(() => {}) }
                        Pill { label: "Leave"; onClicked: root.leave() }
                    }
                }
            }
        }
    }
}
