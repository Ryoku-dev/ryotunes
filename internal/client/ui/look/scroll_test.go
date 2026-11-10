package look

import (
	"image"
	"math"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
)

func closeEnough(first, second float32) bool {
	return math.Abs(float64(first-second)) < .001
}

func TestScrollDeltaWaylandAppliesImmediately(t *testing.T) {
	if got, want := mapScrollDelta(150, 1.5, false), float32(225); !closeEnough(got, want) {
		t.Fatalf("mapScrollDelta(150, 1.5, false) = %v, want %v", got, want)
	}
	if got, want := mapScrollDelta(7.25, 2, false), float32(14.5); !closeEnough(got, want) {
		t.Fatalf("mapScrollDelta(7.25, 2, false) = %v, want %v", got, want)
	}
}

func TestScrollDeltaX11Notch(t *testing.T) {
	if got, want := mapScrollDelta(10, 1.25, true), float32(60); !closeEnough(got, want) {
		t.Fatalf("mapScrollDelta(10, 1.25, true) = %v, want %v", got, want)
	}
	if got, want := mapScrollDelta(-20, 1.25, true), float32(-120); !closeEnough(got, want) {
		t.Fatalf("mapScrollDelta(-20, 1.25, true) = %v, want %v", got, want)
	}
}

func TestScrollRangeOnlyAdvertisesAvailableDirections(t *testing.T) {
	tests := []struct {
		name     string
		position layout.Position
		backward bool
		forward  bool
	}{
		{name: "empty"},
		{name: "start", position: layout.Position{BeforeEnd: true}, forward: true},
		{name: "middle", position: layout.Position{First: 3, BeforeEnd: true}, backward: true, forward: true},
		{name: "offset", position: layout.Position{Offset: 1}, backward: true},
		{name: "end", position: layout.Position{First: 8}, backward: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bounds := verticalScrollRange(test.position)
			if got := bounds.Min < 0; got != test.backward {
				t.Fatalf("backward range = %v, want %v", got, test.backward)
			}
			if got := bounds.Max > 0; got != test.forward {
				t.Fatalf("forward range = %v, want %v", got, test.forward)
			}
		})
	}
}

func TestScrollFractionalAccumulation(t *testing.T) {
	position := layout.Position{BeforeEnd: true}
	fraction := float32(0)
	if applied := addListOffset(&position, &fraction, .6); applied != 0 {
		t.Fatalf("first applied = %d, want 0", applied)
	}
	if applied := addListOffset(&position, &fraction, .6); applied != 1 {
		t.Fatalf("second applied = %d, want 1", applied)
	}
	if position.Offset != 1 || !closeEnough(fraction, .2) {
		t.Fatalf("position = %d, fraction = %v, want 1 and .2", position.Offset, fraction)
	}
}

func TestScrollerUserScrolledOnlyForRealInput(t *testing.T) {
	for _, x11 := range []bool{false, true} {
		name := "wayland"
		if x11 {
			name = "x11"
		}
		t.Run(name, func(t *testing.T) {
			saved := scrollOnX11
			scrollOnX11 = x11
			t.Cleanup(func() { scrollOnX11 = saved })
			testScrollerUserScrolledOnlyForRealInput(t)
		})
	}
}

func testScrollerUserScrolledOnlyForRealInput(t *testing.T) {
	const row = 24
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	var scroller Scroller
	var operations op.Ops
	var router input.Router
	now := time.Unix(200, 0)
	element := func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(120, row))}
	}
	context := func() layout.Context {
		return layout.Context{
			Ops: &operations, Source: router.Source(), Now: now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(120, 96)),
		}
	}
	// the list moves whole rows into First, so compare the distance scrolled
	scrolled := func() int { return scroller.Position.First*row + scroller.Position.Offset }
	frame := func() {
		operations.Reset()
		scroller.Layout(context(), theme, 20, element)
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
	}

	frame()
	if !scroller.Position.BeforeEnd {
		t.Fatalf("initial list position is not scrollable: %+v", scroller.Position)
	}
	router.Queue(
		pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(20, 20)},
		pointer.Event{
			Kind: pointer.Scroll, Source: pointer.Mouse, PointerID: 1,
			Position: f32.Pt(20, 20), Scroll: f32.Pt(0, 10),
		},
	)
	operations.Reset()
	outerGTX := context()
	outerGTX.Constraints = layout.Exact(image.Pt(120, 500))
	scroller.UpdateInput(outerGTX)
	afterInput := scrolled()
	if afterInput <= 0 {
		t.Fatalf("real pointer scroll did not move the list: %+v", scroller.Position)
	}
	if !scroller.UserScrolled() {
		t.Fatal("real pointer scroll did not set UserScrolled")
	}
	if scroller.UserScrolled() {
		t.Fatal("UserScrolled did not clear on read")
	}
	scroller.Layout(context(), theme, 20, element)
	if got := scrolled(); got != afterInput {
		t.Fatalf("Layout consumed input twice: scrolled %d px became %d", afterInput, got)
	}
	router.Frame(&operations)

	scroller.AnchorTo(3, -15)
	if scroller.UserScrolled() {
		t.Fatal("AnchorTo counted as real input")
	}
	operations.Reset()
	now = now.Add(time.Millisecond)
	scroller.Layout(context(), theme, 20, element)
	router.Frame(&operations)
	scroller.SpringBy(20)
	operations.Reset()
	now = now.Add(time.Second / 60)
	scroller.UpdateInput(context())
	if scroller.UserScrolled() {
		t.Fatal("SpringBy counted as real input")
	}
}

func TestScrollerAnchorToNegativeOffset(t *testing.T) {
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	var scroller Scroller
	var operations op.Ops
	gtx := layout.Context{
		Ops: &operations, Now: time.Unix(300, 0),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(120, 100)),
	}
	scroller.AnchorTo(3, -15)
	scroller.Layout(gtx, theme, 10, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(120, 30))}
	})
	if scroller.Position.First != 3 || scroller.Position.Offset != 15 {
		t.Fatalf("position = first %d offset %d, want element 3 at y -15", scroller.Position.First, scroller.Position.Offset)
	}
}

func listDistance(position layout.Position, sizes []int) int {
	distance := position.Offset
	for index := range position.First {
		distance += sizes[index]
	}
	return distance
}

func assertScrollDistance(t *testing.T, got int, want float32) {
	t.Helper()
	if delta := math.Abs(float64(float32(got) - want)); delta > 1 {
		t.Fatalf("scrolled %d px, want %.2f px (delta %.2f)", got, want, delta)
	}
}

func TestScrollerMixedRowsApplyEveryDeltaExactly(t *testing.T) {
	for _, x11 := range []bool{false, true} {
		name := "wayland"
		if x11 {
			name = "x11"
		}
		t.Run(name, func(t *testing.T) {
			saved := scrollOnX11
			scrollOnX11 = x11
			t.Cleanup(func() { scrollOnX11 = saved })

			sizes := []int{24, 300, 40, 300, 56, 180, 72, 260, 44, 220, 60}
			theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
			var scroller Scroller
			var operations op.Ops
			var router input.Router
			now := time.Unix(400, 0)
			context := func() layout.Context {
				return layout.Context{
					Ops: &operations, Source: router.Source(), Now: now,
					Metric:      unit.Metric{PxPerDp: 2.2, PxPerSp: 2.2},
					Constraints: layout.Exact(image.Pt(160, 180)),
				}
			}
			frame := func() {
				operations.Reset()
				scroller.Layout(context(), theme, len(sizes), func(gtx layout.Context, index int) layout.Dimensions {
					return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(160, sizes[index]))}
				})
				router.Frame(&operations)
				now = now.Add(time.Millisecond)
			}
			scroll := func(deltas ...float32) {
				events := make([]event.Event, 0, len(deltas)+1)
				events = append(events, pointer.Event{
					Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(20, 20),
				})
				for _, delta := range deltas {
					events = append(events, pointer.Event{
						Kind: pointer.Scroll, Source: pointer.Mouse, PointerID: 1,
						Position: f32.Pt(20, 20), Scroll: f32.Pt(0, delta),
					})
				}
				router.Queue(events...)
				frame()
			}

			frame()
			var expected float32
			bursts := [][]float32{{150, 80, 25}, {73}, {140}, {19}, {-80, -25}}
			if x11 {
				bursts = [][]float32{{10, 20, 10}, {10}, {20}, {10}, {-10, -20}}
			}
			for _, burst := range bursts {
				scroll(burst...)
				for _, delta := range burst {
					expected += mapScrollDelta(delta, 2.2, x11)
				}
				assertScrollDistance(t, listDistance(scroller.Position, sizes), expected)
			}
		})
	}
}

func TestHScrollerMixedRowsApplyEveryDeltaExactly(t *testing.T) {
	for _, x11 := range []bool{false, true} {
		name := "wayland"
		if x11 {
			name = "x11"
		}
		t.Run(name, func(t *testing.T) {
			saved := scrollOnX11
			scrollOnX11 = x11
			t.Cleanup(func() { scrollOnX11 = saved })

			sizes := []int{24, 300, 40, 300, 56, 180, 72, 260, 44, 220, 60}
			theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
			var scroller HScroller
			var operations op.Ops
			var router input.Router
			now := time.Unix(500, 0)
			context := func() layout.Context {
				return layout.Context{
					Ops: &operations, Source: router.Source(), Now: now,
					Metric:      unit.Metric{PxPerDp: 2.2, PxPerSp: 2.2},
					Constraints: layout.Exact(image.Pt(180, 100)),
				}
			}
			frame := func() {
				operations.Reset()
				scroller.Layout(context(), theme, len(sizes), func(gtx layout.Context, index int) layout.Dimensions {
					return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(sizes[index], 100))}
				})
				router.Frame(&operations)
				now = now.Add(time.Millisecond)
			}
			scroll := func(deltas ...float32) {
				events := make([]event.Event, 0, len(deltas)+1)
				events = append(events, pointer.Event{
					Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(20, 20),
				})
				for _, delta := range deltas {
					events = append(events, pointer.Event{
						Kind: pointer.Scroll, Source: pointer.Mouse, PointerID: 1,
						Position: f32.Pt(20, 20), Scroll: f32.Pt(delta, 0),
					})
				}
				router.Queue(events...)
				frame()
			}

			frame()
			var expected float32
			bursts := [][]float32{{150, 80, 25}, {73}, {140}, {19}, {-80, -25}}
			if x11 {
				bursts = [][]float32{{10, 20, 10}, {10}, {20}, {10}, {-10, -20}}
			}
			for _, burst := range bursts {
				scroll(burst...)
				for _, delta := range burst {
					expected += mapScrollDelta(delta, 2.2, x11)
				}
				assertScrollDistance(t, listDistance(scroller.Position, sizes), expected)
			}
		})
	}
}

func TestScrollerSettlesWithoutIdleFrames(t *testing.T) {
	saved := scrollOnX11
	scrollOnX11 = false
	t.Cleanup(func() { scrollOnX11 = saved })

	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	var scroller Scroller
	var operations op.Ops
	var router input.Router
	now := time.Unix(600, 0)
	context := func() layout.Context {
		return layout.Context{
			Ops: &operations, Source: router.Source(), Now: now,
			Metric:      unit.Metric{PxPerDp: 2.2, PxPerSp: 2.2},
			Constraints: layout.Exact(image.Pt(160, 180)),
		}
	}
	frame := func() {
		operations.Reset()
		scroller.Layout(context(), theme, 20, func(gtx layout.Context, _ int) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(160, 80))}
		})
		router.Frame(&operations)
	}

	frame()
	router.Queue(
		pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(20, 20)},
		pointer.Event{
			Kind: pointer.Scroll, Source: pointer.Mouse, PointerID: 1,
			Position: f32.Pt(20, 20), Scroll: f32.Pt(0, 25),
		},
	)
	now = now.Add(time.Millisecond)
	frame()
	router.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(300, 300),
	})
	now = now.Add(time.Millisecond)
	frame()
	now = now.Add(thumbIdle + theme.Motion.HoverOut + time.Millisecond)
	frame()
	now = now.Add(theme.Motion.HoverOut + time.Millisecond)
	frame()
	router.WakeupTime()
	now = now.Add(time.Millisecond)
	frame()
	if wake, ok := router.WakeupTime(); ok {
		t.Fatalf("settled scroller requested an idle frame at %v", wake)
	}
}
