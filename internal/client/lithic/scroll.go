package lithic

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
	"gioui.org/widget"
)

const (
	wheelStepDP        = 120
	notchScrollTime    = 140 * time.Millisecond
	scrollRangeLimit   = 1 << 30
	notchDecayDivisor  = 5
	scrollFloatEpsilon = 0.001
)

var scrollOnX11 = os.Getenv("WAYLAND_DISPLAY") == ""

// List is a vertical widget.List with Lithic's thin scroll thumb. It owns
// wheel and touchpad scroll events so Wayland surface-pixel deltas can be
// converted to the buffer-pixel offsets used by Gio's list.
type List struct {
	widget.List

	scrollTag  uint8
	pending    float32
	fraction   float32
	lastDrain  time.Time
	drainUntil time.Time
}

func mapScrollDelta(delta, pxPerDP float32, x11 bool) (pixels float32, notch bool) {
	notchSize := float32(150)
	if x11 {
		notchSize = 10
	}
	notches := float32(math.Round(float64(delta / notchSize)))
	if notches != 0 && float32(math.Abs(float64(delta-notches*notchSize))) <= 0.5 {
		return notches * wheelStepDP * pxPerDP, true
	}
	return delta * pxPerDP, false
}

func (l *List) scrollRange() pointer.ScrollRange {
	var r pointer.ScrollRange
	if l.Position.First > 0 || l.Position.Offset > 0 {
		r.Min = -scrollRangeLimit
	}
	if l.Position.BeforeEnd {
		r.Max = scrollRangeLimit
	}
	return r
}

func (l *List) addOffset(delta float32) int {
	if delta < 0 {
		l.Position.BeforeEnd = true
	}
	total := l.fraction + delta
	whole := int(total)
	l.fraction = total - float32(whole)
	l.Position.Offset += whole
	return whole
}

func (l *List) queueNotch(gtx layout.Context, pixels float32) {
	l.pending += pixels
	if float32(math.Abs(float64(l.pending))) < scrollFloatEpsilon {
		l.clearPending()
		return
	}
	if l.lastDrain.IsZero() {
		l.lastDrain = gtx.Now
	}
	l.drainUntil = gtx.Now.Add(notchScrollTime)
}

func (l *List) clearPending() {
	l.pending = 0
	l.lastDrain = time.Time{}
	l.drainUntil = time.Time{}
}

func (l *List) drainNotches(gtx layout.Context) int {
	if l.pending == 0 {
		return 0
	}
	if Reduced() || !gtx.Now.Before(l.drainUntil) {
		delta := l.pending
		l.clearPending()
		return l.addOffset(delta)
	}
	if l.lastDrain.IsZero() || !gtx.Now.After(l.lastDrain) {
		l.lastDrain = gtx.Now
		gtx.Execute(op.InvalidateCmd{})
		return 0
	}

	elapsed := gtx.Now.Sub(l.lastDrain)
	tau := float64(notchScrollTime) / notchDecayDivisor
	portion := float32(1 - math.Exp(-float64(elapsed)/tau))
	delta := l.pending * portion
	l.pending -= delta
	l.lastDrain = gtx.Now
	applied := l.addOffset(delta)
	if l.pending != 0 {
		gtx.Execute(op.InvalidateCmd{})
	}
	return applied
}

func (l *List) updateScroll(gtx layout.Context) (layout.Position, int) {
	before := l.Position
	filter := pointer.Filter{
		Target:  &l.scrollTag,
		Kinds:   pointer.Scroll,
		ScrollY: l.scrollRange(),
	}
	applied := 0
	for {
		event, ok := gtx.Event(filter)
		if !ok {
			break
		}
		e, ok := event.(pointer.Event)
		if !ok {
			continue
		}
		pixels, notch := mapScrollDelta(e.Scroll.Y, gtx.Metric.PxPerDp, scrollOnX11)
		if notch && !Reduced() {
			l.queueNotch(gtx, pixels)
			continue
		}
		applied += l.addOffset(pixels)
	}
	applied += l.drainNotches(gtx)
	return before, applied
}

func sameListPosition(a, b layout.Position) bool {
	return a.First == b.First && a.Offset == b.Offset
}

func scrollThumb(viewport, content, first, offset, count int) (top, height int, ok bool) {
	if viewport <= 0 || content <= viewport || count <= 0 {
		return 0, 0, false
	}
	height = max(12, viewport*viewport/content)
	height = min(viewport, height)
	travel := viewport - height
	average := float64(content) / float64(count)
	scrolled := float64(first)*average + float64(offset)
	maxScroll := float64(content - viewport)
	if maxScroll > 0 {
		top = int(math.Round(float64(travel) * max(0, min(1, scrolled/maxScroll))))
	}
	return top, height, true
}

// Layout renders a vertical list with wheel glide and a vein thumb.
func (l *List) Layout(gtx layout.Context, th *Theme, n int, el layout.ListElement) layout.Dimensions {
	l.Axis = layout.Vertical
	before, applied := l.updateScroll(gtx)
	dims := l.List.Layout(gtx, n, el)

	// The catcher sits above the rows, so it must pass presses through: a
	// non-pass area would hide every row from the pointer. Gio hands a scroll
	// to the topmost handler first and only the part outside its range goes
	// further, so the list still gets every scroll it can use.
	pass := pointer.PassOp{}.Push(gtx.Ops)
	area := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	event.Op(gtx.Ops, &l.scrollTag)
	area.Pop()
	pass.Pop()

	if applied != 0 && sameListPosition(before, l.Position) && l.pending != 0 {
		l.clearPending()
		l.fraction = 0
	}

	top, thumbH, ok := scrollThumb(dims.Size.Y, l.Position.Length, l.Position.First, l.Position.Offset, n)
	if !ok {
		return dims
	}
	width := max(1, gtx.Dp(3))
	x := dims.Size.X - width
	cut := min(width/2, thumbH/2)
	FillCut(gtx.Ops, image.Rect(x, top, x+width, top+thumbH), cut, th.R.Vein)
	return dims
}
