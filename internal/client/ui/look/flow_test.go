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

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
)

func TestFlowChoiceWrapsAndReportsSelectionInReleaseFrame(t *testing.T) {
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	var choices FlowChoice
	choices.Labels = []string{"A", "B", "C"}
	var operations op.Ops
	var router input.Router
	now := time.Unix(100, 0)
	frame := func(width int) (layout.Dimensions, int, bool) {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now, Metric: metric,
			Constraints: layout.Constraints{Max: image.Pt(width, 200)},
		}
		dimensions := choices.Layout(gtx, theme)
		changed, ok := choices.Changed()
		router.Frame(&operations)
		now = now.Add(time.Millisecond)
		return dimensions, changed, ok
	}

	dimensions, _, _ := frame(72)
	if dimensions.Size.Y <= 32 {
		t.Fatalf("narrow choice set height = %d, want wrapped rows", dimensions.Size.Y)
	}

	frame(160)
	point := f32.Pt(55, 16)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: point})
	frame(160)
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: point})
	_, changed, ok := frame(160)
	if !ok || changed != 1 || choices.Selected != 1 {
		t.Fatalf("release frame selection = (%d, %v), selected %d; want (1, true), selected 1", changed, ok, choices.Selected)
	}
}

func TestFlowChoiceSelectionGlidesOverSelectMotion(t *testing.T) {
	theme := NewTheme(palette.Signature(true), libtheme.Comfortable)
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	choices := FlowChoice{Labels: []string{"First", "Second"}}
	var operations op.Ops
	var router input.Router
	now := time.Unix(200, 0)
	frame := func() float32 {
		operations.Reset()
		gtx := layout.Context{
			Ops: &operations, Source: router.Source(), Now: now, Metric: metric,
			Constraints: layout.Constraints{Max: image.Pt(240, 100)},
		}
		choices.Layout(gtx, theme)
		position := choices.selectionX.Value(gtx)
		router.Frame(&operations)
		return position
	}

	frame()
	choices.Selected = 1
	now = now.Add(time.Millisecond)
	start := frame()
	target := float32(choices.positions[1].X)
	if start != 0 || target <= 0 {
		t.Fatalf("selection glide started at %.1f toward %.1f, want 0 toward a positive target", start, target)
	}
	now = now.Add(theme.Motion.Select / 2)
	middle := frame()
	if middle <= start || middle >= target {
		t.Fatalf("selection halfway position = %.1f, want between %.1f and %.1f", middle, start, target)
	}
	now = now.Add(theme.Motion.Select)
	if end := frame(); end != target {
		t.Fatalf("settled selection position = %.1f, want %.1f", end, target)
	}
}
