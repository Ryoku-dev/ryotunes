pragma Singleton
import QtQuick
import Quickshell
import Quickshell.Io
import Ryoku.Ui.Singletons

// The skin: every colour, type face, radius and duration the chrome reads, resolved from ONE
// source the user picks (Prefs.skin):
//
//   "system"   Ryoku's live theme through Tokens (the wallpaper palette or the named scheme,
//              shell.json's font and decor) - the default on a Ryoku desktop.
//   <id>       a skin directory `<dir>/<id>/skin.json`, looked up in order:
//                $RYOTUNES_SKIN_DIRS (colon list, dev)   > ~/.config/ryotunes/skins  > <shellDir>/../skins
//              so a user skin shadows a shipped one of the same id. `matugen` is just a skin
//              whose file matugen writes (matugen/ryotunes.json is the template).
//
// A skin file is watched: editing it re-applies live, which is how skins get authored. Colours
// are applied the way the old light/dark pins were - as a Material scheme on Tokens.namedScheme
// - so every surface that reads Tokens re-renders at once with no per-file plumbing. Type, shape,
// motion, decor, accent policy and wash come straight from here through Style. docs/SKINS.md is
// the format reference; skins/skin.schema.json the schema.
Singleton {
    id: root

    // --- paths -------------------------------------------------------------------------------
    readonly property string configHome: Quickshell.env("XDG_CONFIG_HOME") || (Quickshell.env("HOME") + "/.config")
    readonly property string dataHome: Quickshell.env("XDG_DATA_HOME") || (Quickshell.env("HOME") + "/.local/share")
    readonly property string userDir: configHome + "/ryotunes/skins"
    // What RyoStore installs (category `ryotunes-skins`, one folder per product with a receipt).
    readonly property string storeDir: dataHome + "/ryoku/ryotunes-skins"
    readonly property string shippedDir: Quickshell.shellDir + "/../skins"
    readonly property var devDirs: (Quickshell.env("RYOTUNES_SKIN_DIRS") || "").split(":").filter((d) => d.length > 0)
    // Search order; first hit per id wins: a user's own copy shadows a store install, which
    // shadows a shipped skin of the same id.
    readonly property var dirs: root.devDirs.concat([root.userDir, root.storeDir, root.shippedDir])

    // RyoStore on this box (Ryoku): Settings offers "Get more skins" only then.
    property bool storeAvailable: false
    Process {
        command: ["sh", "-c", "command -v ryostore >/dev/null 2>&1 && echo yes || echo no"]
        running: true
        stdout: StdioCollector { onStreamFinished: root.storeAvailable = this.text.trim() === "yes" }
    }

    // --- selection ---------------------------------------------------------------------------
    // RYOTUNES_SKIN pins a skin for a preview run (scripts/dev/skin-preview.sh); Prefs otherwise.
    readonly property string forced: Quickshell.env("RYOTUNES_SKIN") || ""
    readonly property string id: root.forced || Prefs.skin || "system"
    readonly property bool followSystem: root.id === "system"

    // --- catalogue ---------------------------------------------------------------------------
    // [{ id, name, author, description, version, source: "shipped"|"store"|"user"|"dev", generated,
    //    dir, path, manifest }] in search order, one entry per id.
    property var all: []
    property string error: ""     // last load/parse problem, for Settings
    function byId(id) {
        for (var i = 0; i < root.all.length; i++)
            if (root.all[i].id === id)
                return root.all[i];
        return null;
    }
    readonly property var entry: root.byId(root.id)
    readonly property var paper: root.byId("paper")

    // The built-in fallback: the paper skin's values, so the chrome renders even with no skins
    // directory at all (a bare checkout run with `qs -p client`).
    readonly property var fallback: ({
        id: "paper", name: "Paper", "default": "dark",
        modes: {
            dark: { paper: "#050505", paperLift: "#0d0d0c", ink: "#d7cfc6", inkDim: "#b6aea5",
                    bone: "#d7cfc6", inkOnBone: "#090807", sun: "#e2342a", alert: "#d33b32" },
            light: { paper: "#c8c4bc", paperLift: "#d5d0c7", ink: "#211f1c", inkDim: "#403b35",
                     bone: "#292620", inkOnBone: "#eee8de", sun: "#e2342a", alert: "#d33b32" }
        },
        type: { display: "Fraunces", ui: "Space Grotesk", mono: "SpaceMono Nerd Font" },
        shape: { radius: 8, radiusCard: 12 },
        motion: { snap: 90, move: 170, swap: 210, slow: 200 },
        decor: "rich", accent: "artwork", wash: 1.0, fonts: []
    })
    readonly property var paperManifest: (root.paper && root.paper.manifest) ? root.paper.manifest : root.fallback
    // The active manifest: the chosen skin, or paper for "system" (its modes are what Light/Dark
    // pin to while following the desktop).
    readonly property var manifest: (root.entry && root.entry.manifest) ? root.entry.manifest : root.paperManifest

    // --- mode --------------------------------------------------------------------------------
    // Prefs.themeMode "light"/"dark" pins a mode; "system" means the skin's own default, or, for
    // the system skin, whatever the desktop is (no pin at all).
    // RYOTUNES_SKIN_MODE pins the mode for a preview run (so a light-first skin renders light
    // whatever the user's own pin is); Prefs otherwise.
    readonly property string wantMode: Quickshell.env("RYOTUNES_SKIN_MODE") || Prefs.themeMode || "system"
    readonly property string mode: root.wantMode !== "system" ? root.wantMode
        : (root.followSystem ? "system" : (root.manifest["default"] || "dark"))
    readonly property bool unpinned: root.followSystem && root.mode === "system"
    // The skin lacks the pinned mode: its other mode is used and Settings says so.
    readonly property bool modeMissing: !root.unpinned && !(root.manifest.modes && root.manifest.modes[root.mode])
    function palette(m, source) {
        var modes = (source && source.modes) ? source.modes : {};
        if (modes[m]) return modes[m];
        return modes[m === "dark" ? "light" : "dark"] || root.fallback.modes.dark;
    }
    // The palette painted right now: the skin's mode, or Tokens' live roles while unpinned.
    readonly property var colors: {
        if (root.unpinned) {
            return { paper: Tokens.paper, paperLift: Tokens.paperLift, ink: Tokens.ink, inkDim: Tokens.inkDim,
                     bone: Tokens.bone, inkOnBone: Tokens.inkOnBone, sun: Tokens.sun, alert: root.fallback.modes.dark.alert };
        }
        var p = root.palette(root.mode, root.manifest);
        return Object.assign({}, root.fallback.modes[root.mode === "light" ? "light" : "dark"], p);
    }

    // --- the rest of the skin (type, shape, motion, decor, accent, wash) ----------------------
    function pick(section, key, base) {
        var s = root.manifest[section];
        if (s && s[key] !== undefined && s[key] !== null && s[key] !== "") return s[key];
        var f = root.fallback[section];
        return (f && f[key] !== undefined) ? f[key] : base;
    }
    // Type is the skin's, never the desktop's: shell.json's fontFamily is the bar's face (often a
    // mono Nerd font) and would put the whole app in it. The system skin keeps Paper's type.
    readonly property var type: ({ display: root.pick("type", "display"), ui: root.pick("type", "ui"), mono: root.pick("type", "mono") })
    readonly property var shape: ({ radius: root.pick("shape", "radius"), radiusCard: root.pick("shape", "radiusCard") })
    // Durations pass through Tokens' motion gate (reduce motion, the motion scale) like Ryoku's own.
    readonly property var motion: {
        var r = Tokens.reduceMotion, s = Tokens.motionScale;  // bind
        function d(ms) { return r ? 0 : Math.round(ms * s); }
        if (root.followSystem)
            return { snap: Tokens.snap, move: Tokens.move, swap: Tokens.swap, slow: Tokens.durDefaultEffects };
        return { snap: d(root.pick("motion", "snap")), move: d(root.pick("motion", "move")),
                 swap: d(root.pick("motion", "swap")), slow: d(root.pick("motion", "slow")) };
    }
    readonly property string decor: root.followSystem ? Tokens.decor : (root.manifest.decor || root.fallback.decor)
    // "artwork" (the playing cover's colour), "provider" (the catalogue's brand colour), "sun"
    // (the skin's own primary), or a fixed "#rrggbb".
    readonly property string accent: root.manifest.accent || root.fallback.accent
    // A multiplier on the cover wash behind pages and the stage; 0 keeps the paper flat.
    readonly property real wash: (typeof root.manifest.wash === "number") ? Math.max(0, Math.min(2, root.manifest.wash)) : 1.0

    // --- applying colours through Tokens -------------------------------------------------------
    // The same Material scheme the old light/dark pins wrote; `__skin` marks it as ours so an
    // overwrite by Tokens.refreshNamed (a shell.json edit) is recognised and re-applied.
    function scheme(p) {
        return {
            __skin: root.id + "/" + root.mode,
            surface: p.paper,
            surfaceContainerLow: p.paperLift,
            onSurface: p.ink,
            onSurfaceVariant: p.inkDim,
            inverseSurface: p.bone,
            inverseOnSurface: p.inkOnBone,
            primary: p.sun
        };
    }
    function apply() {
        if (root.unpinned) {
            if (Tokens.namedScheme && Tokens.namedScheme.__skin)
                Tokens.refreshNamed();
            return;
        }
        var want = root.scheme(root.palette(root.mode, root.manifest));
        var have = Tokens.namedScheme;
        if (have && have.__skin === want.__skin && have.surface === want.surface && have.onSurface === want.onSurface
            && have.primary === want.primary && have.surfaceContainerLow === want.surfaceContainerLow)
            return;
        Tokens.namedScheme = want;
    }
    onIdChanged: root.apply()
    onModeChanged: root.apply()
    onManifestChanged: root.apply()
    Connections {
        target: Tokens
        // shell.json changed: Tokens re-read its named palette over ours - put the skin back.
        function onNamedSchemeChanged(): void { if (!root.unpinned) root.apply(); }
    }

    // --- discovery ---------------------------------------------------------------------------
    // One shell pass lists every <dir>/<id>/skin.json in search order and streams each file
    // between a record separator (path) and a unit separator (contents); no jq, no python.
    function reload() { lister.running = true; }
    Process {
        id: lister
        command: ["sh", "-c",
            'for d in "$@"; do for f in "$d"/*/skin.json; do [ -f "$f" ] || continue; printf "\\036%s\\037" "$f"; cat "$f"; done; done',
            "sh"].concat(root.dirs)
        stdout: StdioCollector { onStreamFinished: root.ingest(this.text) }
    }
    function ingest(text) {
        var out = [], seen = {}, err = "";
        var recs = text.split("\u001e");
        for (var i = 1; i < recs.length; i++) {
            var cut = recs[i].indexOf("\u001f");
            if (cut < 0) continue;
            var path = recs[i].slice(0, cut);
            var body = recs[i].slice(cut + 1);
            var dir = path.slice(0, path.lastIndexOf("/"));
            var dirId = dir.slice(dir.lastIndexOf("/") + 1);
            var m = null;
            try { m = JSON.parse(body); } catch (e) { err = path + ": " + e; continue; }
            if (!m || typeof m !== "object" || !m.modes) { err = path + ": not a skin (no modes)"; continue; }
            var id = (typeof m.id === "string" && m.id.length) ? m.id : dirId;
            if (id !== dirId) { err = path + ": id \"" + id + "\" does not match its folder \"" + dirId + "\""; continue; }
            if (seen[id]) continue;   // an earlier (higher-precedence) dir already provided it
            seen[id] = true;
            var source = root.devDirs.some((d) => path.indexOf(d + "/") === 0) ? "dev"
                : (path.indexOf(root.userDir + "/") === 0 ? "user"
                : (path.indexOf(root.storeDir + "/") === 0 ? "store" : "shipped"));
            out.push({
                id: id, name: m.name || id, author: m.author || "", description: m.description || "",
                version: m.version || "", source: source, generated: m.generated || "",
                dir: dir, path: path, manifest: m
            });
        }
        root.all = out;
        root.error = err;
        if (!root.followSystem && !root.byId(root.id))
            root.error = "skin \"" + root.id + "\" is not installed; showing Paper";
    }

    // The active skin's file, watched: a save in an editor re-applies it live.
    FileView {
        id: activeFile
        path: (root.entry && root.entry.path) ? root.entry.path : ""
        watchChanges: true
        printErrors: false
        onFileChanged: reload()
        onLoaded: {
            if (!root.entry) return;
            try {
                var m = JSON.parse(activeFile.text());
                if (!m || !m.modes) return;
                var next = root.all.slice();
                for (var i = 0; i < next.length; i++)
                    if (next[i].id === root.entry.id) {
                        next[i] = Object.assign({}, next[i], { manifest: m, name: m.name || next[i].id,
                            author: m.author || "", description: m.description || "", version: m.version || "" });
                        break;
                    }
                root.all = next;
                root.error = "";
            } catch (e) {
                root.error = root.entry.path + ": " + e;
            }
        }
    }

    // A skin's bundled faces (`"fonts": ["fonts/Foo.ttf"]`, paths relative to its folder).
    Instantiator {
        model: (root.entry && root.manifest.fonts) ? root.manifest.fonts : []
        delegate: FontLoader {
            id: face
            required property var modelData
            source: "file://" + root.entry.dir + "/" + face.modelData
        }
    }

    // The store's library and the user dir are watched as directories: a RyoStore install or
    // remove, or a folder dropped in by hand, rescans without a manual Reload. FileView on a
    // directory reports changes to its entry list; the debounce folds a multi-file install into
    // one scan.
    FileView { path: root.storeDir; watchChanges: true; printErrors: false; onFileChanged: rescan.restart() }
    FileView { path: root.userDir; watchChanges: true; printErrors: false; onFileChanged: rescan.restart() }
    Timer { id: rescan; interval: 400; onTriggered: root.reload() }

    // ~/.config/ryotunes/skins exists from the first run, so "Open skins folder" and a matugen
    // template have somewhere to land.
    Process {
        id: mkuser
        command: ["mkdir", "-p", root.userDir]
        running: true
        onExited: (code, status) => root.reload()
    }

    // Fork the painted palette into a new user skin the author can edit live.
    // Returns the manifest path; Settings opens it.
    function forkCurrent(newId) {
        var m = JSON.parse(JSON.stringify(root.manifest));
        m.id = newId; m.name = newId; m.author = Quickshell.env("USER") || ""; m.version = "0.1.0";
        m.description = "Forked from " + (root.followSystem ? "the desktop theme" : root.manifest.name || root.id);
        delete m.generated; delete m["$schema"];
        m.format = 1;
        var painted = root.colors;
        var modeKey = root.unpinned ? (Tokens.light ? "light" : "dark") : root.mode;
        m.modes = m.modes || {};
        m.modes[modeKey] = {
            paper: String(painted.paper), paperLift: String(painted.paperLift), ink: String(painted.ink),
            inkDim: String(painted.inkDim), bone: String(painted.bone), inkOnBone: String(painted.inkOnBone),
            sun: String(painted.sun), alert: String(painted.alert)
        };
        m["default"] = modeKey;
        m.type = root.type; m.shape = root.shape;
        var dir = root.userDir + "/" + newId;
        forkWriter.dir = dir;
        forkWriter.body = JSON.stringify(m, null, 2) + "\n";
        forkWriter.running = true;
        return dir + "/skin.json";
    }
    Process {
        id: forkWriter
        property string dir
        property string body
        command: ["sh", "-c", 'mkdir -p "$1" && printf "%s" "$2" > "$1/skin.json"', "sh", forkWriter.dir, forkWriter.body]
        onExited: (code, status) => root.reload()
    }
}
