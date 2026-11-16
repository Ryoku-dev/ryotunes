package look

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryotunes/internal/model"
)

// metric is the harness's scale, one pixel a dp.
func (h *motionHarness) metric() unit.Metric { return unit.Metric{PxPerDp: 1, PxPerSp: 1} }

// run draws frames at 60 Hz for d, as a window does while something moves:
// a chase closes its gap frame by frame, never more than 100 ms at a time.
func (h *motionHarness) run(d time.Duration, draw func(gtx layout.Context)) {
	for passed := time.Duration(0); passed < d; passed += 16 * time.Millisecond {
		h.advance(16 * time.Millisecond)
		h.frame(draw)
	}
}

func motionEqualizer() model.Equalizer {
	return model.Equalizer{Enabled: true, Bands: []model.EQBand{
		{Type: model.EQPeak, Frequency: 100, Gain: 3, Q: 1, Enabled: true},
		{Type: model.EQPeak, Frequency: 2000, Gain: -2, Q: 1, Enabled: true},
	}}
}

func TestEqualizerGraphHoverGrowsTheNodeAndSettles(t *testing.T) {
	h := newMotionHarness(t)
	equalizer := motionEqualizer()
	var graph EqualizerGraph
	draw := func(gtx layout.Context) { graph.Layout(gtx, h.theme, &equalizer, nil, false, 300) }
	h.frame(draw)
	bounds := graph.plotBounds(layout.Context{Metric: h.metric()}, h.size)
	point := graphNodePoint(&equalizer, 1, bounds)
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: point})
	h.frame(draw)
	h.advance(60 * time.Millisecond)
	h.frame(draw)
	if hover := graph.hovers[1].Value(layout.Context{Now: h.now}); hover <= 0 || hover >= 1 {
		t.Fatalf("hover 60 ms in = %.2f, want it on its way (120 ms on OutCubic)", hover)
	}
	if graph.hovers[0].Value(layout.Context{Now: h.now}) != 0 {
		t.Fatal("a node the pointer is not over grew")
	}
	h.settled("hovered graph", draw)
	h.router.Queue(pointer.Event{Kind: pointer.Leave, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(-10, -10)})
	h.frame(draw)
	h.settled("left graph", draw)
	if hover := graph.hovers[1].Value(layout.Context{Now: h.now}); hover != 0 {
		t.Fatalf("hover after leaving = %.2f, want 0", hover)
	}
}

func TestEqualizerGraphHeldNodeStandsAndAKeyStepGlides(t *testing.T) {
	h := newMotionHarness(t)
	h.size = image.Pt(600, 300)
	equalizer := motionEqualizer()
	var graph EqualizerGraph
	draw := func(gtx layout.Context) { graph.Layout(gtx, h.theme, &equalizer, nil, false, 300) }
	h.frame(draw)
	bounds := graph.plotBounds(layout.Context{Metric: h.metric()}, h.size)
	from := graphNodePoint(&equalizer, 0, bounds)
	h.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: from})
	h.frame(draw)
	to := f32.Pt(from.X+200, from.Y+60)
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: to})
	h.frame(draw)
	h.advance(16 * time.Millisecond)
	h.frame(draw)
	if graph.chasing {
		t.Fatal("a held node lagged behind the pointer (R2: it stands under it)")
	}
	h.router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: to})
	h.frame(draw)
	held := equalizer.Bands[0]

	// the keys step the focused graph's node: it glides to the new value
	h.router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	h.frame(draw)
	h.advance(16 * time.Millisecond)
	h.frame(draw)
	if !graph.chasing {
		t.Fatal("a key step jumped the node instead of gliding it")
	}
	if shown := graph.display.Bands[0].Gain; shown <= held.Gain || shown >= equalizer.Bands[0].Gain {
		t.Fatalf("gliding node shows %.2f dB, want between %.2f and %.2f", shown, held.Gain, equalizer.Bands[0].Gain)
	}
	h.router.Queue(key.Event{Name: key.NameUpArrow, State: key.Release})
	h.run(time.Second, draw)
	if graph.chasing {
		t.Fatal("the glide had not landed a second later")
	}
	h.settled("graph after a key step", draw)
}

func TestEqualizerGraphChangesNobodyHoldsStandAtOnce(t *testing.T) {
	h := newMotionHarness(t)
	equalizer := motionEqualizer()
	var graph EqualizerGraph
	draw := func(gtx layout.Context) { graph.Layout(gtx, h.theme, &equalizer, nil, false, 300) }
	h.frame(draw)
	equalizer.Bands[0].Gain = -6
	equalizer.Bands[0].Frequency = 400
	h.frame(draw)
	if graph.chasing {
		t.Fatal("a preset or fader change made the node chase")
	}
	h.settled("graph after an outside change", draw)
}

func TestEqualizerGraphAtRestAllocatesNothing(t *testing.T) {
	h := newMotionHarness(t)
	equalizer := motionEqualizer()
	var graph EqualizerGraph
	draw := func(gtx layout.Context) { graph.Layout(gtx, h.theme, &equalizer, nil, false, 300) }
	for range 3 {
		h.frame(draw)
	}
	gtx := layout.Context{Ops: &h.ops, Now: h.now, Metric: h.metric(), Constraints: layout.Exact(h.size)}
	bounds := graph.plotBounds(gtx, h.size)
	allocs := testing.AllocsPerRun(20, func() {
		h.ops.Reset()
		shown := graph.displayed(gtx, equalizer)
		graph.drawnCurve.get(shown)
		graph.Curve(equalizer)
		graph.hatch(gtx, responseArea(gtx.Ops, graph.points, float32(bounds.Min.Y)+gainY(0, bounds.Dy())), bounds, h.theme.R.Fg)
	})
	if allocs != 0 {
		t.Fatalf("a graph at rest allocated %.0f times a frame for its curve, chase and hatch", allocs)
	}
}

func TestFaderHandleGlidesToACommittedValueAndSettles(t *testing.T) {
	h := newMotionHarness(t)
	h.size = image.Pt(40, 120)
	style := FaderStyle{Min: -12, Max: 12, Step: .5, Height: 120, Label: "Gain"}
	var fader Fader
	value := 0.0
	draw := func(gtx layout.Context) { fader.Layout(gtx, h.theme, value, style) }
	h.frame(draw)
	value = 12
	h.frame(draw)
	h.advance(16 * time.Millisecond)
	h.frame(draw)
	if shown := fader.chase.Value(layout.Context{Now: h.now}); shown <= .5 || shown >= 1 {
		t.Fatalf("handle 16 ms after a change stands at %.3f, want on its way from .5 to 1", shown)
	}
	h.run(time.Second, draw)
	h.settled("fader after a change", draw)
	if shown := fader.chase.Value(layout.Context{Now: h.now}); shown != 1 {
		t.Fatalf("handle landed at %.3f, want 1", shown)
	}
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(20, 60)})
	h.frame(draw)
	h.settled("hovered fader", draw)
	if hover, _ := fader.motion(layout.Context{Now: h.now}, h.theme, false); hover != 1 {
		t.Fatalf("hovered fader stands at %.2f, want 1", hover)
	}
}

func TestValueRollRollsOnAStepAndFollowsAHeldControl(t *testing.T) {
	h := newMotionHarness(t)
	var roll ValueRoll
	value, live := 1.0, false
	draw := func(gtx layout.Context) {
		roll.Layout(gtx, h.theme, h.theme.T.MonoSm, h.theme.R.Fg, value, FormatGain, live)
	}
	h.frame(draw)
	if roll.Rolling() {
		t.Fatal("the first value rolled in")
	}
	value = 2
	h.frame(draw)
	if !roll.Rolling() {
		t.Fatal("a step did not roll the figure")
	}
	h.settled("rolled readout", draw)
	if roll.Rolling() {
		t.Fatal("the roll never landed")
	}
	live = true
	value = 2.5
	h.frame(draw)
	if roll.Rolling() {
		t.Fatal("a held control's readout rolled instead of following")
	}
	h.settled("held readout", draw)
}

func TestHoverReversedMidwayCarriesOnFromWhereItStands(t *testing.T) {
	h := newMotionHarness(t)
	equalizer := motionEqualizer()
	var graph EqualizerGraph
	draw := func(gtx layout.Context) { graph.Layout(gtx, h.theme, &equalizer, nil, false, 300) }
	h.frame(draw)
	bounds := graph.plotBounds(layout.Context{Metric: h.metric()}, h.size)
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: graphNodePoint(&equalizer, 1, bounds)})
	h.frame(draw)
	h.advance(60 * time.Millisecond)
	h.frame(draw)
	in := graph.hovers[1].Value(layout.Context{Now: h.now})
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(float32(bounds.Min.X+2), float32(bounds.Max.Y-2))})
	h.advance(time.Millisecond)
	h.frame(draw)
	if out := graph.hovers[1].Value(layout.Context{Now: h.now}); out > in+.02 || out < in-.1 {
		t.Fatalf("node hover jumped from %.2f to %.2f when the pointer left mid-way", in, out)
	}

	h.size = image.Pt(40, 120)
	style := FaderStyle{Min: -12, Max: 12, Step: .5, Height: 120, Label: "Gain"}
	var fader Fader
	drawFader := func(gtx layout.Context) { fader.Layout(gtx, h.theme, 0, style) }
	h.frame(drawFader)
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 2, Position: f32.Pt(20, 60)})
	h.frame(drawFader)
	h.advance(60 * time.Millisecond)
	h.frame(drawFader)
	in = fader.hover.Value(layout.Context{Now: h.now})
	h.router.Queue(pointer.Event{Kind: pointer.Leave, Source: pointer.Mouse, PointerID: 2, Position: f32.Pt(-5, 60)})
	h.advance(time.Millisecond)
	h.frame(drawFader)
	if out := fader.hover.Value(layout.Context{Now: h.now}); out > in+.02 || out < in-.1 {
		t.Fatalf("fader hover jumped from %.2f to %.2f when the pointer left mid-way", in, out)
	}
}

func TestFaderHandleStandsUnderTheHoldingPointer(t *testing.T) {
	h := newMotionHarness(t)
	h.size = image.Pt(40, 120)
	style := FaderStyle{Min: -12, Max: 12, Step: .5, Height: 120, Label: "Gain"}
	var fader Fader
	value := 0.0
	draw := func(gtx layout.Context) {
		fader.Layout(gtx, h.theme, value, style)
		if preview, held := fader.Dragging(); held {
			value = preview
		}
	}
	h.frame(draw)
	h.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(20, 60)})
	h.frame(draw)
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(20, 10)})
	h.frame(draw)
	want := float32(faderFraction(value, style))
	if value <= 0 || fader.chase.Value(layout.Context{Now: h.now}) != want {
		t.Fatalf("held handle stands at %.3f, want exactly the pointer's %.3f", fader.chase.Value(layout.Context{Now: h.now}), want)
	}
}

func TestHScrollerTwoQuickPagesTravelTwoShelfWidths(t *testing.T) {
	h := newMotionHarness(t)
	h.size = image.Pt(180, 60)
	var strip HScroller
	draw := func(gtx layout.Context) {
		strip.Layout(gtx, h.theme, 30, func(gtx layout.Context, _ int) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(56, 60)}
		})
	}
	h.frame(draw)
	h.frame(func(gtx layout.Context) { strip.Page(gtx, 1); draw(gtx) })
	h.advance(30 * time.Millisecond)
	h.frame(func(gtx layout.Context) { strip.Page(gtx, 1); draw(gtx) })
	h.run(time.Second, draw)
	h.settled("paged shelf", draw)
	if distance := strip.Position.First*56 + strip.Position.Offset; distance != 360 {
		t.Fatalf("two quick pages moved the shelf %d px, want 360", distance)
	}
}
