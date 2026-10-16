pragma ComponentBehavior: Bound
import QtQuick
import Ryoku.Ui.Singletons
import "../"
import "../lib/browse.js" as Browse

// The context menu for anything a browse item can be: a card on a shelf, a row in "Recently played",
// a search result. A song gets the track menu, because that is what it is. An album, artist or
// playlist is not a track, so it gets the three actions that do mean something for it, run through
// the host's own play/open functions rather than a second copy of them.
Item {
    id: root

    // function(item) -> void. The host's own handlers, so a card plays the way a card plays.
    property var playItem: null
    property var openItem: null
    // Shown only when the host can pin things.
    property bool canPin: true

    function openFor(item, sx, sy) {
        if (!item)
            return;
        var p = menu.mapFromItem(null, sx, sy);
        if (item.kind === "song") {
            menu.customItems = null;
            menu.song = Browse.asSong(item);
        } else {
            menu.song = null;
            let out = [];
            if (root.playItem)
                out.push({ icon: "play", label: "Play", danger: false, act: () => root.playItem(item) });
            if (root.openItem)
                out.push({ icon: "arrow-right", label: "Open", danger: false, act: () => root.openItem(item) });
            if (root.canPin)
                out.push({ icon: "dashboard", label: "Add to shortcuts", danger: false,
                    act: () => Playback.toast(Personal.addPick(item) ? "Added to shortcuts" : "Already in shortcuts", "success") });
            menu.customItems = out;
        }
        menu.openAt(p.x, p.y);
    }

    Menu { id: menu }
}
