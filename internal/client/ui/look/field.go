package look

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
)

// Field paints the active music field behind body.
func Field(gtx layout.Context, theme *Theme, cuts [4]int, body layout.Widget) layout.Dimensions {
	return FieldAccent(gtx, theme, theme.Accent, cuts, body)
}

// FieldAccent paints a retained local accent without changing the window accent.
func FieldAccent(gtx layout.Context, theme *Theme, accent Accent, cuts [4]int, body layout.Widget) layout.Dimensions {
	record := op.Record(gtx.Ops)
	dimensions := body(gtx)
	call := record.Stop()
	bounds := image.Rectangle{Max: dimensions.Size}
	area := clip.Rect(bounds).Push(gtx.Ops)
	kit.FillCutCorners(gtx.Ops, bounds, cuts[0], cuts[1], cuts[2], cuts[3], accent.Field)
	call.Add(gtx.Ops)
	area.Pop()
	return dimensions
}
