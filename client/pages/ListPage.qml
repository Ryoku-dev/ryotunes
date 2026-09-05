pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"

// A browse grid, ported from ui/src/routes/list/+page.svelte. One get_browse_grid call for the
// {id, params} the caller routed with (a shelf's "See all"); the result is however many cards
// YouTube sends, drawn in a reused GridView so a long grid never mounts more than a couple of
// screenfuls. A browse list has no cover or play-all, so it opens with a plain page-title header
// (tracked eyebrow + Fraunces title) rather than the media pages' PageHero, then the card grid.
Item {
    id: page

    readonly property var params: Router.current ? Router.current.params : ({})
    readonly property string browseId: page.params && page.params.id ? page.params.id : ""
    readonly property string browseParams: page.params && page.params.params ? page.params.params : ""
    readonly property string title: page.params && page.params.title ? page.params.title : "More"

    property var items: []
    property bool loading: true
    property string errorMsg: ""

    onParamsChanged: page.load()
    Component.onCompleted: page.load()

    function load() {
        if (!page.browseId)
            return;
        page.loading = true;
        page.errorMsg = "";
        page.items = [];
        var reqId = page.browseId;
        Daemon.call("get_browse_grid", { id: page.browseId, params: page.browseParams ? page.browseParams : null })
            .then((items) => {
                if (page.browseId !== reqId)
                    return;
                page.items = items || [];
                page.loading = false;
            })
            .catch((e) => {
                if (page.browseId !== reqId)
                    return;
                page.errorMsg = (e && e.message) ? e.message : String(e);
                page.loading = false;
            });
    }

    GridView {
        id: grid
        anchors.fill: parent
        topMargin: Style.sp(2)
        bottomMargin: Style.sp(20)
        clip: true
        reuseItems: true
        cacheBuffer: Math.max(0, Math.round(height * 1.5))
        boundsBehavior: Flickable.StopAtBounds
        readonly property int cols: Math.max(2, Math.floor(width / (Style.cardW + Style.sp(4))))
        cellWidth: Math.floor(width / grid.cols)
        cellHeight: grid.cellWidth + Style.sp(16)
        model: page.loading || page.errorMsg ? [] : page.items

        header: Item {
            width: grid.width
            implicitHeight: headCol.implicitHeight + Style.sp(6)
            ColumnLayout {
                id: headCol
                width: parent.width
                spacing: Style.sp(1)
                Text {
                    text: "BROWSE"
                    color: Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
                }
                Text {
                    Layout.fillWidth: true
                    text: page.title
                    color: Tokens.ink
                    font.family: Style.fontDisplay
                    font.pixelSize: Style.fs.title
                    elide: Text.ElideRight
                }
            }
        }

        delegate: Item {
            required property var modelData
            width: grid.cellWidth
            height: grid.cellHeight
            MediaCard {
                anchors.horizontalCenter: parent.horizontalCenter
                y: 0
                item: parent.modelData
                cardWidth: grid.cellWidth - Style.sp(4)
            }
        }
    }

    // loading / error / empty
    ColumnLayout {
        anchors.centerIn: parent
        spacing: Style.sp(2)
        visible: page.loading || page.errorMsg !== "" || (!page.items.length)
        Text {
            Layout.alignment: Qt.AlignHCenter
            text: page.loading ? "Loading…" : page.errorMsg !== "" ? page.errorMsg : "Nothing here."
            color: Tokens.inkMuted
            font.family: Style.fontUi
            font.pixelSize: Style.fs.md
        }
        Rectangle {
            Layout.alignment: Qt.AlignHCenter
            visible: page.errorMsg !== ""
            implicitWidth: Style.sp(20)
            implicitHeight: Style.ctlH
            radius: Style.radius
            color: retryHover.hovered ? Tokens.tint10 : "transparent"
            border.width: 1
            border.color: Tokens.line
            Text {
                anchors.centerIn: parent
                text: "Try again"
                color: Tokens.ink
                font.family: Style.fontUi
                font.pixelSize: Style.fs.sm
            }
            HoverHandler { id: retryHover }
            MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: page.load() }
        }
    }
}
