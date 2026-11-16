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

// WindowControl is a quiet window control, 24 dp unless Size says otherwise (the
// library's smallest icon plaque is 32 dp), that answers the pointer as the Hub's IconButton does: the row wash under it
// (Roles.Hover, turning to Roles.Press while held, 120 in and 240 out on OutCubic), the
// plaque's press cut and sink, and the glyph at .85 while pressed. A Close control's glyph
// also turns a quarter while hovered (CloseTurn). Its corners follow the desktop's design.
type WindowControl struct {
	kit.Click
	Icon        *widget.Icon
	Description string
	// Close turns the glyph a quarter while hovered, as the Hub's close button.
	Close bool
	// Size is the control's side; zero is 24 dp. From 32 dp the glyph is 18.
	Size unit.Dp

	wash  RowWash
	turn  CloseTurn
	press anim.Tween
	focus anim.Tween
}

// Clicked reports one activation.
func (control *WindowControl) Clicked(gtx layout.Context) bool {
	return control.Click.Clicked(gtx)
}

// Layout draws the compact quiet window control.
func (control *WindowControl) Layout(gtx layout.Context, th *Theme) layout.Dimensions {
	const cutDp = unit.Dp(4)
	sizeDp, iconDp := control.Size, unit.Dp(16)
	if sizeDp <= 0 {
		sizeDp = 24
	}
	if sizeDp >= 32 {
		iconDp = 18
	}
	original := gtx
	size := gtx.Dp(sizeDp)
	cut := gtx.Dp(cutDp)
	dimensions := control.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(control.Description).Add(gtx.Ops)
		semantic.EnabledOp(gtx.Source.Enabled()).Add(gtx.Ops)

		enabled := gtx.Source.Enabled()
		hovered, held := enabled && control.Hovered(), enabled && control.Pressed()
		control.wash.Update(original, th, hovered, held)
		pressed := controlTransition(original, &control.press, held, th.Motion.Press, th.Motion.Press)
		cut = gtx.Dp(cutDp) + int(float32(gtx.Dp(unit.Dp(tokens.SizeControlPressCutGrow)))*pressed+.5)
		sink := int(float32(gtx.Dp(unit.Dp(tokens.SizeControlPressSink)))*pressed + .5)
		bounds := image.Rectangle{Max: gtx.Constraints.Constrain(image.Pt(size, size))}
		control.wash.Paint(gtx, th, bounds, cut)
		angle, scale := float32(0), float32(1)
		if control.Close {
			angle, scale = control.turn.Update(original, th, hovered, held)
		} else {
			scale = 1 - (1-glyphPressedButton)*control.wash.pressed
		}
		if control.Icon != nil {
			iconSize := min(gtx.Dp(iconDp), min(bounds.Dx(), bounds.Dy()))
			offset := op.Offset(image.Pt((bounds.Dx()-iconSize)/2, (bounds.Dy()-iconSize)/2+sink)).Push(gtx.Ops)
			DrawGlyph(gtx, control.Icon, palette.Mix(th.R.Fg2, th.R.Fg, control.wash.hovered), iconSize, angle, scale)
			offset.Pop()
		}
		return layout.Dimensions{Size: bounds.Size()}
	})
	focused := controlTransition(original, &control.focus, original.Focused(&control.Click.Clickable), th.Motion.FocusIn, th.Motion.FocusOut)
	kit.FocusRingAlpha(original, th.Theme, image.Rectangle{Max: dimensions.Size}, cut, focused)
	return dimensions
}

// controlTransition moves a control's state tween: chisel, as the library's plaques move.
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
