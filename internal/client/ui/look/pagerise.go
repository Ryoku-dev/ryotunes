package look

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
)

// PageRise is the Hub's page change (anim.Rise, Hub.qml:1004-1022): the page
// that just opened fades in over 240 ms while it slides in from 14 dp to the
// right over 300, both on OutCubic, and the page it replaced leaves at once.
// The first page stands; under reduced motion nothing plays, and a settled
// page draws as it is and asks for no frame.
type PageRise struct {
	rise  anim.Rise
	key   string
	ready bool
}

// Layout draws w, the page named key, rising when key changed since the last
// frame.
func (p *PageRise) Layout(gtx layout.Context, key string, w layout.Widget) layout.Dimensions {
	if !p.ready {
		p.ready, p.key = true, key
	} else if key != p.key {
		p.key = key
		p.rise.Start(gtx)
	}
	opacity, offset := p.rise.Value(gtx)
	if opacity >= 1 && offset == 0 {
		return w(gtx)
	}
	fade := paint.PushOpacity(gtx.Ops, opacity)
	shift := op.Offset(image.Pt(round32(offset), 0)).Push(gtx.Ops)
	dimensions := w(gtx)
	shift.Pop()
	fade.Pop()
	return dimensions
}

// Active reports whether the page is still rising.
func (p *PageRise) Active() bool { return p.rise.Active() }
