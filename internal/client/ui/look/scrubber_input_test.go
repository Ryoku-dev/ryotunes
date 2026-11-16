package look

import (
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
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
)

// TestScrubberBubbleLeavesWithACancelledPointer hovers a seek scrubber
// through the real router, then has the pointer cancelled (another handler
// grabs it, or the window loses it), which sends no Leave: the time bubble
// must go, never stand at the track's start reading the cancel's 0, 0.
func TestScrubberBubbleLeavesWithACancelledPointer(t *testing.T) {
	anim.SetReduce(false)
	th := NewTheme(palette.Signature(true), libtheme.Comfortable)
	var (
		scrub  Scrubber
		ops    op.Ops
		router input.Router
	)
	now := time.Unix(4000, 0)
	var bubbles []float32
	style := ScrubberStyle{Bubble: func(fraction float32) string {
		bubbles = append(bubbles, fraction)
		return ""
	}}
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops: &ops, Source: router.Source(), Now: now,
			Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(200, 24)),
		}
		scrub.Layout(gtx, th, .8, style)
		router.Frame(&ops)
		now = now.Add(16 * time.Millisecond)
	}
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(120, 12)})
	for range 20 {
		frame()
	}
	if len(bubbles) == 0 || !closeEnough(bubbles[len(bubbles)-1], .6) {
		t.Fatalf("a hovered scrubber shows its bubble at the pointer, got %v", bubbles)
	}

	router.Queue(pointer.Event{Kind: pointer.Cancel, Source: pointer.Mouse, PointerID: 1})
	bubbles = bubbles[:0]
	for range 30 {
		frame()
	}
	if len(bubbles) != 0 {
		t.Fatalf("a cancelled pointer left the bubble showing at %v", bubbles)
	}
}
