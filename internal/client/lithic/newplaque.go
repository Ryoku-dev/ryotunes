package lithic

import (
	"image"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
)

// NewPlaque is the rail's dedicated create control.
type NewPlaque struct {
	Click
	press, hover, focus Tween
}

// Layout renders the expanded or compact create plaque.
func (p *NewPlaque) Layout(gtx layout.Context, th *Theme, compact, open bool) layout.Dimensions {
	w, h, iconSize, cut := gtx.Dp(62), gtx.Dp(52), gtx.Dp(21), gtx.Dp(th.Cut.Mid)
	if compact {
		w, h, iconSize, cut = gtx.Dp(48), gtx.Dp(40), gtx.Dp(20), gtx.Dp(th.Cut.Small)
	}
	finalCut := cut
	dims := p.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.LabelOp("New").Add(gtx.Ops)
		p.press.Duration = th.Motion.Press
		p.press.Ease = EaseChisel
		if p.Pressed() && gtx.Source.Enabled() {
			p.press.Set(gtx, 1)
		} else {
			p.press.Set(gtx, 0)
		}
		pressed := p.press.Value(gtx)
		finalCut = cut + int(float32(gtx.Dp(4))*pressed+0.5)
		sink := int(float32(gtx.Dp(1))*pressed + 0.5)
		r := image.Rectangle{Max: constrainMax(gtx, image.Pt(w, h))}
		hover := hoverValue(gtx, th, &p.hover, p.Hovered())
		ink := drawButtonSurface(gtx, th, r, finalCut, Gold, hover)
		icon := Icon("add")
		if open {
			icon = Icon("close")
		}
		if compact {
			offset := op.Offset(image.Pt((r.Dx()-iconSize)/2, (r.Dy()-iconSize)/2+sink)).Push(gtx.Ops)
			icon.Layout(exact(gtx, image.Pt(iconSize, iconSize)), ink)
			offset.Pop()
		} else {
			contentGTX := exact(gtx, image.Pt(r.Dx(), r.Dy()))
			offset := op.Offset(image.Pt(0, sink)).Push(gtx.Ops)
			layout.Center.Layout(contentGTX, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return icon.Layout(exact(gtx, image.Pt(iconSize, iconSize)), ink)
					}),
					layout.Rigid(layout.Spacer{Height: 2}.Layout),
					layout.Rigid(rawText(th, th.T.Label, ink, "New")),
				)
			})
			offset.Pop()
		}
		return layout.Dimensions{Size: r.Size()}
	})
	focus := focusValue(gtx, th, &p.focus, gtx.Focused(&p.Click.Clickable))
	FocusRingAlpha(gtx, th, image.Rectangle{Max: dims.Size}, finalCut, focus)
	return dims
}
