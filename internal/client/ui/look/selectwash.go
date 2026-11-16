package look

import (
	"image"
	"time"

	"gioui.org/layout"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
)

// Selection landing, from the Hub's tree (TreeView.qml): the wash fades in
// over th.Motion.Move while it widens from .6 of its width to full over
// selectLandScale.
const (
	selectLandFrom  = .6
	selectLandScale = 320 * time.Millisecond
)

// SelectWash is the one mark of where you are, as the Hub's tree and lists
// draw it: a wash of the selection (Roles.Select, sel at .16 dark / .14
// light) under the chosen line, with fg ink on it. Within one group it
// travels to the next line over 240 ms on OutCubic (th.Motion.Move). Arriving
// in another group (a folder, a list) it lands instead: it stands at the new
// line and fades in while it grows from .6 of its width to full over 320 ms.
// The first placement stands at once; a settled wash asks for no frame, and
// under reduced motion every move is immediate.
type SelectWash struct {
	x, y, w, h  anim.Tween
	fade, scale anim.Tween
	group       int
	started     bool
	placed      bool
	shown       image.Rectangle
}

// Layout moves the wash toward target, a line of group, and paints it with
// cut (pixels) in the theme's design. An empty target hides the wash; the
// next target then lands. It returns the rectangle painted.
func (s *SelectWash) Layout(gtx layout.Context, th *Theme, target image.Rectangle, group, cut int) image.Rectangle {
	rect, opacity := s.Step(gtx, th, target, group)
	if opacity > 0 && !rect.Empty() {
		kit.Fill(gtx, th.Theme, rect, cut, palette.WithAlpha(th.R.Select(), opacity))
	}
	return rect
}

// Step moves the wash toward target without painting, returning where it
// stands and its opacity, for a host that paints the wash itself.
func (s *SelectWash) Step(gtx layout.Context, th *Theme, target image.Rectangle, group int) (image.Rectangle, float32) {
	if target.Empty() {
		s.placed, s.shown = false, image.Rectangle{}
		return s.shown, 0
	}
	move := th.Motion.Move
	switch {
	case !s.started:
		// the first placement stands
		s.snap(gtx, move, target, 1)
	case !s.placed || group != s.group:
		// arriving in another group: stand at the line and land
		s.snap(gtx, move, target, 0)
		s.fade.Set(gtx, 1)
		s.scale.Set(gtx, 1)
	default:
		s.x.Set(gtx, float32(target.Min.X))
		s.y.Set(gtx, float32(target.Min.Y))
		s.w.Set(gtx, float32(target.Dx()))
		s.h.Set(gtx, float32(target.Dy()))
	}
	s.started, s.placed, s.group = true, true, group
	x, y, w, h := s.x.Value(gtx), s.y.Value(gtx), s.w.Value(gtx), s.h.Value(gtx)
	opacity, scale := s.fade.Value(gtx), s.scale.Value(gtx)
	width := w * scale
	x += (w - width) / 2
	s.shown = image.Rect(round32(x), round32(y), round32(x+width), round32(y+h))
	return s.shown, opacity
}

// Rect is where the wash stood at the last Layout or Step.
func (s *SelectWash) Rect() image.Rectangle { return s.shown }

// snap stands the wash at target, landed as far as landed says (1: landed).
func (s *SelectWash) snap(gtx layout.Context, move time.Duration, target image.Rectangle, landed float32) {
	snapTween(gtx, &s.x, move, float32(target.Min.X))
	snapTween(gtx, &s.y, move, float32(target.Min.Y))
	snapTween(gtx, &s.w, move, float32(target.Dx()))
	snapTween(gtx, &s.h, move, float32(target.Dy()))
	snapTween(gtx, &s.fade, move, landed)
	snapTween(gtx, &s.scale, selectLandScale, selectLandFrom+(1-selectLandFrom)*landed)
}

// snapTween stands an OutCubic tween of duration d at v; its next Set moves.
func snapTween(gtx layout.Context, tween *anim.Tween, d time.Duration, v float32) {
	*tween = anim.Tween{Duration: d, Ease: anim.EaseOutCubic}
	tween.Set(gtx, v)
}

func round32(v float32) int {
	if v < 0 {
		return -int(-v + .5)
	}
	return int(v + .5)
}
