package lithic

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
)

// Skeleton returns static tablet placeholders for a loading list.
func Skeleton(th *Theme, lines int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lines = max(0, lines)
		if lines == 0 {
			return layout.Dimensions{}
		}
		rowH := max(1, gtx.Dp(th.RowHeight()))
		gap := gtx.Dp(4)
		total := lines*rowH + max(0, lines-1)*gap
		size := gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, total))
		viewport := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
		barH := max(1, gtx.Dp(8))
		barGap := gtx.Dp(8)
		x := gtx.Dp(16)
		available := max(0, size.X-2*x)
		for i := range lines {
			y := i * (rowH + gap)
			FillCut(gtx.Ops, image.Rect(0, y, size.X, y+rowH), 0, th.R.Stone2)
			barsH := 2*barH + barGap
			barY := y + max(0, (rowH-barsH)/2)
			first := int(float32(available) * .6)
			second := int(float32(available) * .35)
			FillCut(gtx.Ops, image.Rect(x, barY, x+first, barY+barH), 0, th.R.Stone3)
			FillCut(gtx.Ops, image.Rect(x, barY+barH+barGap, x+second, barY+2*barH+barGap), 0, th.R.Stone3)
		}
		viewport.Pop()
		return layout.Dimensions{Size: size}
	}
}
