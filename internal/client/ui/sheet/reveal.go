package sheet

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// revealRise is how far below its place a group starts (tokens
// SizeWorkPaneRise).
const revealRise unit.Dp = 8

// Reveal is a page's entrance as the Hub's pages play it (anim.Reveal,
// SettingsPage.qml:773-777): the page's first five groups (a header, a strip
// of tabs, the first shelves) each fade in and settle from 8 dp below over
// 200 ms on the chisel curve (th.Motion.Navigate), 40 ms after the group
// before it (th.Motion.RevealStagger); later groups stand. Each group plays
// once: a data refresh or a rebuild moves nothing until Reset, when the page
// is opened anew. Under reduced motion nothing plays; a page that has risen
// asks for no frame.
type Reveal struct {
	groups [anim.RevealGroups]anim.Reveal
}

// Reset arms the groups again, for a page that has just opened.
func (r *Reveal) Reset() {
	r.groups = [anim.RevealGroups]anim.Reveal{}
}

// Group lays w out as the page's group at order (0 for the first) and rises
// it in on its turn.
func (r *Reveal) Group(gtx layout.Context, th *look.Theme, order int, w layout.Widget) layout.Dimensions {
	if order < 0 || order >= len(r.groups) {
		return w(gtx)
	}
	group := &r.groups[order]
	group.Order = order
	if group.Done() {
		return w(gtx)
	}
	opacity, offset := group.Step(gtx, th.Motion.Navigate, th.Motion.RevealStagger, float32(gtx.Dp(revealRise)))
	if opacity >= 1 && offset == 0 {
		return w(gtx)
	}
	fade := paint.PushOpacity(gtx.Ops, opacity)
	rise := op.Offset(image.Pt(0, int(offset+.5))).Push(gtx.Ops)
	dimensions := w(gtx)
	rise.Pop()
	fade.Pop()
	return dimensions
}
