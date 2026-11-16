package settings

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/design"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

// harness lays the Settings page (or a part of it) out frame by frame under
// a real input router, as the window does.
type harness struct {
	t      *testing.T
	page   *page
	env    *env.Env
	ops    op.Ops
	router input.Router
	now    time.Time
	size   image.Point
}

func newHarness(t *testing.T, size image.Point) *harness {
	t.Helper()
	// never the owner's preferences
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	anim.SetReduce(false)
	t.Cleanup(func() { anim.SetReduce(false) })
	store := state.New(protocol.Snapshot{Settings: model.SettingsSnapshot{string(model.SettingAutoplay): "true"}})
	e := &env.Env{
		Th:       look.NewTheme(palette.Signature(true), libtheme.Comfortable),
		Overlays: new(look.Overlays),
		Store:    store,
		Go:       func(run func(context.Context)) { run(context.Background()) },
	}
	return &harness{t: t, page: New(env.Route{Kind: env.RouteSettings}).(*page), env: e, now: time.Unix(1000, 0), size: size}
}

func (h *harness) context() layout.Context {
	return layout.Context{
		Ops: &h.ops, Source: h.router.Source(), Now: h.now,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(h.size),
	}
}

// frame lays w out once and hands the frame to the router.
func (h *harness) frame(w layout.Widget) layout.Dimensions {
	h.ops.Reset()
	dims := w(h.context())
	h.router.Frame(&h.ops)
	h.now = h.now.Add(16 * time.Millisecond)
	return dims
}

// pageFrame lays the whole page out.
func (h *harness) pageFrame() layout.Dimensions {
	return h.frame(func(gtx layout.Context) layout.Dimensions { return h.page.Layout(gtx, h.env) })
}

// section lays one section's body out on its own, as the page does.
func (h *harness) section(id string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		index := sectionIndex(id)
		return h.page.sectionBody(index, h.env, h.env.Store.Snapshot())(gtx)
	}
}

func (h *harness) click(at f32.Point) {
	h.router.Queue(
		pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: at},
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: at},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: at},
	)
}

func TestStyleClaimsFollowAChangedDesktop(t *testing.T) {
	h := newHarness(t, image.Pt(900, 2400))
	h.frame(h.section("appearance"))
	claims := h.page.appearance.claims
	want := styleClaims{claimDesign: "Lithic", claimColours: "Ryoku", claimMode: "Dark", claimContrast: "Standard", claimMotion: "Off"}
	if claims != want {
		t.Fatalf("claims on the signature = %q, want %q", claims, want)
	}

	// the owner picks Material on wallpaper colours, light, high contrast in
	// Ryoku Settings; the window takes the new reading (WatchDesktop)
	h.env.Th.SetDesktop(palette.Desktop{
		Design: design.Material, Colours: palette.Wallpaper, Mode: palette.Follow,
		Look:  palette.Look{Contrast: palette.High, Tint: palette.TintMedium},
		Gloss: true, Palette: palette.Palette{Dark: false}, Roles: palette.Signature(false),
	})
	h.frame(h.section("appearance"))
	claims = h.page.appearance.claims
	want = styleClaims{claimDesign: "Material", claimColours: "Wallpaper", claimMode: "Light", claimContrast: "High", claimMotion: "Off"}
	if claims != want {
		t.Fatalf("claims after the change = %q, want %q (tint is only Lithic's)", claims, want)
	}
	if !h.page.appearance.claimRoll[claimDesign].Rolling() {
		t.Fatal("the design claim does not roll to its new word")
	}
	for range 40 {
		h.frame(h.section("appearance"))
	}
	if h.page.appearance.claimRoll[claimDesign].Rolling() {
		t.Fatal("the design claim still rolls after its 240 ms")
	}

	// back to Lithic on the wallpaper, with motion reduced: the tint claim
	// shows and reduce motion reads on
	h.env.Th.SetDesktop(palette.Desktop{
		Design: design.Lithic, Colours: palette.Wallpaper, Mode: palette.Follow,
		Look: palette.Look{Tint: palette.TintMedium}, ReduceMotion: true,
		Gloss: true, Palette: palette.Palette{Dark: true}, Roles: palette.Signature(true),
	})
	h.frame(h.section("appearance"))
	claims = h.page.appearance.claims
	if claims[claimTint] != "Medium" || claims[claimMotion] != "On" || claims[claimDesign] != "Lithic" {
		t.Fatalf("claims on Lithic wallpaper, reduced = %q", claims)
	}
}

func TestHandOffRunsTheHubLauncherOnTheStylePage(t *testing.T) {
	var opened []string
	launcher := openHub
	openHub = func(_ context.Context, page string) error {
		opened = append(opened, page)
		return nil
	}
	t.Cleanup(func() { openHub = launcher })

	h := newHarness(t, image.Pt(900, 1600))
	claims := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		h.page.rowSection, h.page.rowCursor, h.page.slabStart, h.page.th = sectionIndex("appearance"), 0, 0, h.env.Th
		return h.page.layoutStyleClaims(gtx, h.env)
	}
	dims := h.frame(claims)
	// the hand-off is the slab's last row; press its words, away from the
	// plaque: the whole row hands off, as the Hub's hand-off rows do
	h.click(f32.Pt(60, float32(dims.Size.Y-21-14)))
	h.frame(claims)
	h.frame(claims)
	if len(opened) != 1 || opened[0] != "style" {
		t.Fatalf("hand-off opened %q, want [style]", opened)
	}
}

func TestOpenHubRunsRyokuShellHubOpen(t *testing.T) {
	bin := t.TempDir()
	record := filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\nprintf '%s ' \"$@\" > " + record + "\n"
	if err := os.WriteFile(filepath.Join(bin, "ryoku-shell"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	// no user manager here, so the launch runs directly
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if err := openHub(context.Background(), stylePage); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "hub open style" {
		t.Fatalf("launcher ran with %q, want \"hub open style\"", got)
	}
}

func TestOutlineGlidesToTheSectionAndHoldsIt(t *testing.T) {
	h := newHarness(t, image.Pt(1000, 700))
	h.page.SetOutline(true)
	for range 30 {
		h.pageFrame()
	}
	if h.page.sectionChips.Labels != nil {
		t.Fatal("the page shows its own chips while the sidebar shows the outline")
	}
	appearance := sectionIndex("appearance")
	h.page.GoTo(appearance)
	if _, current := h.page.Outline(); current != appearance {
		t.Fatalf("outline current = %d right after GoTo, want %d", current, appearance)
	}
	h.pageFrame()
	if !h.page.scroll.Gliding() {
		t.Fatal("GoTo jumped instead of gliding")
	}
	for range 60 {
		h.pageFrame()
	}
	if h.page.scroll.Gliding() {
		t.Fatal("the glide never landed")
	}
	if first, offset := h.page.scroll.Offset(); first != appearance+1 || offset != 0 {
		t.Fatalf("landed at item %d offset %d, want %d at 0", first, offset, appearance+1)
	}
	names, current := h.page.Outline()
	number, part, _, _ := h.page.Masthead()
	if current != appearance || number != "05.8" || part != "Appearance" || names[current] != "Appearance" {
		t.Fatalf("outline %q current %d, masthead %q %q", names, current, number, part)
	}

	// without the outline (the medium class), the page carries the chips
	h.page.SetOutline(false)
	h.pageFrame()
	if len(h.page.sectionChips.Labels) != len(sections) || h.page.sectionChips.Selected != appearance {
		t.Fatalf("chips %v selected %d", h.page.sectionChips.Labels, h.page.sectionChips.Selected)
	}
}

func TestSettledSettingsAskForNoFrame(t *testing.T) {
	h := newHarness(t, image.Pt(1000, 700))
	h.page.SetOutline(true)
	h.pageFrame()
	// the first sections rise in as the page opens, so it asks for frames
	if _, ok := h.router.WakeupTime(); !ok {
		t.Fatal("the page did not rise in on its first open")
	}
	for range 120 {
		h.pageFrame()
	}
	// the router keeps a wakeup until it is read; read the rising frames'
	h.router.WakeupTime()
	h.pageFrame()
	if at, ok := h.router.WakeupTime(); ok {
		t.Fatalf("a settled Settings page asks for a frame at %v", at)
	}
}

func TestToggleRowTogglesFromAnywhereOnTheRow(t *testing.T) {
	h := newHarness(t, image.Pt(700, 400))
	var written []string
	value := true
	row := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		h.page.rowSection, h.page.rowCursor, h.page.slabStart = 0, 0, 0
		return h.page.toggleRow(h.env.Th, &h.page.playback.autoplay, value, false, rowSpec{
			title: "Autoplay", detail: "Continue with related music when the queue reaches its end.",
		}, func(on bool) {
			value = on
			written = append(written, boolString(on))
		})(gtx)
	}
	h.frame(row)
	h.click(f32.Pt(40, 20))
	h.frame(row)
	h.frame(row)
	if len(written) != 1 || written[0] != "false" || value {
		t.Fatalf("row press wrote %q, value %v", written, value)
	}
}

func TestChangedCountsSettingsAwayFromTheirDefaults(t *testing.T) {
	p := New(env.Route{Kind: env.RouteSettings}).(*page)
	off := false
	snapshot := state.State{
		Settings: model.SettingsSnapshot{
			string(model.SettingAutoplay):   "false",
			string(model.SettingQuality):    string(model.QualityHigh),
			string(model.SettingDiscordRPC): "true",
		},
		Client: state.ClientPreferences{Accent: state.AccentSource, WordByWord: &off},
	}
	if got := p.changedCount(snapshot); got != 4 {
		t.Fatalf("changedCount = %d, want 4 (autoplay, discord, accent, word by word)", got)
	}
	// a write on its way counts as written
	p.pendingSettings[model.SettingAutoplay] = "true"
	if got := p.changedCount(snapshot); got != 3 {
		t.Fatalf("changedCount with autoplay pending = %d, want 3", got)
	}
}

func TestSectionsAreNumberedUnderSettings(t *testing.T) {
	if len(sections) != 12 {
		t.Fatalf("%d sections", len(sections))
	}
	for index, section := range sections {
		want := "05." + intString(index+1)
		if section.number != want {
			t.Fatalf("%s numbered %q, want %q", section.id, section.number, want)
		}
		if sectionIndex(section.id) != index {
			t.Fatalf("sectionIndex(%q) = %d", section.id, sectionIndex(section.id))
		}
	}
	if sections[sectionIndex("sound")].gloss != "音" {
		t.Fatal("sound takes the Hub's word")
	}
}

func TestRowLeansOnHoverAndSettles(t *testing.T) {
	h := newHarness(t, image.Pt(700, 400))
	row := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		h.page.rowSection, h.page.rowCursor, h.page.slabStart = 0, 0, 0
		return h.page.settingRow(h.env.Th, "Destination", "Where downloads go.", nil)(gtx)
	}
	h.frame(row)
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(100, 20)})
	for range 20 {
		h.frame(row)
	}
	leaning := h.page.rows[0][0]
	if leaning.wash.Lean(h.context()) != 2 {
		t.Fatalf("a hovered row leans %d px, want 2", leaning.wash.Lean(h.context()))
	}
	h.router.Queue(pointer.Event{Kind: pointer.Leave, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(100, 380)})
	for range 30 {
		h.frame(row)
	}
	h.router.WakeupTime()
	h.frame(row)
	if leaning.wash.Lean(h.context()) != 0 {
		t.Fatal("the row still leans after the pointer left")
	}
	if at, ok := h.router.WakeupTime(); ok {
		t.Fatalf("a row at rest asks for a frame at %v", at)
	}
}

func TestMoreOptionsUnfoldsItsRows(t *testing.T) {
	h := newHarness(t, image.Pt(700, 600))
	h.page.streamClients = []model.StreamClient{model.StreamClientWebRemix, model.StreamClientTVHTML5}
	var dims layout.Dimensions
	frame := func() {
		dims = h.frame(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{}
			h.page.rowSection, h.page.rowCursor, h.page.slabStart, h.page.th = 0, 0, 0, h.env.Th
			return h.page.playback.more.layout(gtx, h.env.Th, "More options", func(gtx layout.Context) layout.Dimensions {
				return stackRows(gtx, h.page.streamClientRows(h.env, h.env.Store.Snapshot()))
			})
		})
	}
	frame()
	closed := dims.Size.Y
	if len(h.page.rows[0]) != 0 {
		t.Fatal("the fold made its rows before it opened")
	}
	h.click(f32.Pt(60, float32(closed-10)))
	frame()
	frame()
	if !h.page.playback.more.fold.Open || len(h.page.rows[0]) == 0 {
		t.Fatal("pressing More options did not open the fold")
	}
	partway := dims.Size.Y
	for range 30 {
		frame()
	}
	open := dims.Size.Y
	if !(closed < partway && partway < open) {
		t.Fatalf("fold heights closed %d, partway %d, open %d: it should unfold, not jump", closed, partway, open)
	}
	if h.page.playback.more.chevron.Angle(h.context(), h.env.Th, true) != 90 {
		t.Fatal("the chevron has not turned")
	}
}

func TestResetOffersUndoThatPutsTheValueBack(t *testing.T) {
	h := newHarness(t, image.Pt(700, 400))
	var notice string
	var undo func()
	h.env.Undoable = func(message string, run func()) { notice, undo = message, run }
	preferences := h.env.Store.Snapshot().Client
	preferences.Accent = state.AccentSource
	h.env.Store.SetClientPreferences(preferences)
	h.page.env = h.env

	// the row's reset, as the reset plaque runs it
	h.page.rowSection, h.page.rowCursor, h.page.slabStart = 0, 0, 0
	h.page.row(h.env.Th, rowSpec{
		title: "Accent colour", changed: true,
		reset: func() {
			next := h.env.Store.Snapshot().Client
			next.Accent = ""
			saveClientPreferences(h.env, next)
		},
	})
	reset := h.page.rows[0][0].spec.reset
	reset()
	if got := h.env.Store.Snapshot().Client.AccentMode(); got != state.AccentArt {
		t.Fatalf("after reset the accent is %q, want the default", got)
	}
	if notice != "Accent colour is back to its default." || undo == nil {
		t.Fatalf("notice %q", notice)
	}
	// another row changes after the reset; Undo leaves it as it was set
	next := h.env.Store.Snapshot().Client
	next.Density = "compact"
	h.env.Store.SetClientPreferences(next)
	h.page.pendingSettings[model.SettingAutoplay] = "false"
	undo()
	now := h.env.Store.Snapshot().Client
	if now.AccentMode() != state.AccentSource {
		t.Fatalf("after undo the accent is %q, want source", now.AccentMode())
	}
	if now.Density != "compact" || h.page.pendingSettings[model.SettingAutoplay] != "false" {
		t.Fatalf("undo reverted a later change: density %q, autoplay %q", now.Density, h.page.pendingSettings[model.SettingAutoplay])
	}

	// the same row changed again after a reset: Undo leaves the new choice
	reset()
	next = h.env.Store.Snapshot().Client
	next.Accent = state.AccentDesktop
	h.env.Store.SetClientPreferences(next)
	undo()
	if got := h.env.Store.Snapshot().Client.AccentMode(); got != state.AccentDesktop {
		t.Fatalf("undo overwrote a later choice: accent %q", got)
	}
}
