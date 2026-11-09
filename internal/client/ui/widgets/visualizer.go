package widgets

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// Visualizer connects a retained visualizer to playback and spectrum state.
func Visualizer(gtx layout.Context, e *env.Env, v *look.Visualizer, st look.VisualizerStyle) layout.Dimensions {
	if e == nil || e.Th == nil || v == nil {
		return layout.Dimensions{}
	}
	if !e.Fx.Visualizer || e.Store == nil || e.Spectrum == nil {
		return v.Layout(gtx, e.Th, nil, false, st)
	}
	snapshot := e.Store.Snapshot()
	if snapshot.Playback.Now == nil || snapshot.Playback.Paused {
		return v.Layout(gtx, e.Th, nil, false, st)
	}
	e.Spectrum.Want(gtx.Now)
	levels, live := e.Spectrum.Levels(gtx.Now)
	return v.Layout(gtx, e.Th, levels, live, st)
}

func headerLaneMetrics(class look.Class) (bottom, height unit.Dp) {
	if class == look.Compact {
		return 24, 18
	}
	return 40, 32
}

func headerVisualizerStyle(e *env.Env) look.VisualizerStyle {
	alpha := float32(.35)
	if e.Th.R.Dark {
		alpha = .45
	}
	return look.VisualizerStyle{
		Ink:   palette.WithAlpha(e.Th.Src.Ink, alpha),
		Bar:   6,
		Gap:   3,
		Floor: 2,
	}
}

func headerVisualizerLane(gtx layout.Context, e *env.Env, v *look.Visualizer, dimensions layout.Dimensions, height unit.Dp) {
	laneHeight := min(gtx.Dp(height), dimensions.Size.Y)
	if dimensions.Size.X <= 0 || laneHeight <= 0 {
		return
	}
	stack := op.Offset(image.Pt(0, dimensions.Size.Y-laneHeight)).Push(gtx.Ops)
	laneGTX := gtx
	laneGTX.Constraints = layout.Exact(image.Pt(dimensions.Size.X, laneHeight))
	Visualizer(laneGTX, e, v, headerVisualizerStyle(e))
	stack.Pop()
}

func headerField(gtx layout.Context, e *env.Env, v *look.Visualizer, cuts [4]int, body layout.Widget) layout.Dimensions {
	return look.Field(gtx, e.Th, cuts, func(gtx layout.Context) layout.Dimensions {
		record := op.Record(gtx.Ops)
		dimensions := body(gtx)
		content := record.Stop()
		_, laneHeight := headerLaneMetrics(e.Th.Class)
		headerVisualizerLane(gtx, e, v, dimensions, laneHeight)
		content.Add(gtx.Ops)
		return dimensions
	})
}
