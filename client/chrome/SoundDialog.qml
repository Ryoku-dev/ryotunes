pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import QtQuick.Effects
import Ryoku.Ui.Singletons
import "../"
import "../components"

// The Sound dialog (spec section 7): a Ryotunes feature ordinary players do not have. A centred
// modal over a snapshot-blurred freeze of the frame behind it (spec 9 — the page is captured once
// with our own visuals hidden, blurred, then a scrim, so no live subtree is ever blurred). Five
// sliders drive the daemon's audio-fx chain (tempo, pitch, reverb, bass, width) with a row of
// presets; every change is applied debounced 80 ms and the daemon echoes the authoritative values
// back through Playback.audioFx. The App hosts this in a Loader { active: app.soundOpen } and wires
// onCloseRequested; it needs nothing else from us.
Item {
    id: root

    anchors.fill: parent
    focus: true
    signal closeRequested()

    // The working copy the sliders edit, seeded from the daemon mirror when the dialog opens. Kept
    // local (not a live binding to Playback.audioFx) so a drag is not fought by the echo it causes.
    property var fx: ({ speed: 1, semitones: 0, reverb: 0, bass: 0, width: 0 })
    // Snapshot state. `snapReady` gates the blur (the frozen frame is drawn once, then blurred);
    // `shown` reveals the card and scrim. Both flip when the capture completes, so the snapshot is
    // the frame alone — but a fallback timer flips `shown` regardless, so a build where the capture
    // never completes still shows a usable dialog (over the unblurred page) instead of nothing.
    property bool snapReady: false
    property bool shown: false

    readonly property var presets: [
        { name: "Flat", fx: { speed: 1, semitones: 0, reverb: 0, bass: 0, width: 0 } },
        { name: "Slowed + Reverb", fx: { speed: 0.85, semitones: 0, reverb: 0.6, bass: 0, width: 0 } },
        { name: "Nightcore", fx: { speed: 1.25, semitones: 2, reverb: 0, bass: 0, width: 0 } },
        { name: "Bass Boost", fx: { speed: 1, semitones: 0, reverb: 0, bass: 8, width: 0 } },
        { name: "Wide", fx: { speed: 1, semitones: 0, reverb: 0, bass: 0, width: 0.8 } }
    ]

    Component.onCompleted: {
        var a = Playback.audioFx;
        root.fx = { speed: a.speed, semitones: a.semitones, reverb: a.reverb, bass: a.bass, width: a.width };
        // Snapshot the frame behind us. Capture the frame-level item (the child of the window's
        // content root), not the root itself — a plain Item renders to a texture, the window root
        // does not — walking up until the next parent would be that root, whatever the nesting.
        var p = root.parent;
        while (p && p.parent && p.parent.parent)
            p = p.parent;
        frozen.sourceItem = p;
        frozen.scheduleUpdate();
        root.forceActiveFocus();
        reveal.start();
    }
    Keys.onEscapePressed: (e) => { root.closeRequested(); e.accepted = true; }

    function clamp(v, lo, hi) { return Math.max(lo, Math.min(hi, v)); }
    // Reassign fx as a whole so the slider bindings (and preset-active state) refresh, then apply
    // after 80 ms of quiet — a drag coalesces into one daemon call instead of one per frame.
    function patch(p) { root.fx = Object.assign({}, root.fx, p); apply.restart(); }
    function setSpeed(v) { root.patch({ speed: root.clamp(Math.round(v / 0.05) * 0.05, 0.25, 2.0) }); }
    function setPitch(v) { root.patch({ semitones: root.clamp(Math.round(v), -12, 12) }); }
    function setReverb(v) { root.patch({ reverb: root.clamp(Math.round(v * 100) / 100, 0, 1) }); }
    function setBass(v) { root.patch({ bass: root.clamp(Math.round(v), -6, 12) }); }
    function setWidth(v) { root.patch({ width: root.clamp(Math.round(v * 100) / 100, 0, 1) }); }
    function applyPreset(p) { root.fx = { speed: p.speed, semitones: p.semitones, reverb: p.reverb, bass: p.bass, width: p.width }; apply.restart(); }
    function presetActive(p) {
        return root.fx.speed === p.speed && root.fx.semitones === p.semitones
            && root.fx.reverb === p.reverb && root.fx.bass === p.bass && root.fx.width === p.width;
    }

    Timer {
        id: apply
        interval: 80
        onTriggered: Playback.setAudioFx(root.fx).catch((e) => Playback.toast((e && e.message) ? e.message : "Sound settings failed", "error"))
    }
    // The capture normally reveals the card; this is the floor so it appears even if the frame's
    // one-shot render never signals completion.
    Timer { id: reveal; interval: 140; onTriggered: root.shown = true }

    // ── the spec-9 snapshot blur ─────────────────────────────────────────────────────────────
    ShaderEffectSource {
        id: frozen
        anchors.fill: parent
        live: false
        recursive: false
        visible: false
        onScheduledUpdateCompleted: { root.snapReady = true; root.shown = true; }
    }
    MultiEffect {
        anchors.fill: parent
        source: frozen
        visible: Style.blurEnabled && root.snapReady
        blurEnabled: true
        blur: 1.0
        blurMax: 32
    }
    Rectangle {
        anchors.fill: parent
        color: "#000000"
        opacity: root.shown ? 0.5 : 0
        Behavior on opacity { NumberAnimation { duration: Style.motion.swap } }
    }
    MouseArea {
        anchors.fill: parent
        enabled: root.shown
        onClicked: root.closeRequested()
    }

    // ── the card ─────────────────────────────────────────────────────────────────────────────
    Rectangle {
        id: card
        anchors.centerIn: parent
        width: Style.sp(120)
        implicitHeight: col.implicitHeight + Style.sp(12)
        height: implicitHeight
        radius: Style.radiusCard
        color: Tokens.paper
        border.width: 1
        border.color: Tokens.line
        visible: root.shown
        opacity: root.shown ? 1 : 0
        scale: root.shown ? 1 : 0.98
        Behavior on opacity { NumberAnimation { duration: Style.motion.swap } }
        Behavior on scale { NumberAnimation { duration: Style.motion.swap; easing.type: Easing.OutCubic } }

        // Swallow clicks so they never reach the dismiss layer behind.
        MouseArea { anchors.fill: parent }

        ColumnLayout {
            id: col
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.margins: Style.sp(6)
            spacing: Style.sp(5)

            // header
            RowLayout {
                Layout.fillWidth: true
                spacing: Style.sp(3)
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: Style.sp(1)
                    Text {
                        text: "SOUND"
                        color: Tokens.inkMuted
                        font.family: Style.fontMono
                        font.pixelSize: Style.fs.micro
                        font.letterSpacing: Style.trackMicro
                    }
                    Text {
                        text: "Sound"
                        color: Tokens.ink
                        font.family: Style.fontDisplay
                        font.pixelSize: Style.fs.xl
                    }
                }
                IconButton {
                    Layout.alignment: Qt.AlignTop
                    icon: "close"
                    iconSize: Style.fs.md
                    onClicked: root.closeRequested()
                }
            }

            // sliders
            ColumnLayout {
                Layout.fillWidth: true
                spacing: Style.sp(4)
                FxRow {
                    label: "Tempo"
                    value: root.fx.speed.toFixed(2) + "\u00D7"
                    from: 0.25; to: 2.0; val: root.fx.speed
                    onMoved: (v) => root.setSpeed(v)
                }
                FxRow {
                    label: "Pitch"
                    value: (root.fx.semitones > 0 ? "+" : "") + root.fx.semitones + " st"
                    from: -12; to: 12; val: root.fx.semitones
                    onMoved: (v) => root.setPitch(v)
                }
                FxRow {
                    label: "Reverb"
                    value: Math.round(root.fx.reverb * 100) + "%"
                    from: 0; to: 1; val: root.fx.reverb
                    onMoved: (v) => root.setReverb(v)
                }
                FxRow {
                    label: "Bass"
                    value: (root.fx.bass > 0 ? "+" : "") + root.fx.bass + " dB"
                    from: -6; to: 12; val: root.fx.bass
                    onMoved: (v) => root.setBass(v)
                }
                FxRow {
                    label: "Width"
                    value: Math.round(root.fx.width * 100) + "%"
                    from: 0; to: 1; val: root.fx.width
                    onMoved: (v) => root.setWidth(v)
                }
            }

            Hairline { Layout.fillWidth: true }

            // presets
            ColumnLayout {
                Layout.fillWidth: true
                spacing: Style.sp(3)
                Text {
                    text: "PRESETS"
                    color: Tokens.inkFaint
                    font.family: Style.fontMono
                    font.pixelSize: Style.fs.micro
                    font.letterSpacing: Style.trackMicro
                }
                Flow {
                    Layout.fillWidth: true
                    spacing: Style.sp(2)
                    Repeater {
                        model: root.presets
                        delegate: Chip {
                            id: chip
                            required property var modelData
                            text: chip.modelData.name
                            active: root.presetActive(chip.modelData.fx)
                            onClicked: root.applyPreset(chip.modelData.fx)
                        }
                    }
                }
            }
        }
    }

    // One slider row: a label, its live value, and the drag control. `moved` carries the raw slider
    // value up; the parent quantizes and clamps it per field.
    component FxRow: RowLayout {
        id: r
        property string label: ""
        property string value: ""
        property real from: 0
        property real to: 1
        property real val: 0
        signal moved(real v)

        Layout.fillWidth: true
        spacing: Style.sp(3)

        Text {
            Layout.preferredWidth: Style.sp(16)
            text: r.label
            color: Tokens.ink
            font.family: Style.fontUi
            font.pixelSize: Style.fs.md
        }
        Slider {
            Layout.fillWidth: true
            from: r.from
            to: r.to
            value: r.val
            onMoved: (v) => r.moved(v)
            onCommitted: (v) => r.moved(v)
        }
        Text {
            Layout.preferredWidth: Style.sp(14)
            horizontalAlignment: Text.AlignRight
            text: r.value
            color: Tokens.inkMuted
            font.family: Style.fontMono
            font.pixelSize: Style.fs.xs
        }
    }
}
