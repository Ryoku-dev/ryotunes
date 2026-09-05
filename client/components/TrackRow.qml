pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../lib/ids.js" as Ids

// One track-table row (Sonora's list in Ryoku's skin, spec section 4). The whole row is the play
// target; the leading slot carries the track number, swapping to a play glyph on hover and to an
// accent note while this is the row that is playing. Columns — index | art | title / artist | album
// | plays | length | actions — line up with the micro header TrackList draws when `showHeader`.
// Rating is optimistic and hidden for local files and radio, which YouTube cannot rate.
Rectangle {
    id: root

    property var song: null
    property int index: -1
    property bool active: false
    // Full-table opt-ins (default off, so the queue / library rows stay lean): the album column
    // (only when the row is at least 900 px wide), the plays column and the ⋯ menu with its
    // context items. `menuRequested` carries scene coords for the list's shared Menu.
    property bool showAlbum: false
    property bool showPlays: false
    // A true table (separate title / artist columns aligned to TrackList's header). Off by
    // default, so a narrow surface (the queue panel, search rows) stacks artist beneath the title.
    property bool tableColumns: false
    property bool hideThumb: false
    property bool menu: false
    property bool canAdd: false
    property bool canRemove: false
    property string removeLabel: "Remove from playlist"
    signal play()
    signal menuRequested(real sx, real sy)

    // Column widths — shared with TrackList's header so the two line up (same Style.sp constants,
    // same RowLayout margins and spacing).
    readonly property bool wideAlbum: root.showAlbum && root.width >= Style.sp(225)

    // Seeded from the row's own snapshot; a rating call updates it optimistically. Reset when the
    // delegate is reused for a different song.
    property string rated: (song && song.rating) ? song.rating : "indifferent"
    onSongChanged: rated = (song && song.rating) ? song.rating : "indifferent"

    readonly property bool canRate: !!song && !Ids.isLocalId(song.video_id) && !Ids.isRadioId(song.video_id)
    readonly property string duration: (song && song.duration && /^[\d:]+$/.test(song.duration)) ? song.duration : ""
    readonly property string album: (song && song.album) ? song.album : ""
    readonly property string plays: (song && song.play_count) ? song.play_count : ""

    implicitHeight: Style.rowH
    radius: Style.radius
    color: root.active ? Tokens.tint10 : rowHover.hovered ? Tokens.tint5 : "transparent"

    function toggleLike() {
        if (!root.canRate || !root.song)
            return;
        var next = root.rated === "like" ? "indifferent" : "like";
        root.rated = next;
        Daemon.call("rate", { videoId: root.song.video_id, rating: next })
            .catch((e) => Playback.toast((e && e.message) ? e.message : "Could not rate", "error"));
    }

    MouseArea {
        id: playArea
        anchors.fill: parent
        cursorShape: Qt.PointingHandCursor
        acceptedButtons: root.menu ? (Qt.LeftButton | Qt.RightButton) : Qt.LeftButton
        onClicked: (mouse) => {
            if (mouse.button === Qt.RightButton) {
                var p = playArea.mapToItem(null, mouse.x, mouse.y);
                root.menuRequested(p.x, p.y);
                return;
            }
            root.play();
        }
    }

    // The recurring one-pixel divider between rows (Sonora's table rule). Softened under the
    // playing row so its tint reads as one block.
    Rectangle {
        anchors { left: parent.left; right: parent.right; bottom: parent.bottom }
        anchors.leftMargin: Style.sp(2)
        anchors.rightMargin: Style.sp(2)
        height: 1
        color: Tokens.lineSoft
        visible: !root.active
    }

    RowLayout {
        anchors.fill: parent
        anchors.leftMargin: Style.sp(2)
        anchors.rightMargin: Style.sp(2)
        spacing: Style.sp(3)

        // index / play-on-hover / accent note while playing
        Item {
            Layout.preferredWidth: Style.sp(10)
            Layout.fillHeight: true
            Text {
                anchors.centerIn: parent
                visible: !root.active && !rowHover.hovered && root.index >= 0
                text: root.index + 1
                color: Tokens.inkMuted
                font.family: Style.fontMono
                font.pixelSize: Style.fs.sm
            }
            Icon {
                anchors.centerIn: parent
                visible: !root.active && rowHover.hovered
                name: "play"
                size: Style.fs.md
                color: Tokens.ink
            }
            Icon {
                anchors.centerIn: parent
                visible: root.active
                name: "music"
                size: Style.fs.md
                color: Style.accent
            }
        }

        // artwork (album track lists hide it — the number is the identity there)
        Artwork {
            visible: !root.hideThumb
            Layout.preferredWidth: Style.sp(10)
            Layout.preferredHeight: Style.sp(10)
            url: root.song && root.song.thumbnail ? root.song.thumbnail : ""
            px: Style.sp(10)
        }

        // title / artist — stacked beneath each other on a narrow surface (queue, search rows)
        ColumnLayout {
            visible: !root.tableColumns
            Layout.fillWidth: true
            Layout.preferredWidth: Style.sp(60)
            spacing: 0
            RowLayout {
                Layout.fillWidth: true
                spacing: Style.sp(1.5)
                Text {
                    Layout.fillWidth: true
                    text: root.song ? root.song.title : ""
                    color: root.active ? Style.accent : Tokens.ink
                    font.family: Style.fontUi
                    font.pixelSize: Style.fs.md
                    font.weight: Font.Medium
                    elide: Text.ElideRight
                }
                Rectangle {
                    visible: !!(root.song && root.song.explicit)
                    Layout.preferredWidth: Style.sp(4)
                    Layout.preferredHeight: Style.sp(4)
                    radius: Style.sp(1)
                    color: "transparent"
                    border.width: 1
                    border.color: Tokens.inkFaint
                    Text { anchors.centerIn: parent; text: "E"; color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.xs; font.weight: Font.DemiBold }
                }
            }
            Text {
                Layout.fillWidth: true
                text: (root.song && root.song.artists) ? root.song.artists : ""
                color: Tokens.inkMuted
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
                elide: Text.ElideRight
            }
        }

        // title column — full table (aligns to the # TITLE header)
        RowLayout {
            visible: root.tableColumns
            Layout.fillWidth: true
            Layout.preferredWidth: Style.sp(60)
            spacing: Style.sp(1.5)
            Text {
                Layout.fillWidth: true
                text: root.song ? root.song.title : ""
                color: root.active ? Style.accent : Tokens.ink
                font.family: Style.fontUi
                font.pixelSize: Style.fs.md
                font.weight: Font.Medium
                elide: Text.ElideRight
            }
            Rectangle {
                visible: !!(root.song && root.song.explicit)
                Layout.preferredWidth: Style.sp(4)
                Layout.preferredHeight: Style.sp(4)
                radius: Style.sp(1)
                color: "transparent"
                border.width: 1
                border.color: Tokens.inkFaint
                Text { anchors.centerIn: parent; text: "E"; color: Tokens.inkMuted; font.family: Style.fontUi; font.pixelSize: Style.fs.xs; font.weight: Font.DemiBold }
            }
        }

        // artist column — full table
        Text {
            visible: root.tableColumns
            Layout.fillWidth: true
            Layout.preferredWidth: Style.sp(40)
            text: (root.song && root.song.artists) ? root.song.artists : ""
            color: Tokens.inkMuted
            font.family: Style.fontUi
            font.pixelSize: Style.fs.sm
            elide: Text.ElideRight
        }

        // album
        Text {
            visible: root.wideAlbum
            Layout.preferredWidth: Style.sp(48)
            text: root.album
            color: Tokens.inkMuted
            font.family: Style.fontUi
            font.pixelSize: Style.fs.sm
            elide: Text.ElideRight
        }

        // plays
        Text {
            visible: root.showPlays
            Layout.preferredWidth: Style.sp(22)
            horizontalAlignment: Text.AlignRight
            text: root.plays
            color: Tokens.inkFaint
            font.family: Style.fontUi
            font.pixelSize: Style.fs.sm
            elide: Text.ElideRight
        }

        // length
        Text {
            Layout.preferredWidth: Style.sp(14)
            horizontalAlignment: Text.AlignRight
            text: root.duration
            color: Tokens.inkFaint
            font.family: Style.fontMono
            font.pixelSize: Style.fs.xs
        }

        // actions: like + more
        Item {
            Layout.preferredWidth: root.menu ? Style.sp(19) : Style.sp(9)
            Layout.fillHeight: true
            Row {
                anchors.right: parent.right
                anchors.verticalCenter: parent.verticalCenter
                spacing: Style.sp(1)
                IconButton {
                    visible: root.canRate
                    icon: "heart"
                    iconSize: Style.fs.md
                    diameter: Style.sp(9)
                    opacity: (rowHover.hovered || root.rated === "like") ? 1 : 0
                    active: root.rated === "like"
                    iconColor: root.rated === "like" ? Style.accent : Tokens.inkMuted
                    onClicked: root.toggleLike()
                }
                IconButton {
                    id: moreBtn
                    visible: root.menu
                    icon: "more"
                    iconSize: Style.fs.md
                    diameter: Style.sp(9)
                    opacity: rowHover.hovered ? 1 : 0
                    iconColor: Tokens.inkMuted
                    onClicked: {
                        var p = moreBtn.mapToItem(null, 0, moreBtn.height);
                        root.menuRequested(p.x, p.y);
                    }
                }
            }
        }
    }

    HoverHandler { id: rowHover }
}
