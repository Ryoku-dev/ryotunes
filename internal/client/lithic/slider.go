package lithic

import (
	"image"
	"strconv"

	"gioui.org/f32"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

// Slider is a Lithic blade track with a cut selection handle.
type Slider struct {
	widget.Float
}

func sliderHandlePoints(r image.Rectangle) [4]f32.Point {
	cut := min(r.Dx()/2, r.Dy()/4)
	return [4]f32.Point{
		f32.Pt(float32(r.Min.X+cut), float32(r.Min.Y)),
		f32.Pt(float32(r.Max.X), float32(r.Min.Y)),
		f32.Pt(float32(r.Max.X-cut), float32(r.Max.Y)),
		f32.Pt(float32(r.Min.X), float32(r.Max.Y)),
	}
}

// Layout renders and updates a normalized slider over the given value range.
func (s *Slider) Layout(gtx layout.Context, th *Theme, minValue, maxValue float32) layout.Dimensions {
	if maxValue < minValue {
		minValue, maxValue = maxValue, minValue
	}
	s.Value = min(max(s.Value, 0), 1)
	w := gtx.Constraints.Max.X
	if w <= 0 {
		w = gtx.Dp(160)
	}
	h := gtx.Dp(32)
	size := gtx.Constraints.Constrain(image.Pt(w, h))
	gtx = exact(gtx, size)
	s.Update(gtx)

	trackHeight := max(1, gtx.Dp(6))
	trackY := (size.Y - trackHeight) / 2
	track := image.Rect(0, trackY, size.X, trackY+trackHeight)
	FillCut(gtx.Ops, track, 0, disabledInk(gtx, th.R.Stone3))

	handleW, handleH := max(1, gtx.Dp(8)), max(1, gtx.Dp(20))
	handleX := int(float32(max(0, size.X-handleW))*s.Value + 0.5)
	doneEnd := min(size.X, handleX+handleW/2)
	if doneEnd > 0 {
		Blade(gtx.Ops, image.Rect(0, trackY, doneEnd, trackY+trackHeight), disabledInk(gtx, th.R.Fg2))
	}
	handle := image.Rect(handleX, (size.Y-handleH)/2, handleX+handleW, (size.Y+handleH)/2)
	points := sliderHandlePoints(handle)
	paint.FillShape(gtx.Ops, disabledInk(gtx, th.R.Sel), clip.Outline{Path: pathFromPoints(gtx.Ops, points[:])}.Op())

	semantic.DescriptionOp(sliderDescription(minValue, maxValue, s.Value)).Add(gtx.Ops)
	return s.Float.Layout(gtx, layout.Horizontal, 8)
}

func sliderDescription(minValue, maxValue, value float32) string {
	return strconv.FormatFloat(float64(minValue+(maxValue-minValue)*value), 'g', -1, 32)
}
