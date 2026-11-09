package lithic

import (
	"image"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
)

// SplitButton owns independent main and more plaque segments.
type SplitButton struct {
	Main, More                                 Click
	mainPress, morePress                       Tween
	mainHover, moreHover, mainFocus, moreFocus Tween
}

func splitPress(gtx layout.Context, th *Theme, tween *Tween, pressed bool) float32 {
	tween.Duration = th.Motion.Press
	tween.Ease = EaseChisel
	if pressed && gtx.Source.Enabled() {
		tween.Set(gtx, 1)
	} else {
		tween.Set(gtx, 0)
	}
	return tween.Value(gtx)
}

// Layout renders a shared-outline split plaque.
func (s *SplitButton) Layout(gtx layout.Context, th *Theme, kind ButtonKind, icon *widget.Icon, label string, open bool) layout.Dimensions {
	gtx.Constraints.Min = image.Point{}
	h := gtx.Dp(40)
	gap := gtx.Dp(2)
	moreWidth := gtx.Dp(40)
	baseCut := gtx.Dp(th.Cut.Mid)

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var cut int
			dims := s.Main.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.Button.Add(gtx.Ops)
				semantic.LabelOp(label).Add(gtx.Ops)
				pressed := splitPress(gtx, th, &s.mainPress, s.Main.Pressed())
				cut = baseCut + int(float32(gtx.Dp(4))*pressed+0.5)
				sink := int(float32(gtx.Dp(1))*pressed + 0.5)
				hover := hoverValue(gtx, th, &s.mainHover, s.Main.Hovered())
				palette := interactiveColors(gtx, th, kind, hover)

				contentGTX := gtx
				contentGTX.Constraints.Min = image.Point{}
				contentGTX.Constraints.Max.Y = h
				m := op.Record(gtx.Ops)
				content := layout.Inset{Left: 18, Right: 18}.Layout(contentGTX, func(gtx layout.Context) layout.Dimensions {
					children := make([]layout.FlexChild, 0, 3)
					if icon != nil {
						children = append(children,
							layout.Rigid(iconWidget(icon, 19, palette.ink)),
							layout.Rigid(layout.Spacer{Width: 8}.Layout),
						)
					}
					children = append(children, layout.Rigid(rawText(th, th.T.Label, palette.ink, label)))
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
				})
				call := m.Stop()
				size := constrainMax(gtx, image.Pt(content.Size.X, h))
				r := image.Rectangle{Max: size}
				fillCutCorners(gtx.Ops, r, cut, 0, 0, cut, palette.fill)
				if palette.stroke {
					strokeCutCorners(gtx.Ops, r, cut, 0, 0, cut, max(1, gtx.Dp(1)), palette.outline)
				}
				offset := op.Offset(image.Pt((size.X-content.Size.X)/2, (h-content.Size.Y)/2+sink)).Push(gtx.Ops)
				call.Add(gtx.Ops)
				offset.Pop()
				return layout.Dimensions{Size: size}
			})
			focus := focusValue(gtx, th, &s.mainFocus, gtx.Focused(&s.Main.Clickable))
			if focus > 0 {
				pad := gtx.Dp(2)
				strokeCutCorners(gtx.Ops, image.Rectangle{Max: dims.Size}.Inset(-pad), cut+pad, 0, 0, cut+pad, max(1, gtx.Dp(2)), WithAlpha(th.R.Fg2, focus))
			}
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = moreWidth, moreWidth
			var cut int
			dims := s.More.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.Button.Add(gtx.Ops)
				semantic.DescriptionOp("More actions").Add(gtx.Ops)
				pressed := splitPress(gtx, th, &s.morePress, s.More.Pressed())
				cut = baseCut + int(float32(gtx.Dp(4))*pressed+0.5)
				sink := int(float32(gtx.Dp(1))*pressed + 0.5)
				hover := hoverValue(gtx, th, &s.moreHover, s.More.Hovered())
				palette := interactiveColors(gtx, th, kind, hover)
				r := image.Rect(0, 0, moreWidth, h)
				fillCutCorners(gtx.Ops, r, 0, cut, cut, 0, palette.fill)
				if palette.stroke {
					strokeCutCorners(gtx.Ops, r, 0, cut, cut, 0, max(1, gtx.Dp(1)), palette.outline)
				}
				arrow := Icon("expand_more")
				if open {
					arrow = Icon("expand_less")
				}
				is := gtx.Dp(20)
				offset := op.Offset(image.Pt((moreWidth-is)/2, (h-is)/2+sink)).Push(gtx.Ops)
				arrow.Layout(exact(gtx, image.Pt(is, is)), palette.ink)
				offset.Pop()
				return layout.Dimensions{Size: image.Pt(moreWidth, h)}
			})
			focus := focusValue(gtx, th, &s.moreFocus, gtx.Focused(&s.More.Clickable))
			if focus > 0 {
				pad := gtx.Dp(2)
				strokeCutCorners(gtx.Ops, image.Rectangle{Max: dims.Size}.Inset(-pad), 0, cut+pad, cut+pad, 0, max(1, gtx.Dp(2)), WithAlpha(th.R.Fg2, focus))
			}
			return dims
		}),
	)
}
