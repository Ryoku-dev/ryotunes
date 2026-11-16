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

// valueDragCut is the cut of a value drag's wash, a small control's (the
// Hub's segment cut).
const valueDragCut = 5

// ValueDrag adds direct vertical drag and accumulated wheel input to a label.
// Under the pointer it wears the Hub's row wash (RowWash: 120 in, 240 out on
// OutCubic), deepening to the press wash while it is held; at rest it paints
// nothing and asks for no frame.
type ValueDrag struct {
	tag       struct{}
	active    bool
	hovered   bool
	pointerID pointer.ID
	startY    float32
	start     float64
	preview   float64
	committed float64
	ready     bool
	wheel     float32
	wash      RowWash
}

// Layout draws child and attaches logarithmic Q-style input to it.
func (drag *ValueDrag) Layout(gtx layout.Context, th *Theme, value, minValue, maxValue float64, child layout.Widget) layout.Dimensions {
	for {
		raw, ok := gtx.Event(pointer.Filter{
			Target:  &drag.tag,
			Kinds:   pointer.Enter | pointer.Leave | pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: -100000, Max: 100000},
		})
		if !ok {
			break
		}
		e := raw.(pointer.Event)
		switch e.Kind {
		case pointer.Enter:
			drag.hovered = true
		case pointer.Leave:
			drag.hovered = false
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
			drag.wheel += e.Scroll.Y / 150
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
	drag.wash.Update(gtx, th, drag.hovered || drag.active, drag.active)
	record := op.Record(gtx.Ops)
	dimensions := child(gtx)
	call := record.Stop()
	bounds := image.Rectangle{Max: dimensions.Size}
	drag.wash.Paint(gtx, th, bounds, gtx.Dp(valueDragCut))
	call.Add(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(bounds).Push(gtx.Ops)
	event.Op(gtx.Ops, &drag.tag)
	pointer.CursorRowResize.Add(gtx.Ops)
	area.Pop()
	pass.Pop()
	return dimensions
}

// Dragging reports a live value during a vertical drag.
func (drag *ValueDrag) Dragging() (float64, bool) { return drag.preview, drag.active }

// Held reports whether the pointer holds the value.
func (drag *ValueDrag) Held() bool { return drag.active }

// Committed reports and clears a release or wheel value.
func (drag *ValueDrag) Committed() (float64, bool) {
	value, ready := drag.committed, drag.ready
	drag.ready = false
	return value, ready
}
