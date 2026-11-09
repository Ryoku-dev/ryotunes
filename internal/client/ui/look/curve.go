package look

import (
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"math"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
)

func resizePoints(points []f32.Point, size int) []f32.Point {
	if cap(points) < size {
		return make([]f32.Point, size)
	}
	return points[:size]
}

func resizeFloats(values []float32, size int) []float32 {
	if cap(values) < size {
		return make([]float32, size)
	}
	return values[:size]
}

func monotoneTangents(points []f32.Point, slopes, tangents []float32) {
	if len(points) < 2 {
		return
	}
	for index := range len(points) - 1 {
		width := points[index+1].X - points[index].X
		if width <= 0 {
			slopes[index] = 0
			continue
		}
		slopes[index] = (points[index+1].Y - points[index].Y) / width
	}
	tangents[0] = slopes[0]
	tangents[len(points)-1] = slopes[len(points)-2]
	for index := 1; index < len(points)-1; index++ {
		left, right := slopes[index-1], slopes[index]
		if left == 0 || right == 0 || left*right <= 0 {
			tangents[index] = 0
		} else {
			tangents[index] = 2 * left * right / (left + right)
		}
	}
	for index, slope := range slopes {
		if slope == 0 {
			tangents[index], tangents[index+1] = 0, 0
			continue
		}
		a, b := tangents[index]/slope, tangents[index+1]/slope
		length := a*a + b*b
		if length > 9 {
			scale := 3 / sqrt32(length)
			tangents[index], tangents[index+1] = scale*a*slope, scale*b*slope
		}
	}
}

func sqrt32(value float32) float32 {
	return float32(math.Sqrt(float64(max(0, value))))
}

func appendMonotone(path *clip.Path, points []f32.Point, tangents []float32) {
	for index := range len(points) - 1 {
		left, right := points[index], points[index+1]
		third := (right.X - left.X) / 3
		path.CubeTo(
			f32.Pt(left.X+third, left.Y+tangents[index]*third),
			f32.Pt(right.X-third, right.Y-tangents[index+1]*third),
			right,
		)
	}
}

func (visualizer *Visualizer) drawCurve(gtx layout.Context, style VisualizerStyle) {
	count := visualizer.used
	if count == 0 || style.Ink.A == 0 {
		return
	}
	size := gtx.Constraints.Max
	visible := false
	for _, value := range visualizer.values[:count] {
		visible = visible || value > .01
	}
	if !visible {
		return
	}
	pointCount := count
	if pointCount == 1 {
		pointCount = 2
	}
	visualizer.curvePoints = resizePoints(visualizer.curvePoints, pointCount)
	visualizer.curveSlopes = resizeFloats(visualizer.curveSlopes, pointCount-1)
	visualizer.curveTangents = resizeFloats(visualizer.curveTangents, pointCount)
	for index := range pointCount {
		source := min(index, count-1)
		x := float32(0)
		if pointCount > 1 {
			x = float32(index) * float32(size.X) / float32(pointCount-1)
		}
		height := visualizerDisplayHeight(visualizer.values[source], float32(size.Y), visualizer.kick)
		visualizer.curvePoints[index] = f32.Pt(x, float32(size.Y)-height)
	}
	monotoneTangents(visualizer.curvePoints, visualizer.curveSlopes, visualizer.curveTangents)

	var fill clip.Path
	fill.Begin(gtx.Ops)
	fill.MoveTo(visualizer.curvePoints[0])
	appendMonotone(&fill, visualizer.curvePoints, visualizer.curveTangents)
	fill.LineTo(f32.Pt(float32(size.X), float32(size.Y)))
	fill.LineTo(f32.Pt(0, float32(size.Y)))
	fill.Close()
	paint.FillShape(gtx.Ops, palette.WithAlpha(style.Ink, 1.0/3.0), clip.Outline{Path: fill.End()}.Op())

	var stroke clip.Path
	stroke.Begin(gtx.Ops)
	stroke.MoveTo(visualizer.curvePoints[0])
	appendMonotone(&stroke, visualizer.curvePoints, visualizer.curveTangents)
	width := max(float32(1), (2+visualizer.kick)*gtx.Metric.PxPerDp)
	paint.FillShape(gtx.Ops, style.Ink, clip.Stroke{Path: stroke.End(), Width: width}.Op())
}
