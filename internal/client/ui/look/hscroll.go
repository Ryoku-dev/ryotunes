package look

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
)

// HScroller virtualises a horizontal shelf without consuming vertical scroll.
//
// It keeps R8's cue to the caller (step plaques at the shelf's ends and the
// last item cut off at its edge) and pages with the Hub's travel: Page glides
// one shelf width over 240 ms (th.Motion.Move) on OutCubic, through the list
// so every item it passes is laid out at its real size (R9), and real input
// stops the glide where it stands. Under reduced motion a page jumps.
type HScroller struct {
	layout.List

	fraction    float32
	viewport    int
	duration    time.Duration
	listGesture bool

	probeOps        op.Ops
	itemSizes       []image.Point
	itemCount       int
	cacheReady      bool
	listConstraints layout.Constraints

	page        anim.Tween
	pageReady   bool
	pageTarget  float32
	pageApplied float32
}

func (s *HScroller) stopPage() {
	if !s.pageReady {
		return
	}
	current := float32(s.Position.Offset)
	s.page = anim.Tween{Duration: s.duration, Ease: anim.EaseOutCubic}
	s.page.Set(layout.Context{}, current)
	s.pageTarget = current
	s.pageApplied = current
}

func (s *HScroller) probeInput(gtx layout.Context) int {
	if !s.cacheReady || s.itemCount <= 0 {
		return 0
	}
	before := s.Position
	fallback := max(1, before.Length/max(s.itemCount, 1))
	s.probeOps.Reset()
	probe := gtx
	probe.Ops = &s.probeOps
	probe.Constraints = s.listConstraints
	s.List.Layout(probe, s.itemCount, func(_ layout.Context, index int) layout.Dimensions {
		size := image.Point{}
		if index >= 0 && index < len(s.itemSizes) {
			size = s.itemSizes[index]
		}
		if size.X <= 0 {
			size.X = fallback
		}
		return layout.Dimensions{Size: size}
	})
	after := s.Position
	if sameListPosition(before, after) {
		if !s.List.Dragging() {
			s.listGesture = false
		}
		return 0
	}

	raw := float32(listPositionDelta(before, after, s.itemSizes, fallback, s.Gap, layout.Horizontal))
	dragging := s.List.Dragging()
	if !dragging && !s.listGesture {
		raw = mapScrollDelta(raw, gtx.Metric.PxPerDp, scrollOnX11)
	}
	s.Position = before
	s.stopPage()
	addListOffset(&s.Position, &s.fraction, raw)
	s.listGesture = dragging || s.listGesture
	return 1
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
	s.duration = th.Motion.Move
	before := s.Position
	applied := s.probeInput(gtx)
	applied += s.advancePage(gtx)
	s.listConstraints = gtx.Constraints
	if len(s.itemSizes) < count {
		s.itemSizes = append(s.itemSizes, make([]image.Point, count-len(s.itemSizes))...)
	}
	s.itemCount = count
	dimensions := s.List.Layout(gtx, count, func(gtx layout.Context, index int) layout.Dimensions {
		dimensions := el(gtx, index)
		s.itemSizes[index] = dimensions.Size
		return dimensions
	})
	s.cacheReady = true
	s.viewport = dimensions.Size.X

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
		s.page = anim.Tween{Duration: s.duration, Ease: anim.EaseOutCubic}
		s.page.Set(gtx, 0)
		s.pageApplied = 0
		s.pageTarget = 0
		s.pageReady = true
	}
	// a press while a page still glides adds a page to where it is going,
	// so two quick presses travel two shelf widths
	s.pageTarget += float32(dir * s.viewport)
	s.page.Duration = s.duration
	s.page.Ease = anim.EaseOutCubic
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
