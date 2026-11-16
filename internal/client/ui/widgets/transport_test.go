package widgets

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/anim"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/env"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

func TestPressGlyphPressesAndSettles(t *testing.T) {
	anim.SetReduce(false)
	th := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	var (
		ops    op.Ops
		router input.Router
		glyph  PressGlyph
		value  float32
	)
	now := time.Unix(9000, 0)
	frame := func(pressed bool) bool {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: now, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
		value = glyph.Value(gtx, th, pressed)
		router.Frame(&ops)
		_, wants := router.WakeupTime()
		return wants
	}
	if frame(false) || value != 0 {
		t.Fatalf("a glyph at rest stands at %v and asks for no frame", value)
	}
	if !frame(true) {
		t.Fatal("a press in progress asks for the next frame")
	}
	now = now.Add(th.Motion.Press / 2)
	frame(true)
	if value <= 0 || value >= 1 {
		t.Fatalf("half a press in, the glyph should be between rest and pressed, got %v", value)
	}
	now = now.Add(th.Motion.Press)
	frame(true)
	now = now.Add(time.Millisecond)
	if frame(true) || value != 1 {
		t.Fatalf("a held press stands at 1 (%v) and asks for no frame", value)
	}
}

func TestPlaqueInkFollowsTheKitsKinds(t *testing.T) {
	th := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	var router input.Router
	gtx := layout.Context{Ops: new(op.Ops), Source: router.Source()}
	cases := map[kit.PlaqueKind]bool{
		kit.PlaqueQuiet: PlaqueInk(gtx, th, kit.PlaqueQuiet) == th.R.Fg2,
		kit.PlaqueOn:    PlaqueInk(gtx, th, kit.PlaqueOn) == th.R.OnSel,
		kit.PlaqueGold:  PlaqueInk(gtx, th, kit.PlaqueGold) == th.R.OnGold,
	}
	for kind, ok := range cases {
		if !ok {
			t.Fatalf("plaque kind %v draws its glyph in the wrong ink", kind)
		}
	}
	if PlaqueInk(gtx.Disabled(), th, kit.PlaqueQuiet) == th.R.Fg2 {
		t.Fatal("a disabled plaque's glyph must fade")
	}
}

func TestSeekTimeBoxesFitEveryTime(t *testing.T) {
	th := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	e := &env.Env{Th: th}
	var router input.Router
	gtx := layout.Context{
		Ops: new(op.Ops), Source: router.Source(), Metric: unit.Metric{PxPerDp: 1.75, PxPerSp: 1.75},
		Constraints: layout.Exact(image.Pt(800, 100)),
	}
	var seek Seek
	for _, value := range []string{"0:00", "3:18", "59:59", "1:02:45", "12:59:59"} {
		width := seek.timeBox(gtx, e, len(value))
		if got := measureMono(gtx, e, value); got > width {
			t.Fatalf("%q needs %d px, its box gives %d", value, got, width)
		}
		if template := seekTimeTemplate(len(value)); len(template) != len(value) || strings.Count(template, ":") != strings.Count(value, ":") {
			t.Fatalf("template %q does not stand for %q", template, value)
		}
	}
	short, long := seek.timeBox(gtx, e, 4), seek.timeBox(gtx, e, 7)
	if long <= short {
		t.Fatalf("an hour-long time needs a wider box (%d against %d)", long, short)
	}
	if seek.timeBox(gtx, e, 7) != long {
		t.Fatal("the box is measured once per digit count")
	}
}
