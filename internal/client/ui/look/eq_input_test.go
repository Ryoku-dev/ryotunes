package look

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

func TestEqualizerGraphDragKeepsRawBandGainAndKeysRemainFocused(t *testing.T) {
	const width, height = 600, 300
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	equalizer := model.Equalizer{Enabled: true, Preamp: -6, Bands: []model.EQBand{{Type: model.EQPeak, Frequency: 1000, Gain: 3, Q: 1, Enabled: true}}}
	var graph EqualizerGraph
	var operations op.Ops
	var router input.Router
	now := time.Unix(100, 0)
	frame := func() EQGraphResult {
		operations.Reset()
		gtx := layout.Context{Ops: &operations, Source: router.Source(), Now: now, Metric: metric, Constraints: layout.Exact(image.Pt(width, height))}
		_, result := graph.Layout(gtx, theme, &equalizer, nil, false, height)
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
		return result
	}
	frame()
	bounds := graph.plotBounds(layout.Context{Metric: metric}, image.Pt(width, height))
	point := graphNodePoint(&equalizer, 0, bounds)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(point.X+1, point.Y)})
	result := frame()
	if !result.Dragging {
		t.Fatal("graph drag was not reported")
	}
	if equalizer.Bands[0].Gain != 3 {
		t.Fatalf("horizontal drag changed gain to %.1f with preamp %.1f", equalizer.Bands[0].Gain, equalizer.Preamp)
	}
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(point.X+1, point.Y)})
	frame()
	before := equalizer.Bands[0].Frequency
	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Press})
	result = frame()
	if !result.Dragging || equalizer.Bands[0].Frequency <= before {
		t.Fatalf("focused Right arrow did not edit frequency: before %.1f after %.1f", before, equalizer.Bands[0].Frequency)
	}
	router.Queue(key.Event{Name: key.NameRightArrow, State: key.Release})
	if result = frame(); !result.Released {
		t.Fatal("key release did not finish the throttled edit")
	}
}

func TestFaderKeysAndCancelCommit(t *testing.T) {
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	style := FaderStyle{Min: -12, Max: 12, Step: .5, Height: 120, Label: "Gain"}
	var fader Fader
	var operations op.Ops
	var router input.Router
	now := time.Unix(300, 0)
	value := 0.0
	frame := func() {
		operations.Reset()
		gtx := layout.Context{Ops: &operations, Source: router.Source(), Now: now, Metric: metric, Constraints: layout.Exact(image.Pt(40, 120))}
		fader.Layout(gtx, theme, value, style)
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
		if preview, dragging := fader.Dragging(); dragging {
			value = preview
		}
		if committed, ok := fader.Committed(); ok {
			value = committed
		}
	}
	frame()
	point := f32.Pt(20, 60)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 3, Buttons: pointer.ButtonPrimary, Position: point})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Cancel, Source: pointer.Mouse, PointerID: 3, Position: point})
	frame()
	if value != 0 {
		t.Fatalf("cancel committed %.1f, want 0", value)
	}
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 4, Buttons: pointer.ButtonPrimary, Position: point})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 4, Position: point})
	frame()
	router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	frame()
	if value != .5 {
		t.Fatalf("focused Up arrow value = %.1f, want 0.5", value)
	}
	router.Queue(key.Event{Name: key.NameUpArrow, State: key.Release})
	frame()
	if value != .5 {
		t.Fatalf("released key value = %.1f, want 0.5", value)
	}
}

func TestHScrollerHorizontalScrollDoesNotEditFaders(t *testing.T) {
	const (
		cellCount = 12
		cellWidth = 56
		height    = 120
	)
	saved := scrollOnX11
	scrollOnX11 = false
	t.Cleanup(func() { scrollOnX11 = saved })

	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	style := FaderStyle{Min: -12, Max: 12, Step: .5, Height: height, Label: "Gain"}
	var strip HScroller
	var faders [cellCount]Fader
	var gains [cellCount]float64
	var operations op.Ops
	var router input.Router
	now := time.Unix(400, 0)
	frame := func() {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now, Metric: metric,
			Constraints: layout.Exact(image.Pt(180, height)),
		}
		strip.Layout(gtx, theme, cellCount, func(gtx layout.Context, index int) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(cellWidth, height))
			return faders[index].Layout(gtx, theme, gains[index], style)
		})
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
	}

	frame()
	router.Queue(
		pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(20, 60)},
		pointer.Event{
			Kind: pointer.Scroll, Source: pointer.Mouse, PointerID: 1,
			Position: f32.Pt(20, 60), Scroll: f32.Pt(200, 0),
		},
	)
	frame()
	if distance := strip.Position.First*cellWidth + strip.Position.Offset; distance != 200 {
		t.Fatalf("strip moved %d px, want 200", distance)
	}
	for index := range faders {
		if value, ok := faders[index].Committed(); ok {
			t.Fatalf("horizontal strip scroll committed fader %d to %.1f", index, value)
		}
		if gains[index] != 0 {
			t.Fatalf("horizontal strip scroll changed gain %d to %.1f", index, gains[index])
		}
	}
}

func TestHScrollerHorizontalScrollDoesNotEditValueDrags(t *testing.T) {
	const (
		cellCount = 12
		cellWidth = 56
		height    = 120
	)
	saved := scrollOnX11
	scrollOnX11 = false
	t.Cleanup(func() { scrollOnX11 = saved })

	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	var strip HScroller
	var drags [cellCount]ValueDrag
	var operations op.Ops
	var router input.Router
	now := time.Unix(500, 0)
	frame := func() {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now, Metric: metric,
			Constraints: layout.Exact(image.Pt(180, height)),
		}
		strip.Layout(gtx, theme, cellCount, func(gtx layout.Context, index int) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(cellWidth, height))
			return drags[index].Layout(gtx, 1, .1, 10, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		})
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
	}

	frame()
	router.Queue(
		pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 2, Position: f32.Pt(20, 60)},
		pointer.Event{
			Kind: pointer.Scroll, Source: pointer.Mouse, PointerID: 2,
			Position: f32.Pt(20, 60), Scroll: f32.Pt(200, 0),
		},
	)
	frame()
	if distance := strip.Position.First*cellWidth + strip.Position.Offset; distance != 200 {
		t.Fatalf("strip moved %d px, want 200", distance)
	}
	for index := range drags {
		if value, ok := drags[index].Committed(); ok {
			t.Fatalf("horizontal strip scroll committed value drag %d to %.2f", index, value)
		}
	}
}
