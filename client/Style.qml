pragma Singleton
import QtQuick
import Quickshell
import Quickshell.Io
import Ryoku.Ui.Singletons
import "lib/style.js" as Fns

// The app's own scale, radii, type and motion, on top of Ryoku's Tokens (which supply the Follow
// System colours). Colours come from Tokens for the live theme; `light`/`dark` are the two local
// palettes ported from ui/src/lib/ryotunes.css --ryo-* variables, used when the theme mode pins a
// fixed palette (Task 7). thumb()/fmtTime() are re-exposed from lib/style.js so a QtTest process,
// which cannot load Quickshell, can test the same code.
Singleton {
    id: root

    // Follows whatever scale the active window pins on Tokens (Tokens.uiScaleFor(screen)); 1 until
    // a window sets it. sp(n) is the 4 px grid step scaled to match.
    readonly property real uiScale: Tokens.uiScale
    function sp(n) { return Math.round(n * 4 * root.uiScale); }

    // Geometry (docs/superpowers/specs/2026-09-05-native-client-visual-design.md, section 2).
    readonly property int radius: Math.round(8 * root.uiScale)        // controls
    readonly property int radiusCard: Math.round(12 * root.uiScale)   // cards, hero art
    readonly property int rowH: Math.round(52 * root.uiScale)         // table rows
    readonly property int ctlH: Math.round(36 * root.uiScale)         // buttons, fields
    readonly property int heroArt: Math.round(168 * root.uiScale)     // page hero art, card
    readonly property int cardW: Math.round(168 * root.uiScale)
    readonly property int sidebarW: Math.round(248 * root.uiScale)
    readonly property int panelW: Math.round(340 * root.uiScale)
    readonly property int titleBarH: Math.round(44 * root.uiScale)
    readonly property int playerBarH: Math.round(88 * root.uiScale)
    readonly property int pagePad: Math.round(32 * root.uiScale)

    readonly property string fontUi: "Space Grotesk"
    readonly property string fontMono: "SpaceMono Nerd Font"
    readonly property string fontCjk: "Noto Sans CJK JP"
    readonly property string fontDisplay: Tokens.display

    // Type roles (px at uiScale 1). At most four per screen. micro is the tracked mono label
    // (letterSpacing 1.4); xl/title/hero are Fraunces.
    readonly property var fs: ({
        micro: Math.round(10 * root.uiScale),
        xs: Math.round(11 * root.uiScale),
        sm: Math.round(13 * root.uiScale),
        md: Math.round(15 * root.uiScale),
        lg: Math.round(18 * root.uiScale),
        xl: Math.round(24 * root.uiScale),
        title: Math.round(36 * root.uiScale),
        hero: Math.round(44 * root.uiScale)
    })
    readonly property real trackMicro: 1.4

    // Durations (ms), from Ryoku's motion tokens so reduce-motion and the motion scale reach every
    // animation. snap: hover/press; move: a selector travelling; swap: content exchanging;
    // slow: a panel or page.
    readonly property var motion: ({
        snap: Tokens.snap,
        move: Tokens.move,
        swap: Tokens.swap,
        slow: Tokens.durDefaultEffects
    })

    // --- ambient motion and the power gate -----------------------------------------------------
    // Passive animation (bloom drift, LIVE breath, the spectrum) runs only while something plays,
    // motion is not reduced, and the machine is not in power-saver. The profile is polled from
    // powerprofilesctl every 30 s; a missing tool reads as "not saving".
    property bool powerSaver: false
    readonly property bool ambient: !!Playback.now && !Playback.paused && !Tokens.reduceMotion && !root.powerSaver
    Process {
        id: profileProbe
        command: ["sh", "-c", "command -v powerprofilesctl >/dev/null 2>&1 && powerprofilesctl get || echo balanced"]
        stdout: StdioCollector { onStreamFinished: root.powerSaver = this.text.trim() === "power-saver" }
    }
    Timer { interval: 30000; running: true; repeat: true; triggeredOnStart: true; onTriggered: profileProbe.running = true }

    // The two local palettes, --ryo-* from ryotunes.css. Selected by the theme mode in Task 7; the
    // live Follow System palette is read straight from Tokens.
    readonly property var dark: ({
        paper: "#050505", paperLift: "#0d0d0c", panel: "#090908", card: "#0d0d0c",
        sidebar: "#070706", player: "#080807",
        ink: "#d7cfc6", inkDim: "#b6aea5", inkMuted: "#999188", inkFaint: "#817a72",
        bone: "#d7cfc6", inkOnBone: "#090807", sun: "#e2342a", alert: "#d33b32"
    })
    readonly property var light: ({
        paper: "#c8c4bc", paperLift: "#d5d0c7", panel: "#beb9b0", card: "#d0cbc2",
        sidebar: "#bbb6ad", player: "#c3beb5",
        ink: "#211f1c", inkDim: "#403b35", inkMuted: "#625c53", inkFaint: "#786f65",
        bone: "#292620", inkOnBone: "#eee8de", sun: "#e2342a", alert: "#d33b32"
    })

    // The one MultiEffect blur the Now Playing wash spends. shell.services Perf (Perf.blurDisabled)
    // is not importable under `qs -p client`, so this is the plan's fallback gate: a bool defaulting
    // true that a surface checks before enabling its blur.
    readonly property bool blurEnabled: true

    // --- theme mode -------------------------------------------------------------------------
    // "system": colours follow Tokens (Follow System / named scheme / wallpaper). "light"/"dark":
    // pin Tokens' Material roles to the matching local palette so the whole chrome — every surface
    // that reads Tokens — re-renders at once, with no per-file colour plumbing. scheme() maps the
    // app palette (paper/ink/bone/…) onto the role names Tokens.role() resolves. applyTheme runs on
    // every switch: a fixed palette overwrites namedScheme; "system" re-reads Tokens' file-driven
    // scheme so the chrome returns to the live theme (deterministic both ways — a Binding's
    // restore-on-deactivate did not reliably revert the Quickshell singleton property). Client-local:
    // the daemon has no UI_SETTINGS key for the theme, so the mode is session state.
    property string themeMode: "system"   // "system" | "light" | "dark"

    // --- artwork accent -----------------------------------------------------------------------
    // The one saturated colour the chrome borrows: the playing cover's accent (sampled by
    // components/ArtAccent into Playback.artAccent) while a track plays, the wallpaper's primary
    // otherwise. Progress fills, the live meter, active chips and the mini's glow read this.
    // Sonora's derivation keeps the cover's hue but pins saturation to 0.6-0.85 and the lightness
    // to 0.72 on dark paper / 0.42 on light, so the accent always reads against the surface.
    readonly property bool paperDark: (Tokens.paper.r + Tokens.paper.g + Tokens.paper.b) / 3 < 0.5
    // The provider's own colour, the one clear tell of which catalogue is on: YouTube Music's red,
    // Spotify's green, SoundCloud's orange. It tints the room's light (Backdrop's glow, the mini's
    // glow), the provider pill and the accent's fallback when nothing plays, so switching provider
    // changes the mood of the whole window at once.
    readonly property var providerColors: ({ spotify: "#1db954", soundcloud: "#ff5500", youtube: "#ff2d2d" })
    readonly property color providerColor: root.providerColors[Playback.provider] || root.providerColors.youtube
    readonly property color accent: {
        var c = Playback.artAccent;
        if (!Playback.now || c.a <= 0)
            return root.providerColor;
        return Qt.hsla(c.hslHue < 0 ? 0 : c.hslHue, Math.max(0.6, Math.min(0.85, c.hslSaturation)), root.paperDark ? 0.72 : 0.42, 1);
    }
    readonly property color accentDeep: Qt.hsla(accent.hslHue < 0 ? 0 : accent.hslHue, accent.hslSaturation, root.paperDark ? 0.44 : 0.5, 1)
    readonly property color accentSoft: Qt.rgba(accent.r, accent.g, accent.b, 0.16)

    // --- decor level --------------------------------------------------------------------------
    // Ryoku's calm / rich switch, owned by this client (Prefs.decor) rather than the desktop's
    // hubDecor: Tokens re-reads shell.json on every change and would revert it, so the level is
    // re-applied whenever Tokens moves.
    readonly property bool decorRich: Prefs.decor === "rich"
    function applyDecor() {
        var want = root.decorRich ? "rich" : "calm";
        if (Tokens.decor !== want)
            Tokens.decor = want;
    }
    function applyPrefs() {
        root.themeMode = Prefs.themeMode;
        root.applyDecor();
    }
    Connections {
        target: Tokens
        function onDecorChanged(): void { root.applyDecor(); }
    }
    Connections {
        target: Prefs
        function onDecorChanged(): void { root.applyDecor(); }
        function onThemeModeChanged(): void { root.themeMode = Prefs.themeMode; }
    }

    function scheme(p) {
        return {
            surface: p.paper,
            surfaceContainerLow: p.paperLift,
            onSurface: p.ink,
            onSurfaceVariant: p.inkDim,
            inverseSurface: p.bone,
            inverseOnSurface: p.inkOnBone,
            primary: p.sun
        };
    }

    function applyTheme() {
        if (root.themeMode === "system")
            Tokens.refreshNamed();
        else
            Tokens.namedScheme = root.scheme(root.themeMode === "light" ? root.light : root.dark);
    }
    onThemeModeChanged: {
        root.applyTheme();
        if (Prefs.themeMode !== root.themeMode) {
            Prefs.themeMode = root.themeMode;
            Prefs.save();
        }
    }

    function thumb(url, px) { return Fns.thumb(url, px); }
    function fmtTime(secs) { return Fns.fmtTime(secs); }
    function fmtCount(v) { return Fns.fmtCount(v); }
}
