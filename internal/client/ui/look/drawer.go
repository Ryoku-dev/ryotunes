package look

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
)

// DrawerEdge is the window edge a drawer slides from.
type DrawerEdge uint8

const (
	DrawerRight DrawerEdge = iota
	DrawerLeft
	DrawerBottom
)

// Drawer is a sheet that slides over the page from an edge, as the Hub's
// narrow drawer does (Hub.qml:1024-1048): in and out over 240 ms on OutCubic
// (th.Motion.Move), over the scrim (Roles.Scrim), which fades with it. A
// press on the scrim reports Dismissed; presses on the sheet stay in it. A
// closed drawer draws nothing and asks for no frame; under reduced motion it
// opens and closes at once.
type Drawer struct {
	slide anim.Tween
	ready bool
	// the input tags take a byte each: zero-size fields side by side share
	// one address, and Gio would read them as one handler
	scrim, sheet byte
	dismissed    bool
	progress     float32
}

// Layout fills gtx.Constraints.Max with the scrim and lays body out at
// extent pixels across (wide for the side edges, tall for the bottom) along
// the edge, as far in as the slide stands.
func (d *Drawer) Layout(gtx layout.Context, th *Theme, open bool, edge DrawerEdge, extent int, body layout.Widget) layout.Dimensions {
	d.slide.Duration, d.slide.Ease = th.Motion.Move, anim.EaseOutCubic
	if !d.ready {
		d.ready = true
		d.slide.Set(gtx, 0)
	}
	d.slide.Set(gtx, boolValue(open))
	d.progress = d.slide.Value(gtx)
	for {
		raw, ok := gtx.Event(pointer.Filter{Target: &d.scrim, Kinds: pointer.Press})
		if !ok {
			break
		}
		if press, ok := raw.(pointer.Event); ok && press.Kind == pointer.Press && open {
			d.dismissed = true
		}
	}
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: &d.sheet, Kinds: pointer.Press | pointer.Release}); !ok {
			break
		}
	}
	size := gtx.Constraints.Max
	if d.progress <= 0 {
		return layout.Dimensions{Size: size}
	}

	full := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	paint.Fill(gtx.Ops, palette.WithAlpha(th.R.Scrim, d.progress))
	event.Op(gtx.Ops, &d.scrim)
	full.Pop()

	extent = max(0, extent)
	travel := round32(float32(extent) * d.progress)
	sheet := image.Rect(size.X-travel, 0, size.X-travel+extent, size.Y)
	switch edge {
	case DrawerLeft:
		sheet = image.Rect(travel-extent, 0, travel, size.Y)
	case DrawerBottom:
		sheet = image.Rect(0, size.Y-travel, size.X, size.Y-travel+extent)
	}
	area := clip.Rect(sheet).Push(gtx.Ops)
	event.Op(gtx.Ops, &d.sheet)
	area.Pop()
	child := gtx
	child.Constraints = layout.Exact(sheet.Size())
	if !open {
		child = child.Disabled()
	}
	if body != nil {
		offset := op.Offset(sheet.Min).Push(gtx.Ops)
		body(child)
		offset.Pop()
	}
	return layout.Dimensions{Size: size}
}

// Dismissed reports and clears a press on the scrim.
func (d *Drawer) Dismissed() bool {
	dismissed := d.dismissed
	d.dismissed = false
	return dismissed
}

// Visible reports whether the drawer showed at the last Layout.
func (d *Drawer) Visible() bool { return d.progress > 0 }
