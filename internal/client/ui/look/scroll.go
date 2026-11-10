package look

import (
	"image"
	"math"
	"os"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
)

const (
	scrollRangeLimit = 1 << 30
	x11Notch         = 10
	x11NotchDP       = 48
	thumbIdle        = 600 * time.Millisecond
)

var scrollOnX11 = os.Getenv("WAYLAND_DISPLAY") == ""

// Scroller virtualises a vertical list and owns its wheel and trackpad input.
type Scroller struct {
	layout.List

	tag          struct{}
	fraction     float32
	hovered      bool
	lastScroll   time.Time
	thumb        anim.Tween
	thumbOn      bool
	thumbReady   bool
	userScrolled bool
	listGesture  bool

	inputOps     *op.Ops
	inputAt      time.Time
	inputReady   bool
	inputBefore  layout.Position
	inputApplied int

	probeOps        op.Ops
	itemSizes       []image.Point
	itemCount       int
	cacheReady      bool
	listConstraints layout.Constraints
	programmatic    bool

	spring        anim.Spring
	springActive  bool
	springApplied float32
}

func mapScrollDelta(delta, pixelsPerDP float32, x11 bool) float32 {
	if x11 {
		notches := float32(math.Round(float64(delta / x11Notch)))
		if notches != 0 && float32(math.Abs(float64(delta-notches*x11Notch))) <= .5 {
			return notches * x11NotchDP * pixelsPerDP
		}
	}
	return delta * pixelsPerDP
}

func verticalScrollRange(position layout.Position) pointer.ScrollRange {
	var bounds pointer.ScrollRange
	if position.First > 0 || position.Offset > 0 {
		bounds.Min = -scrollRangeLimit
	}
	if position.BeforeEnd {
		bounds.Max = scrollRangeLimit
	}
	return bounds
}

func addListOffset(position *layout.Position, fraction *float32, delta float32) int {
	if delta < 0 {
		position.BeforeEnd = true
	}
	total := *fraction + delta
	whole := int(total)
	*fraction = total - float32(whole)
	position.Offset += whole
	return whole
}

func sameListPosition(a, b layout.Position) bool {
	return a.First == b.First && a.Offset == b.Offset
}

func itemExtent(sizes []image.Point, index, fallback int, axis layout.Axis) int {
	if index >= 0 && index < len(sizes) {
		size := axis.Convert(sizes[index]).X
		if size > 0 {
			return size
		}
	}
	return max(1, fallback)
}

func listPositionDelta(before, after layout.Position, sizes []image.Point, fallback, gap int, axis layout.Axis) int {
	delta := after.Offset - before.Offset
	if after.First > before.First {
		for index := before.First; index < after.First; index++ {
			delta += itemExtent(sizes, index, fallback, axis) + gap
		}
	} else {
		for index := after.First; index < before.First; index++ {
			delta -= itemExtent(sizes, index, fallback, axis) + gap
		}
	}
	return delta
}

func scrollThumb(viewport, content, first, offset, count, minimum int) (top, height int, ok bool) {
	if viewport <= 0 || content <= viewport || count <= 0 {
		return 0, 0, false
	}
	height = max(minimum, viewport*viewport/content)
	height = min(viewport, height)
	travel := viewport - height
	average := float64(content) / float64(count)
	scrolled := float64(first)*average + float64(offset)
	maximum := float64(content - viewport)
	if maximum > 0 {
		top = int(math.Round(float64(travel) * min(max(scrolled/maximum, 0), 1)))
	}
	return top, height, true
}

func (s *Scroller) setThumb(gtx layout.Context, th *Theme, shown bool) float32 {
	duration := th.Motion.HoverOut
	if shown {
		duration = th.Motion.HoverIn
	}
	s.thumb.Duration = duration
	s.thumb.Ease = anim.EaseChisel
	if !s.thumbReady {
		s.thumb.Set(gtx, 0)
		s.thumbReady = true
	}
	if shown != s.thumbOn {
		s.thumbOn = shown
		if shown {
			s.thumb.Set(gtx, 1)
		} else {
			s.thumb.Set(gtx, 0)
		}
	}
	return s.thumb.Value(gtx)
}

func (s *Scroller) stopSpring() {
	if !s.springActive {
		return
	}
	s.spring.Snap(s.springApplied)
	s.springActive = false
}

func (s *Scroller) updateHover(gtx layout.Context) {
	filter := pointer.Filter{Target: &s.tag, Kinds: pointer.Enter | pointer.Leave}
	for {
		raw, ok := gtx.Event(filter)
		if !ok {
			return
		}
		e, ok := raw.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Enter:
			s.hovered = true
		case pointer.Leave:
			s.hovered = false
		}
	}
}

func (s *Scroller) probeInput(gtx layout.Context) int {
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
		if size.Y <= 0 {
			size.Y = fallback
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

	raw := float32(listPositionDelta(before, after, s.itemSizes, fallback, s.Gap, layout.Vertical))
	dragging := s.List.Dragging()
	if !dragging && !s.listGesture {
		raw = mapScrollDelta(raw, gtx.Metric.PxPerDp, scrollOnX11)
	}
	s.Position = before
	addListOffset(&s.Position, &s.fraction, raw)
	s.listGesture = dragging || s.listGesture
	if !s.programmatic {
		s.stopSpring()
		s.userScrolled = true
		s.lastScroll = gtx.Now
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(thumbIdle)})
	}
	return 1
}

// UpdateInput applies real input and programmatic spring motion once for this frame.
func (s *Scroller) UpdateInput(gtx layout.Context) {
	if s.inputReady && s.inputOps == gtx.Ops && s.inputAt == gtx.Now {
		return
	}
	s.inputOps = gtx.Ops
	s.inputAt = gtx.Now
	s.inputReady = true
	s.inputBefore = s.Position
	s.updateHover(gtx)
	s.inputApplied = s.probeInput(gtx)
	s.inputApplied += s.advanceSpring(gtx)
}

func (s *Scroller) advanceSpring(gtx layout.Context) int {
	if !s.springActive {
		return 0
	}
	value := s.spring.Value(gtx)
	delta := value - s.springApplied
	s.springApplied = value
	applied := addListOffset(&s.Position, &s.fraction, delta)
	if !s.spring.Moving() {
		s.springActive = false
	}
	return applied
}

// Layout renders a vertical virtual list with immediate physical scrolling.
func (s *Scroller) Layout(gtx layout.Context, th *Theme, count int, el layout.ListElement) layout.Dimensions {
	s.Axis = layout.Vertical
	s.UpdateInput(gtx)
	s.listConstraints = gtx.Constraints
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
	event.Op(gtx.Ops, &s.tag)
	area.Pop()
	pass.Pop()

	beforeList := s.Position
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
	dragging := s.List.Dragging()
	movedByList := !sameListPosition(beforeList, s.Position)
	if !s.programmatic && movedByList && (dragging || s.listGesture) {
		s.stopSpring()
		s.userScrolled = true
		s.lastScroll = gtx.Now
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(thumbIdle)})
	}
	if dragging {
		s.listGesture = true
	} else if !movedByList {
		s.listGesture = false
	}

	if s.inputApplied != 0 && sameListPosition(s.inputBefore, s.Position) {
		s.fraction = 0
		s.stopSpring()
	}
	s.programmatic = false

	shown := s.hovered || (!s.lastScroll.IsZero() && gtx.Now.Before(s.lastScroll.Add(thumbIdle)))
	alpha := s.setThumb(gtx, th, shown)
	if alpha <= 0 {
		return dimensions
	}
	top, height, ok := scrollThumb(dimensions.Size.Y, s.Position.Length, s.Position.First, s.Position.Offset, count, gtx.Dp(32))
	if !ok {
		return dimensions
	}
	width := max(1, gtx.Dp(3))
	x := dimensions.Size.X - gtx.Dp(4) - width
	if x < 0 {
		x = 0
	}
	kit.FillCut(gtx.Ops, image.Rect(x, top, x+width, top+height), min(width/2, height/2), palette.WithAlpha(th.R.Fg3, alpha))
	return dimensions
}

// AnchorTo places index at offsetPx from the viewport top.
func (s *Scroller) AnchorTo(index, offsetPx int) {
	s.stopSpring()
	s.Position.First = max(index, 0)
	s.Position.Offset = -offsetPx
	s.Position.BeforeEnd = true
	s.fraction = 0
	s.programmatic = true
}

// ScrollTo places index at the start of the viewport.
func (s *Scroller) ScrollTo(index int) {
	s.AnchorTo(index, 0)
}

// SpringBy glides content by deltaPx; positive values move content up.
func (s *Scroller) SpringBy(deltaPx int) {
	s.stopSpring()
	s.spring = anim.Spring{Omega: 4 / .240, Rest: .25}
	s.spring.Snap(0)
	s.spring.Set(float32(deltaPx))
	s.springApplied = 0
	s.springActive = s.spring.Moving()
	s.programmatic = true
}

// SpringTo glides the current first item's pixel offset to px.
func (s *Scroller) SpringTo(px int) {
	s.SpringBy(px - s.Position.Offset)
}

// UserScrolled reports and clears whether real input moved the list.
func (s *Scroller) UserScrolled() bool {
	scrolled := s.userScrolled
	s.userScrolled = false
	return scrolled
}

// Offset returns the first visible item and its pixel offset.
func (s *Scroller) Offset() (first, px int) {
	return s.Position.First, s.Position.Offset
}
