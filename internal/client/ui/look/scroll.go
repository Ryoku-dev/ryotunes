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

	tag        struct{}
	fraction   float32
	hovered    bool
	lastScroll time.Time
	thumb      anim.Tween
	thumbOn    bool
	thumbReady bool

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
	if s.springActive {
		s.spring.Snap(float32(s.Position.Offset))
		s.springApplied = float32(s.Position.Offset)
		s.springActive = false
	}
}

func (s *Scroller) updateInput(gtx layout.Context) int {
	filter := pointer.Filter{
		Target:  &s.tag,
		Kinds:   pointer.Enter | pointer.Leave | pointer.Scroll,
		ScrollY: verticalScrollRange(s.Position),
	}
	applied := 0
	for {
		raw, ok := gtx.Event(filter)
		if !ok {
			break
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
		case pointer.Scroll:
			s.stopSpring()
			s.lastScroll = gtx.Now
			gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(thumbIdle)})
			applied += addListOffset(&s.Position, &s.fraction, mapScrollDelta(e.Scroll.Y, gtx.Metric.PxPerDp, scrollOnX11))
		}
	}
	return applied
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
	before := s.Position
	applied := s.updateInput(gtx)
	applied += s.advanceSpring(gtx)
	dimensions := s.List.Layout(gtx, count, el)

	// The catcher must not hide row handlers drawn before it.
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: dimensions.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &s.tag)
	area.Pop()
	pass.Pop()

	if applied != 0 && sameListPosition(before, s.Position) {
		s.fraction = 0
		s.stopSpring()
	}

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

// ScrollTo places index at the start of the viewport.
func (s *Scroller) ScrollTo(index int) {
	s.stopSpring()
	s.Position.First = max(index, 0)
	s.Position.Offset = 0
	s.Position.BeforeEnd = true
	s.fraction = 0
}

// SpringTo glides the current first item's pixel offset to px.
func (s *Scroller) SpringTo(px int) {
	current := float32(s.Position.Offset)
	s.spring = anim.Spring{Omega: 4 / .240, Rest: .25}
	s.spring.Snap(current)
	s.spring.Set(float32(px))
	s.springApplied = current
	s.springActive = s.spring.Moving()
}

// Offset returns the first visible item and its pixel offset.
func (s *Scroller) Offset() (first, px int) {
	return s.Position.First, s.Position.Offset
}
