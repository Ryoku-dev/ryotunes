package look

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
)

const wheelSettle = 300 * time.Millisecond

// ScrubberStyle configures seek and volume scrubbers.
type ScrubberStyle struct {
	Fill         color.NRGBA
	AlwaysHandle bool
	Bubble       func(f float32) string
	WheelStep    float32
	Disabled     bool
}

// Scrubber is a normalized pointer-grabbing seek or volume control.
type Scrubber struct {
	tag struct{}

	hovered bool
	pointer image.Point
	hover   anim.Tween
	hoverOn bool
	ready   bool

	active    bool
	pointerID pointer.ID
	preview   float32

	committed      float32
	committedReady bool

	wheelPending  bool
	wheelValue    float32
	wheelFraction float32
	wheelDeadline time.Time
}

func clampFraction(value float32) float32 {
	return min(max(value, 0), 1)
}

func scrubberFraction(x float32, width int) float32 {
	if width <= 0 {
		return 0
	}
	return clampFraction(x / float32(width))
}

func scrubberTrackInk(roles palette.Roles) color.NRGBA {
	return palette.Mix(roles.Stone3, roles.Fg, .22)
}

func scrubberWheel(value, delta, fraction, step, notch float32) (next, remainder float32) {
	if step <= 0 || notch <= 0 {
		return clampFraction(value), fraction
	}
	fraction += delta / notch
	whole := int(fraction)
	fraction -= float32(whole)
	return clampFraction(value - float32(whole)*step), fraction
}

func (s *Scrubber) commit(gtx layout.Context, value float32) {
	s.committed = clampFraction(value)
	s.committedReady = true
	gtx.Execute(op.InvalidateCmd{})
}

func (s *Scrubber) settleWheel(gtx layout.Context) {
	if !s.wheelPending || gtx.Now.Before(s.wheelDeadline) {
		return
	}
	s.wheelPending = false
	s.wheelFraction = 0
	s.commit(gtx, s.wheelValue)
}

func (s *Scrubber) updatePointer(gtx layout.Context, value float32, width int, style ScrubberStyle) {
	s.settleWheel(gtx)
	filter := pointer.Filter{
		Target: &s.tag,
		Kinds: pointer.Enter | pointer.Leave | pointer.Move | pointer.Press |
			pointer.Drag | pointer.Release | pointer.Cancel,
	}
	if !style.Disabled && style.WheelStep > 0 {
		filter.Kinds |= pointer.Scroll
		filter.ScrollX = pointer.ScrollRange{Min: -scrollRangeLimit, Max: scrollRangeLimit}
		filter.ScrollY = pointer.ScrollRange{Min: -scrollRangeLimit, Max: scrollRangeLimit}
	}
	for {
		raw, ok := gtx.Event(filter)
		if !ok {
			break
		}
		e, ok := raw.(pointer.Event)
		if !ok {
			continue
		}
		s.pointer = image.Pt(int(math.Round(float64(e.Position.X))), int(math.Round(float64(e.Position.Y))))
		switch e.Kind {
		case pointer.Enter:
			s.hovered = true
		case pointer.Leave:
			s.hovered = false
		case pointer.Press:
			if style.Disabled || e.Buttons&pointer.ButtonPrimary == 0 {
				continue
			}
			s.active = true
			s.pointerID = e.PointerID
			s.preview = scrubberFraction(e.Position.X, width)
			s.wheelPending = false
			s.wheelFraction = 0
			gtx.Execute(pointer.GrabCmd{Tag: &s.tag, ID: e.PointerID})
			gtx.Execute(key.FocusCmd{Tag: &s.tag})
		case pointer.Drag:
			if s.active && e.PointerID == s.pointerID {
				s.preview = scrubberFraction(e.Position.X, width)
			}
		case pointer.Release:
			if s.active && e.PointerID == s.pointerID {
				s.preview = scrubberFraction(e.Position.X, width)
				s.active = false
				s.commit(gtx, s.preview)
			}
		case pointer.Cancel:
			if s.active && e.PointerID == s.pointerID {
				s.active = false
			}
		case pointer.Scroll:
			if style.Disabled || style.WheelStep <= 0 {
				continue
			}
			delta := e.Scroll.Y
			if delta == 0 {
				delta = e.Scroll.X
			}
			if delta == 0 {
				continue
			}
			if !s.wheelPending {
				s.wheelValue = clampFraction(value)
				s.wheelFraction = 0
			}
			notch := float32(150)
			if scrollOnX11 {
				notch = x11Notch
			}
			s.wheelValue, s.wheelFraction = scrubberWheel(s.wheelValue, delta, s.wheelFraction, style.WheelStep, notch)
			s.wheelPending = true
			s.wheelDeadline = gtx.Now.Add(wheelSettle)
			gtx.Execute(op.InvalidateCmd{})
			gtx.Execute(op.InvalidateCmd{At: s.wheelDeadline})
		}
	}
}

func (s *Scrubber) updateKeys(gtx layout.Context, value float32, style ScrubberStyle) {
	step := style.WheelStep
	if step <= 0 {
		step = .01
	}
	for {
		raw, ok := gtx.Event(
			key.Filter{Focus: &s.tag, Name: key.NameLeftArrow},
			key.Filter{Focus: &s.tag, Name: key.NameRightArrow},
		)
		if !ok {
			break
		}
		e, ok := raw.(key.Event)
		if !ok || e.State != key.Press || style.Disabled {
			continue
		}
		next := value
		if e.Name == key.NameLeftArrow {
			next -= step
		} else {
			next += step
		}
		s.commit(gtx, next)
		value = s.committed
	}
}

func (s *Scrubber) hoverValue(gtx layout.Context, th *Theme) float32 {
	on := s.hovered || s.active
	duration := th.Motion.HoverOut
	if on {
		duration = th.Motion.HoverIn
	}
	s.hover.Duration = duration
	s.hover.Ease = anim.EaseChisel
	if !s.ready {
		s.hover.Set(gtx, 0)
		s.ready = true
	}
	if on != s.hoverOn {
		s.hoverOn = on
		if on {
			s.hover.Set(gtx, 1)
		} else {
			s.hover.Set(gtx, 0)
		}
	}
	return s.hover.Value(gtx)
}

// Layout draws and operates the scrubber.
func (s *Scrubber) Layout(gtx layout.Context, th *Theme, value float32, style ScrubberStyle) layout.Dimensions {
	width := gtx.Constraints.Max.X
	if width <= 0 {
		width = gtx.Dp(160)
	}
	size := gtx.Constraints.Constrain(image.Pt(width, gtx.Dp(24)))
	width = size.X
	s.updatePointer(gtx, value, width, style)
	s.updateKeys(gtx, value, style)

	display := clampFraction(value)
	if s.wheelPending {
		display = s.wheelValue
	}
	if s.active {
		display = s.preview
	}
	hover := s.hoverValue(gtx, th)
	trackHeight := gtx.Dp(unit.Dp(4) + unit.Dp(2)*unit.Dp(hover))
	trackHeight = max(1, trackHeight)
	trackY := (size.Y - trackHeight) / 2
	track := image.Rect(0, trackY, width, trackY+trackHeight)
	trackInk := scrubberTrackInk(th.R)
	fillInk := style.Fill
	handleInk := th.R.Fg
	if style.Disabled {
		trackInk, fillInk, handleInk = th.R.Fg3, th.R.Fg3, th.R.Fg3
	}

	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	kit.FillCut(gtx.Ops, track, trackHeight/2, trackInk)
	filled := int(math.Round(float64(display * float32(width))))
	if filled > 0 {
		kit.FillCut(gtx.Ops, image.Rect(0, trackY, min(filled, width), trackY+trackHeight), trackHeight/2, fillInk)
	}
	handleAlpha := hover
	if style.AlwaysHandle {
		handleAlpha = 1
	}
	handleSize := max(1, gtx.Dp(12))
	handleX := int(math.Round(float64(display*float32(width)))) - handleSize/2
	if handleAlpha > 0 {
		handle := image.Rect(handleX, (size.Y-handleSize)/2, handleX+handleSize, (size.Y+handleSize)/2)
		kit.FillCut(gtx.Ops, handle, gtx.Dp(3), palette.WithAlpha(handleInk, handleAlpha))
	}
	semantic.Editor.Add(gtx.Ops)
	semantic.EnabledOp(!style.Disabled).Add(gtx.Ops)
	event.Op(gtx.Ops, &s.tag)
	area.Pop()

	if style.Bubble != nil && !style.Disabled && handleAlpha > 0 && (s.hovered || s.active) {
		at := s.pointer
		bubbleValue := scrubberFraction(float32(at.X), width)
		if s.active {
			at = image.Pt(handleX+handleSize/2, size.Y/2)
			bubbleValue = s.preview
		}
		Bubble(gtx, th, at, style.Bubble(bubbleValue), handleAlpha)
	}
	return layout.Dimensions{Size: size}
}

// Dragging reports the live preview while the pointer is held.
func (s *Scrubber) Dragging() (f float32, ok bool) {
	return s.preview, s.active
}

// Committed reports and clears the latest release, click, key, or wheel value.
func (s *Scrubber) Committed() (f float32, ok bool) {
	value, ready := s.committed, s.committedReady
	s.committedReady = false
	return value, ready
}
