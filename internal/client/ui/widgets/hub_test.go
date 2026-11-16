package widgets

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/sheet"
	"github.com/ryoku-dev/ryotunes/internal/model"
)

// hubFrame lays w out once on a 1 px per dp context at now and routes the
// frame, so the router's wake-up time is what w asked for.
type hubFrame struct {
	ops    op.Ops
	router input.Router
	now    time.Time
	size   image.Point
}

func newHubFrame(width, height int) *hubFrame {
	return &hubFrame{now: time.Unix(1000, 0), size: image.Pt(width, height)}
}

func (f *hubFrame) context() layout.Context {
	f.ops.Reset()
	return layout.Context{
		Ops: &f.ops, Source: f.router.Source(), Now: f.now,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: f.size},
	}
}

// run lays w out as one frame; WakeupTime then reports what this frame alone
// asked for (the router keeps a request until it is read).
func (f *hubFrame) run(w func(gtx layout.Context) layout.Dimensions) layout.Dimensions {
	f.router.WakeupTime()
	gtx := f.context()
	dimensions := w(gtx)
	f.router.Frame(&f.ops)
	return dimensions
}

func (f *hubFrame) advance(d time.Duration) { f.now = f.now.Add(d) }

func hubTheme() *look.Theme {
	theme := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	theme.Class = look.Expanded
	return theme
}

func TestHeaderTategakiFollowsTheDesktopGloss(t *testing.T) {
	theme := hubTheme()
	e := &env.Env{Th: theme}
	height := func(gloss bool) int {
		theme.Desktop.Gloss = gloss
		header := Header{NoLane: true, Place: sheet.Library, Part: "Songs", PartGloss: "曲"}
		frame := newHubFrame(1000, 800)
		return frame.run(func(gtx layout.Context) layout.Dimensions {
			return header.Layout(gtx, e, "Library", "", nil)
		}).Size.Y
	}
	off, on := height(false), height(true)
	if on <= off {
		t.Fatalf("head with glosses on is %d tall, off %d: the tategaki took no room", on, off)
	}
	header := Header{Place: sheet.Library, PartGloss: "曲"}
	if page, category := header.Gloss(); page != "曲" || category != sheet.Library.Gloss {
		t.Fatalf("tategaki gloss = %q over %q, want the part's over the place's", page, category)
	}
	if part, gloss := (&Header{Part: "Songs", PartGloss: "曲"}).Masthead(); part != "Songs" || gloss != "曲" {
		t.Fatalf("Masthead() = %q, %q", part, gloss)
	}
}

func TestHeaderStateAndSpecStripStandUnderTheWords(t *testing.T) {
	theme := hubTheme()
	e := &env.Env{Th: theme}
	measure := func(header *Header) int {
		frame := newHubFrame(1000, 800)
		return frame.run(func(gtx layout.Context) layout.Dimensions {
			return header.Layout(gtx, e, "Library", "Everything you saved.", nil)
		}).Size.Y
	}
	plain := measure(&Header{NoLane: true})
	stated := measure(&Header{NoLane: true, State: "Synced a minute ago."})
	specced := measure(&Header{NoLane: true, State: "Synced a minute ago.", Specs: []sheet.Spec{{Label: "Songs", Value: "1204"}}})
	if stated <= plain || specced < stated+int(sheet.SpecStripAbove) {
		t.Fatalf("heights plain %d, with state %d, with strip %d: state and strip must each add their line", plain, stated, specced)
	}
}

func TestTrackOrdinalsPadToTheListsDigits(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ n, digits int }{{0, 2}, {9, 2}, {99, 2}, {100, 3}, {1204, 4}} {
		if got := ordinalDigits(test.n); got != test.digits {
			t.Errorf("ordinalDigits(%d) = %d, want %d", test.n, got, test.digits)
		}
	}
	var row trackRow
	if got := row.ordinalText(6, 3); got != "007" {
		t.Fatalf("ordinal = %q, want 007", got)
	}
	if got := row.ordinalText(1203, 4); got != "1204" {
		t.Fatalf("ordinal = %q, want 1204", got)
	}
}

func TestTrackSelectionLandsTravelsAndRests(t *testing.T) {
	theme := hubTheme()
	e := &env.Env{Th: theme}
	items := make([]model.Track, 8)
	for index := range items {
		items[index] = model.Track{VideoID: "video" + string(rune('a'+index)), Title: "Track"}
	}
	var table TrackTable
	frame := newHubFrame(900, 600)
	rows := func() {
		frame.run(func(gtx layout.Context) layout.Dimensions {
			for index := range items {
				offset := op.Offset(image.Pt(0, index*48)).Push(gtx.Ops)
				table.Row(gtx, e, items, index, TrackTableOpts{})
				offset.Pop()
			}
			return layout.Dimensions{}
		})
	}
	// step reads the wash inside a routed frame, so what it asks for is the
	// frame's.
	step := func() (at, opacity, width float32) {
		frame.run(func(gtx layout.Context) layout.Dimensions {
			at, opacity, width = table.stepSelection(gtx, theme)
			return layout.Dimensions{}
		})
		return at, opacity, width
	}
	rows()
	table.selected, table.selectionReady = 2, true
	rows()
	if _, opacity, width := step(); opacity >= 1 || width >= 1 {
		t.Fatalf("first selection stood at once (opacity %.2f, width %.2f), want it to land", opacity, width)
	}
	frame.advance(400 * time.Millisecond)
	rows()
	if at, opacity, width := step(); at != 2 || opacity != 1 || width != 1 {
		t.Fatalf("landed selection at %.2f, opacity %.2f, width %.2f", at, opacity, width)
	}
	table.selected = 5
	rows()
	frame.advance(100 * time.Millisecond)
	rows()
	if at, _, _ := step(); at <= 2 || at >= 5 {
		t.Fatalf("selection mid-travel at %.2f, want between rows 2 and 5", at)
	}
	frame.advance(400 * time.Millisecond)
	rows()
	if at, _, _ := step(); at != 5 {
		t.Fatalf("selection settled at %.2f, want 5", at)
	}
	rows()
	if wake, ok := frame.router.WakeupTime(); ok {
		t.Fatalf("a settled table asked for a frame at %v", wake)
	}
}

func TestWaitShowsNothingForASecondThenWorks(t *testing.T) {
	theme := hubTheme()
	e := &env.Env{Th: theme}
	frame := newHubFrame(900, 600)
	start := frame.now
	frame.run(Loading(e, LoadingTable))
	if wake, ok := frame.router.WakeupTime(); !ok || !wake.Equal(start.Add(waitQuiet)) {
		t.Fatalf("a new wait asked for a frame at %v (%v), want the end of its quiet second", wake, ok)
	}
	frame.advance(500 * time.Millisecond)
	if elapsed, _ := waitFor(frame.context(), e, LoadingTable); elapsed != 500*time.Millisecond {
		t.Fatalf("wait elapsed %v, want 500ms", elapsed)
	}
	frame.advance(700 * time.Millisecond)
	quiet := newHubFrame(900, 600)
	quiet.now = frame.now
	shown := frame.run(Loading(e, LoadingTable)).Size.Y
	if fresh := quiet.run(Loading(&env.Env{Th: theme}, LoadingTable)).Size.Y; shown <= fresh {
		t.Fatalf("past its second the wait is %d tall, a fresh one %d: no working line over the outlines", shown, fresh)
	}
	frame.advance(waitGap + time.Second)
	if elapsed, _ := waitFor(frame.context(), e, LoadingTable); elapsed != 0 {
		t.Fatalf("a wait drawn again after %v went on from the old one (%v)", waitGap+time.Second, elapsed)
	}
}

func TestItemMenuNamesItsTargetAndMovesFocusWithTheKeys(t *testing.T) {
	theme := hubTheme()
	e := &env.Env{Th: theme}
	track := model.Track{VideoID: "local:song", Title: "Night drive", Artists: "Someone"}
	target := env.MenuTarget{Track: &track, Context: "queue"}
	kind, title, detail := menuHead(target)
	if kind != "In the queue" || title != "Night drive" || detail != "Someone" {
		t.Fatalf("menu head = %q, %q, %q", kind, title, detail)
	}
	var menu ItemMenu
	frame := newHubFrame(600, 800)
	layoutMenu := func() layout.Dimensions {
		return frame.run(func(gtx layout.Context) layout.Dimensions { return menu.Layout(gtx, e, target) })
	}
	headed := layoutMenu()
	entries := len(targetEntries(&menu, e, target))
	if minimum := entries * 40; headed.Size.Y <= minimum {
		t.Fatalf("menu is %d tall for %d rows: no head", headed.Size.Y, entries)
	}
	gtx := frame.context()
	gtx.Execute(key.FocusCmd{Tag: &menu.rows[0].click.Clickable})
	menu.Layout(gtx, e, target)
	frame.router.Frame(&frame.ops)
	frame.router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press})
	layoutMenu()
	layoutMenu()
	gtx = frame.context()
	if !gtx.Focused(&menu.rows[1].click.Clickable) {
		t.Fatal("Down did not move focus to the second row")
	}
	frame.router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	layoutMenu()
	frame.router.Queue(key.Event{Name: key.NameUpArrow, State: key.Press})
	layoutMenu()
	layoutMenu()
	if gtx := frame.context(); !gtx.Focused(&menu.rows[entries-1].click.Clickable) {
		t.Fatal("Up from the first row did not wrap to the last")
	}
}

func TestHeaderSpecValuesRollAndRest(t *testing.T) {
	theme := hubTheme()
	e := &env.Env{Th: theme}
	header := Header{NoLane: true, Specs: []sheet.Spec{{Label: "Songs", Value: "12"}}}
	frame := newHubFrame(1000, 800)
	head := func(gtx layout.Context) layout.Dimensions { return header.Layout(gtx, e, "Library", "", nil) }
	frame.run(head)
	if wake, ok := frame.router.WakeupTime(); ok {
		t.Fatalf("a head at rest asked for a frame at %v", wake)
	}
	header.Specs[0].Value = "13"
	frame.run(head)
	if !header.rolls[0].Rolling() {
		t.Fatal("a changed count did not roll")
	}
	if _, ok := frame.router.WakeupTime(); !ok {
		t.Fatal("a rolling count asked for no frame")
	}
	frame.advance(300 * time.Millisecond)
	frame.run(head)
	frame.run(head)
	if wake, ok := frame.router.WakeupTime(); ok || header.rolls[0].Rolling() {
		t.Fatalf("a settled count still rolls (frame at %v)", wake)
	}
}
