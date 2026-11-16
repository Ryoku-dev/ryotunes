package chrome

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/state"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// playerHarness runs frames through a real input router at a fake clock, so
// a test can see whether a settled motion still asks for frames.
type playerHarness struct {
	t      *testing.T
	ops    op.Ops
	router input.Router
	now    time.Time
	theme  *look.Theme
	wanted bool
}

func newPlayerHarness(t *testing.T) *playerHarness {
	t.Helper()
	anim.SetReduce(false)
	t.Cleanup(func() { anim.SetReduce(false) })
	return &playerHarness{t: t, now: time.Unix(7000, 0), theme: look.NewTheme(palette.Signature(true), libtheme.Comfortable)}
}

func (h *playerHarness) frame(draw func(gtx layout.Context)) {
	h.ops.Reset()
	gtx := layout.Context{
		Ops: &h.ops, Source: h.router.Source(), Now: h.now,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 300)),
	}
	draw(gtx)
	h.router.Frame(&h.ops)
	// the router keeps a wakeup until it is read: read it per frame
	_, h.wanted = h.router.WakeupTime()
}

func (h *playerHarness) advance(d time.Duration) { h.now = h.now.Add(d) }

// wants reports whether the last frame asked for another.
func (h *playerHarness) wants() bool { return h.wanted }

func TestHeartPopsOnLikeOnlyAndSettles(t *testing.T) {
	h := newPlayerHarness(t)
	var glyph heart
	var scale float32
	draw := func(liked bool) func(layout.Context) {
		return func(gtx layout.Context) { scale = glyph.scale(gtx, h.theme, liked) }
	}

	h.frame(draw(false))
	if scale != 1 || h.wants() {
		t.Fatalf("a first heart stands at scale %v and asks for frames: %v", scale, h.wants())
	}

	// a like pops: the glyph grows past its size, then settles back
	h.frame(draw(true))
	peak := scale
	for range 20 {
		h.advance(16 * time.Millisecond)
		h.frame(draw(true))
		peak = max(peak, scale)
	}
	if peak < 1.15 {
		t.Fatalf("a like should pop the heart (peak %v)", peak)
	}
	h.advance(h.theme.Motion.Field)
	h.frame(draw(true))
	h.advance(time.Millisecond)
	h.frame(draw(true))
	if scale != 1 || h.wants() {
		t.Fatalf("a settled pop stands at %v and asks for frames: %v", scale, h.wants())
	}

	// an unlike only empties the heart; the glyph keeps its size
	for range 20 {
		h.frame(draw(false))
		if scale != 1 {
			t.Fatalf("an unlike scaled the heart to %v", scale)
		}
		h.advance(16 * time.Millisecond)
	}
}

func TestHeartPopIsImmediateUnderReducedMotion(t *testing.T) {
	h := newPlayerHarness(t)
	var glyph heart
	h.frame(func(gtx layout.Context) { glyph.scale(gtx, h.theme, false) })
	anim.SetReduce(true)
	var scale float32
	h.frame(func(gtx layout.Context) { scale = glyph.scale(gtx, h.theme, true) })
	if scale != 1 || h.wants() {
		t.Fatalf("reduced motion should not pop (scale %v, wants frames %v)", scale, h.wants())
	}
}

func TestVolumeChasesOutsideChangesAndTracksThePointerExactly(t *testing.T) {
	h := newPlayerHarness(t)
	var volume volumeControl
	var shown float32
	draw := func(gtx layout.Context) { shown = volume.shownValue(gtx) }

	volume.target = 40
	h.frame(draw)
	if shown != .4 || h.wants() {
		t.Fatalf("the first volume should stand at .4, got %v (wants frames %v)", shown, h.wants())
	}

	// another client sets 90: the handle follows, between the two at first
	volume.target = 90
	h.frame(draw)
	h.advance(16 * time.Millisecond)
	h.frame(draw)
	if shown <= .4 || shown >= .9 {
		t.Fatalf("an outside change should be chased, got %v one frame in", shown)
	}
	if !h.wants() {
		t.Fatal("a chase in progress must ask for the next frame")
	}
	for range 30 {
		h.advance(16 * time.Millisecond)
		h.frame(draw)
		if shown > .9 {
			t.Fatalf("the chase overshot to %v", shown)
		}
	}
	h.advance(time.Millisecond)
	h.frame(draw)
	if shown != .9 || h.wants() {
		t.Fatalf("a landed chase stands at the target (%v) and asks for no frame (%v)", shown, h.wants())
	}

	// under the pointer the value is the hand's own, at once
	volume.dragging = true
	volume.target = 12
	h.advance(16 * time.Millisecond)
	h.frame(draw)
	if shown != .12 {
		t.Fatalf("a drag must show the pointer's value exactly, got %v", shown)
	}
}

func TestPlayerFactsBuildOnlyOnChange(t *testing.T) {
	var facts playerFacts
	facts.update(model.ProviderYouTube, 3, 12)
	if facts.name != "YouTube Music" || facts.place != "04/12" {
		t.Fatalf("facts = %q %q, want YouTube Music 04/12", facts.name, facts.place)
	}
	key := facts.key
	allocs := testing.AllocsPerRun(100, func() { facts.update(model.ProviderYouTube, 3, 12) })
	if allocs != 0 || facts.key != key {
		t.Fatalf("unchanged facts rebuilt (%v allocations)", allocs)
	}
	facts.update(model.ProviderRadio, 0, 1)
	if facts.place != "" {
		t.Fatalf("a radio station has no place in a queue, got %q", facts.place)
	}
	facts.update(model.ProviderSpotify, -1, 0)
	if facts.place != "" || facts.name != "Spotify" {
		t.Fatalf("no queue means no place, got %q %q", facts.name, facts.place)
	}
}

func TestFactsLineDropsThePlaceBeforeOverflowing(t *testing.T) {
	h := newPlayerHarness(t)
	measure := func(width int) (size image.Point) {
		h.frame(func(gtx layout.Context) {
			gtx.Constraints = layout.Constraints{Max: image.Pt(width, 100)}
			size = layoutFacts(gtx, h.theme, model.ProviderYouTube, "YouTube Music", "04/12").Size
		})
		return size
	}
	wide := measure(600)
	nameOnly := func() (size image.Point) {
		h.frame(func(gtx layout.Context) {
			gtx.Constraints = layout.Constraints{Max: image.Pt(600, 100)}
			size = layoutFacts(gtx, h.theme, model.ProviderYouTube, "YouTube Music", "").Size
		})
		return size
	}()
	if wide.X <= nameOnly.X {
		t.Fatalf("a wide bar shows the place too (%d against the name's %d)", wide.X, nameOnly.X)
	}
	narrow := measure(wide.X - 1)
	if narrow.X != nameOnly.X {
		t.Fatalf("one pixel short, the place should go and the name stand alone (%d, want %d)", narrow.X, nameOnly.X)
	}
	if tiny := measure(40); tiny.X > 40 {
		t.Fatalf("the facts drew %d px into a 40 px line", tiny.X)
	}
}

func TestQueueEntriesNumberRowsWithinTheirGroups(t *testing.T) {
	h := newPlayerHarness(t)
	var queue Queue
	row := func(id string, index int) state.QueueRow {
		return state.QueueRow{Index: index, Item: model.Track{VideoID: id}}
	}
	now := row("a", 0)
	groups := state.QueueGroups{
		NowPlaying:   &now,
		UpNextManual: []state.QueueRow{row("b", 1), row("c", 2)},
		Continuation: []state.QueueRow{row("d", 3)},
	}
	h.frame(func(gtx layout.Context) { queue.buildEntries(gtx, h.theme.Motion.Notch, groups) })
	type want struct {
		kind    queueEntryKind
		ordinal int
		last    bool
	}
	wants := []want{
		{queueSection, 0, false}, {queueTrack, 1, true},
		{queueSection, 0, false}, {queueTrack, 1, false}, {queueTrack, 2, true},
		{queueSection, 0, false}, {queueTrack, 1, true},
	}
	if len(queue.entries) != len(wants) {
		t.Fatalf("%d entries, want %d", len(queue.entries), len(wants))
	}
	for index, entry := range queue.entries {
		got := want{entry.kind, entry.ordinal, entry.last}
		if got != wants[index] {
			t.Fatalf("entry %d = %+v, want %+v", index, got, wants[index])
		}
	}
}
