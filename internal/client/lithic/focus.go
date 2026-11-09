// This file centralizes hover, focus, disabled, and focus-ring behaviour.
package lithic

import (
	"image"
	"image/color"
	"time"

	"gioui.org/layout"
)

func transitionValue(gtx layout.Context, tween *Tween, on bool, in, out time.Duration) float32 {
	tween.Ease = EaseChisel
	if on {
		tween.Set(gtx, 1)
		tween.Duration = in
	} else {
		tween.Set(gtx, 0)
		tween.Duration = out
	}
	return tween.Value(gtx)
}

func hoverValue(gtx layout.Context, th *Theme, tween *Tween, hovered bool) float32 {
	return transitionValue(gtx, tween, hovered && gtx.Source.Enabled(), th.Motion.HoverIn, th.Motion.HoverOut)
}

func focusValue(gtx layout.Context, th *Theme, tween *Tween, focused bool) float32 {
	return transitionValue(gtx, tween, focused, th.Motion.FocusIn, th.Motion.FocusOut)
}

func disabledInk(gtx layout.Context, ink color.NRGBA) color.NRGBA {
	if !gtx.Source.Enabled() {
		return WithAlpha(ink, 0.38)
	}
	return ink
}

// FocusRingAlpha draws a partially visible keyboard focus ring.
func FocusRingAlpha(gtx layout.Context, th *Theme, bounds image.Rectangle, cut int, alpha float32) {
	if alpha <= 0 {
		return
	}
	gap, width := gtx.Dp(2), max(1, gtx.Dp(2))
	StrokeCut(gtx.Ops, bounds.Inset(-gap), cut+gap, width, WithAlpha(th.R.Fg2, min(alpha, 1)))
}

// FocusRing draws the required 2dp ring 2dp outside the cut.
func FocusRing(gtx layout.Context, th *Theme, bounds image.Rectangle, cut int) {
	FocusRingAlpha(gtx, th, bounds, cut, 1)
}
