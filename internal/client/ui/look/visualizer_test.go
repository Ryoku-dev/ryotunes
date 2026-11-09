package look

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
)

func TestVisualizerResamplesLinearly(t *testing.T) {
	levels := []float32{0, .5, 1}
	want := []float32{0, .25, .5, .75, 1}
	for index, expected := range want {
		if got := resampleLevel(levels, index, len(want)); !closeEnough(got, expected) {
			t.Fatalf("resampleLevel index %d = %v, want %v", index, got, expected)
		}
	}
	if got := resampleLevel([]float32{-.5, 2}, 1, 2); got != 1 {
		t.Fatalf("clamped resample = %v, want 1", got)
	}
}

func TestVisualizerSettlesWithoutRequestingFrames(t *testing.T) {
	reduced := anim.Reduced()
	anim.SetReduce(false)
	defer anim.SetReduce(reduced)

	roles := palette.Signature(true)
	theme := NewTheme(roles, libtheme.Comfortable)
	style := VisualizerStyle{Ink: roles.Fg, Bar: 6, Gap: 3, Floor: 2}
	levels := []float32{0, .25, .5, .75, 1}
	var visualizer Visualizer
	var operations op.Ops
	var router input.Router
	now := time.Unix(100, 0)
	frame := func(live bool) bool {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(180, 40)),
		}
		visualizer.Layout(gtx, theme, levels, live, style)
		router.Frame(&operations)
		_, wake := router.WakeupTime()
		now = now.Add(time.Second / 60)
		return wake
	}

	if !frame(true) {
		t.Fatal("live visualizer did not request a frame")
	}
	for range 120 {
		frame(true)
	}
	for range 240 {
		frame(false)
	}
	if frame(false) {
		t.Fatal("settled visualizer requested another frame")
	}
	for index, value := range visualizer.values[:visualizer.used] {
		if value != 0 {
			t.Fatalf("bar %d settled at %v, want 0", index, value)
		}
	}
}

func TestPlaybackMotionSchedulesAtSixtyFPS(t *testing.T) {
	reduced := anim.Reduced()
	anim.SetReduce(false)
	defer anim.SetReduce(reduced)

	roles := palette.Signature(true)
	theme := NewTheme(roles, libtheme.Comfortable)
	now := time.Unix(400, 0)
	assertNextFrame := func(t *testing.T, draw func(layout.Context)) {
		t.Helper()
		var operations op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(180, 40)),
		}
		draw(gtx)
		router.Frame(&operations)
		wake, ok := router.WakeupTime()
		if !ok {
			t.Fatal("live motion did not request another frame")
		}
		if delta := wake.Sub(now); delta < 16*time.Millisecond || delta > 18*time.Millisecond {
			t.Fatalf("next frame scheduled after %v, want about 16.7ms", delta)
		}
	}

	t.Run("visualizer", func(t *testing.T) {
		var visualizer Visualizer
		assertNextFrame(t, func(gtx layout.Context) {
			visualizer.Layout(gtx, theme, []float32{0, .5, 1}, true, VisualizerStyle{
				Ink: roles.Fg, Bar: 6, Gap: 3, Floor: 2,
			})
		})
	})
	t.Run("wave", func(t *testing.T) {
		var wave WaveState
		assertNextFrame(t, func(gtx layout.Context) {
			DrawWave(gtx, theme, &wave, image.Rect(0, 0, 180, 24), .5, roles.Fg, roles.Fg3, 4, 3, 20, true)
		})
	})
}
