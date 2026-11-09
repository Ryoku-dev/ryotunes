package lithic

import (
	"image"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func workingPoints(r image.Rectangle) [4]f32.Point {
	cut := min(r.Dx()/2, r.Dy()/2)
	return [4]f32.Point{
		f32.Pt(float32(r.Min.X+cut), float32(r.Min.Y)),
		f32.Pt(float32(r.Max.X), float32(r.Min.Y)),
		f32.Pt(float32(r.Max.X-cut), float32(r.Max.Y)),
		f32.Pt(float32(r.Min.X), float32(r.Max.Y)),
	}
}

// Working draws three offset flakes for active background work.
func Working(gtx layout.Context, th *Theme, size unit.Dp) layout.Dimensions {
	sz := max(1, gtx.Dp(size))
	flakeWidth := max(2, sz/5)
	flakeHeight := max(2, sz/2)
	gap := max(1, sz/10)
	totalWidth := flakeWidth*3 + gap*2
	x := (sz - totalWidth) / 2
	y := (sz - flakeHeight) / 2
	phase := Phase(gtx, th.Motion.Working)
	if Reduced() {
		phase = 1
	}
	for i := range 3 {
		r := image.Rect(x+i*(flakeWidth+gap), y, x+i*(flakeWidth+gap)+flakeWidth, y+flakeHeight)
		points := workingPoints(r)
		path := pathFromPoints(gtx.Ops, points[:])
		paint.FillShape(gtx.Ops, WithAlpha(th.R.Vein, 0.55), clip.Outline{Path: path}.Op())
		fraction := min(max(phase*3-float32(i), 0), 1)
		if fraction > 0 {
			fillClip := clip.Rect(image.Rect(r.Min.X, r.Min.Y, r.Min.X+int(float32(r.Dx())*fraction+0.5), r.Max.Y)).Push(gtx.Ops)
			paint.FillShape(gtx.Ops, th.R.Lapis, clip.Outline{Path: path}.Op())
			fillClip.Pop()
		}
	}
	return layout.Dimensions{Size: gtx.Constraints.Constrain(image.Pt(sz, sz))}
}
