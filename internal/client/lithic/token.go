package lithic

import (
	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func flakeGeometry(width, height int) [4]image.Point {
	cut := min(max(0, height), max(0, width/2))
	return [4]image.Point{
		{X: cut},
		{X: width},
		{X: width - cut, Y: height},
		{Y: height},
	}
}

func flakeFillWidth(width int, frac float64) int {
	return int(math.Round(float64(max(0, width)) * max(0, min(1, frac))))
}

func flakePath(ops *op.Ops, points [4]image.Point) clip.PathSpec {
	var path clip.Path
	path.Begin(ops)
	path.MoveTo(f32.Pt(float32(points[0].X), float32(points[0].Y)))
	for _, point := range points[1:] {
		path.LineTo(f32.Pt(float32(point.X), float32(point.Y)))
	}
	path.Close()
	return path.End()
}

// Flake renders an outlined, mineral, or fractional state token.
func Flake(gtx layout.Context, th *Theme, tone Tone, frac float64, size unit.Dp) layout.Dimensions {
	height := max(1, int(float32(gtx.Dp(size))*.7+.5))
	width := max(1, int(float32(gtx.Dp(size))*1.4+.5))
	dims := layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(width, height))}
	points := flakeGeometry(dims.Size.X, dims.Size.Y)
	if tone == Idle {
		outline := [4]f32.Point{}
		for i, point := range points {
			outline[i] = f32.Pt(float32(point.X), float32(point.Y))
		}
		Polyline(gtx.Ops, outline[:], true, float32(max(1, gtx.Dp(unit.Dp(1.5)))), th.R.Vein)
		return dims
	}
	path := flakePath(gtx.Ops, points)
	shape := clip.Outline{Path: path}.Op()
	stone := th.R.Stone(tone)
	if tone == Bad {
		stone = th.R.Jasper
	}
	if tone == Busy && frac >= 0 && frac < 1 {
		paint.FillShape(gtx.Ops, th.R.Vein, shape)
		filled := flakeFillWidth(dims.Size.X, frac)
		if filled > 0 {
			stack := clip.Rect(image.Rect(0, 0, filled, dims.Size.Y)).Push(gtx.Ops)
			paint.FillShape(gtx.Ops, stone, shape)
			stack.Pop()
		}
		return dims
	}
	paint.FillShape(gtx.Ops, stone, shape)
	return dims
}

// Claim returns the compact state word in its matching mineral ink.
func Claim(th *Theme, tone Tone, word string) layout.Widget {
	ink := th.R.Stone(tone)
	if tone == Idle {
		ink = th.R.Fg2
	}
	return Text(th, th.T.Label, ink, word)
}
