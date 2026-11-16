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

	glide         anim.Tween
	glideActive   bool
	glidePending  bool
	glideApplied  float32
	glideDistance float32
	glideTarget   int
	glideDuration time.Duration

	// viewport is the list's height at the last Layout, for Progress
	viewport int
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
	s.stopGlide()
	if !s.springActive {
		return
	}
	s.spring.Snap(s.springApplied)
	s.springActive = false
}

func (s *Scroller) stopGlide() {
	s.glideActive, s.glidePending = false, false
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
	s.inputApplied += s.advanceGlide(gtx)
}

// startGlide measures the way from the current position to the target's
// start with the rows' real sizes (an average for rows not laid out yet) and
// starts the glide over it.
func (s *Scroller) startGlide(gtx layout.Context) {
	s.glidePending = false
	target := max(s.glideTarget, 0)
	if s.itemCount > 0 {
		target = min(target, s.itemCount-1)
	}
	fallback := max(1, s.Position.Length/max(s.itemCount, 1))
	distance := listPositionDelta(s.Position, layout.Position{First: target}, s.itemSizes, fallback, s.Gap, layout.Vertical)
	if anim.Reduced() || s.glideDuration <= 0 || distance == 0 || !s.cacheReady {
		s.anchor(target, 0)
		return
	}
	s.glideTarget = target
	s.glide = anim.Tween{Duration: s.glideDuration, Ease: anim.EaseOutCubic}
	s.glide.Set(gtx, 0)
	s.glide.Set(gtx, float32(distance))
	s.glideApplied, s.glideDistance = 0, float32(distance)
	s.glideActive = true
}

func (s *Scroller) advanceGlide(gtx layout.Context) int {
	if s.glidePending {
		s.startGlide(gtx)
	}
	if !s.glideActive {
		return 0
	}
	value := s.glide.Value(gtx)
	delta := value - s.glideApplied
	s.glideApplied = value
	s.programmatic = true
	if value == s.glideDistance {
		// land exactly, whatever the estimate of rows not yet laid out
		s.glideActive = false
		s.anchor(s.glideTarget, 0)
		return 1
	}
	return addListOffset(&s.Position, &s.fraction, delta)
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
	if s.glidePending {
		// a GlideTo asked for after this frame's input still starts this frame
		s.inputApplied += s.advanceGlide(gtx)
	}
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
	s.viewport = dimensions.Size.Y
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
	kit.Fill(gtx, th.Theme, image.Rect(x, top, x+width, top+height), min(width/2, height/2), palette.WithAlpha(th.R.Fg3, alpha))
	return dimensions
}

// AnchorTo places index at offsetPx from the viewport top.
func (s *Scroller) AnchorTo(index, offsetPx int) {
	s.stopSpring()
	s.anchor(index, offsetPx)
}

// anchor places index at offsetPx without stopping a glide in progress.
func (s *Scroller) anchor(index, offsetPx int) {
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

// GlideTo moves index to the top of the viewport over duration on OutCubic
// (the Hub moves a jump over th.Motion.Move), through the list itself: each
// frame moves the offset by the eased step, so the list lays out every row it
// passes at its real size and skips none, and the last frame anchors index
// exactly. Under reduced motion, or with no duration, it anchors at once.
// Real input stops it where it stands. Ask before the scroller's Layout in a
// frame (a kit click already asks for the frame that paints it).
func (s *Scroller) GlideTo(index int, duration time.Duration) {
	s.stopSpring()
	s.glideTarget, s.glideDuration, s.glidePending = index, duration, true
	s.programmatic = true
}

// Gliding reports whether a GlideTo is under way.
func (s *Scroller) Gliding() bool { return s.glideActive || s.glidePending }

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

// Progress is how far the list is read, 0 at the top to 1 at the end, for
// the masthead's ruler marker, measured with the rows' real sizes (an
// average for rows not laid out yet). It is -1 while the content fits the
// viewport and there is nothing to scroll. It allocates nothing.
func (s *Scroller) Progress() float32 {
	if !s.cacheReady || s.itemCount <= 0 || s.viewport <= 0 {
		return -1
	}
	if s.Position.First == 0 && s.Position.Offset <= 0 && !s.Position.BeforeEnd {
		return -1
	}
	if !s.Position.BeforeEnd {
		return 1
	}
	fallback := max(1, s.Position.Length/max(s.itemCount, 1))
	total := 0
	for index := range s.itemCount {
		total += itemExtent(s.itemSizes, index, fallback, layout.Vertical)
	}
	total += s.Gap * max(0, s.itemCount-1)
	scrollable := total - s.viewport
	if scrollable <= 0 {
		return -1
	}
	read := listPositionDelta(layout.Position{}, s.Position, s.itemSizes, fallback, s.Gap, layout.Vertical)
	return min(max(float32(read)/float32(scrollable), 0), 1)
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
