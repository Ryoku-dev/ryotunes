package look

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// the Ryoku mark is a trademark, see NOTICE
const (
	ryokuMarkWidth    = 556
	ryokuMarkHeight   = 482
	ryokuMarkBonePath = "M482 92H317L345 10H288L258 92H176L131 142H242C227 193 205 254 170 316C154 343 138 361 123 372H195C229 332 253 269 276 207L299 142H454Z M405 156H459V385L390 472H321L405 365Z M188 380H114C83 414 47 449 10 472C59 463 108 441 150 414C169 402 182 390 188 380Z"
	ryokuMarkTipPath  = "M499 92H545L523 142H470Z"
)

var ryokuMarkViewBox = f32.Pt(ryokuMarkWidth, ryokuMarkHeight)

// RyokuMark draws the current Ryoku cut-stone mark.
func RyokuMark(size unit.Dp, ink color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		pixels := gtx.Dp(size)
		dimensions := image.Pt(pixels, pixels)
		height := (pixels*ryokuMarkHeight + ryokuMarkWidth/2) / ryokuMarkWidth
		offset := op.Offset(image.Pt(0, (pixels-height)/2)).Push(gtx.Ops)
		bounds := image.Pt(pixels, height)
		bone, _, boneErr := buildSVGPath(gtx.Ops, ryokuMarkBonePath, ryokuMarkViewBox, bounds)
		if boneErr == nil {
			paint.FillShape(gtx.Ops, ink, clip.Outline{Path: bone}.Op())
		}
		tip, _, tipErr := buildSVGPath(gtx.Ops, ryokuMarkTipPath, ryokuMarkViewBox, bounds)
		if tipErr == nil {
			paint.FillShape(gtx.Ops, RyokuOrange, clip.Outline{Path: tip}.Op())
		}
		offset.Pop()
		return layout.Dimensions{Size: dimensions}
	}
}
