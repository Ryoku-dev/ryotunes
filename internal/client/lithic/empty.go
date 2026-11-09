package lithic

import (
	"image"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget"
)

// EmptyAction describes one next-step plaque in an empty state.
type EmptyAction struct {
	Button *Button
	Kind   ButtonKind
	Icon   *widget.Icon
	Label  string
}

// Empty returns the centred octagonal empty-state composition.
func Empty(th *Theme, icon *widget.Icon, title, body string, actions ...EmptyAction) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					d := gtx.Dp(72)
					size := gtx.Constraints.Constrain(image.Pt(d, d))
					center := f32.Pt(float32(size.X)/2, float32(size.Y)/2)
					apothem := float32(min(size.X, size.Y))/2 - float32(max(1, gtx.Dp(1)))
					octagon := OctagonPoints(center, max(0, apothem))
					Polyline(gtx.Ops, octagon[:], true, float32(max(1, gtx.Dp(1))), th.R.Vein)
					if icon != nil {
						iconCtx := gtx
						iconCtx.Constraints = layout.Exact(size)
						layout.Center.Layout(iconCtx, func(gtx layout.Context) layout.Dimensions {
							px := min(gtx.Dp(30), min(gtx.Constraints.Max.X, gtx.Constraints.Max.Y))
							gtx.Constraints = layout.Exact(image.Pt(px, px))
							return icon.Layout(gtx, th.R.Fg3)
						})
					}
					return layout.Dimensions{Size: size}
				}),
				layout.Rigid(layout.Spacer{Height: th.Sp.S4}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return Label{Style: th.T.Section, Color: th.R.Fg, MaxLines: 2, Align: text.Middle}.Layout(gtx, th, title)
				}),
				layout.Rigid(layout.Spacer{Height: th.Sp.S2}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(420))
					return Label{Style: th.T.Body, Color: th.R.Fg2, Align: text.Middle}.Layout(gtx, th, body)
				}),
			}
			validActions := 0
			for _, action := range actions {
				if action.Button != nil {
					validActions++
				}
			}
			if validActions > 0 {
				children = append(children,
					layout.Rigid(layout.Spacer{Height: th.Sp.S4}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						buttons := make([]layout.FlexChild, 0, validActions)
						for _, action := range actions {
							if action.Button == nil {
								continue
							}
							buttons = append(buttons, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return action.Button.Layout(gtx, th, action.Kind, action.Icon, action.Label)
							}))
						}
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
					}),
				)
			}
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx, children...)
		})
	}
}
