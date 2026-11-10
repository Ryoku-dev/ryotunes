package widgets

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

const heroBottom = 120

// stateProbe lays out a page shaped like album and playlist: a hero, then a
// page state whose body is a fixed-size hit area, inside a 600 x height pane.
type stateProbe struct {
	theme      *look.Theme
	operations op.Ops
	router     input.Router
	now        time.Time
	tag        struct{}
	hit        bool
	bodyMaxY   int
	page       func(gtx layout.Context, state layout.Widget) layout.Dimensions
	height     int
}

func newStateProbe(height int, page func(gtx layout.Context, state layout.Widget) layout.Dimensions) *stateProbe {
	theme := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	theme.Class = look.Expanded
	return &stateProbe{theme: theme, now: time.Unix(100, 0), page: page, height: height}
}

func (p *stateProbe) inset() int { return int(look.ContentInset(p.theme.Class)) }

func (p *stateProbe) frame(bodyHeight int) {
	state := stateLayout(&env.Env{Th: p.theme}, func(gtx layout.Context) layout.Dimensions {
		p.bodyMaxY = gtx.Constraints.Max.Y
		if _, ok := gtx.Event(pointer.Filter{Target: &p.tag, Kinds: pointer.Press}); ok {
			p.hit = true
		}
		size := image.Pt(100, min(bodyHeight, gtx.Constraints.Max.Y))
		area := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
		event.Op(gtx.Ops, &p.tag)
		area.Pop()
		return layout.Dimensions{Size: size}
	})
	p.operations.Reset()
	p.hit = false
	gtx := layout.Context{
		Ops: &p.operations, Source: p.router.Source(), Now: p.now,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(600, p.height)),
	}
	p.page(gtx, state)
	p.router.Frame(&p.operations)
	p.now = p.now.Add(time.Millisecond)
}

// startsAt reports whether the body's first row is exactly y.
func (p *stateProbe) startsAt(y int, bodyHeight int) bool {
	p.frame(bodyHeight)
	press := func(id pointer.ID, at int) bool {
		p.router.Queue(pointer.Event{
			Kind: pointer.Press, Source: pointer.Mouse, PointerID: id,
			Buttons: pointer.ButtonPrimary, Position: f32.Pt(300, float32(at)+.5),
		})
		p.frame(bodyHeight)
		return p.hit
	}
	return !press(1, y-1) && press(2, y)
}

func flexedUnderHero(gtx layout.Context, state layout.Widget) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, heroBottom)}
		}),
		layout.Flexed(1, state),
	)
}

func scrolledUnderHero(scroll *look.Scroller, theme *look.Theme, minimum bool) func(gtx layout.Context, state layout.Widget) layout.Dimensions {
	return func(gtx layout.Context, state layout.Widget) layout.Dimensions {
		viewport := gtx.Constraints.Max.Y
		return scroll.Layout(gtx, theme, 2, func(gtx layout.Context, index int) layout.Dimensions {
			if index == 0 {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, heroBottom)}
			}
			if minimum {
				gtx.Constraints.Min.Y = max(0, viewport-heroBottom)
			}
			return state(gtx)
		})
	}
}

func TestPageStateCentersBelowHeroAtContentInset(t *testing.T) {
	probe := newStateProbe(400, flexedUnderHero)
	// 280 left under the hero, an 80 tall body: centred at 120 + 100.
	if !probe.startsAt(heroBottom+100, 80) {
		t.Fatal("state was not centred in the height remaining below the hero")
	}
}

func TestPageStateInScrollerCentersInRemainingHeight(t *testing.T) {
	var scroll look.Scroller
	probe := newStateProbe(400, nil)
	probe.page = scrolledUnderHero(&scroll, probe.theme, true)
	// The same 280 under the hero as a minimum height, not an exact one.
	if !probe.startsAt(heroBottom+100, 80) {
		t.Fatal("state with a minimum height was not centred in the height remaining below the hero")
	}
}

func TestPageStateInShortScrollerKeepsInsetAndFullHeight(t *testing.T) {
	var scroll look.Scroller
	probe := newStateProbe(400, nil)
	probe.page = scrolledUnderHero(&scroll, probe.theme, true)
	// 280 left under the hero is less than a 300 tall body plus two insets.
	if !probe.startsAt(heroBottom+probe.inset(), 300) {
		t.Fatal("short state did not start one content inset under the hero")
	}
	if probe.bodyMaxY < 300 {
		t.Fatalf("short state body was squeezed to %d, want room to scroll", probe.bodyMaxY)
	}
}

func TestPageStateWithoutMinimumIsNotCentred(t *testing.T) {
	var scroll look.Scroller
	probe := newStateProbe(800, nil)
	probe.page = scrolledUnderHero(&scroll, probe.theme, false)
	if !probe.startsAt(heroBottom+probe.inset(), 80) {
		t.Fatal("state in a list with no minimum height moved off the content inset")
	}
}
