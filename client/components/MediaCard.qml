pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../lib/ids.js" as Ids
import "../lib/browse.js" as Browse

// A shelf / grid card (spec section 4): 168 px square artwork (a circle for an artist) at
// radiusCard, a title in md and a subtitle in sm. Hover lifts the whole card two pixels and rings
// the art with a lineStrong border, animated over Style.motion.snap. A primary click opens the item
// (a song plays, a collection routes to its page); the hover play button plays the whole thing
// without leaving the page.
Item {
    id: root

    property var item: null
    property int cardWidth: Style.cardW
    readonly property bool round: root.item && root.item.kind === "artist"

    width: cardWidth
    implicitHeight: col.implicitHeight

    // active hover lift (transform, so it never disturbs layout)
    transform: Translate {
        y: cardHover.hovered ? -2 : 0
        Behavior on y { NumberAnimation { duration: Style.motion.snap; easing.type: Easing.OutQuad } }
    }

    function open() {
        if (!root.item)
            return;
        if (root.item.kind === "song")
            Playback.play(Browse.asSong(root.item));
        else
            Router.push(root.item.kind, { id: root.item.id, title: root.item.title });
    }

    function playNow() {
        var it = root.item;
        if (!it)
            return;
        if (it.kind === "song") {
            Playback.play(Browse.asSong(it));
            return;
        }
        if (it.kind === "album") {
            Daemon.call("get_album", { id: it.id })
                .then((a) => Daemon.call("play_playlist", { items: a.items, sourceId: a.playlistId, sourceName: it.title }))
                .catch(() => Playback.toast("Could not play — try opening it", "error"));
        } else {
            Daemon.call("get_playlist", { id: it.id })
                .then((p) => Daemon.call("play_playlist", {
                    items: p.items,
                    sourceId: Ids.isSmartPlaylistId(it.id) ? undefined : it.id,
                    sourceName: it.title,
                    continuation: p.continuation
                }))
                .catch(() => Playback.toast("Could not play — try opening it", "error"));
        }
    }

    MouseArea {
        anchors.fill: parent
        cursorShape: Qt.PointingHandCursor
        onClicked: root.open()
    }

    ColumnLayout {
        id: col
        width: parent.width
        spacing: Style.sp(2)

        Item {
            Layout.fillWidth: true
            implicitHeight: width

            Artwork {
                id: art
                anchors.fill: parent
                url: root.item && root.item.thumbnail ? root.item.thumbnail : ""
                px: root.cardWidth
                round: root.round
                cornerRadius: root.round ? art.width / 2 : Style.radiusCard
                placeholderIcon: root.round ? "user"
                    : (root.item && Ids.isOnRepeatId(root.item.id)) ? "on-repeat" : "music"
            }

            // hover ring
            Rectangle {
                anchors.fill: parent
                visible: cardHover.hovered
                radius: root.round ? width / 2 : Style.radiusCard
                color: "transparent"
                border.width: 1
                border.color: Tokens.lineStrong
            }

            // hover play (everything but an artist)
            Rectangle {
                visible: !root.round && cardHover.hovered
                anchors.right: parent.right
                anchors.bottom: parent.bottom
                anchors.margins: Style.sp(2)
                width: Style.sp(9)
                height: width
                radius: Style.radius
                color: Tokens.paper
                border.width: 1
                border.color: Tokens.line
                Icon {
                    anchors.centerIn: parent
                    name: "play"
                    size: Style.fs.md
                    color: Tokens.ink
                }
                MouseArea {
                    anchors.fill: parent
                    cursorShape: Qt.PointingHandCursor
                    onClicked: root.playNow()
                }
            }
        }

        Text {
            Layout.fillWidth: true
            horizontalAlignment: root.round ? Text.AlignHCenter : Text.AlignLeft
            text: root.item ? root.item.title : ""
            color: Tokens.ink
            font.family: Style.fontUi
            font.pixelSize: Style.fs.md
            font.weight: Font.Medium
            elide: Text.ElideRight
        }
        Text {
            Layout.fillWidth: true
            visible: !!(root.item && root.item.subtitle)
            horizontalAlignment: root.round ? Text.AlignHCenter : Text.AlignLeft
            text: root.item && root.item.subtitle ? root.item.subtitle : ""
            color: Tokens.inkMuted
            font.family: Style.fontUi
            font.pixelSize: Style.fs.sm
            elide: Text.ElideRight
        }
    }

    HoverHandler { id: cardHover }
}
