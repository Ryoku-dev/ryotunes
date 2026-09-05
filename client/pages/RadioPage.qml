pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The internet-radio directory, ported from ui/src/routes/radio/+page.svelte. Discovery is strictly
// demand-driven: radio_stations is called on open, on a search, and on "load more" — never on a
// timer — so an idle Radio page contacts nothing. Stations paginate 36 at a time into a reuseItems
// GridView; a generation counter drops the reply of a superseded query. play_radio_station streams
// the station through the same engine, open_external opens a station's site in the real browser.
Item {
    id: page

    readonly property int pageSize: 36

    property string input: ""
    property string query: ""
    property var stations: []
    property bool loading: false
    property bool loadingMore: false
    property string errorMsg: ""
    property bool hasMore: true
    property string playing: ""
    // Bumped on every reset so a slow reply from a previous query is discarded, not rendered.
    property int generation: 0

    Component.onCompleted: page.load(true)

    function load(reset) {
        if ((reset && page.loading) || (!reset && (page.loadingMore || !page.hasMore)))
            return;
        var myGen = reset ? ++page.generation : page.generation;
        var offset = reset ? 0 : page.stations.length;
        if (reset) {
            page.loading = true;
            page.errorMsg = "";
        } else {
            page.loadingMore = true;
        }
        Daemon.call("radio_stations", { query: page.query, offset: offset, limit: page.pageSize })
            .then((rows) => {
                if (myGen !== page.generation)
                    return;
                rows = rows || [];
                if (reset) {
                    page.stations = rows;
                } else {
                    var seen = ({});
                    page.stations.forEach((s) => { seen[s.stationUuid] = true; });
                    page.stations = page.stations.concat(rows.filter((s) => !seen[s.stationUuid]));
                }
                page.hasMore = rows.length === page.pageSize;
                page.loading = false;
                page.loadingMore = false;
            })
            .catch((e) => {
                if (myGen !== page.generation)
                    return;
                page.errorMsg = (e && e.message) ? e.message : String(e);
                page.loading = false;
                page.loadingMore = false;
            });
    }

    function search() {
        page.query = page.input.trim().replace(/\s+/g, " ");
        page.load(true);
    }
    function clearSearch() {
        page.input = "";
        page.query = "";
        page.load(true);
    }

    function playStation(station) {
        if (page.playing)
            return;
        page.playing = station.stationUuid;
        Daemon.call("play_radio_station", { stationUuid: station.stationUuid })
            .then(() => { page.playing = ""; Playback.toast("Playing " + station.name, "success"); })
            .catch((e) => { page.playing = ""; Playback.toast((e && e.message) ? e.message : String(e), "error"); });
    }
    function openHomepage(station) {
        if (!station.homepage)
            return;
        Daemon.call("open_external", { url: station.homepage })
            .catch((e) => Playback.toast((e && e.message) ? e.message : String(e), "error"));
    }

    function detail(station) {
        var parts = [
            station.countryCode || station.country,
            station.codec,
            (station.bitrate && station.bitrate > 0) ? (station.bitrate + " kbps") : ""
        ].filter(Boolean);
        return parts.join(" · ") || "Live stream";
    }
    function stationTags(station) {
        return (station.tags || "")
            .split(",")
            .map((t) => t.trim())
            .filter(Boolean)
            .slice(0, 3);
    }

    // App draws the Backdrop + paper wash behind the page; no full-bleed paper base here, and the
    // page fills the padded rect App gives it with no outer margins of its own.
    GridView {
        id: grid
        anchors.fill: parent
        clip: true
        reuseItems: true
        cacheBuffer: Math.max(0, Math.round(height * 1.5))
        boundsBehavior: Flickable.StopAtBounds
        topMargin: 0
        bottomMargin: Style.sp(20)
        // The full grid clears while a fresh query resolves, mirroring the Svelte loading branch;
        // paging keeps the current cards and only appends beneath them.
        model: page.loading ? [] : page.stations

        // The same column maths CardGrid uses: fill the width in ~192 px columns.
        cellWidth: Math.floor((width - 1) / Math.max(1, Math.floor(width / Style.sp(48))))
        cellHeight: Style.sp(74)

        // --- hero + search console + state -------------------------------------------------
        header: Item {
            width: grid.width
            implicitHeight: heroCol.implicitHeight + Style.sp(4)

            ColumnLayout {
                id: heroCol
                width: parent.width
                spacing: Style.sp(4)

                PageHero {
                    Layout.fillWidth: true
                    eyebrow: "RADIO"
                    title: "Airwaves"
                    meta: (page.query ? "Results for “" + page.query + "”" : "Top stations")
                        + " · " + page.stations.length + " stations"
                    placeholderIcon: "radio"
                    likeable: false
                    showMore: false
                    primaryLabel: "Top stations"
                    onPrimary: page.clearSearch()
                }

                RowLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(2)
                    Rectangle {
                        Layout.preferredWidth: Style.sp(90)
                        Layout.maximumWidth: Style.sp(120)
                        Layout.preferredHeight: Style.sp(10)
                        radius: Style.radius
                        color: Tokens.paperLift
                        border.width: 1
                        border.color: searchField.activeFocus ? Tokens.lineStrong : Tokens.line
                        RowLayout {
                            anchors.fill: parent
                            anchors.leftMargin: Style.sp(3)
                            anchors.rightMargin: Style.sp(3)
                            spacing: Style.sp(2)
                            Icon { name: "search"; size: Style.fs.md; color: Tokens.inkMuted }
                            TextInput {
                                id: searchField
                                Layout.fillWidth: true
                                verticalAlignment: TextInput.AlignVCenter
                                clip: true
                                color: Tokens.ink
                                font.family: Style.fontUi
                                font.pixelSize: Style.fs.md
                                text: page.input
                                onTextChanged: page.input = text
                                onAccepted: page.search()
                                Text {
                                    anchors.verticalCenter: parent.verticalCenter
                                    visible: searchField.text.length === 0
                                    text: "Search stations by name…"
                                    color: Tokens.inkFaint
                                    font: searchField.font
                                }
                            }
                        }
                    }
                    Pill { label: "Search"; icon: "search"; primary: true; enabled: !page.loading; onClicked: page.search() }
                    Item { Layout.fillWidth: true }
                }

                Hairline { Layout.fillWidth: true }

                // Loading / error / empty state, matching the Svelte branches. Flows under the
                // console so the hero and search stay put while a query resolves.
                ColumnLayout {
                    Layout.fillWidth: true
                    Layout.topMargin: Style.sp(4)
                    spacing: Style.sp(2)
                    visible: page.loading || page.stations.length === 0

                    Text {
                        text: page.loading ? "TUNING"
                            : (page.errorMsg ? "SIGNAL LOST" : "NO MATCH")
                        color: Tokens.inkFaint
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        font.letterSpacing: Style.trackMicro
                    }
                    Text {
                        text: page.loading ? "Finding live stations."
                            : (page.errorMsg ? "Radio directory unavailable." : "No stations found.")
                        color: Tokens.inkDim
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        font.weight: Font.DemiBold
                    }
                    Text {
                        Layout.fillWidth: true
                        Layout.maximumWidth: Style.sp(140)
                        text: page.loading ? "Trying available Radio Browser mirrors without blocking the player."
                            : (page.errorMsg ? page.errorMsg : "Try a shorter station name or return to the popular directory.")
                        color: Tokens.inkMuted
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.sm
                        wrapMode: Text.WordWrap
                    }
                    Pill {
                        visible: !page.loading && (page.errorMsg !== "" || page.query !== "")
                        label: page.errorMsg ? "Try again" : "Top stations"
                        onClicked: page.errorMsg ? page.load(true) : page.clearSearch()
                    }
                }
            }
        }

        // --- pager --------------------------------------------------------------------------
        footer: Item {
            width: grid.width
            implicitHeight: (!page.loading && page.stations.length > 0) ? Style.sp(20) : 0
            visible: !page.loading && page.stations.length > 0
            ColumnLayout {
                anchors.centerIn: parent
                spacing: Style.sp(1)
                Pill {
                    Layout.alignment: Qt.AlignHCenter
                    visible: page.hasMore
                    label: page.loadingMore ? "Finding more…" : "Load more stations"
                    enabled: !page.loadingMore
                    onClicked: page.load(false)
                }
                Text {
                    Layout.alignment: Qt.AlignHCenter
                    visible: !page.hasMore
                    text: "END OF THIS SIGNAL SET"
                    color: Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
                }
            }
        }

        // --- station card: art 96, name md, tags micro (tracked) ---------------------------
        delegate: Item {
            id: cell
            required property var modelData
            width: grid.cellWidth
            height: grid.cellHeight

            Rectangle {
                anchors.fill: parent
                anchors.margins: Style.sp(1.5)
                radius: Style.radiusCard
                color: Tokens.paperLift
                border.width: 1
                border.color: cardHover.hovered ? Tokens.lineStrong : Tokens.line

                ColumnLayout {
                    anchors.fill: parent
                    anchors.margins: Style.sp(3)
                    spacing: Style.sp(2)

                    // favicon over the neutral plate, with a LIVE badge and a radio fallback.
                    Item {
                        id: artBox
                        Layout.preferredWidth: Style.sp(24)
                        Layout.preferredHeight: Style.sp(24)
                        Layout.alignment: Qt.AlignLeft

                        Artwork {
                            anchors.fill: parent
                            url: cell.modelData.favicon || ""
                            px: Style.sp(24)
                            placeholderIcon: "radio"
                        }
                        Rectangle {
                            anchors.left: parent.left
                            anchors.bottom: parent.bottom
                            anchors.margins: Style.sp(1)
                            implicitWidth: liveTag.implicitWidth + Style.sp(2)
                            implicitHeight: liveTag.implicitHeight + Style.sp(1)
                            radius: Style.sp(1)
                            color: Qt.rgba(Tokens.paper.r, Tokens.paper.g, Tokens.paper.b, 0.82)
                            Text {
                                id: liveTag
                                anchors.centerIn: parent
                                text: "LIVE"
                                color: Tokens.sun
                                font.family: Style.fontMono
                                font.pixelSize: Style.fs.micro
                                font.letterSpacing: Style.trackMicro
                            }
                        }
                    }

                    Text {
                        Layout.fillWidth: true
                        text: cell.modelData.name
                        color: Tokens.ink
                        font.family: Style.fontUi
                        font.pixelSize: Style.fs.md
                        font.weight: Font.DemiBold
                        elide: Text.ElideRight
                        maximumLineCount: 2
                        wrapMode: Text.WordWrap
                    }
                    Text {
                        Layout.fillWidth: true
                        text: page.detail(cell.modelData)
                        color: Tokens.inkMuted
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        elide: Text.ElideRight
                    }

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(1)
                        visible: page.stationTags(cell.modelData).length > 0
                        Repeater {
                            model: page.stationTags(cell.modelData)
                            delegate: Rectangle {
                                id: tagChip
                                required property var modelData
                                implicitWidth: tagText.implicitWidth + Style.sp(3)
                                implicitHeight: Style.sp(5)
                                radius: Style.radius
                                color: "transparent"
                                border.width: 1
                                border.color: Tokens.lineSoft
                                Text {
                                    id: tagText
                                    anchors.centerIn: parent
                                    text: tagChip.modelData.toUpperCase()
                                    color: Tokens.inkFaint
                                    font.family: Style.fontMono
                                    font.pixelSize: Style.fs.micro
                                    font.letterSpacing: Style.trackMicro
                                }
                            }
                        }
                        Item { Layout.fillWidth: true }
                    }

                    Item { Layout.fillHeight: true }

                    RowLayout {
                        Layout.fillWidth: true
                        spacing: Style.sp(2)
                        Pill {
                            label: (page.playing === cell.modelData.stationUuid) ? "Tuning…" : "Play"
                            icon: "play"
                            primary: true
                            enabled: !page.playing
                            onClicked: page.playStation(cell.modelData)
                        }
                        Pill {
                            visible: !!cell.modelData.homepage
                            label: "Site"
                            icon: "link"
                            onClicked: page.openHomepage(cell.modelData)
                        }
                        Item { Layout.fillWidth: true }
                    }
                }
                HoverHandler { id: cardHover }
            }
        }
    }
}
