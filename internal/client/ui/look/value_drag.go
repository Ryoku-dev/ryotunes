package look

import (
	"image"
	"math"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
)

// ValueDrag adds direct vertical drag and accumulated wheel input to a label.
type ValueDrag struct {
	tag       struct{}
	active    bool
	pointerID pointer.ID
	startY    float32
	start     float64
	preview   float64
	committed float64
	ready     bool
	wheel     float32
}

// Layout draws child and attaches logarithmic Q-style input to it.
func (drag *ValueDrag) Layout(gtx layout.Context, value, minValue, maxValue float64, child layout.Widget) layout.Dimensions {
	for {
		raw, ok := gtx.Event(pointer.Filter{
			Target:  &drag.tag,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: -100000, Max: 100000}, ScrollY: pointer.ScrollRange{Min: -100000, Max: 100000},
		})
		if !ok {
			break
		}
		e := raw.(pointer.Event)
		switch e.Kind {
		case pointer.Press:
			if !e.Buttons.Contain(pointer.ButtonPrimary) {
				continue
			}
			drag.active, drag.pointerID = true, e.PointerID
			drag.startY, drag.start = e.Position.Y, value
			drag.preview = value
			gtx.Execute(pointer.GrabCmd{Tag: &drag.tag, ID: e.PointerID})
		case pointer.Drag:
			if drag.active && e.PointerID == drag.pointerID {
				drag.preview = drag.start * math.Pow(1.1, float64((drag.startY-e.Position.Y)/10))
				drag.preview = min(max(drag.preview, minValue), maxValue)
			}
		case pointer.Release:
			if drag.active && e.PointerID == drag.pointerID {
				drag.active = false
				drag.committed, drag.ready = math.Round(drag.preview*100)/100, true
				gtx.Execute(op.InvalidateCmd{})
			}
		case pointer.Cancel:
			if drag.active && e.PointerID == drag.pointerID {
				drag.active = false
				drag.committed, drag.ready = math.Round(drag.preview*100)/100, true
				gtx.Execute(op.InvalidateCmd{})
			}
		case pointer.Scroll:
			delta := e.Scroll.Y
			if delta == 0 {
				delta = e.Scroll.X
			}
			drag.wheel += delta / 150
			whole := int(drag.wheel)
			drag.wheel -= float32(whole)
			if whole != 0 {
				drag.committed = value * math.Pow(1.1, float64(-whole))
				drag.committed = math.Round(min(max(drag.committed, minValue), maxValue)*100) / 100
				drag.ready = true
				gtx.Execute(op.InvalidateCmd{})
			}
		}
	}
	dimensions := child(gtx)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &drag.tag)
	pointer.CursorRowResize.Add(gtx.Ops)
	area.Pop()
	pass.Pop()
	return dimensions
}

// Dragging reports a live value during a vertical drag.
func (drag *ValueDrag) Dragging() (float64, bool) { return drag.preview, drag.active }

// Committed reports and clears a release or wheel value.
func (drag *ValueDrag) Committed() (float64, bool) {
	value, ready := drag.committed, drag.ready
	drag.ready = false
	return value, ready
}
