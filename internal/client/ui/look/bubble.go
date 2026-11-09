package look

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
)

// Bubble draws deferred mono detail above a local point.
func Bubble(gtx layout.Context, th *Theme, at image.Point, value string, alpha float32) {
	alpha = min(max(alpha, 0), 1)
	if value == "" || alpha <= 0 {
		return
	}
	measure := gtx
	measure.Constraints.Min = image.Point{}
	measure.Constraints.Max = image.Pt(max(0, gtx.Constraints.Max.X), 1<<20)
	record := op.Record(gtx.Ops)
	dimensions := layout.Inset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(measure, func(gtx layout.Context) layout.Dimensions {
		return kit.Label{Style: th.T.Mono, Color: th.R.Fg, MaxLines: 1, Align: text.Middle}.Layout(gtx, th.Theme, value)
	})
	content := record.Stop()

	x := at.X - dimensions.Size.X/2
	maximumX := max(0, gtx.Constraints.Max.X-dimensions.Size.X)
	x = min(max(x, 0), maximumX)
	y := at.Y - gtx.Dp(8) - dimensions.Size.Y
	overlay := op.Record(gtx.Ops)
	opacity := paint.PushOpacity(gtx.Ops, alpha)
	move := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	kit.FillCut(gtx.Ops, image.Rectangle{Max: dimensions.Size}, gtx.Dp(4), th.R.Stone4)
	content.Add(gtx.Ops)
	move.Pop()
	opacity.Pop()
	op.Defer(gtx.Ops, overlay.Stop())
}
