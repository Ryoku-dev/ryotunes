package look

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
)

// Hover reports pointer presence as a zero-to-one transition without taking input.
type Hover struct {
	tag     struct{}
	hovered bool
	ready   bool
	tween   anim.Tween
}

// Layout lays out w and returns its hover transition.
func (h *Hover) Layout(gtx layout.Context, th *Theme, w layout.Widget) (layout.Dimensions, float32) {
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: &h.tag, Kinds: pointer.Enter | pointer.Leave})
		if !ok {
			break
		}
		e, ok := raw.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Enter:
			h.hovered = true
		case pointer.Leave:
			h.hovered = false
		}
	}

	dimensions := w(gtx)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &h.tag)
	area.Pop()
	pass.Pop()

	target := float32(0)
	duration := th.Motion.HoverOut
	if h.hovered {
		target = 1
		duration = th.Motion.HoverIn
	}
	h.tween.Duration = duration
	h.tween.Ease = anim.EaseChisel
	if !h.ready {
		h.tween.Set(gtx, 0)
		h.ready = true
	}
	h.tween.Set(gtx, target)
	return dimensions, h.tween.Value(gtx)
}

// Swap crossfades keyed content with a short vertical exchange.
type Swap struct {
	current  string
	previous string
	ready    bool
	moving   bool
	tween    anim.Tween
}

func largerDimensions(first, second layout.Dimensions) layout.Dimensions {
	return layout.Dimensions{
		Size:     image.Pt(max(first.Size.X, second.Size.X), max(first.Size.Y, second.Size.Y)),
		Baseline: max(first.Baseline, second.Baseline),
	}
}

func addFadedOffset(ops *op.Ops, call op.CallOp, alpha float32, offset image.Point) {
	if alpha <= 0 {
		return
	}
	opacity := paint.PushOpacity(ops, min(alpha, 1))
	move := op.Offset(offset).Push(ops)
	call.Add(ops)
	move.Pop()
	opacity.Pop()
}

// Layout draws only the current key at rest and both keys during a change.
func (s *Swap) Layout(gtx layout.Context, _ *Theme, key string, rise unit.Dp, duration time.Duration, w func(gtx layout.Context, key string) layout.Dimensions) layout.Dimensions {
	if !s.ready {
		s.current = key
		s.ready = true
		return w(gtx, key)
	}
	if key != s.current {
		s.previous = s.current
		s.current = key
		s.tween = anim.Tween{Duration: duration, Ease: anim.EaseChisel}
		s.tween.Set(gtx, 0)
		s.tween.Set(gtx, 1)
		s.moving = duration > 0 && !anim.Reduced()
	}
	if !s.moving {
		return w(gtx, s.current)
	}
	progress := s.tween.Value(gtx)
	if progress >= 1 {
		s.previous = ""
		s.moving = false
		return w(gtx, s.current)
	}

	oldRecord := op.Record(gtx.Ops)
	oldDimensions := w(gtx.Disabled(), s.previous)
	oldCall := oldRecord.Stop()
	newRecord := op.Record(gtx.Ops)
	newDimensions := w(gtx, s.current)
	newCall := newRecord.Stop()
	risePX := gtx.Dp(rise)
	addFadedOffset(gtx.Ops, oldCall, 1-progress, image.Pt(0, -int(math.Round(float64(float32(risePX)*progress)))))
	addFadedOffset(gtx.Ops, newCall, progress, image.Pt(0, int(math.Round(float64(float32(risePX)*(1-progress))))))
	return largerDimensions(oldDimensions, newDimensions)
}

// Morph scales and crossfades icon changes.
type Morph struct {
	current  *widget.Icon
	previous *widget.Icon
	ready    bool
	moving   bool
	tween    anim.Tween
}

func drawMorphIcon(gtx layout.Context, icon *widget.Icon, ink color.NRGBA, size int, scale, alpha float32) layout.Dimensions {
	dimensions := layout.Dimensions{Size: image.Pt(size, size)}
	if icon == nil || alpha <= 0 {
		return dimensions
	}
	child := gtx
	child.Constraints = layout.Exact(dimensions.Size)
	opacity := paint.PushOpacity(gtx.Ops, min(alpha, 1))
	centre := f32.Pt(float32(size)/2, float32(size)/2)
	transform := op.Affine(f32.AffineId().Scale(centre, f32.Pt(scale, scale))).Push(gtx.Ops)
	icon.Layout(child, ink)
	transform.Pop()
	opacity.Pop()
	return dimensions
}

// Layout draws an icon and animates only when its identity changes.
func (m *Morph) Layout(gtx layout.Context, th *Theme, icon *widget.Icon, ink color.NRGBA, size unit.Dp) layout.Dimensions {
	pixels := max(0, gtx.Dp(size))
	if !m.ready {
		m.current = icon
		m.ready = true
		return drawMorphIcon(gtx, icon, ink, pixels, 1, 1)
	}
	if icon != m.current {
		m.previous = m.current
		m.current = icon
		m.tween = anim.Tween{Duration: th.Motion.Notch, Ease: anim.EaseChisel}
		m.tween.Set(gtx, 0)
		m.tween.Set(gtx, 1)
		m.moving = th.Motion.Notch > 0 && !anim.Reduced()
	}
	if !m.moving {
		return drawMorphIcon(gtx, m.current, ink, pixels, 1, 1)
	}
	progress := m.tween.Value(gtx)
	if progress >= 1 {
		m.previous = nil
		m.moving = false
		return drawMorphIcon(gtx, m.current, ink, pixels, 1, 1)
	}
	drawMorphIcon(gtx, m.previous, ink, pixels, 1-.15*progress, 1-progress)
	return drawMorphIcon(gtx, m.current, ink, pixels, .85+.15*progress, progress)
}

// Fill reveals a filled glyph through a circle growing from its centre.
type Fill struct {
	on    bool
	ready bool
	tween anim.Tween
}

// Layout draws off, then clips onW to the animated fill circle.
func (f *Fill) Layout(gtx layout.Context, th *Theme, on bool, off, onW layout.Widget) layout.Dimensions {
	f.tween.Duration = th.Motion.Notch
	f.tween.Ease = anim.EaseChisel
	if !f.ready {
		initial := float32(0)
		if on {
			initial = 1
		}
		f.tween.Set(gtx, initial)
		f.on = on
		f.ready = true
	} else if on != f.on {
		f.on = on
		if on {
			f.tween.Set(gtx, 1)
		} else {
			f.tween.Set(gtx, 0)
		}
	}
	progress := f.tween.Value(gtx)
	offRecord := op.Record(gtx.Ops)
	offDimensions := off(gtx)
	offCall := offRecord.Stop()
	offCall.Add(gtx.Ops)
	if progress <= 0 {
		return offDimensions
	}
	onRecord := op.Record(gtx.Ops)
	onDimensions := onW(gtx)
	onCall := onRecord.Stop()
	dimensions := largerDimensions(offDimensions, onDimensions)
	halfWidth := float64(dimensions.Size.X) / 2
	halfHeight := float64(dimensions.Size.Y) / 2
	radius := int(math.Ceil(math.Hypot(halfWidth, halfHeight) * float64(progress)))
	centre := image.Pt(dimensions.Size.X/2, dimensions.Size.Y/2)
	circle := clip.Ellipse(image.Rect(centre.X-radius, centre.Y-radius, centre.X+radius, centre.Y+radius)).Push(gtx.Ops)
	onCall.Add(gtx.Ops)
	circle.Pop()
	return dimensions
}
