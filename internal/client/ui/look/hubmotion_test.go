package look

import (
	"image"
	"math"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/symbols"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
)

type motionHarness struct {
	t      *testing.T
	ops    op.Ops
	router input.Router
	now    time.Time
	theme  *Theme
	size   image.Point
}

func newMotionHarness(t *testing.T) *motionHarness {
	t.Helper()
	anim.SetReduce(false)
	return &motionHarness{t: t, now: time.Unix(5000, 0), theme: NewTheme(palette.Signature(true), libtheme.Comfortable), size: image.Pt(400, 300)}
}

func (h *motionHarness) frame(draw func(gtx layout.Context)) {
	h.ops.Reset()
	gtx := layout.Context{
		Ops: &h.ops, Source: h.router.Source(), Now: h.now,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(h.size),
	}
	draw(gtx)
	h.router.Frame(&h.ops)
}

func (h *motionHarness) advance(d time.Duration) { h.now = h.now.Add(d) }

// settled asserts that, past every motion, a frame asks for no other frame.
func (h *motionHarness) settled(name string, draw func(gtx layout.Context)) {
	h.t.Helper()
	h.advance(2 * time.Second)
	h.frame(draw)
	h.router.WakeupTime()
	h.advance(time.Millisecond)
	h.frame(draw)
	if wake, ok := h.router.WakeupTime(); ok {
		h.t.Fatalf("%s at rest still asks for a frame at %v", name, wake)
	}
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

func TestEaseOutBackOvershootsAndLands(t *testing.T) {
	for name, ease := range map[string]func(float32) float32{"chevron": EaseOutBackChevron, "node": EaseOutBackNode} {
		if ease(0) != 0 || ease(1) != 1 {
			t.Fatalf("%s ends at %v and %v, want 0 and 1", name, ease(0), ease(1))
		}
		peak := float32(0)
		for step := 1; step < 100; step++ {
			peak = max(peak, ease(float32(step)/100))
		}
		if peak <= 1 {
			t.Fatalf("%s never passes its target (peak %v)", name, peak)
		}
	}
	if EaseOutBackNode(.7) <= EaseOutBackChevron(.7) {
		t.Fatal("the node's overshoot 2 should pass further than the chevron's 1.6")
	}
}

func TestRowWashHubTimingsLeanAndPress(t *testing.T) {
	h := newMotionHarness(t)
	var wash RowWash
	hovered, pressed := false, false
	var hover, press float32
	draw := func(gtx layout.Context) {
		hover, press = wash.Update(gtx, h.theme, hovered, pressed)
		wash.Paint(gtx, h.theme, image.Rect(0, 0, 200, 34), 6)
	}
	h.frame(draw)
	if hover != 0 || press != 0 {
		t.Fatalf("a row at rest stands at hover %v press %v", hover, press)
	}
	hovered = true
	h.frame(draw)
	h.advance(h.theme.Motion.Quick / 2)
	h.frame(draw)
	if hover <= 0 || hover >= 1 {
		t.Fatalf("hover halfway in = %v", hover)
	}
	h.advance(h.theme.Motion.Quick / 2)
	h.frame(draw)
	if hover != 1 || wash.Lean(layout.Context{Metric: unit.Metric{PxPerDp: 1}}) != 2 {
		t.Fatalf("hover in over Quick = %v lean %d, want 1 and 2", hover, wash.Lean(layout.Context{Metric: unit.Metric{PxPerDp: 1}}))
	}
	pressed = true
	h.frame(draw)
	h.advance(h.theme.Motion.Press)
	h.frame(draw)
	if press != 1 || !near(wash.GlyphScale(), glyphPressed) {
		t.Fatalf("pressed = %v glyph %v, want 1 and .86", press, wash.GlyphScale())
	}
	if ink := wash.Ink(h.theme); ink != h.theme.R.Press() {
		t.Fatalf("pressed ink %v, want the press wash %v", ink, h.theme.R.Press())
	}
	hovered, pressed = false, false
	h.frame(draw)
	h.advance(h.theme.Motion.Quick)
	h.frame(draw)
	if hover <= 0 {
		t.Fatal("the hover left within Quick; it leaves over Move")
	}
	h.advance(h.theme.Motion.Move)
	h.frame(draw)
	if hover != 0 {
		t.Fatalf("hover out over Move = %v, want 0", hover)
	}
	h.settled("row wash", draw)
	allocs := testing.AllocsPerRun(20, func() { h.frame(draw) })
	if allocs != 0 {
		t.Fatalf("a row wash at rest allocates %v times a frame", allocs)
	}

	anim.SetReduce(true)
	defer anim.SetReduce(false)
	hovered = true
	h.frame(draw)
	if hover != 1 {
		t.Fatalf("reduced hover = %v, want 1 at once", hover)
	}
}

func TestSelectWashStandsTravelsAndLands(t *testing.T) {
	h := newMotionHarness(t)
	var wash SelectWash
	target, group := image.Rect(0, 0, 200, 34), 0
	var rect image.Rectangle
	var opacity float32
	draw := func(gtx layout.Context) {
		rect, opacity = wash.Step(gtx, h.theme, target, group)
		wash.Layout(gtx, h.theme, target, group, 6)
	}
	h.frame(draw)
	if rect != target || opacity != 1 {
		t.Fatalf("first placement = %v at %v, want standing at %v", rect, opacity, target)
	}
	target = image.Rect(0, 68, 200, 102)
	h.frame(draw)
	h.advance(h.theme.Motion.Move / 2)
	h.frame(draw)
	if rect.Min.Y <= 0 || rect.Min.Y >= 68 || opacity != 1 {
		t.Fatalf("travelling halfway = %v at %v, want between the lines at full", rect, opacity)
	}
	h.advance(h.theme.Motion.Move / 2)
	h.frame(draw)
	if rect != target {
		t.Fatalf("travelled to %v, want %v", rect, target)
	}

	// another group: stand at the line and land
	target, group = image.Rect(0, 200, 200, 230), 1
	h.advance(time.Millisecond)
	h.frame(draw)
	if rect.Min.Y != 200 || opacity != 0 || rect.Dx() >= 200 || rect.Dx() < 100 {
		t.Fatalf("landing start = %v at %v, want at the new line, faded, narrower", rect, opacity)
	}
	h.advance(h.theme.Motion.Move)
	h.frame(draw)
	if opacity != 1 || rect.Dx() >= 200 {
		t.Fatalf("landing at Move = %v at %v, want opaque and still widening", rect, opacity)
	}
	h.advance(selectLandScale - h.theme.Motion.Move)
	h.frame(draw)
	if rect != target {
		t.Fatalf("landed at %v, want %v", rect, target)
	}
	h.settled("selection wash", draw)

	anim.SetReduce(true)
	defer anim.SetReduce(false)
	target = image.Rect(0, 100, 200, 134)
	h.frame(draw)
	if rect != target {
		t.Fatalf("reduced selection = %v, want %v at once", rect, target)
	}
}

func TestPageRiseSlidesInOnAKeyChangeOnly(t *testing.T) {
	h := newMotionHarness(t)
	var rise PageRise
	key := "home"
	var drawn layout.Dimensions
	page := func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(100, 100)} }
	draw := func(gtx layout.Context) { drawn = rise.Layout(gtx, key, page) }
	h.frame(draw)
	if rise.Active() {
		t.Fatal("the first page rose")
	}
	key = "library"
	h.frame(draw)
	if !rise.Active() {
		t.Fatal("a new page did not rise")
	}
	h.advance(anim.RiseSlide / 2)
	h.frame(draw)
	if !rise.Active() || drawn.Size.X != 100 {
		t.Fatalf("page rise halfway: active %v size %v", rise.Active(), drawn.Size)
	}
	h.settled("page rise", draw)
	if rise.Active() {
		t.Fatal("the page is still rising after 2 s")
	}
	anim.SetReduce(true)
	defer anim.SetReduce(false)
	key = "radio"
	h.frame(draw)
	if rise.Active() {
		t.Fatal("a page rose under reduced motion")
	}
}

func TestDrawerSlidesOverScrimAndDismisses(t *testing.T) {
	h := newMotionHarness(t)
	var drawer Drawer
	open := false
	draw := func(gtx layout.Context) {
		drawer.Layout(gtx, h.theme, open, DrawerRight, 160, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
	}
	h.frame(draw)
	if drawer.Visible() {
		t.Fatal("a closed drawer shows")
	}
	h.settled("closed drawer", draw)
	open = true
	h.frame(draw)
	h.advance(h.theme.Motion.Move / 2)
	h.frame(draw)
	if drawer.progress <= 0 || drawer.progress >= 1 {
		t.Fatalf("drawer halfway = %v", drawer.progress)
	}
	h.advance(h.theme.Motion.Move / 2)
	h.frame(draw)
	if drawer.progress != 1 {
		t.Fatalf("drawer open over Move = %v", drawer.progress)
	}
	// a press on the sheet stays in it
	press := func(x float32) {
		h.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, PointerID: 1, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, 50)})
		h.frame(draw)
		h.router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, PointerID: 1, Position: f32.Pt(x, 50)})
		h.frame(draw)
	}
	press(350)
	if drawer.Dismissed() {
		t.Fatal("a press on the sheet dismissed the drawer")
	}
	press(50)
	if !drawer.Dismissed() {
		t.Fatal("a press on the scrim did not dismiss the drawer")
	}
	h.settled("open drawer", draw)

	anim.SetReduce(true)
	defer anim.SetReduce(false)
	open = false
	h.frame(draw)
	if drawer.Visible() {
		t.Fatal("reduced motion left the drawer sliding")
	}
}

func TestChevronTurnsOnOutBackAndCloseGlyphTurns(t *testing.T) {
	h := newMotionHarness(t)
	var chevron Chevron
	var closing CloseTurn
	open, hovered, pressed := false, false, false
	var angle, closeAngle, closeScale float32
	icon, _ := widget.NewIcon(symbols.Close)
	draw := func(gtx layout.Context) {
		angle = chevron.Angle(gtx, h.theme, open)
		closeAngle, closeScale = closing.Update(gtx, h.theme, hovered, pressed)
		DrawGlyph(gtx, icon, h.theme.R.Fg, 18, closeAngle, closeScale)
	}
	h.frame(draw)
	if angle != 0 || closeAngle != 0 || closeScale != 1 {
		t.Fatalf("rest: chevron %v close %v scale %v", angle, closeAngle, closeScale)
	}
	open, hovered = true, true
	h.frame(draw)
	peak := float32(0)
	for range 24 {
		h.advance(h.theme.Motion.Move / 24)
		h.frame(draw)
		peak = max(peak, angle)
	}
	if peak <= 90 || angle != 90 {
		t.Fatalf("chevron peak %v end %v, want past 90 then 90", peak, angle)
	}
	if closeAngle != 90 {
		t.Fatalf("close glyph turned %v on hover, want 90", closeAngle)
	}
	pressed = true
	h.frame(draw)
	h.advance(h.theme.Motion.Quick)
	h.frame(draw)
	if !near(closeScale, glyphPressedButton) {
		t.Fatalf("pressed close glyph scale %v, want .85", closeScale)
	}
	h.settled("chevron and close glyph", draw)

	anim.SetReduce(true)
	defer anim.SetReduce(false)
	open, hovered, pressed = false, true, false
	h.frame(draw)
	if angle != 0 || closeAngle != 0 || closeScale != 1 {
		t.Fatalf("reduced: chevron %v close %v scale %v, want closed, unturned, full size at once", angle, closeAngle, closeScale)
	}
}

func TestCountRollAndCountUp(t *testing.T) {
	h := newMotionHarness(t)
	var roll CountRoll
	var up CountUp
	value, ready := "03", false
	var fraction float32
	draw := func(gtx layout.Context) {
		roll.Layout(gtx, h.theme, h.theme.T.MonoSm, h.theme.R.Fg3, value)
		fraction = up.Fraction(gtx, h.theme, ready)
	}
	h.frame(draw)
	if roll.Rolling() || fraction != 0 {
		t.Fatal("the first value rolled or the count ran before its value")
	}
	value, ready = "04", true
	h.frame(draw)
	if !roll.Rolling() {
		t.Fatal("a changed count did not roll")
	}
	h.advance(h.theme.Motion.Move / 2)
	h.frame(draw)
	if !roll.Rolling() || fraction <= 0 || fraction >= 1 {
		t.Fatalf("halfway: rolling %v count %v", roll.Rolling(), fraction)
	}
	if got := up.Of(120); got <= 0 || got >= 120 {
		t.Fatalf("counting reading = %v, want between 0 and 120", got)
	}
	h.advance(h.theme.Motion.CountUp)
	h.frame(draw)
	if roll.Rolling() || fraction != 1 || up.Of(120) != 120 {
		t.Fatalf("settled: rolling %v count %v", roll.Rolling(), fraction)
	}
	h.settled("count", draw)
	// a later refresh never counts again
	ready = false
	h.frame(draw)
	if fraction != 1 {
		t.Fatalf("count after refresh = %v, want it to stay up", fraction)
	}

	anim.SetReduce(true)
	defer anim.SetReduce(false)
	value = "05"
	h.frame(draw)
	if roll.Rolling() {
		t.Fatal("reduced motion rolled the count")
	}
}

func TestScrollerGlideToPassesEveryRowAndLandsExactly(t *testing.T) {
	h := newMotionHarness(t)
	var scroller Scroller
	heights := make([]int, 60)
	for index := range heights {
		heights[index] = 40 + 40*(index%3/2)
	}
	seen := map[int]bool{}
	draw := func(gtx layout.Context) {
		scroller.Layout(gtx, h.theme, len(heights), func(gtx layout.Context, index int) layout.Dimensions {
			seen[index] = true
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, heights[index])}
		})
	}
	absolute := func() int {
		first, offset := scroller.Offset()
		y := offset
		for index := range first {
			y += heights[index]
		}
		return y
	}
	h.frame(draw)
	h.frame(draw)
	scroller.GlideTo(40, h.theme.Motion.Move)
	clear(seen)
	last := absolute()
	frames := 0
	for elapsed := time.Duration(0); elapsed <= h.theme.Motion.Move+20*time.Millisecond; elapsed += 6 * time.Millisecond {
		h.frame(draw)
		if y := absolute(); y < last {
			t.Fatalf("glide went back from %d to %d", last, y)
		} else {
			last = y
		}
		frames++
		h.advance(6 * time.Millisecond)
	}
	if first, offset := scroller.Offset(); first != 40 || offset != 0 || scroller.Gliding() {
		t.Fatalf("glide landed at row %d offset %d (gliding %v), want row 40 at the top", first, offset, scroller.Gliding())
	}
	for index := range 41 {
		if !seen[index] {
			t.Fatalf("the glide skipped row %d", index)
		}
	}
	if frames < 30 {
		t.Fatalf("glide took %d frames", frames)
	}
	h.settled("glided scroller", draw)

	// real input stops a glide where it stands
	scroller.GlideTo(0, h.theme.Motion.Move)
	h.frame(draw)
	h.advance(20 * time.Millisecond)
	h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(100, 100), Scroll: f32.Pt(0, 30)})
	h.frame(draw)
	h.frame(draw)
	if scroller.Gliding() {
		t.Fatal("a wheel turn did not stop the glide")
	}

	anim.SetReduce(true)
	defer anim.SetReduce(false)
	scroller.GlideTo(10, h.theme.Motion.Move)
	h.frame(draw)
	if first, offset := scroller.Offset(); first != 10 || offset != 0 {
		t.Fatalf("reduced glide stands at row %d offset %d, want row 10 at once", first, offset)
	}
}

func TestScrollerProgressReadsThePage(t *testing.T) {
	h := newMotionHarness(t)
	var scroller Scroller
	count := 3
	draw := func(gtx layout.Context) {
		scroller.Layout(gtx, h.theme, count, func(gtx layout.Context, index int) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 50)}
		})
	}
	h.frame(draw)
	if got := scroller.Progress(); got != -1 {
		t.Fatalf("content that fits reads %v, want -1", got)
	}
	count = 40
	h.frame(draw)
	h.frame(draw)
	if got := scroller.Progress(); got != 0 {
		t.Fatalf("top of a long list reads %v, want 0", got)
	}
	scroller.AnchorTo(17, 0) // 850 of 2000 - 300
	h.frame(draw)
	if got := scroller.Progress(); !near(got, .5) {
		t.Fatalf("middle reads %v, want .5", got)
	}
	scroller.AnchorTo(39, 0)
	h.frame(draw)
	if got := scroller.Progress(); got != 1 {
		t.Fatalf("end reads %v, want 1", got)
	}
	if allocs := testing.AllocsPerRun(20, func() { _ = scroller.Progress() }); allocs != 0 {
		t.Fatalf("Progress allocates %v times", allocs)
	}
}

func TestRowWashReversedMidwayIsContinuous(t *testing.T) {
	h := newMotionHarness(t)
	var wash RowWash
	hovered := false
	var hover float32
	draw := func(gtx layout.Context) { hover, _ = wash.Update(gtx, h.theme, hovered, false) }
	h.frame(draw)
	hovered = true
	h.frame(draw)
	h.advance(h.theme.Motion.Quick / 2)
	h.frame(draw)
	before := hover
	if before <= 0 || before >= 1 {
		t.Fatalf("hover halfway in = %v", before)
	}
	// leave at the same instant: the way out starts where the way in stands
	hovered = false
	h.frame(draw)
	if !near(hover, before) {
		t.Fatalf("reversing mid-way jumped from %v to %v", before, hover)
	}
	h.advance(time.Millisecond)
	h.frame(draw)
	if hover >= before || before-hover > .05 {
		t.Fatalf("a millisecond into the way out: %v from %v, want a small step down", hover, before)
	}
	// and back in again
	h.advance(h.theme.Motion.Move / 4)
	h.frame(draw)
	out := hover
	hovered = true
	h.frame(draw)
	if !near(hover, out) {
		t.Fatalf("reversing the way out jumped from %v to %v", out, hover)
	}
}
