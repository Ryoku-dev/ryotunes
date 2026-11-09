package lithic

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

const disabledAlpha = 0.38

func exact(gtx layout.Context, size image.Point) layout.Context {
	size = constrainMax(gtx, size)
	gtx.Constraints = layout.Exact(size)
	return gtx
}

func constrainMax(gtx layout.Context, size image.Point) image.Point {
	size.X = min(max(0, size.X), gtx.Constraints.Max.X)
	size.Y = min(max(0, size.Y), gtx.Constraints.Max.Y)
	return size
}

func iconWidget(icon *widget.Icon, size unit.Dp, ink color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if icon == nil {
			return layout.Dimensions{}
		}
		pixels := gtx.Dp(size)
		return icon.Layout(exact(gtx, image.Pt(pixels, pixels)), ink)
	}
}

func lerp(a, b, t float32) float32 { return a + (b-a)*t }

func textMaterial(ops *op.Ops, c color.NRGBA) op.CallOp {
	macro := op.Record(ops)
	paint.ColorOp{Color: c}.Add(ops)
	return macro.Stop()
}
