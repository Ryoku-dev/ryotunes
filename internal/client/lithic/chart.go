package lithic

import (
	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
)

func chartRange(vals []float64, lo, hi float64) (float64, float64) {
	autoLo, autoHi := math.IsNaN(lo), math.IsNaN(hi)
	if autoLo {
		lo = math.Inf(1)
	}
	if autoHi {
		hi = math.Inf(-1)
	}
	for _, value := range vals {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		if autoLo && value < lo {
			lo = value
		}
		if autoHi && value > hi {
			hi = value
		}
	}
	if math.IsInf(lo, 0) {
		lo = 0
	}
	if math.IsInf(hi, 0) {
		hi = 1
	}
	if hi <= lo {
		pad := math.Abs(lo) * .05
		if pad < 1 {
			pad = 1
		}
		lo -= pad
		hi += pad
	}
	return lo, hi
}

func chartPoints(vals []float64, lo, hi float64, size image.Point) []f32.Point {
	lo, hi = chartRange(vals, lo, hi)
	points := make([]f32.Point, 0, len(vals))
	last := max(1, len(vals)-1)
	for i, value := range vals {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		x := float32(i) * float32(max(0, size.X-1)) / float32(last)
		frac := max(0, min(1, (value-lo)/(hi-lo)))
		y := float32(max(0, size.Y-1)) * (1 - float32(frac))
		points = append(points, f32.Pt(x, y))
	}
	return points
}

// EngravedChart draws a line, hatched area, baseline, ticks, and labels.
func EngravedChart(gtx layout.Context, th *Theme, vals []float64, lo, hi float64, height unit.Dp, ticks []string) layout.Dimensions {
	labelH := gtx.Sp(th.T.MonoSm.Line)
	tickH := max(1, gtx.Dp(4))
	requestedPlotH := max(1, gtx.Dp(height))
	extra := tickH + labelH
	size := gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, requestedPlotH+extra))
	plotH := max(1, min(requestedPlotH, size.Y-extra))
	plot := image.Rect(0, 0, size.X, plotH)
	baseY := plot.Max.Y - 1
	paint.FillShape(gtx.Ops, th.R.Vein, clip.Rect(image.Rect(0, baseY, size.X, baseY+1)).Op())

	points := chartPoints(vals, lo, hi, plot.Size())
	if len(points) < 2 {
		labelCtx := gtx
		labelCtx.Constraints = layout.Exact(image.Pt(size.X, plotH))
		layout.Center.Layout(labelCtx, Text(th, th.T.Caption, th.R.Fg3, "Collecting"))
		return layout.Dimensions{Size: size}
	}

	var area clip.Path
	area.Begin(gtx.Ops)
	area.MoveTo(points[0])
	for _, point := range points[1:] {
		area.LineTo(point)
	}
	area.LineTo(f32.Pt(points[len(points)-1].X, float32(baseY)))
	area.LineTo(f32.Pt(points[0].X, float32(baseY)))
	area.Close()
	areaSpec := area.End()
	Hatch(gtx.Ops, areaSpec, plot, max(1, gtx.Dp(6)), max(1, gtx.Dp(1)), WithAlpha(th.R.Fg, .3))
	Polyline(gtx.Ops, points, false, float32(max(1, gtx.Dp(unit.Dp(1.5)))), th.R.Fg)

	for i, label := range ticks {
		x := 0
		if len(ticks) > 1 {
			x = i * max(0, size.X-1) / (len(ticks) - 1)
		}
		paint.FillShape(gtx.Ops, th.R.Vein, clip.Rect(image.Rect(x, baseY, x+1, baseY+tickH)).Op())
		left, right := 0, size.X
		if i > 0 {
			previous := (i - 1) * max(0, size.X-1) / max(1, len(ticks)-1)
			left = (previous + x) / 2
		}
		if i+1 < len(ticks) {
			next := (i + 1) * max(0, size.X-1) / max(1, len(ticks)-1)
			right = (x + next + 1) / 2
		}
		align := text.Middle
		if i == 0 {
			align = text.Start
		} else if i == len(ticks)-1 {
			align = text.End
		}
		stack := op.Offset(image.Pt(left, plotH+tickH)).Push(gtx.Ops)
		labelCtx := gtx
		labelCtx.Constraints = layout.Exact(image.Pt(max(0, right-left), labelH))
		Label{Style: th.T.MonoSm, Color: th.R.Fg3, MaxLines: 1, Align: align}.Layout(labelCtx, th, label)
		stack.Pop()
	}
	return layout.Dimensions{Size: size}
}
