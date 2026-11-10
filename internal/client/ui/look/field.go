package look

import (
	"gioui.org/layout"
)

// Field paints the active music field behind body.
func Field(gtx layout.Context, theme *Theme, cuts [4]int, body layout.Widget) layout.Dimensions {
	return FieldAccent(gtx, theme, theme.Accent, cuts, body)
}

// FieldAccent paints a retained local accent without changing the window accent.
func FieldAccent(gtx layout.Context, theme *Theme, accent Accent, cuts [4]int, body layout.Widget) layout.Dimensions {
	return FieldBackdrop(gtx, theme, accent, cuts, nil, nil, "", body)
}
