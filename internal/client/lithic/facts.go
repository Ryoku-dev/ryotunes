package lithic

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// Fact is one label and value pair in a facts table.
type Fact struct {
	Label string
	Value string
	Mono  bool
}

// Facts returns a separated Lithic facts table.
func Facts(th *Theme, facts []Fact) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if len(facts) == 0 {
			return layout.Dimensions{}
		}
		rowH := max(1, gtx.Dp(44))
		width := gtx.Constraints.Max.X
		total := len(facts) * rowH
		size := gtx.Constraints.Constrain(image.Pt(width, total))
		viewport := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
		for i, fact := range facts {
			y := i * rowH
			if i > 0 {
				paint.FillShape(gtx.Ops, th.R.Vein, clip.Rect(image.Rect(0, y, size.X, y+1)).Op())
			}
			stack := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
			rowCtx := gtx
			rowCtx.Constraints = layout.Exact(image.Pt(size.X, rowH))
			value := fact.Value
			if value == "" {
				value = "-"
			}
			valueStyle := th.T.Body
			if fact.Mono {
				valueStyle = th.T.Mono
			}
			layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(rowCtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					labelWidth := min(gtx.Constraints.Max.X, gtx.Dp(140))
					gtx.Constraints.Min.X = labelWidth
					gtx.Constraints.Max.X = labelWidth
					return Text(th, th.T.Body, th.R.Fg3, fact.Label)(gtx)
				}),
				layout.Flexed(1, Text(th, valueStyle, th.R.Fg, value)),
			)
			stack.Pop()
		}
		viewport.Pop()
		return layout.Dimensions{Size: size}
	}
}
