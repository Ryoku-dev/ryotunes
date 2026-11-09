package look

import (
	"fmt"
	"image"
	"math"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
)

// FaderStyle configures an instrument fader.
type FaderStyle struct {
	Min, Max, Reset float64
	Step            float64
	Height          unit.Dp
	Label           string
	Disabled        bool
}

// Fader is a vertical pointer-grabbing control with precise live preview.
type Fader struct {
	tag struct{}

	active     bool
	pointerID  pointer.ID
	pointer    image.Point
	hovered    bool
	preview    float64
	committed  float64
	ready      bool
	keyEditing bool

	lastPress     time.Duration
	lastPressPos  image.Point
	wheelFraction float32
	hover         anim.Tween
	hoverReady    bool
}

func faderFraction(value float64, style FaderStyle) float64 {
	span := style.Max - style.Min
	if span <= 0 {
		return 0
	}
	return min(max((value-style.Min)/span, 0), 1)
}

func faderValue(y float32, height int, style FaderStyle) float64 {
	if height <= 0 {
		return style.Min
	}
	fraction := 1 - min(max(float64(y)/float64(height), 0), 1)
	value := style.Min + fraction*(style.Max-style.Min)
	if style.Step > 0 {
		value = math.Round(value/style.Step) * style.Step
	}
	return min(max(value, style.Min), style.Max)
}

func (fader *Fader) set(value float64) {
	fader.committed, fader.ready = value, true
}

func (fader *Fader) update(gtx layout.Context, value float64, style FaderStyle, size image.Point) {
	filter := pointer.Filter{
		Target:  &fader.tag,
		Kinds:   pointer.Enter | pointer.Leave | pointer.Move | pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
		ScrollX: pointer.ScrollRange{Min: -100000, Max: 100000}, ScrollY: pointer.ScrollRange{Min: -100000, Max: 100000},
	}
	for {
		raw, ok := gtx.Event(filter)
		if !ok {
			break
		}
		e := raw.(pointer.Event)
		fader.pointer = e.Position.Round()
		switch e.Kind {
		case pointer.Enter:
			fader.hovered = true
		case pointer.Leave:
			if !fader.active {
				fader.hovered = false
			}
		case pointer.Press:
			if style.Disabled || !e.Buttons.Contain(pointer.ButtonPrimary) {
				continue
			}
			gtx.Execute(key.FocusCmd{Tag: &fader.tag})
			dx, dy := fader.pointer.X-fader.lastPressPos.X, fader.pointer.Y-fader.lastPressPos.Y
			if e.Time > fader.lastPress && e.Time-fader.lastPress <= graphDoubleClick && dx*dx+dy*dy <= gtx.Dp(8)*gtx.Dp(8) {
				fader.set(style.Reset)
				fader.lastPress = 0
				gtx.Execute(op.InvalidateCmd{})
				continue
			}
			fader.lastPress, fader.lastPressPos = e.Time, fader.pointer
			fader.active, fader.pointerID = true, e.PointerID
			fader.preview = faderValue(e.Position.Y, size.Y, style)
			gtx.Execute(pointer.GrabCmd{Tag: &fader.tag, ID: e.PointerID})
		case pointer.Drag:
			if fader.active && e.PointerID == fader.pointerID {
				fader.preview = faderValue(e.Position.Y, size.Y, style)
			}
		case pointer.Release:
			if fader.active && e.PointerID == fader.pointerID {
				fader.preview = faderValue(e.Position.Y, size.Y, style)
				fader.active = false
				fader.set(fader.preview)
				gtx.Execute(op.InvalidateCmd{})
			}
		case pointer.Cancel:
			if fader.active && e.PointerID == fader.pointerID {
				fader.active = false
				fader.set(fader.preview)
				gtx.Execute(op.InvalidateCmd{})
			}
		case pointer.Scroll:
			if style.Disabled {
				continue
			}
			delta := e.Scroll.Y
			if delta == 0 {
				delta = e.Scroll.X
			}
			fader.wheelFraction += delta / 150
			whole := int(fader.wheelFraction)
			fader.wheelFraction -= float32(whole)
			if whole != 0 {
				step := style.Step
				if step <= 0 {
					step = (style.Max - style.Min) / 100
				}
				fader.set(value - float64(whole)*step)
				fader.committed = min(max(fader.committed, style.Min), style.Max)
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
	for {
		raw, ok := gtx.Event(
			key.FocusFilter{Target: &fader.tag},
			key.Filter{Focus: &fader.tag, Name: key.NameUpArrow},
			key.Filter{Focus: &fader.tag, Name: key.NameDownArrow},
			key.Filter{Focus: &fader.tag, Name: key.NameHome},
		)
		if !ok {
			break
		}
		e, ok := raw.(key.Event)
		if !ok {
			continue
		}
		if e.State == key.Release {
			if fader.keyEditing {
				fader.keyEditing = false
				fader.set(fader.preview)
				gtx.Execute(op.InvalidateCmd{})
			}
			continue
		}
		if e.State != key.Press || style.Disabled {
			continue
		}
		step := style.Step
		if step <= 0 {
			step = (style.Max - style.Min) / 100
		}
		next := value
		if fader.keyEditing {
			next = fader.preview
		}
		switch e.Name {
		case key.NameUpArrow:
			next += step
		case key.NameDownArrow:
			next -= step
		case key.NameHome:
			next = style.Reset
		}
		fader.preview = min(max(next, style.Min), style.Max)
		fader.keyEditing = true
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (fader *Fader) hoverAlpha(gtx layout.Context, th *Theme) float32 {
	on := fader.hovered || fader.active
	fader.hover.Duration, fader.hover.Ease = th.Motion.HoverOut, anim.EaseChisel
	if on {
		fader.hover.Duration = th.Motion.HoverIn
	}
	if !fader.hoverReady {
		fader.hover.Set(gtx, 0)
		fader.hoverReady = true
	}
	if on {
		fader.hover.Set(gtx, 1)
	} else {
		fader.hover.Set(gtx, 0)
	}
	return fader.hover.Value(gtx)
}

// Layout draws and operates a vertical fader.
func (fader *Fader) Layout(gtx layout.Context, th *Theme, value float64, style FaderStyle) layout.Dimensions {
	width := gtx.Constraints.Max.X
	if width <= 0 {
		width = gtx.Dp(40)
	}
	height := gtx.Dp(style.Height)
	if height <= 0 {
		height = gtx.Dp(112)
	}
	size := gtx.Constraints.Constrain(image.Pt(width, height))
	fader.update(gtx, value, style, size)
	display := min(max(value, style.Min), style.Max)
	if fader.active || fader.keyEditing {
		display = fader.preview
	}
	fraction := faderFraction(display, style)
	hover := fader.hoverAlpha(gtx, th)

	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	trackWidth := max(1, gtx.Dp(2))
	trackX := (size.X - trackWidth) / 2
	track := image.Rect(trackX, 0, trackX+trackWidth, size.Y)
	paint.FillShape(gtx.Ops, scrubberTrackInk(th.R), clip.Rect(track).Op())
	y := size.Y - int(math.Round(fraction*float64(size.Y)))
	fill := image.Rect(trackX, y, trackX+trackWidth, size.Y)
	ink := th.Accent.Ink
	if style.Disabled {
		ink = th.R.Fg3
	}
	paint.FillShape(gtx.Ops, ink, clip.Rect(fill).Op())
	handleWidth, handleHeight := gtx.Dp(14), max(1, gtx.Dp(3))
	handle := image.Rect((size.X-handleWidth)/2, y-handleHeight/2, (size.X+handleWidth)/2, y+(handleHeight+1)/2)
	kit.Blade(gtx.Ops, handle, palette.WithAlpha(ink, .7+.3*hover))
	semantic.Editor.Add(gtx.Ops)
	semantic.LabelOp(style.Label).Add(gtx.Ops)
	semantic.EnabledOp(!style.Disabled).Add(gtx.Ops)
	event.Op(gtx.Ops, &fader.tag)
	if fader.active {
		pointer.CursorGrabbing.Add(gtx.Ops)
	} else {
		pointer.CursorRowResize.Add(gtx.Ops)
	}
	area.Pop()
	if gtx.Focused(&fader.tag) {
		kit.FocusRing(gtx, th.Theme, image.Rectangle{Max: size}, 0)
	}
	if (fader.hovered || fader.active) && !style.Disabled {
		Bubble(gtx, th, image.Pt(size.X/2, y), fmt.Sprintf("%+.1f dB", display), hover)
	}
	return layout.Dimensions{Size: size}
}

// Dragging reports the exact live fader value while the pointer is held.
func (fader *Fader) Dragging() (float64, bool) {
	return fader.preview, fader.active || fader.keyEditing
}

// Committed reports and clears a release, wheel, double-click or key value.
func (fader *Fader) Committed() (float64, bool) {
	value, ready := fader.committed, fader.ready
	fader.ready = false
	return value, ready
}
