package lithic

import (
	"image"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// BrandMark returns the Lithic Home navigation mark.
func BrandMark(th *Theme, size unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		requested := gtx.Dp(size)
		dims := gtx.Constraints.Constrain(image.Pt(requested, requested))
		d := min(dims.X, dims.Y)
		if d <= 0 {
			return layout.Dimensions{Size: dims}
		}
		offset := image.Pt((dims.X-d)/2, (dims.Y-d)/2)
		rect := image.Rectangle{Min: offset, Max: offset.Add(image.Pt(d, d))}
		cut := int(float32(d)*.22 + .5)
		FillCut(gtx.Ops, rect, cut, th.R.Sel)

		center := f32.Pt(float32(rect.Min.X)+float32(d)/2, float32(rect.Min.Y)+float32(d)/2)
		apothem := float32(d) * .28
		octagon := OctagonPoints(center, apothem)
		Polyline(gtx.Ops, octagon[:], true, float32(max(1, gtx.Dp(unit.Dp(1.5)))), th.R.OnSel)
		dot := max(1, int(float32(d)*.045))
		cx, cy := int(center.X), int(center.Y)
		paint.FillShape(gtx.Ops, th.R.OnSel, clip.Ellipse(image.Rect(cx-dot, cy-dot, cx+dot+1, cy+dot+1)).Op(gtx.Ops))

		scale := float32(d)
		var pointer clip.Path
		pointer.Begin(gtx.Ops)
		pointer.MoveTo(f32.Pt(center.X+scale*.06, center.Y+scale*.06))
		pointer.LineTo(f32.Pt(center.X+scale*.31, center.Y+scale*.14))
		pointer.LineTo(f32.Pt(center.X+scale*.14, center.Y+scale*.31))
		pointer.Close()
		paint.FillShape(gtx.Ops, th.R.Jasper, clip.Outline{Path: pointer.End()}.Op())
		return layout.Dimensions{Size: dims}
	}
}
