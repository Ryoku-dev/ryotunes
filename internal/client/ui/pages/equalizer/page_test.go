package equalizer

import (
	"context"
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
	"github.com/ryoku-dev/ryotunes/internal/protocol"
)

type harness struct {
	t      *testing.T
	page   *Page
	env    *env.Env
	ops    op.Ops
	router input.Router
	now    time.Time
	size   image.Point
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	anim.SetReduce(false)
	t.Cleanup(func() { anim.SetReduce(false) })
	equalizer := model.Equalizer{Enabled: true, Preamp: -2, Bands: []model.EQBand{
		{Type: model.EQPeak, Frequency: 60, Gain: 3, Q: 1, Enabled: true},
		{Type: model.EQPeak, Frequency: 400, Gain: -2, Q: 1.2, Enabled: true},
		{Type: model.EQPeak, Frequency: 3000, Gain: 2, Q: .9, Enabled: true},
	}}
	e := &env.Env{
		Th:       look.NewTheme(palette.Signature(true), libtheme.Comfortable),
		Overlays: new(look.Overlays),
		Store:    state.New(protocol.Snapshot{Equalizer: equalizer}),
		Go:       func(run func(context.Context)) { run(context.Background()) },
	}
	return &harness{t: t, page: New(env.Route{Kind: env.RouteEqualizer}).(*Page), env: e, now: time.Unix(2000, 0), size: image.Pt(1000, 1100)}
}

func (h *harness) frame(w func(gtx layout.Context) layout.Dimensions) layout.Dimensions {
	h.ops.Reset()
	gtx := layout.Context{
		Ops: &h.ops, Source: h.router.Source(), Now: h.now,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(h.size),
	}
	dims := w(gtx)
	h.router.Frame(&h.ops)
	h.now = h.now.Add(16 * time.Millisecond)
	return dims
}

func (h *harness) pageFrame() layout.Dimensions {
	return h.frame(func(gtx layout.Context) layout.Dimensions { return h.page.Layout(gtx, h.env) })
}

func TestEqualizerPageRisesInAndThenDrawsNoFrames(t *testing.T) {
	h := newHarness(t)
	h.pageFrame()
	if _, ok := h.router.WakeupTime(); !ok {
		t.Fatal("the page's first frame asked for no other: its sections should rise in")
	}
	for range 60 {
		h.pageFrame()
	}
	h.now = h.now.Add(2 * time.Second)
	h.pageFrame()
	h.router.WakeupTime()
	h.pageFrame()
	if wake, ok := h.router.WakeupTime(); ok {
		t.Fatalf("an equalizer page at rest still asks for a frame at %v", wake)
	}
}

func TestPressInABandColumnSelectsItsBand(t *testing.T) {
	h := newHarness(t)
	strip := func(gtx layout.Context) layout.Dimensions { return h.page.layoutStripCard(gtx, h.env) }
	h.page.reconcile(h.env)
	h.page.ensureBandControls()
	for range 3 {
		h.frame(strip)
	}
	if selected := h.page.graph.Selected(); selected == 2 {
		t.Fatal("band 2 was selected before the press")
	}
	inset, pitch := int(cardInset), h.page.stripPitch
	x := inset + int(preampColumn) + int(preampGap) + 2*pitch + pitch/2
	// the column's frequency readout, under the fader: no control of its own
	y := inset + 8 + 16 + 6 + int(stripFaderHeight(h.env.Th.Class)) + 6 + 6
	h.router.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(float32(x), float32(y))},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(float32(x), float32(y))},
	)
	h.frame(strip)
	if selected := h.page.graph.Selected(); selected != 2 {
		t.Fatalf("a press in band 2's column selected band %d", selected)
	}
	if h.page.working.Bands[2].Gain != 2 {
		t.Fatalf("selecting the column changed its gain to %.1f", h.page.working.Bands[2].Gain)
	}
}

func TestDialogsSpeakTheHubsDialogLanguage(t *testing.T) {
	h := newHarness(t)
	h.page.saveDialog.Open()
	h.page.deleteDialog.Open("Bright")
	h.page.importDialog.Open()
	h.pageFrame()
	if kind := h.page.saveDialog.save.Kind; kind != kit.PlaqueGold {
		t.Fatalf("Save is plaque kind %d, want gold", kind)
	}
	if kind := h.page.importDialog.importButton.Kind; kind != kit.PlaqueGold {
		t.Fatalf("Import is plaque kind %d, want gold", kind)
	}
	if kind := h.page.deleteDialog.remove.Kind; kind != kit.PlaqueDanger {
		t.Fatalf("Remove is plaque kind %d, want danger", kind)
	}
	for name, cancel := range map[string]kit.PlaqueKind{"save": h.page.saveDialog.cancel.Kind, "remove": h.page.deleteDialog.cancel.Kind, "import": h.page.importDialog.cancel.Kind} {
		if cancel != kit.PlaqueQuiet {
			t.Fatalf("the %s dialog's Cancel is plaque kind %d, want quiet", name, cancel)
		}
	}
	if h.page.saveDialog.dialog.Width != 520 || h.page.deleteDialog.dialog.Width != 520 || h.page.importDialog.dialog.Width != 640 {
		t.Fatal("dialogs are not the Hub's 520 (the profile editor 640)")
	}
}

func TestScrollProgressReadsThePage(t *testing.T) {
	for _, c := range []struct {
		offset, last int
		want         float32
	}{
		{0, 0, -1},
		{0, 10, -1},
		{0, -300, 0},
		{150, -150, .5},
		{300, 0, 1},
	} {
		if got := scrollProgress(c.offset, c.last); got != c.want {
			t.Fatalf("scrollProgress(%d, %d) = %v, want %v", c.offset, c.last, got, c.want)
		}
	}
}

func TestPresetFiguresRollOnceToWhereThePresetLands(t *testing.T) {
	h := newHarness(t)
	for range 30 {
		h.pageFrame()
	}
	target := cloneEqualizer(h.page.working)
	target.Bands[0].Gain = -6
	h.frame(func(gtx layout.Context) layout.Dimensions {
		h.page.animateTo(gtx, h.env, target)
		return layout.Dimensions{}
	})
	h.pageFrame()
	if !h.page.bands[0].gainRoll.Rolling() {
		t.Fatal("the gain readout did not roll toward the preset")
	}
	if got := h.page.shownBand(0).Gain; got != -6 {
		t.Fatalf("readout shows %.1f mid-morph, want the landing -6", got)
	}
}

// edit changes one band's gain as a fader release does.
func (h *harness) edit(index int, gain float64) {
	h.frame(func(gtx layout.Context) layout.Dimensions {
		h.page.working.Bands[index].Gain = gain
		h.page.markEdit(gtx, h.env, false, true)
		return layout.Dimensions{}
	})
}

func TestUndoPutsBackOnlyTheRemovedBandAndAtOnce(t *testing.T) {
	h := newHarness(t)
	var message string
	var undo func()
	h.env.Undoable = func(m string, u func()) { message, undo = m, u }
	for range 30 {
		h.pageFrame()
	}
	removed := h.page.working.Bands[1]
	h.frame(func(gtx layout.Context) layout.Dimensions {
		h.page.removeBand(gtx, h.env, 1)
		return layout.Dimensions{}
	})
	for range 40 {
		h.pageFrame()
	}
	if len(h.page.working.Bands) != 2 || message != "Removed the band at 400 Hz." {
		t.Fatalf("after removal: %d bands, notice %q", len(h.page.working.Bands), message)
	}
	h.edit(0, 5)
	// Undo is pressed with no frame of the page after it
	undo()
	bands := h.page.working.Bands
	if len(bands) != 3 || bands[1] != removed {
		t.Fatalf("undo left bands %+v, want the removed band back at 1", bands)
	}
	if bands[0].Gain != 5 {
		t.Fatalf("undo wiped the later edit: band 0 gain %.1f, want 5", bands[0].Gain)
	}
	if !h.page.pending || h.page.submitted.Bands[1] != removed {
		t.Fatal("undo did not send the restored curve to the daemon")
	}
	if len(h.page.bands) != 3 || h.page.graph.Selected() != 1 {
		t.Fatalf("undo left %d columns with band %d selected, want 3 and the returning band", len(h.page.bands), h.page.graph.Selected())
	}
}

func TestUndoRaisesThePreampBackAtOnce(t *testing.T) {
	h := newHarness(t)
	var undo func()
	h.env.Undoable = func(_ string, u func()) { undo = u }
	h.pageFrame()
	h.frame(func(gtx layout.Context) layout.Dimensions {
		h.page.lowerPreamp(gtx, h.env, 1.5)
		return layout.Dimensions{}
	})
	if h.page.working.Preamp != -3.5 {
		t.Fatalf("lowered preamp = %.1f, want -3.5", h.page.working.Preamp)
	}
	h.edit(2, -4)
	undo()
	if h.page.working.Preamp != -2 || h.page.working.Bands[2].Gain != -4 {
		t.Fatalf("undo left preamp %.1f and band 2 at %.1f, want -2 and the later -4", h.page.working.Preamp, h.page.working.Bands[2].Gain)
	}
}

func TestUndoOfAPresetKeepsBandsEditedSince(t *testing.T) {
	h := newHarness(t)
	var undo func()
	h.env.Undoable = func(_ string, u func()) { undo = u }
	h.pageFrame()
	before := cloneEqualizer(h.page.working)
	target := model.Equalizer{Enabled: true, Preamp: -5, Preset: "Bright", Bands: []model.EQBand{
		{Type: model.EQLowShelf, Frequency: 100, Gain: 1, Q: .7, Enabled: true},
		{Type: model.EQPeak, Frequency: 1000, Gain: 2, Q: 1, Enabled: true},
		{Type: model.EQHighShelf, Frequency: 8000, Gain: 4, Q: .7, Enabled: true},
	}}
	h.frame(func(gtx layout.Context) layout.Dimensions {
		h.page.animateTo(gtx, h.env, target)
		h.page.offerUndo(h.env, "Switched to Bright.", equalizerChange{before: before, after: target, removed: -1})
		return layout.Dimensions{}
	})
	for range 30 {
		h.pageFrame()
	}
	h.edit(2, 6)
	undo()
	got := h.page.working
	if got.Bands[0] != before.Bands[0] || got.Bands[1] != before.Bands[1] || got.Preamp != before.Preamp {
		t.Fatalf("undo left %+v, want the bands and preamp from before the preset", got)
	}
	if got.Bands[2].Gain != 6 || got.Bands[2].Frequency != 8000 {
		t.Fatalf("undo wiped band 2's later edit: %+v", got.Bands[2])
	}
	if got.Preset != "" {
		t.Fatalf("preset name %q after an edit since, want none", got.Preset)
	}
}

func TestRevertChangeDropsAndRestoresTheBandsItMoved(t *testing.T) {
	one := model.EQBand{Type: model.EQPeak, Frequency: 100, Gain: 1, Q: 1, Enabled: true}
	two := model.EQBand{Type: model.EQPeak, Frequency: 1000, Gain: 2, Q: 1, Enabled: true}
	three := model.EQBand{Type: model.EQPeak, Frequency: 5000, Gain: 3, Q: 1, Enabled: true}
	added := equalizerChange{before: model.Equalizer{Bands: []model.EQBand{one}}, after: model.Equalizer{Bands: []model.EQBand{two, three}}, removed: -1}
	if got := revertChange(added.after, added); len(got.Bands) != 1 || got.Bands[0] != one {
		t.Fatalf("undoing a change that added a band left %+v", got.Bands)
	}
	dropped := equalizerChange{before: model.Equalizer{Bands: []model.EQBand{one, two}}, after: model.Equalizer{Bands: []model.EQBand{three}}, removed: -1}
	if got := revertChange(dropped.after, dropped); len(got.Bands) != 2 || got.Bands[0] != one || got.Bands[1] != two {
		t.Fatalf("undoing a change that dropped a band left %+v", got.Bands)
	}
}
