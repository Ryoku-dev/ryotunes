package lithic

import (
	"image"
	"image/color"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

// IconButton is a square accessible plaque with an optional mineral notch.
type IconButton struct {
	Click
	Small, Busy         bool
	Notch               color.NRGBA
	press, hover, focus Tween
}

// Clicked reports an enabled non-busy icon press.
func (b *IconButton) Clicked(gtx layout.Context) bool {
	if !b.Busy {
		return b.Click.Clicked(gtx)
	}
	for b.Click.Clicked(gtx) {
	}
	return false
}

// Layout renders the icon plaque and accessible description.
func (b *IconButton) Layout(gtx layout.Context, th *Theme, kind ButtonKind, icon *widget.Icon, desc string) layout.Dimensions {
	size, iconSize, baseCut := unit.Dp(40), unit.Dp(20), th.Cut.Small
	if b.Small {
		size, iconSize = 32, 18
	}
	sz := gtx.Dp(size)
	original := gtx
	interaction := gtx
	if b.Busy {
		interaction = interaction.Disabled()
	}
	var finalCut int
	dims := b.Click.Layout(interaction, func(gtx layout.Context) layout.Dimensions {
		gtx.Source = original.Source
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(desc).Add(gtx.Ops)

		b.press.Duration = th.Motion.Press
		b.press.Ease = EaseChisel
		if b.Pressed() && !b.Busy && original.Source.Enabled() {
			b.press.Set(gtx, 1)
		} else {
			b.press.Set(gtx, 0)
		}
		pressed := b.press.Value(gtx)
		finalCut = gtx.Dp(baseCut) + int(float32(gtx.Dp(4))*pressed+0.5)
		sink := int(float32(gtx.Dp(1))*pressed + 0.5)

		r := image.Rectangle{Max: constrainMax(gtx, image.Pt(sz, sz))}
		hover := hoverValue(gtx, th, &b.hover, b.Hovered() && !b.Busy)
		ink := drawButtonSurface(gtx, th, r, finalCut, kind, hover)
		is := gtx.Dp(iconSize)
		offset := op.Offset(image.Pt((r.Dx()-is)/2, (r.Dy()-is)/2+sink)).Push(gtx.Ops)
		if b.Busy {
			Working(exact(gtx, image.Pt(is, is)), th, iconSize)
		} else if icon != nil {
			icon.Layout(exact(gtx, image.Pt(is, is)), ink)
		}
		offset.Pop()
		if b.Notch.A > 0 {
			Notch(gtx.Ops, r.Min, gtx.Dp(th.Cut.Notch), disabledInk(gtx, b.Notch))
		}
		return layout.Dimensions{Size: r.Size()}
	})
	focus := focusValue(original, th, &b.focus, original.Focused(&b.Click.Clickable))
	FocusRingAlpha(original, th, image.Rectangle{Max: dims.Size}, finalCut, focus)
	return dims
}
