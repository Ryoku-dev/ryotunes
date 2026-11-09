package look

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
)

const (
	oscilloscopeWindow     = 2048
	oscilloscopeHysteresis = .02
	oscilloscopeFloor      = .01
	oscilloscopeRelease    = 600 * time.Millisecond
	vectorscopeLimit       = 768
)

type oscilloscopeState struct {
	window []float32
	points []f32.Point
	peak   float32
	last   time.Time
}

func monoSample(samples []float32, frame int) float32 {
	return (samples[frame*2] + samples[frame*2+1]) * .5
}

func oscilloscopeTrigger(samples []float32, window int, hysteresis float32) int {
	frames := len(samples) / 2
	if frames <= window {
		return 0
	}
	first := max(1, frames-window*2)
	last := frames - window
	armed := false
	for frame := first; frame <= last; frame++ {
		previous := monoSample(samples, frame-1)
		current := monoSample(samples, frame)
		if previous <= -hysteresis || current <= -hysteresis {
			armed = true
		}
		if armed && previous < hysteresis && current >= hysteresis {
			return frame
		}
	}
	return last
}

func (scope *oscilloscopeState) update(samples []float32, now time.Time, live bool) {
	if !live || len(samples) < 2 {
		return
	}
	frames := len(samples) / 2
	start := oscilloscopeTrigger(samples, oscilloscopeWindow, oscilloscopeHysteresis)
	count := min(oscilloscopeWindow, frames-start)
	scope.window = resizeFloats(scope.window, count)
	peak := float32(0)
	for frame := range count {
		value := monoSample(samples, start+frame)
		scope.window[frame] = value
		peak = max(peak, float32(math.Abs(float64(value))))
	}
	if scope.last.IsZero() || peak >= scope.peak {
		scope.peak = peak
	} else {
		delta := now.Sub(scope.last)
		if delta > 0 {
			released := scope.peak * float32(math.Exp(-float64(delta)/float64(oscilloscopeRelease)))
			scope.peak = max(peak, released)
		}
	}
	scope.last = now
}

func (visualizer *Visualizer) drawOscilloscope(gtx layout.Context, th *Theme, style VisualizerStyle, level float32) {
	if level <= .001 || len(visualizer.osc.window) < 2 || style.Ink.A == 0 {
		return
	}
	size := gtx.Constraints.Max
	centre := float32(size.Y) / 2
	lineHeight := max(1, gtx.Dp(1))
	lineTop := int(math.Round(float64(centre))) - lineHeight/2
	paint.FillShape(gtx.Ops, palette.WithAlpha(th.R.Vein, level), clip.Rect(image.Rect(0, lineTop, size.X, lineTop+lineHeight)).Op())

	pointCount := min(len(visualizer.osc.window), max(2, size.X/2+1))
	visualizer.osc.points = resizePoints(visualizer.osc.points, pointCount)
	peak := max(oscilloscopeFloor, visualizer.osc.peak)
	halfHeight := max(0, float32(size.Y)/2-float32(gtx.Dp(2)))
	for index := range pointCount {
		source := index * (len(visualizer.osc.window) - 1) / (pointCount - 1)
		x := float32(index) * float32(size.X) / float32(pointCount-1)
		y := centre - visualizer.osc.window[source]/peak*halfHeight*level
		visualizer.osc.points[index] = f32.Pt(x, min(max(y, 0), float32(size.Y)))
	}
	var path clip.Path
	path.Begin(gtx.Ops)
	path.MoveTo(visualizer.osc.points[0])
	for _, point := range visualizer.osc.points[1:] {
		path.LineTo(point)
	}
	paint.FillShape(gtx.Ops, style.Ink, clip.Stroke{Path: path.End(), Width: float32(max(1, gtx.Dp(2)))}.Op())
}

type vectorscopeState struct {
	frames [3][]f32.Point
}

func vectorscopePoint(left, right float32) f32.Point {
	const inverseSqrtTwo = float32(.7071067811865476)
	return f32.Pt((left-right)*inverseSqrtTwo, (left+right)*inverseSqrtTwo)
}

func (scope *vectorscopeState) update(samples []float32) {
	frames := len(samples) / 2
	if frames == 0 {
		return
	}
	count := min(vectorscopeLimit, frames)
	scope.frames[2], scope.frames[1], scope.frames[0] = scope.frames[1], scope.frames[0], scope.frames[2]
	scope.frames[0] = resizePoints(scope.frames[0], count)
	for index := range count {
		source := index * (frames - 1) / max(1, count-1)
		scope.frames[0][index] = vectorscopePoint(samples[source*2], samples[source*2+1])
	}
}

func (visualizer *Visualizer) drawVectorscope(gtx layout.Context, th *Theme, style VisualizerStyle, level float32) {
	if level <= .001 || len(visualizer.vector.frames[0]) == 0 || style.Ink.A == 0 {
		return
	}
	size := gtx.Constraints.Max
	square := size.X*2 <= size.Y*3
	centre := f32.Pt(float32(size.X)/2, float32(size.Y)/2)
	radius := float32(min(size.X, size.Y))/2 - float32(gtx.Dp(3))
	if radius <= 0 {
		return
	}

	var frame clip.Path
	frame.Begin(gtx.Ops)
	if square {
		frame.MoveTo(f32.Pt(centre.X, centre.Y-radius))
		frame.LineTo(f32.Pt(centre.X+radius, centre.Y))
		frame.LineTo(f32.Pt(centre.X, centre.Y+radius))
		frame.LineTo(f32.Pt(centre.X-radius, centre.Y))
		frame.Close()
		frame.MoveTo(f32.Pt(centre.X-radius, centre.Y))
		frame.LineTo(f32.Pt(centre.X+radius, centre.Y))
		frame.MoveTo(f32.Pt(centre.X, centre.Y-radius))
		frame.LineTo(f32.Pt(centre.X, centre.Y+radius))
	} else {
		radius = float32(size.Y) - float32(gtx.Dp(3))
		centre.Y = float32(size.Y) - float32(gtx.Dp(1))
		frame.MoveTo(f32.Pt(centre.X-radius, centre.Y))
		frame.LineTo(f32.Pt(centre.X, centre.Y-radius))
		frame.LineTo(f32.Pt(centre.X+radius, centre.Y))
		frame.MoveTo(f32.Pt(centre.X-radius, centre.Y))
		frame.LineTo(f32.Pt(centre.X+radius, centre.Y))
		frame.MoveTo(f32.Pt(centre.X, centre.Y))
		frame.LineTo(f32.Pt(centre.X, centre.Y-radius))
	}
	paint.FillShape(gtx.Ops, palette.WithAlpha(th.R.Vein, level), clip.Stroke{Path: frame.End(), Width: float32(max(1, gtx.Dp(1)))}.Op())

	alphas := [...]float32{1, .45, .18}
	for age := len(visualizer.vector.frames) - 1; age >= 0; age-- {
		visualizer.drawVectorPoints(gtx, visualizer.vector.frames[age], centre, radius, square, level, palette.WithAlpha(style.Ink, alphas[age]))
	}
}

func (visualizer *Visualizer) drawVectorPoints(gtx layout.Context, points []f32.Point, centre f32.Point, radius float32, square bool, level float32, ink color.NRGBA) {
	if len(points) == 0 || ink.A == 0 {
		return
	}
	dot := max(float32(1), float32(gtx.Dp(unit.Dp(1.5))))
	half := dot / 2
	var path clip.Path
	path.Begin(gtx.Ops)
	for _, point := range points {
		x := centre.X + point.X*radius*level
		y := centre.Y - point.Y*radius*level
		if !square {
			y = centre.Y - float32(math.Abs(float64(point.Y)))*radius*level
		}
		path.MoveTo(f32.Pt(x-half, y-half))
		path.LineTo(f32.Pt(x+half, y-half))
		path.LineTo(f32.Pt(x+half, y+half))
		path.LineTo(f32.Pt(x-half, y+half))
		path.Close()
	}
	paint.FillShape(gtx.Ops, palette.WithAlpha(ink, level), clip.Outline{Path: path.End()}.Op())
}
