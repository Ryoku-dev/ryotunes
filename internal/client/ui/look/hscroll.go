package look

import (
	"image"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
)

// HScroller virtualises a horizontal shelf without consuming vertical scroll.
type HScroller struct {
	layout.List

	tag      struct{}
	fraction float32
	viewport int
	duration time.Duration

	page        anim.Tween
	pageReady   bool
	pageTarget  float32
	pageApplied float32
}

func horizontalScrollRange(position layout.Position) pointer.ScrollRange {
	return verticalScrollRange(position)
}

func (s *HScroller) stopPage() {
	if !s.pageReady {
		return
	}
	current := float32(s.Position.Offset)
	s.page = anim.Tween{Duration: s.duration, Ease: anim.EaseChisel}
	s.page.Set(layout.Context{}, current)
	s.pageTarget = current
	s.pageApplied = current
}

func (s *HScroller) updateInput(gtx layout.Context) int {
	filter := pointer.Filter{
		Target:  &s.tag,
		Kinds:   pointer.Scroll,
		ScrollX: horizontalScrollRange(s.Position),
	}
	applied := 0
	for {
		raw, ok := gtx.Event(filter)
		if !ok {
			break
		}
		e, ok := raw.(pointer.Event)
		if !ok || e.Kind != pointer.Scroll || e.Scroll.X == 0 {
			continue
		}
		s.stopPage()
		applied += addListOffset(&s.Position, &s.fraction, mapScrollDelta(e.Scroll.X, gtx.Metric.PxPerDp, scrollOnX11))
	}
	return applied
}

func (s *HScroller) advancePage(gtx layout.Context) int {
	if !s.pageReady {
		return 0
	}
	value := s.page.Value(gtx)
	delta := value - s.pageApplied
	s.pageApplied = value
	return addListOffset(&s.Position, &s.fraction, delta)
}

// Layout renders a horizontal virtual list with immediate physical scrolling.
func (s *HScroller) Layout(gtx layout.Context, th *Theme, count int, el layout.ListElement) layout.Dimensions {
	s.Axis = layout.Horizontal
	s.duration = th.Motion.Wheel
	before := s.Position
	applied := s.updateInput(gtx)
	applied += s.advancePage(gtx)
	dimensions := s.List.Layout(gtx, count, el)
	s.viewport = dimensions.Size.X

	// A horizontal-only range lets vertical wheel input continue to the page.
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &s.tag)
	area.Pop()
	pass.Pop()

	if applied != 0 && sameListPosition(before, s.Position) {
		s.fraction = 0
		s.pageReady = false
	}
	return dimensions
}

// Page glides one visible shelf width in dir.
func (s *HScroller) Page(gtx layout.Context, dir int) {
	if dir == 0 || s.viewport <= 0 || !s.CanPage(dir) {
		return
	}
	if dir < 0 {
		dir = -1
	} else {
		dir = 1
	}
	if !s.pageReady {
		s.page = anim.Tween{Duration: s.duration, Ease: anim.EaseChisel}
		s.page.Set(gtx, 0)
		s.pageApplied = 0
		s.pageTarget = 0
		s.pageReady = true
	}
	current := s.page.Value(gtx)
	s.pageTarget = current + float32(dir*s.viewport)
	s.page.Duration = s.duration
	s.page.Ease = anim.EaseChisel
	s.page.Set(gtx, s.pageTarget)
}

// CanPage reports whether the shelf can move in dir.
func (s *HScroller) CanPage(dir int) bool {
	if dir < 0 {
		return s.Position.First > 0 || s.Position.Offset > 0
	}
	if dir > 0 {
		return s.Position.BeforeEnd
	}
	return false
}
