package look

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
)

// Chevron is a folder chevron's quarter turn, as the Hub's tree turns it: 0
// degrees closed, 90 open, over 240 ms (th.Motion.Move) on OutBack 1.6, so it
// passes open a little and settles. The first state stands; a settled chevron
// asks for no frame, and under reduced motion it turns at once.
type Chevron struct {
	turn  anim.Tween
	ready bool
}

// Angle returns the chevron's rotation in degrees for open.
func (c *Chevron) Angle(gtx layout.Context, th *Theme, open bool) float32 {
	c.turn.Duration, c.turn.Ease = th.Motion.Move, EaseOutBackChevron
	target := float32(0)
	if open {
		target = 90
	}
	if !c.ready {
		c.ready = true
		c.turn.Set(gtx, target)
	}
	c.turn.Set(gtx, target)
	return c.turn.Value(gtx)
}

// CloseTurn is the window close glyph as the Hub's IconButton moves it: a
// quarter turn while hovered, 240 ms on OutCubic (th.Motion.Move), and .85 of
// its size while pressed, 120 ms (th.Motion.Quick). Under reduced motion the
// glyph does not turn and the press scale is immediate.
type CloseTurn struct {
	turn, press anim.Tween
	ready       bool
}

// Update moves the glyph toward hovered and pressed and returns its rotation
// in degrees and its scale.
func (c *CloseTurn) Update(gtx layout.Context, th *Theme, hovered, pressed bool) (angle, scale float32) {
	c.turn.Duration, c.turn.Ease = th.Motion.Move, anim.EaseOutCubic
	c.press.Duration, c.press.Ease = th.Motion.Quick, anim.EaseOutCubic
	if !c.ready {
		c.ready = true
		c.turn.Set(gtx, 0)
		c.press.Set(gtx, 0)
	}
	turned := float32(0)
	if hovered && !anim.Reduced() {
		turned = 90
	}
	c.turn.Set(gtx, turned)
	c.press.Set(gtx, boolValue(pressed))
	return c.turn.Value(gtx), 1 - (1-glyphPressedButton)*c.press.Value(gtx)
}

// DrawGlyph draws icon size pixels square in ink, rotated by angle degrees
// clockwise and scaled by scale about its centre.
func DrawGlyph(gtx layout.Context, icon *widget.Icon, ink color.NRGBA, size int, angle, scale float32) layout.Dimensions {
	dimensions := layout.Dimensions{Size: image.Pt(size, size)}
	if icon == nil || size <= 0 {
		return dimensions
	}
	child := gtx
	child.Constraints = layout.Exact(dimensions.Size)
	if angle == 0 && scale == 1 {
		icon.Layout(child, ink)
		return dimensions
	}
	centre := f32.Pt(float32(size)/2, float32(size)/2)
	transform := f32.AffineId().Scale(centre, f32.Pt(scale, scale)).Rotate(centre, angle*math.Pi/180)
	stack := op.Affine(transform).Push(gtx.Ops)
	icon.Layout(child, ink)
	stack.Pop()
	return dimensions
}
