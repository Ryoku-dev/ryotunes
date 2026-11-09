package widgets

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// Visualizer connects a retained visualizer to playback and the selected feed.
func Visualizer(gtx layout.Context, e *env.Env, v *look.Visualizer, st look.VisualizerStyle) layout.Dimensions {
	if e == nil || e.Th == nil || v == nil {
		return layout.Dimensions{}
	}
	kind := e.Fx.Style
	playing := false
	if e.Store != nil {
		snapshot := e.Store.Snapshot()
		playing = e.Fx.Visualizer && snapshot.Playback.Now != nil && !snapshot.Playback.Paused
	}
	switch kind {
	case state.VisualizerOscilloscope, state.VisualizerVectorscope:
		var samples []float32
		live := false
		if e.Scope != nil {
			if playing {
				e.Scope.Want(gtx.Now)
			}
			buffer := v.ScopeBuffer()
			frames, feedLive := e.Scope.Read(buffer)
			samples = buffer[:frames*2]
			live = playing && feedLive
		}
		return v.Layout(gtx, e.Th, nil, samples, live, kind, st)
	default:
		var levels []float32
		live := false
		if e.Spectrum != nil {
			if playing {
				e.Spectrum.Want(gtx.Now)
			}
			levels, live = e.Spectrum.Levels(gtx.Now)
			live = playing && live
		}
		return v.Layout(gtx, e.Th, levels, nil, live, kind, st)
	}
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
		Ink:        palette.WithAlpha(e.Th.Accent.Ink, alpha),
		Bar:        6,
		Gap:        3,
		Floor:      2,
		HeaderLane: true,
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
