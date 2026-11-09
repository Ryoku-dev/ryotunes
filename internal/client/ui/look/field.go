package look

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
)

// Field paints the active provider field behind body.
func Field(gtx layout.Context, theme *Theme, cuts [4]int, body layout.Widget) layout.Dimensions {
	record := op.Record(gtx.Ops)
	dimensions := body(gtx)
	call := record.Stop()
	bounds := image.Rectangle{Max: dimensions.Size}
	area := clip.Rect(bounds).Push(gtx.Ops)
	kit.FillCutCorners(gtx.Ops, bounds, cuts[0], cuts[1], cuts[2], cuts[3], theme.Src.Field)
	call.Add(gtx.Ops)
	area.Pop()
	return dimensions
}
