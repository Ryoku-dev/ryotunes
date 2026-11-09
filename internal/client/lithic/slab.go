package lithic

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// Slab lays content over a flat cut stone plane.
func Slab(gtx layout.Context, ink color.NRGBA, cut unit.Dp, content layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dimensions := layout.UniformInset(21).Layout(gtx, content)
	call := macro.Stop()
	FillCut(gtx.Ops, image.Rectangle{Max: dimensions.Size}, gtx.Dp(cut), ink)
	call.Add(gtx.Ops)
	return dimensions
}
