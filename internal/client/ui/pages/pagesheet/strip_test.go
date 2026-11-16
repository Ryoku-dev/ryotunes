package pagesheet

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/palette"
	libtheme "github.com/ryoku-dev/ryoku/docs/lithic/library/go/theme"
	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

type frames struct {
	ops    op.Ops
	router input.Router
}

func (f *frames) context(now time.Time) layout.Context {
	f.ops.Reset()
	return layout.Context{
		Ops: &f.ops, Source: f.router.Source(), Now: now,
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 200)),
	}
}

func (f *frames) wakeup() (time.Time, bool) {
	f.router.Frame(&f.ops)
	return f.router.WakeupTime()
}

func TestStripPadsFiguresAndRestsWithoutWork(t *testing.T) {
	var strip Strip
	strip.Set(0, "Songs", 1204, false)
	strip.Set(1, "Failed", 3, true)
	if specs := strip.Specs(false); specs != nil {
		t.Fatalf("specs before the counts arrive = %v", specs)
	}
	first := strip.Specs(true)
	if len(first) != 2 || first[0].Value != "1204" || first[1].Value != "03" || !first[1].Lit || first[0].Label != "Songs" {
		t.Fatalf("strip = %+v", first)
	}
	rest := strip.Specs(true)
	if &rest[0] != &first[0] {
		t.Fatal("a strip at rest rebuilt its specs")
	}
	if allocs := testing.AllocsPerRun(50, func() { strip.Specs(true) }); allocs != 0 {
		t.Fatalf("a strip at rest allocates %.0f times a frame", allocs)
	}
	strip.Set(0, "Songs", 1205, false)
	if again := strip.Specs(true); again[0].Value != "1205" {
		t.Fatalf("changed figure = %q", again[0].Value)
	}
	strip.Len(1)
	if got := strip.Specs(true); len(got) != 1 {
		t.Fatalf("Len(1) left %d figures", len(got))
	}
}

func TestMoreWaitsASecondThenWorks(t *testing.T) {
	th := look.NewTheme(palette.Signature(true), libtheme.Comfortable)
	var more More
	var f frames
	start := time.Unix(100, 0)

	gtx := f.context(start)
	gtx.Constraints.Min = image.Point{}
	quiet := more.Layout(gtx, th, true, "Finding more")
	if quiet.Size.Y != gtx.Sp(th.T.Body.Line) {
		t.Fatalf("reserved height = %d, want one body line", quiet.Size.Y)
	}
	if at, ok := f.wakeup(); !ok || !at.Equal(start.Add(moreDelay)) {
		t.Fatalf("wakeup = %v, %v; want one frame at the second", at, ok)
	}

	gtx = f.context(start.Add(moreDelay + time.Millisecond))
	gtx.Constraints.Min = image.Point{}
	working := more.Layout(gtx, th, true, "Finding more")
	if working.Size.X <= 0 || working.Size.X >= gtx.Constraints.Max.X {
		t.Fatalf("working line width = %d, want the flakes and the word", working.Size.X)
	}
	f.wakeup()

	gtx = f.context(start.Add(2 * time.Second))
	idle := more.Layout(gtx, th, false, "Finding more")
	if idle.Size.Y != quiet.Size.Y {
		t.Fatal("the idle line does not keep its room")
	}
	if _, ok := f.wakeup(); ok {
		t.Fatal("an idle line asked for a frame")
	}
}
