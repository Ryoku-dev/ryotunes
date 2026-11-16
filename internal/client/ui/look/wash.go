package look

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
)

// Row motion, from the Hub's tree and lists (TreeRow.qml, IconButton.qml).
const (
	// rowLean is how far a hovered row leans right.
	rowLean unit.Dp = 2
	// glyphPressed is a row glyph's scale while pressed.
	glyphPressed = .86
	// glyphPressedButton is an icon button glyph's scale while pressed.
	glyphPressedButton = .85
)

// RowWash is an app-owned row's hover and press as the Hub draws them: a wash
// of fg (Roles.Hover, .06 dark / .07 light) that comes in over 120 ms
// (th.Motion.Quick) and leaves over 240 (th.Motion.Move) on OutCubic, with the
// row leaning 2 dp to the right while hovered, and a stronger press wash
// (Roles.Press, .10 / .12) under the pointer with the glyph at .86. Plaques
// keep the library's own hover. A settled wash asks for no frame; under
// reduced motion every change is immediate.
type RowWash struct {
	hover, press anim.Tween
	ready        bool
	hovered      float32
	pressed      float32
}

// Update moves the wash toward hovered and pressed and returns how far each
// stands, 0..1. Call it once a frame, before Paint, Lean and GlyphScale.
func (w *RowWash) Update(gtx layout.Context, th *Theme, hovered, pressed bool) (hover, press float32) {
	w.hover.Ease, w.press.Ease = anim.EaseOutCubic, anim.EaseOutCubic
	if !w.ready {
		w.ready = true
		w.hover.Set(gtx, 0)
		w.press.Set(gtx, 0)
	}
	// Set first: a reversal starts from where the run in progress stands,
	// read with that run's duration; only the new run takes the new one
	w.hover.Set(gtx, boolValue(hovered))
	w.press.Set(gtx, boolValue(pressed))
	w.hover.Duration = th.Motion.Move
	if hovered {
		w.hover.Duration = th.Motion.Quick
	}
	w.press.Duration = th.Motion.Press
	w.hovered, w.pressed = w.hover.Value(gtx), w.press.Value(gtx)
	return w.hovered, w.pressed
}

// Ink is the wash at its current amounts: the hover wash, turning into the
// press wash as the row is pressed.
func (w *RowWash) Ink(th *Theme) color.NRGBA {
	return WashInk(th.R, w.hovered, w.pressed)
}

// Paint fills bounds with the wash in the theme's design, cut being the row's
// cut in pixels (the Hub's rows use 6). A row at rest paints nothing.
func (w *RowWash) Paint(gtx layout.Context, th *Theme, bounds image.Rectangle, cut int) {
	if w.hovered <= 0 && w.pressed <= 0 {
		return
	}
	kit.Fill(gtx, th.Theme, bounds, cut, w.Ink(th))
}

// Lean is the row's rightward lean, in pixels, at the current hover.
func (w *RowWash) Lean(gtx layout.Context) int {
	return int(float32(gtx.Dp(rowLean))*w.hovered + .5)
}

// GlyphScale is the row glyph's scale at the current press: 1 at rest, .86
// held.
func (w *RowWash) GlyphScale() float32 {
	return 1 - (1-glyphPressed)*w.pressed
}

// WashInk is the Hub's row wash for a hover and a press amount, 0..1 each:
// Roles.Hover faded by hover, moving to Roles.Press as press grows.
func WashInk(roles palette.Roles, hover, press float32) color.NRGBA {
	hover, press = clamp01(hover), clamp01(press)
	rest := palette.WithAlpha(roles.Hover(), max(hover, press))
	if press <= 0 {
		return rest
	}
	return palette.Mix(rest, roles.Press(), press)
}

func boolValue(on bool) float32 {
	if on {
		return 1
	}
	return 0
}

func clamp01(v float32) float32 {
	return min(max(v, 0), 1)
}
