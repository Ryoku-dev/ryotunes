package widgets

import (
	"image"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// LoadingShape selects the shape a wait keeps for the content it stands for.
type LoadingShape uint8

const (
	LoadingTable LoadingShape = iota
	LoadingGrid
	LoadingHero
	LoadingShelf
)

// The Hub's long work (docs/hub.md, WorkLine.qml): nothing for the first
// second, then Working with a word, then past ten seconds a blade and a way
// out.
const (
	waitQuiet = time.Second
	waitLong  = 10 * time.Second
	// waitTick is the frame a wait asks for while it stands, so a wait that
	// is no longer drawn is told apart from one that is: a wait unseen for
	// more than waitGap is over, and the next one starts its quiet second
	// again.
	waitTick = time.Second
	waitGap  = 2 * waitTick
)

// waitClock is one wait's time and its way out.
type waitClock struct {
	since, seen time.Time
	back        kit.Plaque
}

type waitKey struct {
	e     *env.Env
	shape LoadingShape
}

// waits holds the clock of each wait being drawn, by window and shape: the
// state helpers are plain widgets, so their time lives here.
var waits struct {
	sync.Mutex
	clocks map[waitKey]*waitClock
}

// waitFor returns how long the wait for shape has stood and its clock, and
// asks for the frame that moves it on.
func waitFor(gtx layout.Context, e *env.Env, shape LoadingShape) (time.Duration, *waitClock) {
	now := gtx.Now
	if now.IsZero() {
		now = time.Now()
	}
	waits.Lock()
	if waits.clocks == nil {
		waits.clocks = make(map[waitKey]*waitClock)
	}
	key := waitKey{e: e, shape: shape}
	clock := waits.clocks[key]
	if clock == nil {
		clock = new(waitClock)
		waits.clocks[key] = clock
	}
	if clock.since.IsZero() || now.Sub(clock.seen) > waitGap || now.Before(clock.since) {
		clock.since = now
	}
	clock.seen = now
	waits.Unlock()
	elapsed := now.Sub(clock.since)
	next := waitTick
	if elapsed < waitQuiet {
		next = waitQuiet - elapsed
	} else if elapsed < waitLong {
		next = min(waitTick, waitLong-elapsed)
	}
	gtx.Execute(op.InvalidateCmd{At: now.Add(next)})
	return elapsed, clock
}

// outline draws a block's vein outline in the design's corners: the shape
// of what will arrive, never a flash of grey.
func outline(gtx layout.Context, e *env.Env, bounds image.Rectangle, cut unit.Dp) {
	kit.Stroke(gtx, e.Th.Theme, bounds, gtx.Dp(cut), max(1, gtx.Dp(1)), e.Th.R.Vein)
}

// workLine is the wait's line: the three flakes and a word, and past ten
// seconds the blade sweeping under them with a way back.
func workLine(gtx layout.Context, e *env.Env, clock *waitClock, elapsed time.Duration, word string) layout.Dimensions {
	th := e.Th
	gtx.Constraints.Min = image.Point{}
	flakes := kit.Working{Active: true, Size: 20}.Layout(gtx, th.Theme)
	x := flakes.Size.X + gtx.Dp(8)
	label := gtx
	label.Constraints.Max.X = max(0, gtx.Constraints.Max.X-x)
	record := op.Record(gtx.Ops)
	words := kit.Label{Style: th.T.Detail, Color: th.R.Fg2, MaxLines: 1}.Layout(label, th.Theme, word)
	call := record.Stop()
	line := max(flakes.Size.Y, words.Size.Y)
	offset := op.Offset(image.Pt(x, (line-words.Size.Y)/2)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	offset.Pop()
	width := x + words.Size.X
	height := line
	if elapsed < waitLong {
		return layout.Dimensions{Size: image.Pt(width, height)}
	}
	height += gtx.Dp(13)
	blade := gtx
	blade.Constraints = layout.Exact(image.Pt(min(gtx.Constraints.Max.X, gtx.Dp(240)), gtx.Dp(2)))
	offset = op.Offset(image.Pt(0, height)).Push(gtx.Ops)
	bladeSize := kit.BladeProgress{Fraction: -1, Active: true}.Layout(blade, th.Theme).Size
	offset.Pop()
	height += bladeSize.Y
	width = max(width, bladeSize.X)
	if e.Nav != nil && e.Nav.CanBack() {
		if clock.back.Clicked(gtx) {
			e.Nav.Back()
		}
		clock.back.Kind, clock.back.Small, clock.back.Label = kit.PlaqueQuiet, true, "Go back"
		height += gtx.Dp(13)
		offset = op.Offset(image.Pt(0, height)).Push(gtx.Ops)
		back := clock.back.Layout(gtx, th.Theme)
		offset.Pop()
		height += back.Size.Y
		width = max(width, back.Size.X)
	}
	return layout.Dimensions{Size: image.Pt(width, height)}
}

// waitWord is the word a wait says.
func waitWord(shape LoadingShape) string {
	if shape == LoadingTable {
		return "Loading the tracks"
	}
	return "Loading"
}

// loadingTable keeps eight rows' height; after the quiet second their vein
// outlines stand where the rows will.
func loadingTable(gtx layout.Context, e *env.Env, shown bool) layout.Dimensions {
	row := gtx.Dp(e.Th.RowHeight())
	size := image.Pt(gtx.Constraints.Max.X, 8*row)
	if shown {
		gap := gtx.Dp(4)
		for index := range 8 {
			top := index*row + gap/2
			outline(gtx, e, image.Rect(0, top, size.X, top+row-gap), e.Th.Cut.Small)
		}
	}
	return layout.Dimensions{Size: size}
}

// gridCell is one waiting card: its art's square and its title's line.
func gridCell(gtx layout.Context, e *env.Env, width int, shown bool) image.Point {
	size := image.Pt(width, width+gtx.Dp(64))
	if shown {
		outline(gtx, e, image.Rect(0, 0, width, width), e.Th.Cut.Mid)
		top := width + gtx.Dp(8)
		outline(gtx, e, image.Rect(0, top, width*3/4, top+gtx.Dp(16)), e.Th.Cut.Small)
		top += gtx.Dp(22)
		outline(gtx, e, image.Rect(0, top, width/2, top+gtx.Dp(12)), e.Th.Cut.Small)
	}
	return size
}

func loadingGrid(gtx layout.Context, e *env.Env, shown bool) layout.Dimensions {
	gap := gtx.Dp(21)
	columns := gridColumns(gtx.Constraints.Max.X, gap, gtx.Dp(160))
	available := gtx.Constraints.Max.X - gap*(columns-1)
	width := min(gtx.Dp(200), max(gtx.Dp(160), available/max(1, columns)))
	x, height := 0, 0
	for column := range columns {
		if column > 0 {
			x += gap
		}
		offset := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		size := gridCell(gtx, e, width, shown)
		offset.Pop()
		x += size.X
		height = max(height, size.Y)
	}
	return layout.Dimensions{Size: image.Pt(min(x, gtx.Constraints.Max.X), height)}
}

func loadingShelf(gtx layout.Context, e *env.Env, shown bool) layout.Dimensions {
	head := gtx.Dp(28)
	if shown {
		outline(gtx, e, image.Rect(0, gtx.Dp(6), min(gtx.Constraints.Max.X, gtx.Dp(220)), head-gtx.Dp(6)), e.Th.Cut.Small)
	}
	y := head + gtx.Dp(13)
	offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	grid := loadingGrid(gtx, e, shown)
	offset.Pop()
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, y+grid.Size.Y)}
}

// skeleton keeps shape's room, outlined once the quiet second is over.
func skeleton(gtx layout.Context, e *env.Env, shape LoadingShape, shown bool) layout.Dimensions {
	switch shape {
	case LoadingGrid:
		return loadingGrid(gtx, e, shown)
	case LoadingShelf:
		return loadingShelf(gtx, e, shown)
	default:
		return loadingTable(gtx, e, shown)
	}
}

// loadingHero is the hero's field at its height, the art's square outlined
// and the wait's line where the words will stand.
func loadingHero(gtx layout.Context, e *env.Env, clock *waitClock, elapsed time.Duration) layout.Dimensions {
	art := heroArtSize(gtx.Metric.PxToDp(gtx.Constraints.Max.X))
	pixels := gtx.Dp(art)
	cut := gtx.Dp(e.Th.Cut.Large)
	shown := elapsed >= waitQuiet
	inset := look.ContentInset(e.Th.Class)
	bottom, _ := headerLaneMetrics(e.Th.Class)
	return InsidePaneStroke(gtx, e.Th, func(gtx layout.Context) layout.Dimensions {
		return look.Field(gtx, e.Th, [4]int{cut, cut, 0, 0}, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: inset, Right: inset, Bottom: bottom, Left: inset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Max.X, pixels)
				if !shown {
					return layout.Dimensions{Size: size}
				}
				outline(gtx, e, image.Rect(0, 0, pixels, pixels), 7)
				words := gtx
				words.Constraints = layout.Constraints{Max: image.Pt(max(0, size.X-pixels-gtx.Dp(21)), pixels)}
				record := op.Record(gtx.Ops)
				line := workLine(words, e, clock, elapsed, waitWord(LoadingHero))
				call := record.Stop()
				offset := op.Offset(image.Pt(pixels+gtx.Dp(21), pixels-line.Size.Y)).Push(gtx.Ops)
				call.Add(gtx.Ops)
				offset.Pop()
				return layout.Dimensions{Size: size}
			})
		})
	})
}

func insetAll(e *env.Env) layout.Inset {
	inset := look.ContentInset(e.Th.Class)
	return layout.Inset{Top: inset, Right: inset, Bottom: inset, Left: inset}
}

func stateLayout(e *env.Env, body layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if e == nil || e.Th == nil {
			return layout.Dimensions{}
		}
		// layout.Inset leaves Min alone, so take the inset out of it here to
		// centre within the box the caller asked for, not one inset lower.
		inset := 2 * gtx.Dp(look.ContentInset(e.Th.Class))
		gtx.Constraints.Min.X = max(0, gtx.Constraints.Min.X-inset)
		gtx.Constraints.Min.Y = max(0, gtx.Constraints.Min.Y-inset)
		return insetAll(e).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, body)
		})
	}
}

// Loading returns a whole page's wait, as the Hub waits: for the first
// second nothing shows (the room is kept, so nothing flashes), then the
// three flakes with a word over the vein outlines of what will arrive, and
// past ten seconds a sweeping blade with Go back. It stands top-start at the
// content inset; LoadingHero is the hero's own field, flush at its height.
// Never a spinner.
func Loading(e *env.Env, shape LoadingShape) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if e == nil || e.Th == nil {
			return layout.Dimensions{}
		}
		elapsed, clock := waitFor(gtx, e, shape)
		if shape == LoadingHero {
			gtx.Constraints.Min.Y = 0
			return loadingHero(gtx, e, clock, elapsed)
		}
		return insetAll(e).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{}
			if elapsed < waitQuiet {
				return skeleton(gtx, e, shape, false)
			}
			line := workLine(gtx, e, clock, elapsed, waitWord(shape))
			y := line.Size.Y + gtx.Dp(21)
			offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
			body := skeleton(gtx, e, shape, true)
			offset.Pop()
			return layout.Dimensions{Size: image.Pt(max(line.Size.X, body.Size.X), y+body.Size.Y)}
		})
	}
}

// Skeleton returns the room a list keeps for the rows it waits for, with
// their insets, so nothing moves when they arrive: empty for the first
// second, then their vein outlines with the three flakes at the head.
func Skeleton(e *env.Env, shape LoadingShape) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if e == nil || e.Th == nil {
			return layout.Dimensions{}
		}
		elapsed, _ := waitFor(gtx, e, shape)
		shown := elapsed >= waitQuiet
		dimensions := skeleton(gtx, e, shape, shown)
		if shown {
			kit.Working{Active: true, Size: 16}.Layout(gtx, e.Th.Theme)
		}
		return dimensions
	}
}

// Failed returns an error state: what went wrong in jasper, then a quiet
// Retry. message should say what to do next.
func Failed(e *env.Env, message string, retry *kit.Plaque) layout.Widget {
	return stateLayout(e, func(gtx layout.Context) layout.Dimensions {
		var actions []*kit.Plaque
		if retry != nil {
			retry.Kind = kit.PlaqueQuiet
			if retry.Label == "" {
				retry.Label = "Retry"
			}
			actions = []*kit.Plaque{retry}
		}
		return stateCopy(gtx, e, nil, "This page did not load", message, true, actions)
	})
}

// EmptyState returns a centred state that names the next useful step: the
// place's glyph faint, a section title, the body up to 420 wide, actions.
func EmptyState(e *env.Env, icon *widget.Icon, title, body string, actions ...*kit.Plaque) layout.Widget {
	return stateLayout(e, func(gtx layout.Context) layout.Dimensions {
		return stateCopy(gtx, e, icon, title, body, false, actions)
	})
}

// stateBodyMax is the widest a state's body runs (the Hub's empty body).
const stateBodyMax unit.Dp = 420

// stateCopy stacks a state's words centred: glyph, title, body (jasper
// detail for a failure, fg2 body otherwise) and its quiet actions.
func stateCopy(gtx layout.Context, e *env.Env, icon *widget.Icon, title, body string, failed bool, actions []*kit.Plaque) layout.Dimensions {
	th := e.Th
	gtx.Constraints.Min = image.Point{}
	width := min(gtx.Constraints.Max.X, gtx.Dp(stateBodyMax))
	type part struct {
		call op.CallOp
		size image.Point
	}
	var parts [4]part
	count := 0
	add := func(w layout.Widget) {
		record := op.Record(gtx.Ops)
		child := gtx
		child.Constraints = layout.Constraints{Max: image.Pt(width, gtx.Constraints.Max.Y)}
		size := w(child).Size
		parts[count] = part{record.Stop(), size}
		count++
	}
	if icon != nil {
		add(func(gtx layout.Context) layout.Dimensions { return drawIcon(gtx, icon, th.R.Fg3, 24) })
	}
	if title != "" {
		add(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return kit.Label{Style: th.T.Section, Color: th.R.Fg, MaxLines: 2, Align: text.Middle}.Layout(gtx, th.Theme, title)
		})
	}
	if body != "" {
		style, ink := th.T.Body, th.R.Fg2
		if failed {
			style, ink = th.T.Detail, th.R.Jasper
		}
		add(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return kit.Label{Style: style, Color: ink, MaxLines: 0, Align: text.Middle}.Layout(gtx, th.Theme, body)
		})
	}
	valid := 0
	for _, action := range actions {
		if action != nil {
			valid++
		}
	}
	if valid > 0 {
		add(func(gtx layout.Context) layout.Dimensions {
			x, height := 0, 0
			for _, action := range actions {
				if action == nil {
					continue
				}
				if x > 0 {
					x += gtx.Dp(8)
				}
				offset := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
				size := action.Layout(gtx, th.Theme).Size
				offset.Pop()
				x += size.X
				height = max(height, size.Y)
			}
			return layout.Dimensions{Size: image.Pt(x, height)}
		})
	}
	y, total := 0, 0
	for index := range count {
		total = max(total, parts[index].size.X)
	}
	for index := range count {
		if index > 0 {
			switch {
			case icon != nil && index == 1:
				y += gtx.Dp(13)
			case index == count-1 && valid > 0:
				y += gtx.Dp(21)
			default:
				y += gtx.Dp(8)
			}
		}
		offset := op.Offset(image.Pt((total-parts[index].size.X)/2, y)).Push(gtx.Ops)
		parts[index].call.Add(gtx.Ops)
		offset.Pop()
		y += parts[index].size.Y
	}
	return layout.Dimensions{Size: image.Pt(total, y)}
}
