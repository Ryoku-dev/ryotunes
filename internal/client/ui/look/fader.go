package look

import (
	"fmt"
	"image"
	"math"
	"time"

	"gioui.org/f32"
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
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/tokens"
)

// Fader geometry, in dp: the Hub slider's track and handle (RowControl.qml,
// Slider.qml) stood upright, the handle growing a little under the pointer,
// and the engraved scale beside the track.
const (
	faderTrack      unit.Dp = 4
	faderHandleW    unit.Dp = tokens.SizeSliderHandleHeight
	faderHandleH    unit.Dp = tokens.SizeSliderHandleWidth
	faderHandleGrow unit.Dp = 4
	faderTickShort  unit.Dp = 3
	faderTickLong   unit.Dp = 7
	faderTickGap    unit.Dp = 5
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
//
// It is the Hub's slider stood upright and drawn through the kit's style-aware
// painters: a field track that brightens under the pointer, the stretch from
// the reset value to the value in fg2, an engraved vein scale (a long tick at
// the reset value), and a sel handle, a 45 degree lozenge in Lithic and a
// pill in the round designs. Hover grows the handle and brightens it over
// 120 ms in, 240 out on OutCubic; a press deepens it over the press time.
// A held handle stands exactly under the pointer (R2); a step it is not held
// through (a wheel notch, a key, a double-click reset) glides it there
// through anim.Chase without overshoot. At rest it asks for no frame.
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
	press         anim.Tween
	hoverReady    bool
	chase         anim.Chase
	chaseReady    bool
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

// faderValueAt is the value under a pointer at y, read along the handle's
// travel.
func faderValueAt(gtx layout.Context, y float32, size image.Point, style FaderStyle) float64 {
	top, travel := faderTravel(gtx, size)
	return faderValue(y-float32(top), travel, style)
}

func (fader *Fader) update(gtx layout.Context, value float64, style FaderStyle, size image.Point) {
	filter := pointer.Filter{
		Target:  &fader.tag,
		Kinds:   pointer.Enter | pointer.Leave | pointer.Move | pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
		ScrollY: pointer.ScrollRange{Min: -100000, Max: 100000},
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
			fader.preview = faderValueAt(gtx, e.Position.Y, size, style)
			gtx.Execute(pointer.GrabCmd{Tag: &fader.tag, ID: e.PointerID})
		case pointer.Drag:
			if fader.active && e.PointerID == fader.pointerID {
				fader.preview = faderValueAt(gtx, e.Position.Y, size, style)
			}
		case pointer.Release:
			if fader.active && e.PointerID == fader.pointerID {
				fader.preview = faderValueAt(gtx, e.Position.Y, size, style)
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
			fader.wheelFraction += e.Scroll.Y / 150
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

// motion moves the hover (120 in, 240 out) and the press (the press time)
// toward the pointer's state on OutCubic and returns both, 0..1. Each tween
// takes its new target under the duration it was running with, so a hover
// reversed mid-way carries on from where it stands; the new duration then
// times the run it starts.
func (fader *Fader) motion(gtx layout.Context, th *Theme, disabled bool) (hover, press float32) {
	on := (fader.hovered || fader.active) && !disabled
	fader.hover.Ease, fader.press.Ease = anim.EaseOutCubic, anim.EaseOutCubic
	if !fader.hoverReady {
		fader.hover.Set(gtx, 0)
		fader.press.Set(gtx, 0)
		fader.hoverReady = true
	}
	fader.hover.Set(gtx, boolValue(on))
	fader.press.Set(gtx, boolValue(fader.active && !disabled))
	fader.hover.Duration = th.Motion.Move
	if on {
		fader.hover.Duration = th.Motion.Quick
	}
	fader.press.Duration = th.Motion.Press
	return fader.hover.Value(gtx), fader.press.Value(gtx)
}

// faderTicks is the scale's step: 6 dB over a span up to 36, else 12.
func faderTicks(style FaderStyle) float64 {
	if style.Max-style.Min <= 36 {
		return 6
	}
	return 12
}

// faderTravel is where the handle's centre runs: half a handle in from each
// end, so the handle never leaves the fader.
func faderTravel(gtx layout.Context, size image.Point) (top, travel int) {
	handle := max(1, gtx.Dp(faderHandleH))
	return handle / 2, max(1, size.Y-handle)
}

// faderHandle is the handle's outline in Lithic: a lozenge whose short
// sides lean at 45 degrees, as the slider's handle is cut.
func faderHandle(ops *op.Ops, bounds image.Rectangle) clip.PathSpec {
	lean := float32(min(bounds.Dy(), bounds.Dx()/4))
	minX, minY := float32(bounds.Min.X), float32(bounds.Min.Y)
	maxX, maxY := float32(bounds.Max.X), float32(bounds.Max.Y)
	var path clip.Path
	path.Begin(ops)
	path.MoveTo(f32.Pt(minX+lean, minY))
	path.LineTo(f32.Pt(maxX, minY))
	path.LineTo(f32.Pt(maxX-lean, maxY))
	path.LineTo(f32.Pt(minX, maxY))
	path.Close()
	return path.End()
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
	fraction := float32(faderFraction(display, style))
	if !fader.chaseReady || fader.active {
		// a held handle stands under the pointer (R2); only a step it is
		// not held through (a key, a wheel notch, a reset) glides
		fader.chase.Snap(fraction)
		fader.chaseReady = true
	} else {
		fader.chase.Set(fraction)
	}
	shown := fader.chase.Value(gtx)
	hover, press := fader.motion(gtx, th, style.Disabled)

	top, travel := faderTravel(gtx, size)
	at := func(fraction float32) int { return top + int(float32(travel)*(1-fraction)+.5) }
	y := at(shown)

	area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	trackWidth := max(1, gtx.Dp(faderTrack))
	trackX := (size.X - trackWidth) / 2
	track := image.Rect(trackX, top, trackX+trackWidth, top+travel)
	kit.Fill(gtx, th.Theme, track, 0, palette.Mix(th.R.Field, th.R.Fg, tokens.AlphaHoverPlane*hover))

	// the engraved scale, a long tick at the reset value
	hair := max(1, gtx.Dp(unit.Dp(tokens.SizeHairline)))
	if step := faderTicks(style); style.Max > style.Min {
		right := trackX - gtx.Dp(faderTickGap)
		for tick := math.Ceil(style.Min/step) * step; tick <= style.Max+1e-9; tick += step {
			length := gtx.Dp(faderTickShort)
			if math.Abs(tick-style.Reset) < 1e-9 {
				length = gtx.Dp(faderTickLong)
			}
			ty := at(float32(faderFraction(tick, style)))
			paint.FillShape(gtx.Ops, th.R.Vein, clip.Rect(image.Rect(right-length, ty, right, ty+hair)).Op())
		}
	}

	if !style.Disabled {
		// the stretch from the reset value to the value
		from := at(float32(faderFraction(style.Reset, style)))
		if low, high := min(from, y), max(from, y); high > low {
			kit.Fill(gtx, th.Theme, image.Rect(trackX, low, trackX+trackWidth, high), 0, th.R.Fg2)
		}
	}

	handleH := max(1, gtx.Dp(faderHandleH))
	handleW := gtx.Dp(faderHandleW) + int(float32(gtx.Dp(faderHandleGrow))*hover+.5)
	handle := image.Rect((size.X-handleW)/2, y-handleH/2, (size.X-handleW)/2+handleW, y-handleH/2+handleH)
	handleInk := th.R.Fg3
	if !style.Disabled {
		handleInk = palette.Mix(th.R.Sel, th.R.Fg, tokens.AlphaHoverFill*(hover+press))
	}
	handleCut := min(handle.Dx(), handle.Dy()) / 2
	if th.Design.Profile().Round {
		kit.Fill(gtx, th.Theme, handle, handleCut, handleInk)
	} else {
		paint.FillShape(gtx.Ops, handleInk, clip.Outline{Path: faderHandle(gtx.Ops, handle)}.Op())
	}
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
		kit.FocusRing(gtx, th.Theme, handle, handleCut)
	}
	if hover > 0 && !style.Disabled {
		Bubble(gtx, th, image.Pt(size.X/2, handle.Min.Y), fmt.Sprintf("%+.1f dB", display), hover)
	}
	return layout.Dimensions{Size: size}
}

// Held reports whether the pointer holds the fader, the one time its readout
// follows the value without rolling.
func (fader *Fader) Held() bool { return fader.active }

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
