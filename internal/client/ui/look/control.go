package look

import (
	"image"
	"time"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
)

// WindowControl is 24 dp because the library's smallest icon plaque is 32 dp.
type WindowControl struct {
	kit.Click
	Icon        *widget.Icon
	Description string
	press       anim.Tween
	hover       anim.Tween
	focus       anim.Tween
}

// Clicked reports one activation.
func (control *WindowControl) Clicked(gtx layout.Context) bool {
	return control.Click.Clicked(gtx)
}

// Layout draws the compact quiet window control.
func (control *WindowControl) Layout(gtx layout.Context, th *Theme) layout.Dimensions {
	const (
		sizeDp = unit.Dp(24)
		iconDp = unit.Dp(16)
		cutDp  = unit.Dp(4)
	)
	original := gtx
	size := gtx.Dp(sizeDp)
	cut := gtx.Dp(cutDp)
	dimensions := control.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(control.Description).Add(gtx.Ops)
		semantic.EnabledOp(gtx.Source.Enabled()).Add(gtx.Ops)

		pressed := controlTransition(original, &control.press, control.Pressed() && gtx.Source.Enabled(), th.Motion.Press, th.Motion.Press)
		hovered := controlTransition(original, &control.hover, control.Hovered() && gtx.Source.Enabled(), th.Motion.HoverIn, th.Motion.HoverOut)
		cut = gtx.Dp(cutDp) + int(float32(gtx.Dp(unit.Dp(tokens.SizeControlPressCutGrow)))*pressed+.5)
		sink := int(float32(gtx.Dp(unit.Dp(tokens.SizeControlPressSink)))*pressed + .5)
		bounds := image.Rectangle{Max: gtx.Constraints.Constrain(image.Pt(size, size))}
		plane := max(hovered, pressed)
		if plane > 0 {
			fill := palette.Mix(th.R.Stone3, th.R.Fg, tokens.AlphaPressRow*pressed)
			kit.FillCut(gtx.Ops, bounds, cut, palette.WithAlpha(fill, plane))
		}
		if control.Icon != nil {
			iconSize := min(gtx.Dp(iconDp), min(bounds.Dx(), bounds.Dy()))
			offset := op.Offset(image.Pt((bounds.Dx()-iconSize)/2, (bounds.Dy()-iconSize)/2+sink)).Push(gtx.Ops)
			iconGTX := gtx
			iconGTX.Constraints = layout.Exact(image.Pt(iconSize, iconSize))
			control.Icon.Layout(iconGTX, th.R.Fg2)
			offset.Pop()
		}
		return layout.Dimensions{Size: bounds.Size()}
	})
	focused := controlTransition(original, &control.focus, original.Focused(&control.Click.Clickable), th.Motion.FocusIn, th.Motion.FocusOut)
	kit.FocusRingAlpha(original, th.Theme, image.Rectangle{Max: dimensions.Size}, cut, focused)
	return dimensions
}

func controlTransition(gtx layout.Context, tween *anim.Tween, active bool, in, out time.Duration) float32 {
	tween.Ease = anim.EaseChisel
	if active {
		tween.Duration = in
		tween.Set(gtx, 1)
	} else {
		tween.Duration = out
		tween.Set(gtx, 0)
	}
	return tween.Value(gtx)
}
